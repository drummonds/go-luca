package luca

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// utc normalises a time to UTC so that SQLite TEXT comparisons (<=, >=, ORDER BY)
// on RFC3339 timestamps work correctly. Without this, different timezone offsets
// produce strings that sort by offset characters rather than actual instant.
func utc(t time.Time) time.Time { return t.UTC() }

// parseDBTime parses a timestamp string from SQLite. The ncruces/go-sqlite3
// driver returns RFC3339 ("2006-01-02T00:00:00Z"); older modernc returned
// "2006-01-02 15:04:05 -0700 MST". Try both.
func parseDBTime(s string) time.Time {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	t, _ := time.Parse("2006-01-02 15:04:05 -0700 MST", s)
	return t
}

// commodityExponent returns the exponent for a commodity code, or an error if not found.
func (l *SQLLedger) commodityExponent(code string) (int, error) {
	var exp int
	err := l.db.QueryRow(`SELECT exponent FROM commodities WHERE code = $1`, code).Scan(&exp)
	if err != nil {
		return 0, err
	}
	return exp, nil
}

// ensureCommodity creates the commodity if it doesn't exist, or verifies
// the exponent matches an existing commodity.
func (l *SQLLedger) ensureCommodity(code string, exponent int) error {
	var existingExp int
	err := l.db.QueryRow(
		`SELECT exponent FROM commodities WHERE code = $1`, code,
	).Scan(&existingExp)
	if err == sql.ErrNoRows {
		_, err = l.db.Exec(
			`INSERT INTO commodities (id, code, exponent) VALUES ($1, $2, $3)`,
			uuid.New().String(), code, exponent,
		)
		if err != nil {
			return fmt.Errorf("insert commodity %s: %w", code, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("check commodity %s: %w", code, err)
	}
	if existingExp != exponent {
		return fmt.Errorf("commodity %s has exponent %d, got %d", code, existingExp, exponent)
	}
	return nil
}

// CreateAccount inserts a new account and returns it with the generated ID.
// fullPath is parsed to extract Type, Product, AccountID, and Address components.
// The commodity is auto-created if it doesn't exist.
func (l *SQLLedger) CreateAccount(fullPath string, commodity string, exponent int, grossInterestRate float64) (*Account, error) {
	accountType, product, accountID, address, isPending, err := parseFullPath(fullPath)
	if err != nil {
		return nil, fmt.Errorf("parse path: %w", err)
	}

	if err := l.ensureCommodity(commodity, exponent); err != nil {
		return nil, err
	}

	id := uuid.New().String()
	// Default interest method when rate != 0
	var method InterestMethod
	if grossInterestRate != 0 {
		method = InterestMethodDefault
	}

	_, err = l.db.Exec(
		`INSERT INTO accounts (id, full_path, account_type, product, account_id, address, is_pending, commodity, gross_interest_rate, interest_method)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		id, fullPath, string(accountType), product, accountID, address, isPending, commodity, grossInterestRate, string(method),
	)
	if err != nil {
		return nil, fmt.Errorf("insert account: %w", err)
	}

	return &Account{
		ID:                id,
		FullPath:          fullPath,
		Type:              accountType,
		Product:           product,
		AccountID:         accountID,
		Address:           address,
		IsPending:         isPending,
		Commodity:         commodity,
		Exponent:          exponent,
		GrossInterestRate: grossInterestRate,
		InterestMethod:    method,
		CreatedAt:         time.Now(),
	}, nil
}

// SetInterestMethod sets the interest calculation method for an account.
func (l *SQLLedger) SetInterestMethod(accountID string, method InterestMethod) error {
	_, err := l.db.Exec(
		`UPDATE accounts SET interest_method = $1 WHERE id = $2`,
		string(method), accountID,
	)
	return err
}

// SetAccountOpenedAt sets the opened_at timestamp for an account.
func (l *SQLLedger) SetAccountOpenedAt(accountID string, openedAt time.Time) error {
	_, err := l.db.Exec(
		`UPDATE accounts SET opened_at = $1 WHERE id = $2`,
		utc(openedAt), accountID,
	)
	return err
}

// scanAccount scans an account row into an Account struct.
// The query must JOIN commodities to provide exponent (see accountSelect).
// created_at and opened_at are stored as TEXT by SQLite, so we scan them as strings and parse.
func scanAccount(scanner interface{ Scan(...any) error }) (*Account, error) {
	a := &Account{}
	var typeStr, methodStr, createdAtStr string
	var openedAtStr, customerID sql.NullString
	err := scanner.Scan(&a.ID, &a.FullPath, &typeStr, &a.Product, &a.AccountID, &a.Address, &a.IsPending, &a.Commodity, &customerID, &a.Exponent, &a.GrossInterestRate, &methodStr, &a.InterestAccumulator, &openedAtStr, &createdAtStr)
	if err != nil {
		return nil, err
	}
	a.Type = AccountType(typeStr)
	a.InterestMethod = InterestMethod(methodStr)
	if customerID.Valid {
		a.CustomerID = customerID.String
	}
	a.CreatedAt = parseDBTime(createdAtStr)
	if openedAtStr.Valid && openedAtStr.String != "" {
		t := parseDBTime(openedAtStr.String)
		if !t.IsZero() {
			a.OpenedAt = &t
		}
	}
	return a, nil
}

// accountSelect is the SELECT columns for account queries.
// Exponent comes from the commodities table via JOIN.
const accountSelect = `a.id, a.full_path, a.account_type, a.product, a.account_id, a.address, a.is_pending, a.commodity, a.customer_id, c.exponent, a.gross_interest_rate, a.interest_method, a.interest_accumulator, a.opened_at, a.created_at`

const accountFrom = ` FROM accounts a JOIN commodities c ON c.code = a.commodity`

// GetAccount retrieves an account by its full path.
func (l *SQLLedger) GetAccount(fullPath string) (*Account, error) {
	row := l.db.QueryRow(
		`SELECT `+accountSelect+accountFrom+`
		 WHERE a.full_path = $1`, fullPath,
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
func (l *SQLLedger) GetAccountByID(id string) (*Account, error) {
	row := l.db.QueryRow(
		`SELECT `+accountSelect+accountFrom+`
		 WHERE a.id = $1`, id,
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
func (l *SQLLedger) ListAccounts(typeFilter AccountType) ([]*Account, error) {
	var rows *sql.Rows
	var err error
	if typeFilter == "" {
		rows, err = l.db.Query(
			`SELECT ` + accountSelect + accountFrom + `
			 ORDER BY a.full_path`)
	} else {
		rows, err = l.db.Query(
			`SELECT `+accountSelect+accountFrom+`
			 WHERE a.account_type = $1 ORDER BY a.full_path`, string(typeFilter))
	}
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer func() { _ = rows.Close() }()

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

// validateSameExponent checks that both accounts exist and share the same exponent.
// Movements between accounts with different exponents are not allowed — that
// is treated as a currency conversion requiring explicit handling.
func (l *SQLLedger) validateSameExponent(fromAccountID, toAccountID string) error {
	fromAcct, err := l.GetAccountByID(fromAccountID)
	if err != nil {
		return fmt.Errorf("get from account: %w", err)
	}
	if fromAcct == nil {
		return fmt.Errorf("from account %s not found", fromAccountID)
	}
	toAcct, err := l.GetAccountByID(toAccountID)
	if err != nil {
		return fmt.Errorf("get to account: %w", err)
	}
	if toAcct == nil {
		return fmt.Errorf("to account %s not found", toAccountID)
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
func (l *SQLLedger) RecordMovement(fromAccountID, toAccountID string, amount Amount, code string, valueTime time.Time, description string) (*Movement, error) {
	if code == "" {
		return nil, fmt.Errorf("movement code is required")
	}
	if err := l.validateSameExponent(fromAccountID, toAccountID); err != nil {
		return nil, err
	}

	tx, err := l.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	batchID := uuid.New().String()
	movID := uuid.New().String()

	_, err = tx.Exec(
		`INSERT INTO movements (id, batch_id, from_account_id, to_account_id, amount, code, value_time, description)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		movID, batchID, fromAccountID, toAccountID, amount, code, utc(valueTime), description,
	)
	if err != nil {
		return nil, fmt.Errorf("insert movement: %w", err)
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
		Code:          code,
		ValueTime:     valueTime,
		KnowledgeTime: time.Now(),
		Description:   description,
	}, nil
}

// RecordLinkedMovements inserts multiple movements sharing the same batch_id
// within a single database transaction.
// All account pairs must share the same exponent.
func (l *SQLLedger) RecordLinkedMovements(movements []MovementInput, valueTime time.Time) (string, error) {
	if len(movements) == 0 {
		return "", fmt.Errorf("no movements to record")
	}

	for _, m := range movements {
		if m.Code == "" {
			return "", fmt.Errorf("movement code is required")
		}
		if err := l.validateSameExponent(m.FromAccountID, m.ToAccountID); err != nil {
			return "", err
		}
	}

	tx, err := l.db.Begin()
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	batchID := uuid.New().String()

	for _, m := range movements {
		movID := uuid.New().String()
		if m.KnowledgeTime != nil {
			_, err := tx.Exec(
				`INSERT INTO movements (id, batch_id, from_account_id, to_account_id, amount, code, ledger, pending_id, user_data_64, value_time, knowledge_time, description, period_anchor)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
				movID, batchID, m.FromAccountID, m.ToAccountID, m.Amount, m.Code, m.Ledger, m.PendingID, m.UserData64, utc(valueTime), utc(*m.KnowledgeTime), m.Description, m.PeriodAnchor,
			)
			if err != nil {
				return "", fmt.Errorf("insert linked movement: %w", err)
			}
		} else {
			_, err := tx.Exec(
				`INSERT INTO movements (id, batch_id, from_account_id, to_account_id, amount, code, ledger, pending_id, user_data_64, value_time, description, period_anchor)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
				movID, batchID, m.FromAccountID, m.ToAccountID, m.Amount, m.Code, m.Ledger, m.PendingID, m.UserData64, utc(valueTime), m.Description, m.PeriodAnchor,
			)
			if err != nil {
				return "", fmt.Errorf("insert linked movement: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}

	return batchID, nil
}

// AddMovementToBatch appends a movement to an existing batch (transaction).
// The movement shares the batch's value_time.
func (l *SQLLedger) AddMovementToBatch(batchID string, input MovementInput) (*Movement, error) {
	if err := l.validateSameExponent(input.FromAccountID, input.ToAccountID); err != nil {
		return nil, err
	}

	// Query any existing movement in the batch to get value_time
	var valueTimeStr string
	err := l.db.QueryRow(
		`SELECT value_time FROM movements WHERE batch_id = $1 LIMIT 1`,
		batchID,
	).Scan(&valueTimeStr)
	if err != nil {
		return nil, fmt.Errorf("batch not found")
	}
	valueTime := parseDBTime(valueTimeStr)

	movID := uuid.New().String()
	_, err = l.db.Exec(
		`INSERT INTO movements (id, batch_id, from_account_id, to_account_id, amount, code, ledger, pending_id, user_data_64, value_time, description, period_anchor)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		movID, batchID, input.FromAccountID, input.ToAccountID, input.Amount,
		input.Code, input.Ledger, input.PendingID, input.UserData64,
		utc(valueTime), input.Description, input.PeriodAnchor,
	)
	if err != nil {
		return nil, fmt.Errorf("insert movement: %w", err)
	}

	return &Movement{
		ID:            movID,
		BatchID:       batchID,
		FromAccountID: input.FromAccountID,
		ToAccountID:   input.ToAccountID,
		Amount:        input.Amount,
		Code:          input.Code,
		Ledger:        input.Ledger,
		PendingID:     input.PendingID,
		UserData64:    input.UserData64,
		ValueTime:     valueTime,
		KnowledgeTime: time.Now(),
		Description:   input.Description,
		PeriodAnchor:  input.PeriodAnchor,
	}, nil
}

// endOfDayTime returns 23:59:59.999999999 for the date of t.
func endOfDayTime(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, t.Location())
}

// txBalance computes the balance for accountID within a transaction,
// seeing all writes made so far in that tx.
func txBalance(tx *sql.Tx, accountID string, at time.Time) (Amount, error) {
	var balance Amount
	err := tx.QueryRow(
		`SELECT
			COALESCE((SELECT SUM(amount) FROM movements WHERE to_account_id = $1 AND value_time <= $2), 0)
		  - COALESCE((SELECT SUM(amount) FROM movements WHERE from_account_id = $3 AND value_time <= $4), 0)`,
		accountID, utc(at), accountID, utc(at),
	).Scan(&balance)
	return balance, err
}

// RecordMovementWithProjections records a movement and, in the same transaction,
// upserts the end-of-day live balance for the to-account.
// Interest computation (if needed) is handled by gobank-products.
func (l *SQLLedger) RecordMovementWithProjections(fromAccountID, toAccountID string, amount Amount, code string, valueTime time.Time, description string) (*Movement, error) {
	if err := l.validateSameExponent(fromAccountID, toAccountID); err != nil {
		return nil, err
	}

	tx, err := l.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	batchID := uuid.New().String()
	movID := uuid.New().String()

	// 1. Insert the movement
	_, err = tx.Exec(
		`INSERT INTO movements (id, batch_id, from_account_id, to_account_id, amount, code, value_time, description)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		movID, batchID, fromAccountID, toAccountID, amount, code, utc(valueTime), description,
	)
	if err != nil {
		return nil, fmt.Errorf("insert movement: %w", err)
	}

	eod := endOfDayTime(valueTime)

	// 2. Compute end-of-day balance (tx sees own writes)
	balance, err := txBalance(tx, toAccountID, eod)
	if err != nil {
		return nil, fmt.Errorf("compute balance: %w", err)
	}

	// 3. Upsert live balance (delete+insert for pglike compatibility)
	balanceDate := time.Date(valueTime.Year(), valueTime.Month(), valueTime.Day(), 0, 0, 0, 0, valueTime.Location())
	_, err = tx.Exec(
		`DELETE FROM balances_live WHERE account_id = $1 AND balance_date = $2`,
		toAccountID, utc(balanceDate),
	)
	if err != nil {
		return nil, fmt.Errorf("delete old live balance: %w", err)
	}

	_, err = tx.Exec(
		`INSERT INTO balances_live (id, account_id, balance_date, balance)
		 VALUES ($1, $2, $3, $4)`,
		uuid.New().String(), toAccountID, utc(balanceDate), balance,
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
		Code:          code,
		ValueTime:     valueTime,
		KnowledgeTime: time.Now(),
		Description:   description,
	}, nil
}

// GetLiveBalance reads the pre-computed end-of-day balance from balances_live.
// Returns nil if no balance exists for the given account and date.
func (l *SQLLedger) GetLiveBalance(accountID string, date time.Time) (*LiveBalance, error) {
	balanceDate := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	var lb LiveBalance
	var dateStr string
	err := l.db.QueryRow(
		`SELECT account_id, balance_date, balance
		 FROM balances_live
		 WHERE account_id = $1 AND balance_date = $2`,
		accountID, utc(balanceDate),
	).Scan(&lb.AccountID, &dateStr, &lb.Balance)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get live balance: %w", err)
	}
	lb.BalanceDate = parseDBTime(dateStr)
	return &lb, nil
}
