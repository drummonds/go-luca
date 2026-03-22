package luca

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// ImportOptions controls how .goluca files are imported into a Ledger.
type ImportOptions struct {
	AutoCreateAccounts bool   // create accounts that don't exist (default true)
	DefaultCommodity   string // fallback commodity (default "GBP")
}

func (o *ImportOptions) defaults() ImportOptions {
	if o == nil {
		return ImportOptions{AutoCreateAccounts: true, DefaultCommodity: "GBP"}
	}
	out := *o
	if out.DefaultCommodity == "" {
		out.DefaultCommodity = "GBP"
	}
	return out
}

// Import reads a .goluca file and records all transactions and directives into the Ledger.
func (l *SQLLedger) Import(r io.Reader, opts *ImportOptions) error {
	o := opts.defaults()

	gf, err := ParseGoluca(r)
	if err != nil {
		return fmt.Errorf("parse goluca: %w", err)
	}

	// Import directives first (aliases needed for account resolution)
	if err := l.importDirectives(gf, o); err != nil {
		return fmt.Errorf("import directives: %w", err)
	}

	for _, txn := range gf.Transactions {
		if err := l.importTransaction(txn, o); err != nil {
			return fmt.Errorf("import transaction %s: %w", txn.DateTime.String(), err)
		}
	}

	// Import transaction metadata after movements are recorded
	// (we need the batch IDs which are set during importTransaction)

	return nil
}

func (l *SQLLedger) importDirectives(gf *GolucaFile, opts ImportOptions) error {
	// Options
	for _, opt := range gf.Options {
		if err := l.UpsertOption(opt.Key, opt.Value); err != nil {
			return fmt.Errorf("upsert option %q: %w", opt.Key, err)
		}
	}

	// Commodities
	for _, c := range gf.Commodities {
		var dt *time.Time
		if c.DateTime != nil {
			t, err := c.DateTime.ToTime()
			if err == nil {
				dt = &t
			}
		}
		// Exponent defaults to -2; can be overridden by metadata "exponent"
		exp := -2
		if expStr, ok := c.Metadata["exponent"]; ok {
			if _, err := fmt.Sscanf(expStr, "%d", &exp); err != nil {
				return fmt.Errorf("parse exponent for commodity %q: %w", c.Code, err)
			}
		}
		cid, err := l.CreateCommodity(c.Code, exp, dt)
		if err != nil {
			return fmt.Errorf("create commodity %q: %w", c.Code, err)
		}
		for key, value := range c.Metadata {
			if err := l.SetCommodityMetadata(cid, key, value); err != nil {
				return fmt.Errorf("set commodity metadata %q/%q: %w", c.Code, key, err)
			}
		}
	}

	// Customers (before accounts, since accounts.customer_id FKs to customers.id)
	for _, c := range gf.Customers {
		cid, err := l.CreateCustomer(c.Name)
		if err != nil {
			return fmt.Errorf("create customer %q: %w", c.Name, err)
		}
		if c.MaxBalanceAmount != "" {
			if err := l.SetCustomerMaxBalance(cid, c.MaxBalanceAmount, c.MaxBalanceCommodity); err != nil {
				return fmt.Errorf("set customer max balance %q: %w", c.Name, err)
			}
		}
		for key, value := range c.Metadata {
			if err := l.SetCustomerMetadata(cid, key, value); err != nil {
				return fmt.Errorf("set customer metadata %q/%q: %w", c.Name, key, err)
			}
		}
	}

	// Opens — set opened_at on existing account or auto-create (before aliases, which FK to accounts)
	for _, o := range gf.Opens {
		openedAt, err := o.DateTime.ToTime()
		if err != nil {
			return fmt.Errorf("parse open datetime for %q: %w", o.Account, err)
		}
		acct, err := l.GetAccount(o.Account)
		if err != nil {
			return fmt.Errorf("get account %q: %w", o.Account, err)
		}
		if acct == nil && opts.AutoCreateAccounts {
			commodity := opts.DefaultCommodity
			if len(o.Commodities) > 0 {
				commodity = o.Commodities[0]
			}
			acct, err = l.CreateAccount(o.Account, commodity, -2, 0)
			if err != nil {
				return fmt.Errorf("create account %q: %w", o.Account, err)
			}
		}
		if acct != nil {
			if err := l.SetAccountOpenedAt(acct.ID, openedAt); err != nil {
				return fmt.Errorf("set opened_at for %q: %w", o.Account, err)
			}
			if method, ok := o.Metadata["interest-method"]; ok {
				if err := l.SetInterestMethod(acct.ID, InterestMethod(method)); err != nil {
					return fmt.Errorf("set interest method for %q: %w", o.Account, err)
				}
			}
		}
	}

	// Aliases (after opens, since account_path is FK to accounts.full_path)
	for _, a := range gf.Aliases {
		if err := l.CreateAlias(a.Name, a.Account); err != nil {
			return fmt.Errorf("create alias %q: %w", a.Name, err)
		}
	}

	// Customer account links (customers created before accounts for FK;
	// account linking deferred until after opens so accounts exist)
	for _, c := range gf.Customers {
		if c.Account != "" {
			cid, err := l.customerIDByName(c.Name)
			if err != nil {
				return fmt.Errorf("find customer %q: %w", c.Name, err)
			}
			if err := l.SetCustomerAccount(cid, c.Account); err != nil {
				return fmt.Errorf("set customer account %q: %w", c.Name, err)
			}
		}
	}

	// Data points
	for _, dp := range gf.DataPoints {
		vt, err := dp.DateTime.ToTime()
		if err != nil {
			return fmt.Errorf("parse data point datetime: %w", err)
		}
		var kt *time.Time
		if dp.KnowledgeDateTime != nil {
			t, err := dp.KnowledgeDateTime.ToTime()
			if err == nil {
				kt = &t
			}
		}
		value := InferDataPointType(dp.ParamValue)
		if err := l.SetDataPoint(dp.ParamName, vt, kt, value); err != nil {
			return fmt.Errorf("set data point %q: %w", dp.ParamName, err)
		}
	}

	return nil
}

