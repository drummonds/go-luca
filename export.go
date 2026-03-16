package luca

import (
	"fmt"
	"io"
	"time"
)

// MovementWithPaths extends Movement with resolved account paths for export.
type MovementWithPaths struct {
	Movement
	FromPath string
	ToPath   string
}

// ListMovements returns all movements ordered by value_time then id,
// with account paths resolved via JOIN.
func (l *SQLLedger) ListMovements() ([]MovementWithPaths, error) {
	rows, err := l.db.Query(
		`SELECT m.id, m.batch_id, m.from_account_id, m.to_account_id, m.amount,
		        m.code, m.ledger, m.pending_id, m.user_data_64,
		        m.value_time, m.knowledge_time, m.description, m.period_anchor,
		        fa.full_path, ta.full_path
		 FROM movements m
		 JOIN accounts fa ON fa.id = m.from_account_id
		 JOIN accounts ta ON ta.id = m.to_account_id
		 ORDER BY m.value_time, m.batch_id, fa.full_path, ta.full_path`)
	if err != nil {
		return nil, fmt.Errorf("list movements: %w", err)
	}
	defer rows.Close()

	var result []MovementWithPaths
	for rows.Next() {
		var mwp MovementWithPaths
		var valueTimeStr, knowledgeTimeStr string
		err := rows.Scan(
			&mwp.ID, &mwp.BatchID, &mwp.FromAccountID, &mwp.ToAccountID, &mwp.Amount,
			&mwp.Code, &mwp.Ledger, &mwp.PendingID, &mwp.UserData64,
			&valueTimeStr, &knowledgeTimeStr, &mwp.Description, &mwp.PeriodAnchor,
			&mwp.FromPath, &mwp.ToPath,
		)
		if err != nil {
			return nil, fmt.Errorf("scan movement: %w", err)
		}
		mwp.ValueTime, _ = time.Parse("2006-01-02 15:04:05 -0700 MST", valueTimeStr)
		mwp.KnowledgeTime, _ = time.Parse("2006-01-02 15:04:05 -0700 MST", knowledgeTimeStr)
		result = append(result, mwp)
	}
	return result, rows.Err()
}

// Export writes the ledger contents as .goluca formatted text.
func (l *SQLLedger) Export(w io.Writer) error {
	var gf GolucaFile

	// Export directives
	opts, err := l.ListOptions()
	if err != nil {
		return fmt.Errorf("list options: %w", err)
	}
	gf.Options = opts

	commodities, err := l.ListCommodities()
	if err != nil {
		return fmt.Errorf("list commodities: %w", err)
	}
	gf.Commodities = commodities

	// Export opens from accounts with opened_at set
	accounts, err := l.ListAccounts("")
	if err != nil {
		return fmt.Errorf("list accounts: %w", err)
	}
	exponentByID := make(map[string]int)
	currencyByID := make(map[string]string)
	for _, a := range accounts {
		exponentByID[a.ID] = a.Exponent
		currencyByID[a.ID] = a.Currency
		if a.OpenedAt != nil {
			od := OpenDef{
				DateTime:    DateTimeFromTime(*a.OpenedAt),
				Account:     a.FullPath,
				Commodities: []string{a.Currency},
			}
			gf.Opens = append(gf.Opens, od)
		}
	}

	aliases, err := l.ListAliases()
	if err != nil {
		return fmt.Errorf("list aliases: %w", err)
	}
	gf.Aliases = aliases

	dataPoints, err := l.ListDataPoints()
	if err != nil {
		return fmt.Errorf("list data points: %w", err)
	}
	for _, dp := range dataPoints {
		gfDP := DataPoint{
			DateTime:  DateTimeFromTime(dp.ValueTime),
			ParamName: dp.ParamName,
			ParamValue: dp.Value.Raw,
		}
		if !dp.KnowledgeTime.IsZero() && !dp.KnowledgeTime.Equal(dp.ValueTime) {
			kdt := DateTimeFromTime(dp.KnowledgeTime)
			gfDP.KnowledgeDateTime = &kdt
		}
		gf.DataPoints = append(gf.DataPoints, gfDP)
	}

	customers, err := l.ListCustomers()
	if err != nil {
		return fmt.Errorf("list customers: %w", err)
	}
	gf.Customers = customers

	// Export movements
	movements, err := l.ListMovements()
	if err != nil {
		return err
	}

	// Group movements by batch_id
	type batch struct {
		movements []MovementWithPaths
	}
	var batches []batch
	batchIdx := make(map[string]int)
	for _, m := range movements {
		idx, ok := batchIdx[m.BatchID]
		if !ok {
			idx = len(batches)
			batchIdx[m.BatchID] = idx
			batches = append(batches, batch{})
		}
		batches[idx].movements = append(batches[idx].movements, m)
	}

	// Collect all batch IDs for metadata lookup
	allBatchIDs := make(map[string]bool)
	for bid := range batchIdx {
		allBatchIDs[bid] = true
	}

	for _, b := range batches {
		if len(b.movements) == 0 {
			continue
		}
		first := b.movements[0]
		txn := Transaction{
			DateTime: DateTimeFromTime(first.ValueTime),
			Flag:     '*',
		}
		if first.PendingID != 0 {
			txn.Flag = '!'
		}

		// Emit knowledge datetime when it differs from value_time
		if !first.KnowledgeTime.IsZero() && !first.KnowledgeTime.Equal(first.ValueTime) {
			kdt := DateTimeFromTime(first.KnowledgeTime)
			txn.KnowledgeDateTime = &kdt
		}

		// Preserve period anchor
		if first.PeriodAnchor != "" {
			txn.DateTime.PeriodAnchor = first.PeriodAnchor
		}

		linked := len(b.movements) > 1
		for _, m := range b.movements {
			exp := exponentByID[m.FromAccountID]
			amtDec := IntToDecimal(m.Amount, exp)
			commodity := currencyByID[m.FromAccountID]

			tm := TextMovement{
				Linked:      linked,
				From:        m.FromPath,
				To:          m.ToPath,
				Description: m.Description,
				Amount:      amtDec.String(),
				Commodity:   commodity,
			}
			txn.Movements = append(txn.Movements, tm)
		}

		// Use first movement's description as payee if single movement
		if !linked && len(txn.Movements) == 1 {
			txn.Payee = txn.Movements[0].Description
			txn.Movements[0].Description = ""
		}

		// Attach movement metadata
		meta, err := l.GetMovementMetadata(first.BatchID)
		if err != nil {
			return fmt.Errorf("get movement metadata: %w", err)
		}
		if len(meta) > 0 {
			txn.Metadata = meta
		}

		gf.Transactions = append(gf.Transactions, txn)
	}

	_, err = gf.WriteTo(w)
	return err
}
