package luca

import (
	"fmt"
	"io"
	"strings"

	"github.com/shopspring/decimal"
)

// ImportOptions controls how .goluca files are imported into a Ledger.
type ImportOptions struct {
	AutoCreateAccounts bool   // create accounts that don't exist (default true)
	DefaultCurrency    string // fallback currency (default "GBP")
}

func (o *ImportOptions) defaults() ImportOptions {
	if o == nil {
		return ImportOptions{AutoCreateAccounts: true, DefaultCurrency: "GBP"}
	}
	out := *o
	if out.DefaultCurrency == "" {
		out.DefaultCurrency = "GBP"
	}
	return out
}

// Import reads a .goluca file and records all transactions into the Ledger.
func (l *SQLLedger) Import(r io.Reader, opts *ImportOptions) error {
	o := opts.defaults()

	gf, err := ParseGoluca(r)
	if err != nil {
		return fmt.Errorf("parse goluca: %w", err)
	}

	for _, txn := range gf.Transactions {
		if err := l.importTransaction(txn, o); err != nil {
			return fmt.Errorf("import transaction %s: %w", txn.Date.Format("2006-01-02"), err)
		}
	}
	return nil
}

func (l *SQLLedger) importTransaction(txn Transaction, opts ImportOptions) error {
	if len(txn.Movements) == 0 {
		return nil
	}

	// Resolve all accounts first
	type resolvedMovement struct {
		fromID      int64
		toID        int64
		amount      Amount
		description string
		pendingID   int64
	}

	var resolved []resolvedMovement
	for _, m := range txn.Movements {
		fromAcct, err := l.resolveAccount(m.From, m, opts)
		if err != nil {
			return fmt.Errorf("resolve from account %q: %w", m.From, err)
		}
		toAcct, err := l.resolveAccount(m.To, m, opts)
		if err != nil {
			return fmt.Errorf("resolve to account %q: %w", m.To, err)
		}

		// Parse amount and convert to int64 at account exponent
		amtStr := strings.ReplaceAll(m.Amount, ",", "")
		amtDec, err := decimal.NewFromString(amtStr)
		if err != nil {
			return fmt.Errorf("parse amount %q: %w", m.Amount, err)
		}
		amount := DecimalToInt(amtDec, fromAcct.Exponent)

		desc := m.Description
		if desc == "" && txn.Payee != "" && len(txn.Movements) == 1 {
			desc = txn.Payee
		}

		var pendingID int64
		if txn.Flag == '!' {
			pendingID = 1
		}

		resolved = append(resolved, resolvedMovement{
			fromID:      fromAcct.ID,
			toID:        toAcct.ID,
			amount:      amount,
			description: desc,
			pendingID:   pendingID,
		})
	}

	if len(resolved) == 1 {
		rm := resolved[0]
		if rm.pendingID != 0 {
			// Use linked movements to set PendingID
			_, err := l.RecordLinkedMovements([]MovementInput{{
				FromAccountID: rm.fromID,
				ToAccountID:   rm.toID,
				Amount:        rm.amount,
				Description:   rm.description,
				PendingID:     rm.pendingID,
			}}, txn.Date)
			return err
		}
		_, err := l.RecordMovement(rm.fromID, rm.toID, rm.amount, txn.Date, rm.description)
		return err
	}

	// Multiple movements → linked
	var inputs []MovementInput
	for _, rm := range resolved {
		inputs = append(inputs, MovementInput{
			FromAccountID: rm.fromID,
			ToAccountID:   rm.toID,
			Amount:        rm.amount,
			Description:   rm.description,
			PendingID:     rm.pendingID,
		})
	}
	_, err := l.RecordLinkedMovements(inputs, txn.Date)
	return err
}

func (l *SQLLedger) resolveAccount(path string, m TextMovement, opts ImportOptions) (*Account, error) {
	acct, err := l.GetAccount(path)
	if err != nil {
		return nil, err
	}
	if acct != nil {
		return acct, nil
	}
	if !opts.AutoCreateAccounts {
		return nil, fmt.Errorf("account %q not found", path)
	}
	// Infer exponent from decimal places in amount
	exp := inferExponent(m.Amount)
	currency := m.Commodity
	if currency == "" {
		currency = opts.DefaultCurrency
	}
	return l.CreateAccount(path, currency, exp, 0)
}

// inferExponent returns the exponent (e.g. -2) from the decimal places in an amount string.
func inferExponent(amount string) int {
	amount = strings.ReplaceAll(amount, ",", "")
	dotIdx := strings.LastIndex(amount, ".")
	if dotIdx < 0 {
		return 0
	}
	return -(len(amount) - dotIdx - 1)
}

// ImportString is a convenience wrapper that imports from a string.
func (l *SQLLedger) ImportString(s string, opts *ImportOptions) error {
	return l.Import(strings.NewReader(s), opts)
}
