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
		        m.value_time, m.knowledge_time, m.description,
		        fa.full_path, ta.full_path
		 FROM movements m
		 JOIN accounts fa ON fa.id = m.from_account_id
		 JOIN accounts ta ON ta.id = m.to_account_id
		 ORDER BY m.value_time, m.id`)
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
			&valueTimeStr, &knowledgeTimeStr, &mwp.Description,
			&mwp.FromPath, &mwp.ToPath,
		)
		if err != nil {
			return nil, fmt.Errorf("scan movement: %w", err)
		}
		mwp.ValueTime, _ = time.Parse("2006-01-02 15:04:05", valueTimeStr)
		mwp.KnowledgeTime, _ = time.Parse("2006-01-02 15:04:05", knowledgeTimeStr)
		result = append(result, mwp)
	}
	return result, rows.Err()
}

// Export writes the ledger contents as .goluca formatted text.
func (l *SQLLedger) Export(w io.Writer) error {
	movements, err := l.ListMovements()
	if err != nil {
		return err
	}

	// Build account exponent lookup
	accounts, err := l.ListAccounts("")
	if err != nil {
		return fmt.Errorf("list accounts: %w", err)
	}
	exponentByID := make(map[int64]int)
	currencyByID := make(map[int64]string)
	for _, a := range accounts {
		exponentByID[a.ID] = a.Exponent
		currencyByID[a.ID] = a.Currency
	}

	// Group movements by batch_id
	type batch struct {
		movements []MovementWithPaths
	}
	var batches []batch
	batchIdx := make(map[int64]int)
	for _, m := range movements {
		idx, ok := batchIdx[m.BatchID]
		if !ok {
			idx = len(batches)
			batchIdx[m.BatchID] = idx
			batches = append(batches, batch{})
		}
		batches[idx].movements = append(batches[idx].movements, m)
	}

	// Convert to GolucaFile
	var gf GolucaFile
	for _, b := range batches {
		if len(b.movements) == 0 {
			continue
		}
		first := b.movements[0]
		txn := Transaction{
			Date: first.ValueTime,
			Flag: '*',
		}
		if first.PendingID != 0 {
			txn.Flag = '!'
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

		gf.Transactions = append(gf.Transactions, txn)
	}

	_, err = gf.WriteTo(w)
	return err
}
