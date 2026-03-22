package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/drummonds/go-luca"
)

// Client is an HTTP client that implements the Ledger interface.
type Client struct {
	baseURL string
	http    *http.Client
}

// Compile-time interface check.
var _ luca.Ledger = (*Client)(nil)

// NewClient creates a new API client pointing at the given base URL.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) Close() error { return nil }

// --- account methods ---

func (c *Client) CreateAccount(fullPath string, commodity string, exponent int, grossInterestRate float64) (*luca.Account, error) {
	var acct luca.Account
	err := c.post("/accounts/create", createAccountReq{
		FullPath:          fullPath,
		Commodity:         commodity,
		Exponent:          exponent,
		GrossInterestRate: grossInterestRate,
	}, &acct)
	if err != nil {
		return nil, err
	}
	return &acct, nil
}

func (c *Client) GetAccount(fullPath string) (*luca.Account, error) {
	var acct luca.Account
	err := c.get("/accounts/get", url.Values{"path": {fullPath}}, &acct)
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &acct, nil
}

func (c *Client) GetAccountByID(id string) (*luca.Account, error) {
	var acct luca.Account
	err := c.get("/accounts/get-by-id", url.Values{"id": {id}}, &acct)
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &acct, nil
}

func (c *Client) ListAccounts(typeFilter luca.AccountType) ([]*luca.Account, error) {
	var accounts []*luca.Account
	params := url.Values{}
	if typeFilter != "" {
		params.Set("type", string(typeFilter))
	}
	err := c.get("/accounts/list", params, &accounts)
	if err != nil {
		return nil, err
	}
	return accounts, nil
}

// --- movement methods ---

func (c *Client) RecordMovement(fromAccountID, toAccountID string, amount luca.Amount, code string, valueTime time.Time, description string) (*luca.Movement, error) {
	var mov luca.Movement
	err := c.post("/movements/record", recordMovementReq{
		FromAccountID: fromAccountID,
		ToAccountID:   toAccountID,
		Amount:        amount,
		Code:          code,
		ValueTime:     valueTime.Format(time.RFC3339),
		Description:   description,
	}, &mov)
	if err != nil {
		return nil, err
	}
	return &mov, nil
}

func (c *Client) RecordLinkedMovements(movements []luca.MovementInput, valueTime time.Time) (string, error) {
	var resp batchIDResp
	err := c.post("/movements/record-linked", recordLinkedReq{
		Movements: movements,
		ValueTime: valueTime.Format(time.RFC3339),
	}, &resp)
	if err != nil {
		return "", err
	}
	return resp.BatchID, nil
}

// --- balance methods ---

func (c *Client) Balance(accountID string) (luca.Amount, error) {
	var resp balanceResp
	err := c.get("/balances/balance", url.Values{"account_id": {accountID}}, &resp)
	if err != nil {
		return 0, err
	}
	return resp.Balance, nil
}

func (c *Client) BalanceAt(accountID string, at time.Time) (luca.Amount, error) {
	var resp balanceResp
	err := c.get("/balances/balance-at", url.Values{
		"account_id": {accountID},
		"at":         {at.Format(time.RFC3339)},
	}, &resp)
	if err != nil {
		return 0, err
	}
	return resp.Balance, nil
}

func (c *Client) DailyBalances(accountID string, from, to time.Time) ([]luca.DailyBalance, error) {
	var dailies []luca.DailyBalance
	err := c.get("/balances/daily", url.Values{
		"account_id": {accountID},
		"from":       {from.Format(time.RFC3339)},
		"to":         {to.Format(time.RFC3339)},
	}, &dailies)
	if err != nil {
		return nil, err
	}
	return dailies, nil
}

// --- new endpoint stubs (not on Ledger interface) ---

// AddMovementToBatch appends a movement to an existing batch via the API.
func (c *Client) AddMovementToBatch(batchID string, input luca.MovementInput) (*luca.Movement, error) {
	return nil, luca.ErrNotImplemented
}

// SearchMovements searches for movements matching the given query.
func (c *Client) SearchMovements(q luca.SearchQuery) ([]luca.MovementWithPaths, error) {
	return nil, luca.ErrNotImplemented
}

// MovementEvents returns movement events for a time range.
func (c *Client) MovementEvents(from, to time.Time) ([]MovementEvent, error) {
	return nil, luca.ErrNotImplemented
}

// BalanceAsOf returns the bitemporal balance.
func (c *Client) BalanceAsOf(accountID string, valueTime, knowledgeTime time.Time) (luca.Amount, error) {
	return 0, luca.ErrNotImplemented
}

// FirstMovementTime returns the earliest movement time.
func (c *Client) FirstMovementTime(accountID string) (time.Time, error) {
	return time.Time{}, luca.ErrNotImplemented
}

