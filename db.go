package luca

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// CreateAccount inserts a new account and returns it with the generated ID.
// fullPath is parsed to extract Type, Product, AccountID, and Address components.
func (l *Ledger) CreateAccount(fullPath string, currency string, exponent int, annualInterestRate float64) (*Account, error) {
	accountType, product, accountID, address, isPending, err := parseFullPath(fullPath)
	if err != nil {
		return nil, fmt.Errorf("parse path: %w", err)
	}

	var id int64
	err = l.db.QueryRow(
		`INSERT INTO accounts (full_path, account_type, product, account_id, address, is_pending, currency, exponent, annual_interest_rate)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id`,
		fullPath, string(accountType), product, accountID, address, isPending, currency, exponent, annualInterestRate,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("insert account: %w", err)
	}

	return &Account{
		ID:                 id,
		FullPath:           fullPath,
		Type:               accountType,
		Product:            product,
		AccountID:          accountID,
		Address:            address,
		IsPending:          isPending,
		Currency:           currency,
		Exponent:           exponent,
		AnnualInterestRate: annualInterestRate,
		CreatedAt:          time.Now(),
	}, nil
}

// scanAccount scans an account row into an Account struct.
// created_at is stored as TEXT by SQLite, so we scan it as a string and parse it.
func scanAccount(scanner interface{ Scan(...any) error }) (*Account, error) {
	a := &Account{}
	var typeStr, createdAtStr string
	err := scanner.Scan(&a.ID, &a.FullPath, &typeStr, &a.Product, &a.AccountID, &a.Address, &a.IsPending, &a.Currency, &a.Exponent, &a.AnnualInterestRate, &createdAtStr)
	if err != nil {
		return nil, err
	}
	a.Type = AccountType(typeStr)
	a.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAtStr)
	return a, nil
}

const accountColumns = `id, full_path, account_type, product, account_id, address, is_pending, currency, exponent, annual_interest_rate, created_at`

// GetAccount retrieves an account by its full path.
func (l *Ledger) GetAccount(fullPath string) (*Account, error) {
	row := l.db.QueryRow(
		`SELECT `+accountColumns+`
		 FROM accounts WHERE full_path = $1`, fullPath,
	)
	a, err := scanAccount(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}
	return a, nil
}

// GetAccountByID retrieves an account by its database ID.
func (l *Ledger) GetAccountByID(id int64) (*Account, error) {
	row := l.db.QueryRow(
		`SELECT `+accountColumns+`
		 FROM accounts WHERE id = $1`, id,
	)
	a, err := scanAccount(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get account by id: %w", err)
	}
	return a, nil
}