func (l *SQLLedger) importTransaction(txn Transaction, opts ImportOptions) error {
	if len(txn.Movements) == 0 {
		return nil
	}

	// Parse knowledge datetime if present
	var knowledgeTime *time.Time
	if txn.KnowledgeDateTime != nil {
		kt, err := txn.KnowledgeDateTime.ToTime()
		if err == nil {
			knowledgeTime = &kt
		}
	}

	periodAnchor := txn.DateTime.PeriodAnchor

	// Resolve all accounts first
	type resolvedMovement struct {
		fromID      string
		toID        string
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

	valueTime, err := txn.DateTime.ToTime()
	if err != nil {
		return fmt.Errorf("parse datetime: %w", err)
	}

	var batchID string
	if len(resolved) == 1 {
		rm := resolved[0]
		if rm.pendingID != 0 || knowledgeTime != nil || periodAnchor != "" {
			// Use linked movements to set PendingID, KnowledgeTime, or PeriodAnchor
			bid, err := l.RecordLinkedMovements([]MovementInput{{
				FromAccountID: rm.fromID,
				ToAccountID:   rm.toID,
				Amount:        rm.amount,
				Code:          CodeBookTransfer,
				Description:   rm.description,
				PendingID:     rm.pendingID,
				KnowledgeTime: knowledgeTime,
				PeriodAnchor:  periodAnchor,
			}}, valueTime)
			if err != nil {
				return err
			}
			batchID = bid
		} else {
			m, err := l.RecordMovement(rm.fromID, rm.toID, rm.amount, CodeBookTransfer, valueTime, rm.description)
			if err != nil {
				return err
			}
			batchID = m.BatchID
		}
	} else {
		// Multiple movements -> linked
		var inputs []MovementInput
		for _, rm := range resolved {
			inputs = append(inputs, MovementInput{
				FromAccountID: rm.fromID,
				ToAccountID:   rm.toID,
				Amount:        rm.amount,
				Code:          CodeBookTransfer,
				Description:   rm.description,
				PendingID:     rm.pendingID,
				KnowledgeTime: knowledgeTime,
				PeriodAnchor:  periodAnchor,
			})
		}
		bid, err := l.RecordLinkedMovements(inputs, valueTime)
		if err != nil {
			return err
		}
		batchID = bid
	}

	// Store transaction metadata
	if batchID != "" && len(txn.Metadata) > 0 {
		for key, value := range txn.Metadata {
			if err := l.SetMovementMetadata(batchID, key, value); err != nil {
				return fmt.Errorf("set movement metadata: %w", err)
			}
		}
	}
	return nil
}

func (l *SQLLedger) resolveAccount(path string, m TextMovement, opts ImportOptions) (*Account, error) {
	acct, err := l.GetAccount(path)
	if err != nil {
		return nil, err
	}
	if acct != nil {
		return acct, nil
	}

	// Check aliases
	resolved, err := l.ResolveAlias(path)
	if err != nil {
		return nil, err
	}
	if resolved != "" {
		acct, err = l.GetAccount(resolved)
		if err != nil {
			return nil, err
		}
		if acct != nil {
			return acct, nil
		}
		// Alias resolved but account doesn't exist yet — use resolved path for auto-create
		path = resolved
	}

	if !opts.AutoCreateAccounts {
		return nil, fmt.Errorf("account %q not found", path)
	}
	commodity := m.Commodity
	if commodity == "" {
		commodity = opts.DefaultCommodity
	}
	// Use existing commodity exponent if available; otherwise infer from amount
	exp, err := l.commodityExponent(commodity)
	if err != nil {
		exp = inferExponent(m.Amount)
	}
	return l.CreateAccount(path, commodity, exp, 0)
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
