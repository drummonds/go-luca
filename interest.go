package luca

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// InterestResult records one day's interest calculation for one account.
type InterestResult struct {
	AccountID      string
	Date           time.Time
	OpeningBalance Amount
	InterestAmount Amount
	ClosingBalance Amount
	Exponent       int
}

// EnsureInterestAccounts creates the system accounts needed for interest
// processing. These are created idempotently.
func (l *SQLLedger) EnsureInterestAccounts() error {
	for _, path := range []string{"Expense:Interest", "Income:Interest"} {
		existing, err := l.GetAccount(path)
		if err != nil {
			return fmt.Errorf("check account %s: %w", path, err)
		}
		if existing == nil {
			if _, err := l.CreateAccount(path, "GBP", -2, 0); err != nil {
				return fmt.Errorf("create account %s: %w", path, err)
			}
		}
	}
	return nil
}

// CalculateDailyInterest computes one day's interest for a single account.
// Formula: interest = closingBalance * (annualRate / 365)
// The interest is recorded as a movement from Expense:Interest to the account.
// All amounts use the account's exponent.
func (l *SQLLedger) CalculateDailyInterest(accountID string, date time.Time) (*InterestResult, error) {
	acct, err := l.GetAccountByID(accountID)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}
	if acct == nil {
		return nil, fmt.Errorf("account %s not found", accountID)
	}
	if acct.AnnualInterestRate == 0 {
		return nil, nil
	}

	endOfDay := time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, 999999999, date.Location())
	balance, err := l.BalanceAt(accountID, endOfDay)
	if err != nil {
		return nil, fmt.Errorf("get balance: %w", err)
	}

	// Convert balance to decimal, compute interest, convert back
	balDec := IntToDecimal(balance, acct.Exponent)
	rate := decimal.NewFromFloat(acct.AnnualInterestRate)
	dailyRate := rate.Div(decimal.NewFromInt(365))
	interestDec := balDec.Mul(dailyRate)
	interest := DecimalToInt(interestDec, acct.Exponent)

	if interest == 0 {
		return &InterestResult{
			AccountID:      accountID,
			Date:           date,
			OpeningBalance: balance,
			InterestAmount: 0,
			ClosingBalance: balance,
			Exponent:       acct.Exponent,
		}, nil
	}

	expenseAcct, err := l.GetAccount("Expense:Interest")
	if err != nil || expenseAcct == nil {
		return nil, fmt.Errorf("interest expense account not found, call EnsureInterestAccounts first")
	}

	desc := fmt.Sprintf("Daily interest for %s", date.Format("2006-01-02"))
	valueTime := time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, 0, date.Location())

	if interest > 0 {
		// Interest increases account balance: Expense:Interest → account
		_, err = l.RecordMovement(expenseAcct.ID, accountID, interest, CodeInterestAccrual, valueTime, desc)
	} else {
		// Negative interest: account → Income:Interest
		incomeAcct, err2 := l.GetAccount("Income:Interest")
		if err2 != nil || incomeAcct == nil {
			return nil, fmt.Errorf("interest income account not found")
		}
		_, err = l.RecordMovement(accountID, incomeAcct.ID, -interest, CodeInterestAccrual, valueTime, desc)
	}
	if err != nil {
		return nil, fmt.Errorf("record interest movement: %w", err)
	}

	return &InterestResult{
		AccountID:      accountID,
		Date:           date,
		OpeningBalance: balance,
		InterestAmount: interest,
		ClosingBalance: balance + interest,
		Exponent:       acct.Exponent,
	}, nil
}

// RunDailyInterest processes interest for all accounts that have a non-zero
// annual_interest_rate, for the given date.
func (l *SQLLedger) RunDailyInterest(date time.Time) ([]InterestResult, error) {
	rows, err := l.db.Query(
		`SELECT id FROM accounts WHERE annual_interest_rate != 0 ORDER BY full_path`)
	if err != nil {
		return nil, fmt.Errorf("list interest accounts: %w", err)
	}
	defer rows.Close()

	var accountIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan account id: %w", err)
		}
		accountIDs = append(accountIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var results []InterestResult
	for _, id := range accountIDs {
		result, err := l.CalculateDailyInterest(id, date)
		if err != nil {
			return nil, fmt.Errorf("interest for account %s: %w", id, err)
		}
		if result != nil {
			results = append(results, *result)
		}
	}
	return results, nil
}

// RunInterestForPeriod runs daily interest for every day in [from, to] inclusive.
func (l *SQLLedger) RunInterestForPeriod(from, to time.Time) ([]InterestResult, error) {
	var allResults []InterestResult
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		results, err := l.RunDailyInterest(d)
		if err != nil {
			return nil, fmt.Errorf("interest for %s: %w", d.Format("2006-01-02"), err)
		}
		allResults = append(allResults, results...)
	}
	return allResults, nil
}