// LastMovementTime returns the latest movement time.
func (c *Client) LastMovementTime(accountID string) (time.Time, error) {
	return time.Time{}, luca.ErrNotImplemented
}

// ListOptions returns all options.
func (c *Client) ListOptions() ([]luca.Option, error) {
	return nil, luca.ErrNotImplemented
}

// UpsertOption upserts an option.
func (c *Client) UpsertOption(key, value string) error {
	return luca.ErrNotImplemented
}

// ListCommodities returns all commodities.
func (c *Client) ListCommodities() ([]luca.CommodityDef, error) {
	return nil, luca.ErrNotImplemented
}

// ListAliases returns all aliases.
func (c *Client) ListAliases() ([]luca.AliasDef, error) {
	return nil, luca.ErrNotImplemented
}

// ListCustomers returns all customers.
func (c *Client) ListCustomers() ([]luca.CustomerDef, error) {
	return nil, luca.ErrNotImplemented
}

// ListDataPoints returns all data points.
func (c *Client) ListDataPoints(paramName string) ([]luca.DBDataPoint, error) {
	return nil, luca.ErrNotImplemented
}

// SetDataPoint sets a data point value.
func (c *Client) SetDataPoint(paramName string, valueTime time.Time, value string) error {
	return luca.ErrNotImplemented
}

// GetDataPointAt returns the data point value at a given time.
func (c *Client) GetDataPointAt(paramName string, at time.Time) (*luca.DataPointValue, error) {
	return nil, luca.ErrNotImplemented
}

// ImportText imports goluca text via the API.
func (c *Client) ImportText(text string) error {
	return luca.ErrNotImplemented
}

// ExportText exports goluca text via the API.
func (c *Client) ExportText() (string, error) {
	return "", luca.ErrNotImplemented
}

// --- stubbed Ledger methods (not exposed via API yet) ---

func (c *Client) RecordMovementWithProjections(fromAccountID, toAccountID string, amount luca.Amount, code string, valueTime time.Time, description string) (*luca.Movement, error) {
	return nil, luca.ErrNotImplemented
}

func (c *Client) BalanceByPath(pathPrefix string, at time.Time) (luca.Amount, int, error) {
	return 0, 0, luca.ErrNotImplemented
}

func (c *Client) GetLiveBalance(accountID string, date time.Time) (*luca.LiveBalance, error) {
	return nil, luca.ErrNotImplemented
}

func (c *Client) SetInterestMethod(accountID string, method luca.InterestMethod) error {
	var resp map[string]string
	return c.post("/accounts/set-interest-method", setInterestMethodReq{
		AccountID:      accountID,
		InterestMethod: method,
	}, &resp)
}

func (c *Client) EnsureInterestAccounts() error {
	return luca.ErrNotImplemented
}

func (c *Client) CalculateDailyInterest(accountID string, date time.Time) (*luca.InterestResult, error) {
	return nil, luca.ErrNotImplemented
}

func (c *Client) RunDailyInterest(date time.Time) ([]luca.InterestResult, error) {
	return nil, luca.ErrNotImplemented
}

func (c *Client) RunInterestForPeriod(from, to time.Time) ([]luca.InterestResult, error) {
	return nil, luca.ErrNotImplemented
}

func (c *Client) ListMovements() ([]luca.MovementWithPaths, error) {
	return nil, luca.ErrNotImplemented
}

func (c *Client) Export(w io.Writer) error {
	return luca.ErrNotImplemented
}

func (c *Client) Import(r io.Reader, opts *luca.ImportOptions) error {
	return luca.ErrNotImplemented
}

func (c *Client) ImportString(s string, opts *luca.ImportOptions) error {
	return luca.ErrNotImplemented
}

// --- HTTP helpers ---

// apiError is returned when the server responds with a non-2xx status.
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("api: %d %s", e.Status, e.Message)
}

func isNotFound(err error) bool {
	if e, ok := err.(*apiError); ok {
		return e.Status == http.StatusNotFound
	}
	return false
}

func (c *Client) post(path string, body any, result any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	resp, err := c.http.Post(c.baseURL+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("post %s: %w", path, err)
	}
	defer resp.Body.Close()
	return decodeResponse(resp, result)
}

func (c *Client) get(path string, params url.Values, result any) error {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	resp, err := c.http.Get(u)
	if err != nil {
		return fmt.Errorf("get %s: %w", path, err)
	}
	defer resp.Body.Close()
	return decodeResponse(resp, result)
}

func decodeResponse(resp *http.Response, result any) error {
	if resp.StatusCode >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)
		return &apiError{Status: resp.StatusCode, Message: errResp.Error}
	}
	return json.NewDecoder(resp.Body).Decode(result)
}
