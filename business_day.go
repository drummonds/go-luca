package luca

import (
	"database/sql"
	"fmt"
	"time"
)

// BusinessDay is the day the pass is projecting and the business day it
// advanced from. PrevDay is zero when the day was the first.
type BusinessDay struct {
	Day     time.Time
	PrevDay time.Time
}

// AdvanceDay moves the ledger's business day to day (its midnight). The
// pass calls it before projecting the day's positions: from then on an
// account the pass has reached has its position on day, one it has not
// has it on the previous business day, and the live view reads the two
// days as two slices of the day index rather than finding each account's
// latest row. Advancing to the current day is a no-op, for a restarted
// pass. Advancing to an earlier day is an error.
func (l *SQLLedger) AdvanceDay(day time.Time) error {
	day = utc(dayStart(day))
	tx, commit, rollback, err := l.begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = rollback() }()

	cur, err := businessDay(tx)
	if err != nil {
		return err
	}
	switch {
	case cur == nil:
		if _, err := tx.Exec(`INSERT INTO ledger_day (day, prev_day) VALUES ($1, NULL)`, day); err != nil {
			return fmt.Errorf("advance day: %w", err)
		}
	case day.Equal(cur.Day):
		return nil
	case day.Before(cur.Day):
		return fmt.Errorf("advance day: %s is before the business day %s", day.Format("2006-01-02"), cur.Day.Format("2006-01-02"))
	default:
		if _, err := tx.Exec(`UPDATE ledger_day SET prev_day = $1, day = $2`, cur.Day, day); err != nil {
			return fmt.Errorf("advance day: %w", err)
		}
	}
	if err := commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// BusinessDay returns the ledger's business day, or nil before the first
// AdvanceDay.
func (l *SQLLedger) BusinessDay() (*BusinessDay, error) {
	return businessDay(l.db)
}

func businessDay(q dbtx) (*BusinessDay, error) {
	var day string
	var prev sql.NullString
	err := q.QueryRow(`SELECT day, prev_day FROM ledger_day`).Scan(&day, &prev)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("business day: %w", err)
	}
	bd := &BusinessDay{Day: parseDBTime(day)}
	if prev.Valid {
		bd.PrevDay = parseDBTime(prev.String)
	}
	return bd, nil
}
