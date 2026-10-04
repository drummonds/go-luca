package luca

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Positions are the ledger's stored projections: one row per account per
// day in balances_live holding the end-of-day balance and the accrued
// interest carried into the next day. A movement rewrites the positions
// of both its accounts from its value day onwards; Project writes a day
// that has no movement (the daily pass's next-day projection). Each day's
// balance is the previous position plus the movements of the days between,
// so a write costs a range sum, not the account's whole history.
//
// The views contract_ledger_eod_positions and contract_ledger_live_positions
// publish them (see SchemaSQL); the live one adds the movements since the
// latest position and is the dearer of the two.

// dayStart is midnight of t's date in t's location, the key of a position.
func dayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// rangeDelta sums the account's movements with from <= value_time < to.
// A zero from means from the beginning. Bounds are midnights, so the text
// comparison pglike makes is safe whatever the fractional-second form.
func rangeDelta(tx dbtx, accountID string, from, to time.Time) (Amount, error) {
	var in, out Amount
	err := tx.QueryRow(
		`SELECT
			COALESCE((SELECT SUM(amount) FROM movements WHERE to_account_id = $1 AND value_time >= $2 AND value_time < $3), 0),
			COALESCE((SELECT SUM(amount) FROM movements WHERE from_account_id = $4 AND value_time >= $5 AND value_time < $6), 0)`,
		accountID, utc(from), utc(to), accountID, utc(from), utc(to),
	).Scan(&in, &out)
	if err != nil {
		return 0, fmt.Errorf("range delta: %w", err)
	}
	return in - out, nil
}

// reproject recomputes the account's positions from day onwards, creating
// day's row if it does not exist. Each row's balance is rebuilt from the
// row before it and the movements between the two, so a backdated movement
// corrects every later day too.
func reproject(tx dbtx, accountID string, day time.Time) error {
	day = dayStart(day)

	var base Amount
	var baseFrom time.Time // zero: no earlier position
	var baseDay string
	err := tx.QueryRow(
		`SELECT balance_date, balance FROM balances_live
		 WHERE account_id = $1 AND balance_date < $2
		 ORDER BY balance_date DESC LIMIT 1`,
		accountID, utc(day),
	).Scan(&baseDay, &base)
	switch {
	case err == sql.ErrNoRows:
	case err != nil:
		return fmt.Errorf("previous position: %w", err)
	default:
		baseFrom = parseDBTime(baseDay).AddDate(0, 0, 1)
	}

	rows, err := tx.Query(
		`SELECT balance_date FROM balances_live
		 WHERE account_id = $1 AND balance_date >= $2
		 ORDER BY balance_date`,
		accountID, utc(day),
	)
	if err != nil {
		return fmt.Errorf("later positions: %w", err)
	}
	var days []time.Time
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return err
		}
		days = append(days, parseDBTime(s))
	}
	rows.Close()
	if len(days) == 0 || !days[0].Equal(utc(day)) {
		days = append([]time.Time{utc(day)}, days...)
	}

	for _, d := range days {
		next := d.AddDate(0, 0, 1)
		delta, err := rangeDelta(tx, accountID, baseFrom, next)
		if err != nil {
			return err
		}
		base += delta
		if err := upsertPosition(tx, accountID, d, base, nil); err != nil {
			return err
		}
		baseFrom = next
	}
	return nil
}

// upsertPosition writes a day's balance and, when accrued is given, its
// accrual. Update first, insert when the day is new.
func upsertPosition(tx dbtx, accountID string, day time.Time, balance Amount, accrued *Fraction) error {
	var res sql.Result
	var err error
	if accrued != nil {
		res, err = tx.Exec(
			`UPDATE balances_live SET balance = $1, accrued_num = $2, accrued_den = $3
			 WHERE account_id = $4 AND balance_date = $5`,
			balance, accrued.Num, accrued.Den, accountID, utc(day))
	} else {
		res, err = tx.Exec(
			`UPDATE balances_live SET balance = $1 WHERE account_id = $2 AND balance_date = $3`,
			balance, accountID, utc(day))
	}
	if err != nil {
		return fmt.Errorf("update position: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	num, den := int64(0), int64(1)
	if accrued != nil {
		num, den = accrued.Num, accrued.Den
	}
	_, err = tx.Exec(
		`INSERT INTO balances_live (id, account_id, balance_date, next_day, balance, accrued_num, accrued_den)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		uuid.New().String(), accountID, utc(day), utc(day.AddDate(0, 0, 1)), balance, num, den)
	if err != nil {
		return fmt.Errorf("insert position: %w", err)
	}
	return nil
}

// Project writes the account's position for day: the balance the ledger
// computes from the movements up to the end of that day, and the accrued
// interest the caller carries into the next day. Positions after day are
// recomputed. This is the daily pass's next-day projection.
func (l *SQLLedger) Project(accountID string, day time.Time, accrued Fraction) (*Position, error) {
	if accrued.Den == 0 {
		return nil, fmt.Errorf("project: accrued denominator must not be zero")
	}
	tx, commit, rollback, err := l.begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = rollback() }()

	if err := reproject(tx, accountID, day); err != nil {
		return nil, err
	}
	var balance Amount
	if err := tx.QueryRow(
		`SELECT balance FROM balances_live WHERE account_id = $1 AND balance_date = $2`,
		accountID, utc(dayStart(day)),
	).Scan(&balance); err != nil {
		return nil, fmt.Errorf("projected balance: %w", err)
	}
	if err := upsertPosition(tx, accountID, dayStart(day), balance, &accrued); err != nil {
		return nil, err
	}
	if err := commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return &Position{AccountID: accountID, Day: utc(dayStart(day)), Balance: balance, Accrued: accrued}, nil
}

// PositionAt returns the account's latest position on or before day, or
// nil when none has been written yet.
func (l *SQLLedger) PositionAt(accountID string, day time.Time) (*Position, error) {
	var p Position
	var dayStr string
	err := l.db.QueryRow(
		`SELECT account_id, balance_date, balance, accrued_num, accrued_den
		 FROM balances_live
		 WHERE account_id = $1 AND balance_date <= $2
		 ORDER BY balance_date DESC LIMIT 1`,
		accountID, utc(dayStart(day)),
	).Scan(&p.AccountID, &dayStr, &p.Balance, &p.Accrued.Num, &p.Accrued.Den)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("position at: %w", err)
	}
	p.Day = parseDBTime(dayStr)
	return &p, nil
}
