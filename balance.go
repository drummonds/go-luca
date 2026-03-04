package luca

import (
	"fmt"
	"time"
)

// DailyBalance represents the closing balance on a specific date.
type DailyBalance struct {
	Date    time.Time
	Balance int64
}

// Balance returns the current balance of an account (all movements, all time).
// Since movements are only allowed between same-exponent accounts,
// SUM(amount) is always in the account's own exponent.
func (l *SQLLedger) Balance(accountID int64) (int64, error) {
	var balance int64
	err := l.db.QueryRow(
		`SELECT
			COALESCE((SELECT SUM(amount) FROM movements WHERE to_account_id = $1), 0)
		  - COALESCE((SELECT SUM(amount) FROM movements WHERE from_account_id = $2), 0)`,
		accountID, accountID,
	).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("query balance: %w", err)
	}
	return balance, nil
}

// BalanceAt returns the balance of an account as of a specific point in time.
// Only considers movements with value_time <= the given time.
func (l *SQLLedger) BalanceAt(accountID int64, at time.Time) (int64, error) {
	var balance int64
	err := l.db.QueryRow(
		`SELECT
			COALESCE((SELECT SUM(amount) FROM movements WHERE to_account_id = $1 AND value_time <= $2), 0)
		  - COALESCE((SELECT SUM(amount) FROM movements WHERE from_account_id = $3 AND value_time <= $4), 0)`,
		accountID, at, accountID, at,
	).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("query balance at: %w", err)
	}
	return balance, nil
}

// BalanceByPath returns the aggregate balance for all accounts matching a path prefix,
// along with the reporting exponent (the minimum exponent of all matched accounts).
//
// When all matched accounts share the same exponent, this is a simple SUM.
// When they differ (e.g. aggregating across ledger partitions), each movement
// is scaled from its accounts' exponent to the reporting exponent in Go.
func (l *SQLLedger) BalanceByPath(pathPrefix string, at time.Time) (int64, int, error) {
	pattern := pathPrefix + "%"

	// Get the reporting exponent (min of all matched accounts)
	var reportExponent int
	err := l.db.QueryRow(
		`SELECT COALESCE(MIN(exponent), -2) FROM accounts WHERE full_path LIKE $1`,
		pattern,
	).Scan(&reportExponent)
	if err != nil {
		return 0, 0, fmt.Errorf("query report exponent: %w", err)
	}

	// Check if all matched accounts share the same exponent (fast path)
	var maxExponent int
	err = l.db.QueryRow(
		`SELECT COALESCE(MAX(exponent), -2) FROM accounts WHERE full_path LIKE $1`,
		pattern,
	).Scan(&maxExponent)
	if err != nil {
		return 0, 0, fmt.Errorf("query max exponent: %w", err)
	}

	if reportExponent == maxExponent {
		// All same exponent — use simple SUM
		var balance int64
		err = l.db.QueryRow(
			`SELECT
				COALESCE(SUM(CASE WHEN a.id = m.to_account_id THEN m.amount ELSE 0 END), 0)
			  - COALESCE(SUM(CASE WHEN a.id = m.from_account_id THEN m.amount ELSE 0 END), 0)
			 FROM movements m
			 JOIN accounts a ON (a.id = m.from_account_id OR a.id = m.to_account_id)
			 WHERE a.full_path LIKE $1
			   AND m.value_time <= $2`,
			pattern, at,
		).Scan(&balance)
		if err != nil {
			return 0, 0, fmt.Errorf("query balance by path: %w", err)
		}
		return balance, reportExponent, nil
	}

	// Mixed exponents — scale each movement to the reporting exponent in Go.
	// Since movements only happen between same-exponent accounts,
	// the movement exponent equals both accounts' exponent.
	rows, err := l.db.Query(
		`SELECT m.amount, a.id, m.from_account_id, m.to_account_id, fa.exponent, ta.exponent
		 FROM movements m
		 JOIN accounts a ON (a.id = m.from_account_id OR a.id = m.to_account_id)
		 JOIN accounts fa ON fa.id = m.from_account_id
		 JOIN accounts ta ON ta.id = m.to_account_id
		 WHERE a.full_path LIKE $1 AND m.value_time <= $2`,
		pattern, at,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("query balance by path: %w", err)
	}
	defer rows.Close()

	var total int64
	for rows.Next() {
		var amount, matchedAcctID, fromAcctID, toAcctID int64
		var fromExp, toExp int
		if err := rows.Scan(&amount, &matchedAcctID, &fromAcctID, &toAcctID, &fromExp, &toExp); err != nil {
			return 0, 0, fmt.Errorf("scan movement: %w", err)
		}
		// Movement exponent = both accounts' exponent (same-exponent enforced)
		movExp := fromExp
		scaled := ScaleAmount(amount, movExp, reportExponent)
		matchedIsTo := matchedAcctID == toAcctID
		matchedIsFrom := matchedAcctID == fromAcctID
		if matchedIsTo && !matchedIsFrom {
			total += scaled
		} else if matchedIsFrom && !matchedIsTo {
			total -= scaled
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("iterate movements: %w", err)
	}
	return total, reportExponent, nil
}

// DailyBalances returns day-by-day closing balances for an account over a date range.
func (l *SQLLedger) DailyBalances(accountID int64, from, to time.Time) ([]DailyBalance, error) {
	var result []DailyBalance
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		endOfDay := time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 59, 999999999, d.Location())
		bal, err := l.BalanceAt(accountID, endOfDay)
		if err != nil {
			return nil, fmt.Errorf("balance at %s: %w", d.Format("2006-01-02"), err)
		}
		result = append(result, DailyBalance{
			Date:    time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location()),
			Balance: bal,
		})
	}
	return result, nil
}
