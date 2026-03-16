package luca

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// --- Options ---

// UpsertOption sets a key-value option. Uses delete+insert for pglike compatibility.
func (l *SQLLedger) UpsertOption(key, value string) error {
	_, err := l.db.Exec(`DELETE FROM options WHERE key = $1`, key)
	if err != nil {
		return fmt.Errorf("delete option: %w", err)
	}
	_, err = l.db.Exec(
		`INSERT INTO options (id, key, value) VALUES ($1, $2, $3)`,
		uuid.New().String(), key, value,
	)
	if err != nil {
		return fmt.Errorf("insert option: %w", err)
	}
	return nil
}

// GetOption returns the value for a key, or "" if not found.
func (l *SQLLedger) GetOption(key string) (string, error) {
	var value string
	err := l.db.QueryRow(`SELECT value FROM options WHERE key = $1`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get option: %w", err)
	}
	return value, nil
}

// ListOptions returns all options.
func (l *SQLLedger) ListOptions() ([]Option, error) {
	rows, err := l.db.Query(`SELECT key, value FROM options ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("list options: %w", err)
	}
	defer rows.Close()

	var opts []Option
	for rows.Next() {
		var o Option
		if err := rows.Scan(&o.Key, &o.Value); err != nil {
			return nil, fmt.Errorf("scan option: %w", err)
		}
		opts = append(opts, o)
	}
	return opts, rows.Err()
}

// --- Commodities ---

// CreateCommodity inserts a commodity and returns its ID.
func (l *SQLLedger) CreateCommodity(code string, datetime *time.Time) (string, error) {
	id := uuid.New().String()
	_, err := l.db.Exec(
		`INSERT INTO commodities (id, code, datetime) VALUES ($1, $2, $3)`,
		id, code, datetime,
	)
	if err != nil {
		return "", fmt.Errorf("insert commodity: %w", err)
	}
	return id, nil
}

// SetCommodityMetadata sets a metadata key-value on a commodity. Delete+insert for pglike.
func (l *SQLLedger) SetCommodityMetadata(commodityID, key, value string) error {
	_, err := l.db.Exec(
		`DELETE FROM commodity_metadata WHERE commodity_id = $1 AND key = $2`,
		commodityID, key,
	)
	if err != nil {
		return fmt.Errorf("delete commodity metadata: %w", err)
	}
	_, err = l.db.Exec(
		`INSERT INTO commodity_metadata (id, commodity_id, key, value) VALUES ($1, $2, $3, $4)`,
		uuid.New().String(), commodityID, key, value,
	)
	if err != nil {
		return fmt.Errorf("insert commodity metadata: %w", err)
	}
	return nil
}

// ListCommodities returns all commodities with their metadata.
func (l *SQLLedger) ListCommodities() ([]CommodityDef, error) {
	rows, err := l.db.Query(`SELECT id, code, datetime FROM commodities ORDER BY code`)
	if err != nil {
		return nil, fmt.Errorf("list commodities: %w", err)
	}
	defer rows.Close()

	type commodityRow struct {
		id          string
		code        string
		datetimeStr sql.NullString
	}
	var crows []commodityRow
	for rows.Next() {
		var cr commodityRow
		if err := rows.Scan(&cr.id, &cr.code, &cr.datetimeStr); err != nil {
			return nil, fmt.Errorf("scan commodity: %w", err)
		}
		crows = append(crows, cr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Fetch all commodity metadata
	metaRows, err := l.db.Query(`SELECT commodity_id, key, value FROM commodity_metadata ORDER BY commodity_id, key`)
	if err != nil {
		return nil, fmt.Errorf("list commodity metadata: %w", err)
	}
	defer metaRows.Close()

	metaByID := make(map[string]map[string]string)
	for metaRows.Next() {
		var cid, key, value string
		if err := metaRows.Scan(&cid, &key, &value); err != nil {
			return nil, fmt.Errorf("scan commodity metadata: %w", err)
		}
		if metaByID[cid] == nil {
			metaByID[cid] = make(map[string]string)
		}
		metaByID[cid][key] = value
	}
	if err := metaRows.Err(); err != nil {
		return nil, err
	}

	var result []CommodityDef
	for _, cr := range crows {
		cd := CommodityDef{
			Code:     cr.code,
			Metadata: metaByID[cr.id],
		}
		if cr.datetimeStr.Valid && cr.datetimeStr.String != "" {
			t := parseDBTime(cr.datetimeStr.String)
			if !t.IsZero() {
				dt := DateTimeFromTime(t)
				cd.DateTime = &dt
			}
		}
		result = append(result, cd)
	}
	return result, nil
}

// --- Aliases ---

// CreateAlias inserts an alias mapping. Delete+insert for pglike compatibility.
func (l *SQLLedger) CreateAlias(name, accountPath string) error {
	_, err := l.db.Exec(`DELETE FROM aliases WHERE name = $1`, name)
	if err != nil {
		return fmt.Errorf("delete alias: %w", err)
	}
	_, err = l.db.Exec(
		`INSERT INTO aliases (id, name, account_path) VALUES ($1, $2, $3)`,
		uuid.New().String(), name, accountPath,
	)
	if err != nil {
		return fmt.Errorf("insert alias: %w", err)
	}
	return nil
}

// ResolveAlias returns the account_path for a name, or "" if not found.
func (l *SQLLedger) ResolveAlias(name string) (string, error) {
	var path string
	err := l.db.QueryRow(`SELECT account_path FROM aliases WHERE name = $1`, name).Scan(&path)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve alias: %w", err)
	}
	return path, nil
}

// ListAliases returns all aliases.
func (l *SQLLedger) ListAliases() ([]AliasDef, error) {
	rows, err := l.db.Query(`SELECT name, account_path FROM aliases ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list aliases: %w", err)
	}
	defer rows.Close()

	var result []AliasDef
	for rows.Next() {
		var a AliasDef
		if err := rows.Scan(&a.Name, &a.Account); err != nil {
			return nil, fmt.Errorf("scan alias: %w", err)
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// --- Customers ---

// CreateCustomer inserts a customer and returns its ID.
func (l *SQLLedger) CreateCustomer(name string) (string, error) {
	id := uuid.New().String()
	_, err := l.db.Exec(
		`INSERT INTO customers (id, name) VALUES ($1, $2)`,
		id, name,
	)
	if err != nil {
		return "", fmt.Errorf("insert customer: %w", err)
	}
	return id, nil
}

// SetCustomerAccount sets the account_path for a customer.
func (l *SQLLedger) SetCustomerAccount(customerID, accountPath string) error {
	_, err := l.db.Exec(
		`UPDATE customers SET account_path = $1 WHERE id = $2`,
		accountPath, customerID,
	)
	return err
}

// SetCustomerMaxBalance sets the max balance amount and commodity for a customer.
func (l *SQLLedger) SetCustomerMaxBalance(customerID, amount, commodity string) error {
	_, err := l.db.Exec(
		`UPDATE customers SET max_balance_amount = $1, max_balance_commodity = $2 WHERE id = $3`,
		amount, commodity, customerID,
	)
	return err
}

// SetCustomerMetadata sets a metadata key-value on a customer. Delete+insert for pglike.
func (l *SQLLedger) SetCustomerMetadata(customerID, key, value string) error {
	_, err := l.db.Exec(
		`DELETE FROM customer_metadata WHERE customer_id = $1 AND key = $2`,
		customerID, key,
	)
	if err != nil {
		return fmt.Errorf("delete customer metadata: %w", err)
	}
	_, err = l.db.Exec(
		`INSERT INTO customer_metadata (id, customer_id, key, value) VALUES ($1, $2, $3, $4)`,
		uuid.New().String(), customerID, key, value,
	)
	if err != nil {
		return fmt.Errorf("insert customer metadata: %w", err)
	}
	return nil
}

// ListCustomers returns all customers with their metadata.
func (l *SQLLedger) ListCustomers() ([]CustomerDef, error) {
	rows, err := l.db.Query(`SELECT id, name, account_path, max_balance_amount, max_balance_commodity FROM customers ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list customers: %w", err)
	}
	defer rows.Close()

	type customerRow struct {
		id                  string
		name                string
		accountPath         string
		maxBalanceAmount    string
		maxBalanceCommodity string
	}
	var crows []customerRow
	for rows.Next() {
		var cr customerRow
		if err := rows.Scan(&cr.id, &cr.name, &cr.accountPath, &cr.maxBalanceAmount, &cr.maxBalanceCommodity); err != nil {
			return nil, fmt.Errorf("scan customer: %w", err)
		}
		crows = append(crows, cr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Fetch all customer metadata
	metaRows, err := l.db.Query(`SELECT customer_id, key, value FROM customer_metadata ORDER BY customer_id, key`)
	if err != nil {
		return nil, fmt.Errorf("list customer metadata: %w", err)
	}
	defer metaRows.Close()

	metaByID := make(map[string]map[string]string)
	for metaRows.Next() {
		var cid, key, value string
		if err := metaRows.Scan(&cid, &key, &value); err != nil {
			return nil, fmt.Errorf("scan customer metadata: %w", err)
		}
		if metaByID[cid] == nil {
			metaByID[cid] = make(map[string]string)
		}
		metaByID[cid][key] = value
	}
	if err := metaRows.Err(); err != nil {
		return nil, err
	}

	var result []CustomerDef
	for _, cr := range crows {
		cd := CustomerDef{
			Name:                cr.name,
			Account:             cr.accountPath,
			MaxBalanceAmount:    cr.maxBalanceAmount,
			MaxBalanceCommodity: cr.maxBalanceCommodity,
			Metadata:            metaByID[cr.id],
		}
		result = append(result, cd)
	}
	return result, nil
}

// --- Movement Metadata ---

// SetMovementMetadata sets a metadata key-value on a movement batch. Delete+insert for pglike.
func (l *SQLLedger) SetMovementMetadata(batchID, key, value string) error {
	_, err := l.db.Exec(
		`DELETE FROM movement_metadata WHERE batch_id = $1 AND key = $2`,
		batchID, key,
	)
	if err != nil {
		return fmt.Errorf("delete movement metadata: %w", err)
	}
	_, err = l.db.Exec(
		`INSERT INTO movement_metadata (id, batch_id, key, value) VALUES ($1, $2, $3, $4)`,
		uuid.New().String(), batchID, key, value,
	)
	if err != nil {
		return fmt.Errorf("insert movement metadata: %w", err)
	}
	return nil
}

// GetMovementMetadata returns all metadata for a movement batch.
func (l *SQLLedger) GetMovementMetadata(batchID string) (map[string]string, error) {
	rows, err := l.db.Query(
		`SELECT key, value FROM movement_metadata WHERE batch_id = $1 ORDER BY key`,
		batchID,
	)
	if err != nil {
		return nil, fmt.Errorf("get movement metadata: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan movement metadata: %w", err)
		}
		result[key] = value
	}
	return result, rows.Err()
}
