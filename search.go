package luca

import (
	"fmt"
	"strings"
	"time"
)

// SearchQuery describes filters for searching movements.
type SearchQuery struct {
	AccountID   string     // filter by specific account (from or to)
	PathPrefix  string     // filter by account path prefix (either side)
	FromTime    *time.Time // value_time >= FromTime
	ToTime      *time.Time // value_time <= ToTime
	Description string     // LIKE %description%
	Code        *int16     // exact code match
	MinAmount   *Amount    // amount >= MinAmount
	MaxAmount   *Amount    // amount <= MaxAmount
	BatchID     string     // exact batch match
	Limit       int        // max results (0 = no limit)
	Offset      int        // for pagination
}

// searchQueryBuilder builds the shared FROM/WHERE clause and parameters for search queries.
// Returns (fromClause, whereClause, args).
func searchQueryBuilder(q SearchQuery) (string, string, []any) {
	var conditions []string
	var args []any
	paramIdx := 0
	needPathJoin := q.PathPrefix != ""

	nextParam := func() string {
		paramIdx++
		return fmt.Sprintf("$%d", paramIdx)
	}

	if q.AccountID != "" {
		p1 := nextParam()
		p2 := nextParam()
		conditions = append(conditions, fmt.Sprintf("(m.from_account_id = %s OR m.to_account_id = %s)", p1, p2))
		args = append(args, q.AccountID, q.AccountID)
	}

	if q.PathPrefix != "" {
		pattern := q.PathPrefix + "%"
		p1 := nextParam()
		p2 := nextParam()
		conditions = append(conditions, fmt.Sprintf("(fa.full_path LIKE %s OR ta.full_path LIKE %s)", p1, p2))
		args = append(args, pattern, pattern)
	}

	if q.FromTime != nil {
		p := nextParam()
		conditions = append(conditions, fmt.Sprintf("m.value_time >= %s", p))
		args = append(args, *q.FromTime)
	}

	if q.ToTime != nil {
		p := nextParam()
		conditions = append(conditions, fmt.Sprintf("m.value_time <= %s", p))
		args = append(args, *q.ToTime)
	}

	if q.Description != "" {
		p := nextParam()
		conditions = append(conditions, fmt.Sprintf("m.description LIKE '%%' || %s || '%%'", p))
		args = append(args, q.Description)
	}

	if q.Code != nil {
		p := nextParam()
		conditions = append(conditions, fmt.Sprintf("m.code = %s", p))
		args = append(args, *q.Code)
	}

	if q.MinAmount != nil {
		p := nextParam()
		conditions = append(conditions, fmt.Sprintf("m.amount >= %s", p))
		args = append(args, *q.MinAmount)
	}

	if q.MaxAmount != nil {
		p := nextParam()
		conditions = append(conditions, fmt.Sprintf("m.amount <= %s", p))
		args = append(args, *q.MaxAmount)
	}

	if q.BatchID != "" {
		p := nextParam()
		conditions = append(conditions, fmt.Sprintf("m.batch_id = %s", p))
		args = append(args, q.BatchID)
	}

	fromClause := "FROM movements m"
	if needPathJoin {
		fromClause += "\n		 JOIN accounts fa ON fa.id = m.from_account_id\n		 JOIN accounts ta ON ta.id = m.to_account_id"
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	return fromClause, whereClause, args
}

// SearchMovements returns movements matching the query, ordered by value_time desc.
func (l *SQLLedger) SearchMovements(q SearchQuery) ([]MovementWithPaths, error) {
	fromClause, whereClause, args := searchQueryBuilder(q)

	// Always need the path joins for the SELECT columns
	if !strings.Contains(fromClause, "JOIN accounts fa") {
		fromClause += "\n		 JOIN accounts fa ON fa.id = m.from_account_id\n		 JOIN accounts ta ON ta.id = m.to_account_id"
	}

	query := fmt.Sprintf(
		`SELECT m.id, m.batch_id, m.from_account_id, m.to_account_id, m.amount,
		        m.code, m.ledger, m.pending_id, m.user_data_64,
		        m.value_time, m.knowledge_time, m.description, m.period_anchor,
		        fa.full_path, ta.full_path
		 %s%s
		 ORDER BY m.value_time DESC, m.id`,
		fromClause, whereClause,
	)

	if q.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", q.Limit)
	}
	if q.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", q.Offset)
	}

	rows, err := l.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("search movements: %w", err)
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
		mwp.ValueTime = parseDBTime(valueTimeStr)
		mwp.KnowledgeTime = parseDBTime(knowledgeTimeStr)
		result = append(result, mwp)
	}
	return result, rows.Err()
}

// CountMovements returns the count matching the query (for pagination).
func (l *SQLLedger) CountMovements(q SearchQuery) (int, error) {
	fromClause, whereClause, args := searchQueryBuilder(q)

	query := fmt.Sprintf(`SELECT COUNT(*) %s%s`, fromClause, whereClause)

	var count int
	err := l.db.QueryRow(query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count movements: %w", err)
	}
	return count, nil
}
