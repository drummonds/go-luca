package luca

// TransactionDiff describes differences between two transactions.
type TransactionDiff struct {
	DateTimeChanged  bool
	PayeeChanged     bool
	MovementsAdded   []TextMovement
	MovementsRemoved []TextMovement
	MovementsChanged []MovementChange
	MetadataAdded    map[string]string
	MetadataRemoved  map[string]string
	MetadataChanged  map[string][2]string // key -> [old, new]
}

// MovementChange describes a before/after pair for a changed movement.
type MovementChange struct {
	Before TextMovement
	After  TextMovement
}

// DiffTransactions compares two Transaction structs (in-memory goluca representation).
// Movements are matched by (From, To) pair. Unmatched in a are Removed, unmatched in b
// are Added. Matched pairs with different Amount/Description/Commodity are Changed.
func DiffTransactions(a, b Transaction) TransactionDiff {
	var diff TransactionDiff

	diff.DateTimeChanged = a.DateTime.String() != b.DateTime.String()
	diff.PayeeChanged = a.Payee != b.Payee

	// Match movements by (From, To) pair.
	// Build index of b movements keyed by (From, To).
	type ftKey struct{ From, To string }
	bByKey := make(map[ftKey][]int) // key -> indices in b.Movements
	for i, m := range b.Movements {
		k := ftKey{m.From, m.To}
		bByKey[k] = append(bByKey[k], i)
	}

	bMatched := make(map[int]bool)
	for _, am := range a.Movements {
		k := ftKey{am.From, am.To}
		indices := bByKey[k]
		matched := false
		for _, idx := range indices {
			if bMatched[idx] {
				continue
			}
			bm := b.Movements[idx]
			bMatched[idx] = true
			matched = true
			if am.Amount != bm.Amount || am.Description != bm.Description || am.Commodity != bm.Commodity {
				diff.MovementsChanged = append(diff.MovementsChanged, MovementChange{
					Before: am,
					After:  bm,
				})
			}
			break
		}
		if !matched {
			diff.MovementsRemoved = append(diff.MovementsRemoved, am)
		}
	}
	for i, bm := range b.Movements {
		if !bMatched[i] {
			diff.MovementsAdded = append(diff.MovementsAdded, bm)
		}
	}

	// Metadata diff
	diff.MetadataAdded = make(map[string]string)
	diff.MetadataRemoved = make(map[string]string)
	diff.MetadataChanged = make(map[string][2]string)

	for k, va := range a.Metadata {
		vb, ok := b.Metadata[k]
		if !ok {
			diff.MetadataRemoved[k] = va
		} else if va != vb {
			diff.MetadataChanged[k] = [2]string{va, vb}
		}
	}
	for k, vb := range b.Metadata {
		if _, ok := a.Metadata[k]; !ok {
			diff.MetadataAdded[k] = vb
		}
	}

	return diff
}