// ListAccounts returns all accounts, optionally filtered by type.
// Pass empty string to list all accounts.
func (l *Ledger) ListAccounts(typeFilter AccountType) ([]*Account, error) {
	var rows *sql.Rows
	var err error
	if typeFilter == "" {
		rows, err = l.db.Query(
			`SELECT ` + accountColumns + `
			 FROM accounts ORDER BY full_path`)
	} else {
		rows, err = l.db.Query(
			`SELECT `+accountColumns+`
			 FROM accounts WHERE account_type = $1 ORDER BY full_path`, string(typeFilter))
	}
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()

	var accounts []*Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

// nextBatchID returns the next available batch ID.
func (l *Ledger) nextBatchID(tx *sql.Tx) (int64, error) {
	var id int64
	err := tx.QueryRow(`SELECT COALESCE(MAX(batch_id), 0) + 1 FROM movements`).Scan(&id)
	return id, err
}

// validateSameExponent checks that both accounts exist and share the same exponent.
// Movements between accounts with different exponents are not allowed — that
// is treated as a currency conversion requiring explicit handling.
func (l *Ledger) validateSameExponent(fromAccountID, toAccountID int64) error {
	fromAcct, err := l.GetAccountByID(fromAccountID)
	if err != nil {
		return fmt.Errorf("get from account: %w", err)
	}
	if fromAcct == nil {
		return fmt.Errorf("from account %d not found", fromAccountID)
	}
	toAcct, err := l.GetAccountByID(toAccountID)
	if err != nil {
		return fmt.Errorf("get to account: %w", err)
	}
	if toAcct == nil {
		return fmt.Errorf("to account %d not found", toAccountID)
	}
	if fromAcct.Exponent != toAcct.Exponent {
		return fmt.Errorf("exponent mismatch: from account %q has exponent %d, to account %q has exponent %d (use currency conversion for cross-exponent transfers)",
			fromAcct.FullPath, fromAcct.Exponent, toAcct.FullPath, toAcct.Exponent)
	}
	return nil
}

// RecordMovement inserts a single movement and returns it.
// Both accounts must have the same exponent; cross-exponent transfers are rejected.
// amount is an integer in the smallest currency unit at the accounts' shared exponent.
func (l *Ledger) RecordMovement(fromAccountID, toAccountID int64, amount int64, valueTime time.Time, description string) (*Movement, error) {
	if err := l.validateSameExponent(fromAccountID, toAccountID); err != nil {
		return nil, err
	}

	tx, err := l.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	batchID, err := l.nextBatchID(tx)
	if err != nil {
		return nil, fmt.Errorf("next batch id: %w", err)
	}

	var id int64
	err = tx.QueryRow(
		`INSERT INTO movements (batch_id, from_account_id, to_account_id, amount, value_time, description)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id`,
		batchID, fromAccountID, toAccountID, amount, valueTime, description,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("insert movement: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &Movement{
		ID:            id,
		BatchID:       batchID,
		FromAccountID: fromAccountID,
		ToAccountID:   toAccountID,
		Amount:        amount,
		ValueTime:     valueTime,
		KnowledgeTime: time.Now(),
		Description:   description,
	}, nil
}

// RecordLinkedMovements inserts multiple movements sharing the same batch_id
// within a single database transaction.
// All account pairs must share the same exponent.
func (l *Ledger) RecordLinkedMovements(movements []MovementInput, valueTime time.Time) (int64, error) {
	if len(movements) == 0 {
		return 0, fmt.Errorf("no movements to record")
	}

	for _, m := range movements {
		if err := l.validateSameExponent(m.FromAccountID, m.ToAccountID); err != nil {
			return 0, err
		}
	}

	tx, err := l.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	batchID, err := l.nextBatchID(tx)
	if err != nil {
		return 0, fmt.Errorf("next batch id: %w", err)
	}

	for _, m := range movements {
		_, err := tx.Exec(
			`INSERT INTO movements (batch_id, from_account_id, to_account_id, amount, code, ledger, pending_id, user_data_64, value_time, description)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			batchID, m.FromAccountID, m.ToAccountID, m.Amount, m.Code, m.Ledger, m.PendingID, m.UserData64, valueTime, m.Description,
		)
		if err != nil {
			return 0, fmt.Errorf("insert linked movement: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}

	return batchID, nil
}

// endOfDayTime returns 23:59:59.999999999 for the date of t.
func endOfDayTime(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, t.Location())
}

// txBalance computes the balance for accountID within a transaction,
// seeing all writes made so far in that tx.
func txBalance(tx *sql.Tx, accountID int64, at time.Time) (int64, error) {
	var balance int64
	err := tx.QueryRow(
		`SELECT
			COALESCE((SELECT SUM(amount) FROM movements WHERE to_account_id = $1 AND value_time <= $2), 0)
		  - COALESCE((SELECT SUM(amount) FROM movements WHERE from_account_id = $3 AND value_time <= $4), 0)`,
		accountID, at, accountID, at,
	).Scan(&balance)
	return balance, err
}

// RecordMovementWithProjections records a movement and, in the same transaction,
// pre-computes interest accrual and the end-of-day live balance for the
// to-account. This avoids separate end-of-day batch processing.
func (l *Ledger) RecordMovementWithProjections(fromAccountID, toAccountID int64, amount int64, valueTime time.Time, description string) (*Movement, error) {
	if err := l.validateSameExponent(fromAccountID, toAccountID); err != nil {
		return nil, err
	}

	toAcct, err := l.GetAccountByID(toAccountID)
	if err != nil {
		return nil, fmt.Errorf("get to account: %w", err)
	}

	// Look up interest accounts before starting the transaction.
	// With :memory: SQLite, l.db queries during an open tx may hit
	// a different connection (and thus a different empty database).
	var expenseAcctID int64
	if toAcct.AnnualInterestRate != 0 {
		expenseAcct, err := l.GetAccount("Expense:Interest")
		if err != nil || expenseAcct == nil {
			return nil, fmt.Errorf("interest expense account not found, call EnsureInterestAccounts first")
		}
		expenseAcctID = expenseAcct.ID
	}

	tx, err := l.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	batchID, err := l.nextBatchID(tx)
	if err != nil {
		return nil, fmt.Errorf("next batch id: %w", err)
	}

	// 1. Insert the real movement (code=0)
	var movID int64
	err = tx.QueryRow(
		`INSERT INTO movements (batch_id, from_account_id, to_account_id, amount, code, value_time, description)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id`,
		batchID, fromAccountID, toAccountID, amount, CodeNormal, valueTime, description,
	).Scan(&movID)
	if err != nil {
		return nil, fmt.Errorf("insert movement: %w", err)
	}

	eod := endOfDayTime(valueTime)

	// 2. Compute new balance for toAccountID as of end-of-day (tx sees own writes)
	balance, err := txBalance(tx, toAccountID, eod)
	if err != nil {
		return nil, fmt.Errorf("compute balance: %w", err)
	}

	// 3. If account has interest rate, compute and upsert the accrual
	var interestAmount int64
	if toAcct.AnnualInterestRate != 0 {
		balDec := IntToDecimal(balance, toAcct.Exponent)
		rate := decimal.NewFromFloat(toAcct.AnnualInterestRate)
		dailyRate := rate.Div(decimal.NewFromInt(365))
		interestDec := balDec.Mul(dailyRate)
		interestAmount = DecimalToInt(interestDec, toAcct.Exponent)

		if interestAmount != 0 {
			// Delete old accrual for this account+date, then insert the new one
			_, err = tx.Exec(
				`DELETE FROM movements
				 WHERE to_account_id = $1 AND code = $2
				   AND value_time >= $3 AND value_time <= $4`,
				toAccountID, CodeInterestAccrual,
				time.Date(valueTime.Year(), valueTime.Month(), valueTime.Day(), 0, 0, 0, 0, valueTime.Location()),
				eod,
			)
			if err != nil {
				return nil, fmt.Errorf("delete old accrual: %w", err)
			}

			desc := fmt.Sprintf("Daily interest for %s", valueTime.Format("2006-01-02"))
			_, err = tx.Exec(
				`INSERT INTO movements (batch_id, from_account_id, to_account_id, amount, code, value_time, description)
				 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				batchID, expenseAcctID, toAccountID, interestAmount, CodeInterestAccrual, eod, desc,
			)
			if err != nil {
				return nil, fmt.Errorf("insert interest accrual: %w", err)
			}
		}
	}

	// 4. Upsert live balance (delete+insert for pglike compatibility)
	balanceDate := time.Date(valueTime.Year(), valueTime.Month(), valueTime.Day(), 0, 0, 0, 0, valueTime.Location())
	_, err = tx.Exec(
		`DELETE FROM balances_live WHERE account_id = $1 AND balance_date = $2`,
		toAccountID, balanceDate,
	)
	if err != nil {
		return nil, fmt.Errorf("delete old live balance: %w", err)
	}

	// Re-query balance after interest accrual was inserted
	finalBalance, err := txBalance(tx, toAccountID, eod)
	if err != nil {
		return nil, fmt.Errorf("compute final balance: %w", err)
	}

	_, err = tx.Exec(
		`INSERT INTO balances_live (account_id, balance_date, balance)
		 VALUES ($1, $2, $3)`,
		toAccountID, balanceDate, finalBalance,
	)
	if err != nil {
		return nil, fmt.Errorf("insert live balance: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &Movement{
		ID:            movID,
		BatchID:       batchID,
		FromAccountID: fromAccountID,
		ToAccountID:   toAccountID,
		Amount:        amount,
		Code:          CodeNormal,
		ValueTime:     valueTime,
		KnowledgeTime: time.Now(),
		Description:   description,
	}, nil
}

// GetLiveBalance reads the pre-computed end-of-day balance from balances_live.
// Returns nil if no balance exists for the given account and date.
func (l *Ledger) GetLiveBalance(accountID int64, date time.Time) (*LiveBalance, error) {
	balanceDate := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	var lb LiveBalance
	var dateStr string
	err := l.db.QueryRow(
		`SELECT account_id, balance_date, balance
		 FROM balances_live
		 WHERE account_id = $1 AND balance_date = $2`,
		accountID, balanceDate,
	).Scan(&lb.AccountID, &dateStr, &lb.Balance)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get live balance: %w", err)
	}
	lb.BalanceDate, _ = time.Parse("2006-01-02 15:04:05", dateStr)
	return &lb, nil
}
