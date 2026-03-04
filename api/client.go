package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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

func (c *Client) CreateAccount(fullPath string, currency string, exponent int, annualInterestRate float64) (*luca.Account, error) {
	var acct luca.Account
	err := c.post("/accounts/create", createAccountReq{
		FullPath:           fullPath,
		Currency:           currency,
		Exponent:           exponent,
		AnnualInterestRate: annualInterestRate,
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

func (c *Client) GetAccountByID(id int64) (*luca.Account, error) {
	var acct luca.Account
	err := c.get("/accounts/get-by-id", url.Values{"id": {strconv.FormatInt(id, 10)}}, &acct)
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

func (c *Client) RecordMovement(fromAccountID, toAccountID int64, amount int64, valueTime time.Time, description string) (*luca.Movement, error) {
	var mov luca.Movement
	err := c.post("/movements/record", recordMovementReq{
		FromAccountID: fromAccountID,
		ToAccountID:   toAccountID,
		Amount:        amount,
		ValueTime:     valueTime.Format(time.RFC3339),
		Description:   description,
	}, &mov)
	if err != nil {
		return nil, err
	}
	return &mov, nil
}

func (c *Client) RecordLinkedMovements(movements []luca.MovementInput, valueTime time.Time) (int64, error) {
	var resp batchIDResp
	err := c.post("/movements/record-linked", recordLinkedReq{
		Movements: movements,
		ValueTime: valueTime.Format(time.RFC3339),
	}, &resp)
	if err != nil {
		return 0, err
	}
	return resp.BatchID, nil
}

// --- balance methods ---

func (c *Client) Balance(accountID int64) (int64, error) {
	var resp balanceResp
	err := c.get("/balances/balance", url.Values{"account_id": {strconv.FormatInt(accountID, 10)}}, &resp)
	if err != nil {
		return 0, err
	}
	return resp.Balance, nil
}

func (c *Client) BalanceAt(accountID int64, at time.Time) (int64, error) {
	var resp balanceResp
	err := c.get("/balances/balance-at", url.Values{
		"account_id": {strconv.FormatInt(accountID, 10)},
		"at":         {at.Format(time.RFC3339)},
	}, &resp)
	if err != nil {
		return 0, err
	}
	return resp.Balance, nil
}

func (c *Client) DailyBalances(accountID int64, from, to time.Time) ([]luca.DailyBalance, error) {
	var dailies []luca.DailyBalance
	err := c.get("/balances/daily", url.Values{
		"account_id": {strconv.FormatInt(accountID, 10)},
		"from":       {from.Format(time.RFC3339)},
		"to":         {to.Format(time.RFC3339)},
	}, &dailies)
	if err != nil {
		return nil, err
	}
	return dailies, nil
}

// --- stubbed methods (not exposed via API yet) ---

func (c *Client) RecordMovementWithProjections(fromAccountID, toAccountID int64, amount int64, valueTime time.Time, description string) (*luca.Movement, error) {
	return nil, luca.ErrNotImplemented
}

func (c *Client) BalanceByPath(pathPrefix string, at time.Time) (int64, int, error) {
	return 0, 0, luca.ErrNotImplemented
}

func (c *Client) GetLiveBalance(accountID int64, date time.Time) (*luca.LiveBalance, error) {
	return nil, luca.ErrNotImplemented
}

func (c *Client) EnsureInterestAccounts() error {
	return luca.ErrNotImplemented
}

func (c *Client) CalculateDailyInterest(accountID int64, date time.Time) (*luca.InterestResult, error) {
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
