package luca

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// DataPointValue wraps a typed data point value.
type DataPointValue struct {
	Type string // "string", "number", "boolean", "null"
	Raw  string
}

// AsString returns the raw string value.
func (v DataPointValue) AsString() string { return v.Raw }

// AsFloat64 parses the value as float64.
func (v DataPointValue) AsFloat64() (float64, error) {
	return strconv.ParseFloat(v.Raw, 64)
}

// AsBool parses the value as boolean.
func (v DataPointValue) AsBool() (bool, error) {
	return strconv.ParseBool(v.Raw)
}

// AsDecimal parses the value as a shopspring Decimal.
func (v DataPointValue) AsDecimal() (decimal.Decimal, error) {
	return decimal.NewFromString(v.Raw)
}

// InferDataPointType infers the type from a raw string value.
func InferDataPointType(raw string) DataPointValue {
	if raw == "" || strings.EqualFold(raw, "null") {
		return DataPointValue{Type: "null", Raw: raw}
	}
	if strings.EqualFold(raw, "true") || strings.EqualFold(raw, "false") {
		return DataPointValue{Type: "boolean", Raw: raw}
	}
	if _, err := strconv.ParseFloat(raw, 64); err == nil {
		return DataPointValue{Type: "number", Raw: raw}
	}
	return DataPointValue{Type: "string", Raw: raw}
}

// DBDataPoint is a database-oriented data point with time.Time fields.
type DBDataPoint struct {
	ValueTime     time.Time
	KnowledgeTime time.Time
	ParamName     string
	Value         DataPointValue
}

// SetDataPoint inserts a data point. If knowledgeTime is nil, defaults to NOW().
func (l *SQLLedger) SetDataPoint(paramName string, valueTime time.Time, knowledgeTime *time.Time, value DataPointValue) error {
	id := uuid.New().String()
	if knowledgeTime != nil {
		_, err := l.db.Exec(
			`INSERT INTO data_points (id, value_time, knowledge_time, param_name, param_type, param_value) VALUES ($1, $2, $3, $4, $5, $6)`,
			id, utc(valueTime), utc(*knowledgeTime), paramName, value.Type, value.Raw,
		)
		if err != nil {
			return fmt.Errorf("insert data point: %w", err)
		}
	} else {
		_, err := l.db.Exec(
			`INSERT INTO data_points (id, value_time, param_name, param_type, param_value) VALUES ($1, $2, $3, $4, $5)`,
			id, utc(valueTime), paramName, value.Type, value.Raw,
		)
		if err != nil {
			return fmt.Errorf("insert data point: %w", err)
		}
	}
	return nil
}

// GetDataPoint returns the latest data point for a param at or before the given time.
func (l *SQLLedger) GetDataPoint(paramName string, at time.Time) (*DataPointValue, error) {
	var paramType, paramValue string
	err := l.db.QueryRow(
		`SELECT param_type, param_value FROM data_points
		 WHERE param_name = $1 AND value_time <= $2
		 ORDER BY value_time DESC, knowledge_time DESC
		 LIMIT 1`,
		paramName, utc(at),
	).Scan(&paramType, &paramValue)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get data point: %w", err)
	}
	return &DataPointValue{Type: paramType, Raw: paramValue}, nil
}

// GetDataPointAsOf returns the data point known at knowledgeTime for a value time.
func (l *SQLLedger) GetDataPointAsOf(paramName string, valueTime, knowledgeTime time.Time) (*DataPointValue, error) {
	var paramType, paramValue string
	err := l.db.QueryRow(
		`SELECT param_type, param_value FROM data_points
		 WHERE param_name = $1 AND value_time <= $2 AND knowledge_time <= $3
		 ORDER BY value_time DESC, knowledge_time DESC
		 LIMIT 1`,
		paramName, utc(valueTime), utc(knowledgeTime),
	).Scan(&paramType, &paramValue)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get data point as of: %w", err)
	}
	return &DataPointValue{Type: paramType, Raw: paramValue}, nil
}

// DataPointRange returns all data points for a param within a time range.
func (l *SQLLedger) DataPointRange(paramName string, from, to time.Time) ([]DBDataPoint, error) {
	rows, err := l.db.Query(
		`SELECT value_time, knowledge_time, param_name, param_type, param_value
		 FROM data_points
		 WHERE param_name = $1 AND value_time >= $2 AND value_time <= $3
		 ORDER BY value_time, knowledge_time`,
		paramName, utc(from), utc(to),
	)
	if err != nil {
		return nil, fmt.Errorf("data point range: %w", err)
	}
	defer rows.Close()

	var result []DBDataPoint
	for rows.Next() {
		var dp DBDataPoint
		var vtStr, ktStr, paramType, paramValue string
		if err := rows.Scan(&vtStr, &ktStr, &dp.ParamName, &paramType, &paramValue); err != nil {
			return nil, fmt.Errorf("scan data point: %w", err)
		}
		dp.ValueTime = parseDBTime(vtStr)
		dp.KnowledgeTime = parseDBTime(ktStr)
		dp.Value = DataPointValue{Type: paramType, Raw: paramValue}
		result = append(result, dp)
	}
	return result, rows.Err()
}

// FirstDataPointTime returns the earliest value_time for a param.
func (l *SQLLedger) FirstDataPointTime(paramName string) (time.Time, error) {
	var vtStr string
	err := l.db.QueryRow(
		`SELECT value_time FROM data_points WHERE param_name = $1 ORDER BY value_time ASC LIMIT 1`,
		paramName,
	).Scan(&vtStr)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("first data point time: %w", err)
	}
	t := parseDBTime(vtStr)
	return t, nil
}

// LastDataPointTime returns the latest value_time for a param.
func (l *SQLLedger) LastDataPointTime(paramName string) (time.Time, error) {
	var vtStr string
	err := l.db.QueryRow(
		`SELECT value_time FROM data_points WHERE param_name = $1 ORDER BY value_time DESC LIMIT 1`,
		paramName,
	).Scan(&vtStr)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("last data point time: %w", err)
	}
	t := parseDBTime(vtStr)
	return t, nil
}

// ListDataPoints returns all data points ordered by param_name and value_time.
func (l *SQLLedger) ListDataPoints() ([]DBDataPoint, error) {
	rows, err := l.db.Query(
		`SELECT value_time, knowledge_time, param_name, param_type, param_value
		 FROM data_points
		 ORDER BY param_name, value_time, knowledge_time`)
	if err != nil {
		return nil, fmt.Errorf("list data points: %w", err)
	}
	defer rows.Close()

	var result []DBDataPoint
	for rows.Next() {
		var dp DBDataPoint
		var vtStr, ktStr, paramType, paramValue string
		if err := rows.Scan(&vtStr, &ktStr, &dp.ParamName, &paramType, &paramValue); err != nil {
			return nil, fmt.Errorf("scan data point: %w", err)
		}
		dp.ValueTime = parseDBTime(vtStr)
		dp.KnowledgeTime = parseDBTime(ktStr)
		dp.Value = DataPointValue{Type: paramType, Raw: paramValue}
		result = append(result, dp)
	}
	return result, rows.Err()
}
