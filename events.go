package luca

import (
	"fmt"
	"sort"
	"time"
)

// EventType identifies the kind of event in a merged timeline.
type EventType string

const (
	EventMovement  EventType = "movement"
	EventDataPoint EventType = "data_point"
)

// Event is a single entry in a merged timeline of movements and data points.
type Event struct {
	Type      EventType
	DateTime  time.Time
	Movement  *MovementWithPaths // non-nil for movement events
	DataPoint *DBDataPoint       // non-nil for data point events
}

// Events returns all events in [from, to] ordered by DateTime.
// Merges movements and data points into a single timeline.
func (l *SQLLedger) Events(from, to time.Time) ([]Event, error) {
	// Query movements in range
	rows, err := l.db.Query(
		`SELECT m.id, m.batch_id, m.from_account_id, m.to_account_id, m.amount,
		        m.code, m.ledger, m.pending_id, m.user_data_64,
		        m.value_time, m.knowledge_time, m.description, m.period_anchor,
		        fa.full_path, ta.full_path
		 FROM movements m
		 JOIN accounts fa ON fa.id = m.from_account_id
		 JOIN accounts ta ON ta.id = m.to_account_id
		 WHERE m.value_time >= $1 AND m.value_time <= $2
		 ORDER BY m.value_time, m.id`,
		utc(from), utc(to),
	)
	if err != nil {
		return nil, fmt.Errorf("query movements: %w", err)
	}
	defer rows.Close()

	var events []Event
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
		m := mwp // copy for pointer
		events = append(events, Event{
			Type:     EventMovement,
			DateTime: mwp.ValueTime,
			Movement: &m,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate movements: %w", err)
	}

	// Query data points in range
	dpRows, err := l.db.Query(
		`SELECT value_time, knowledge_time, param_name, param_type, param_value
		 FROM data_points
		 WHERE value_time >= $1 AND value_time <= $2
		 ORDER BY value_time`,
		utc(from), utc(to),
	)
	if err != nil {
		return nil, fmt.Errorf("query data points: %w", err)
	}
	defer dpRows.Close()

	for dpRows.Next() {
		var dp DBDataPoint
		var vtStr, ktStr, paramType, paramValue string
		if err := dpRows.Scan(&vtStr, &ktStr, &dp.ParamName, &paramType, &paramValue); err != nil {
			return nil, fmt.Errorf("scan data point: %w", err)
		}
		dp.ValueTime = parseDBTime(vtStr)
		dp.KnowledgeTime = parseDBTime(ktStr)
		dp.Value = DataPointValue{Type: paramType, Raw: paramValue}
		d := dp // copy for pointer
		events = append(events, Event{
			Type:      EventDataPoint,
			DateTime:  dp.ValueTime,
			DataPoint: &d,
		})
	}
	if err := dpRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate data points: %w", err)
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].DateTime.Before(events[j].DateTime)
	})

	return events, nil
}
