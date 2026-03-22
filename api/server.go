package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/drummonds/go-luca"
)

// Server wraps a Ledger and exposes it as HTTP/JSON endpoints.
type Server struct {
	ledger    luca.Ledger
	sqlLedger *luca.SQLLedger
	mux       *http.ServeMux
}

// NewServer creates a new API server backed by the given SQLLedger.
// The SQLLedger satisfies the Ledger interface and also provides
// additional methods (search, directives, import/export, etc.).
func NewServer(l *luca.SQLLedger) *Server {
	s := &Server{ledger: l, sqlLedger: l, mux: http.NewServeMux()}
	s.routes()
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("POST /accounts/create", s.handleCreateAccount)
	s.mux.HandleFunc("GET /accounts/get", s.handleGetAccount)
	s.mux.HandleFunc("GET /accounts/get-by-id", s.handleGetAccountByID)
	s.mux.HandleFunc("GET /accounts/list", s.handleListAccounts)

	s.mux.HandleFunc("POST /accounts/set-interest-method", s.handleSetInterestMethod)

	s.mux.HandleFunc("POST /movements/record", s.handleRecordMovement)
	s.mux.HandleFunc("POST /movements/record-linked", s.handleRecordLinkedMovements)
	s.mux.HandleFunc("POST /movements/add-to-batch", s.handleAddMovementToBatch)
	s.mux.HandleFunc("GET /movements/search", s.handleSearchMovements)
	s.mux.HandleFunc("GET /movements/events", s.handleMovementEvents)

	s.mux.HandleFunc("GET /balances/balance", s.handleBalance)
	s.mux.HandleFunc("GET /balances/balance-at", s.handleBalanceAt)
	s.mux.HandleFunc("GET /balances/balance-as-of", s.handleBalanceAsOf)
	s.mux.HandleFunc("GET /balances/daily", s.handleDailyBalances)
	s.mux.HandleFunc("GET /balances/first-time", s.handleFirstTime)
	s.mux.HandleFunc("GET /balances/last-time", s.handleLastTime)

	s.mux.HandleFunc("GET /options", s.handleListOptions)
	s.mux.HandleFunc("POST /options", s.handleUpsertOption)
	s.mux.HandleFunc("GET /commodities", s.handleListCommodities)
	s.mux.HandleFunc("GET /aliases", s.handleListAliases)
	s.mux.HandleFunc("GET /customers", s.handleListCustomers)
	s.mux.HandleFunc("GET /data-points", s.handleListDataPoints)
	s.mux.HandleFunc("POST /data-points", s.handleSetDataPoint)
	s.mux.HandleFunc("GET /data-points/at", s.handleGetDataPointAt)

	s.mux.HandleFunc("POST /import", s.handleImport)
	s.mux.HandleFunc("GET /export", s.handleExport)
}

// --- request/response types ---

type createAccountReq struct {
	FullPath          string  `json:"full_path"`
	Commodity         string  `json:"commodity"`
	Exponent          int     `json:"exponent"`
	GrossInterestRate float64 `json:"gross_interest_rate"`
}

type setInterestMethodReq struct {
	AccountID      string              `json:"account_id"`
	InterestMethod luca.InterestMethod `json:"interest_method"`
}

type recordMovementReq struct {
	FromAccountID string      `json:"from_account_id"`
	ToAccountID   string      `json:"to_account_id"`
	Amount        luca.Amount `json:"amount"`
	Code          string      `json:"code"`
	ValueTime     string      `json:"value_time"` // RFC3339
	Description   string      `json:"description"`
}

type recordLinkedReq struct {
	Movements []luca.MovementInput `json:"movements"`
	ValueTime string               `json:"value_time"` // RFC3339
}

type addToBatchReq struct {
	BatchID  string             `json:"batch_id"`
	Movement luca.MovementInput `json:"movement"`
}

type batchIDResp struct {
	BatchID string `json:"batch_id"`
}

type balanceResp struct {
	Balance luca.Amount `json:"balance"`
}

type timeResp struct {
	Time time.Time `json:"time"`
}

type upsertOptionReq struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type setDataPointReq struct {
	ParamName string `json:"param_name"`
	ValueTime string `json:"value_time"` // RFC3339
	Value     string `json:"value"`
}

// MovementEvent represents a movement with resolved account paths,
// used for event stream responses.
type MovementEvent struct {
	ID            string      `json:"id"`
	BatchID       string      `json:"batch_id"`
	FromAccountID string      `json:"from_account_id"`
	ToAccountID   string      `json:"to_account_id"`
	FromPath      string      `json:"from_path"`
	ToPath        string      `json:"to_path"`
	Amount        luca.Amount `json:"amount"`
	ValueTime     time.Time   `json:"value_time"`
	KnowledgeTime time.Time   `json:"knowledge_time"`
	Description   string      `json:"description"`
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

// requireSQLLedger checks that the server has a concrete SQLLedger.
// Returns false and writes a 501 error if not available.
func (s *Server) requireSQLLedger(w http.ResponseWriter) bool {
	if s.sqlLedger == nil {
		writeError(w, http.StatusNotImplemented, "endpoint requires SQLLedger backend")
		return false
	}
	return true
}

// --- handlers ---

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var req createAccountReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	acct, err := s.ledger.CreateAccount(req.FullPath, req.Commodity, req.Exponent, req.GrossInterestRate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, acct)
}

func (s *Server) handleSetInterestMethod(w http.ResponseWriter, r *http.Request) {
	var req setInterestMethodReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.AccountID == "" {
		writeError(w, http.StatusBadRequest, "account_id required")
		return
	}
	if err := s.ledger.SetInterestMethod(req.AccountID, req.InterestMethod); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "path required")
		return
	}
	acct, err := s.ledger.GetAccount(path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if acct == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("account %q not found", path))
		return
	}
	writeJSON(w, http.StatusOK, acct)
}

func (s *Server) handleGetAccountByID(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id required")
		return
	}
	acct, err := s.ledger.GetAccountByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if acct == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("account %s not found", id))
		return
	}
	writeJSON(w, http.StatusOK, acct)
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	typeFilter := luca.AccountType(r.URL.Query().Get("type"))
	accounts, err := s.ledger.ListAccounts(typeFilter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, accounts)
}

func (s *Server) handleRecordMovement(w http.ResponseWriter, r *http.Request) {
	var req recordMovementReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	vt, err := parseTime(req.ValueTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid value_time: "+err.Error())
		return
	}
	mov, err := s.ledger.RecordMovement(req.FromAccountID, req.ToAccountID, req.Amount, req.Code, vt, req.Description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, mov)
}

func (s *Server) handleRecordLinkedMovements(w http.ResponseWriter, r *http.Request) {
	var req recordLinkedReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	vt, err := parseTime(req.ValueTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid value_time: "+err.Error())
		return
	}
	batchID, err := s.ledger.RecordLinkedMovements(req.Movements, vt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, batchIDResp{BatchID: batchID})
}

func (s *Server) handleAddMovementToBatch(w http.ResponseWriter, r *http.Request) {
	if !s.requireSQLLedger(w) {
		return
	}
	var req addToBatchReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.BatchID == "" {
		writeError(w, http.StatusBadRequest, "batch_id required")
		return
	}
	mov, err := s.sqlLedger.AddMovementToBatch(req.BatchID, req.Movement)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, mov)
}

func (s *Server) handleSearchMovements(w http.ResponseWriter, r *http.Request) {
	if !s.requireSQLLedger(w) {
		return
	}
	q := luca.SearchQuery{
		AccountID:  r.URL.Query().Get("account_id"),
		PathPrefix: r.URL.Query().Get("path_prefix"),
		BatchID:    r.URL.Query().Get("batch_id"),
	}
	if desc := r.URL.Query().Get("description"); desc != "" {
		q.Description = desc
	}
	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		t, err := parseTime(fromStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid from: "+err.Error())
			return
		}
		q.FromTime = &t
	}
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		t, err := parseTime(toStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid to: "+err.Error())
			return
		}
		q.ToTime = &t
	}
	if code := r.URL.Query().Get("code"); code != "" {
		q.Code = &code
	}
	if minStr := r.URL.Query().Get("min_amount"); minStr != "" {
		v, err := strconv.ParseInt(minStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid min_amount: "+err.Error())
			return
		}
		amt := luca.Amount(v)
		q.MinAmount = &amt
	}
	if maxStr := r.URL.Query().Get("max_amount"); maxStr != "" {
		v, err := strconv.ParseInt(maxStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid max_amount: "+err.Error())
			return
		}
		amt := luca.Amount(v)
		q.MaxAmount = &amt
	}
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		v, err := strconv.Atoi(limitStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit: "+err.Error())
			return
		}
		q.Limit = v
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		v, err := strconv.Atoi(offsetStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid offset: "+err.Error())
			return
		}
		q.Offset = v
	}

	results, err := s.sqlLedger.SearchMovements(q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func (s *Server) handleMovementEvents(w http.ResponseWriter, r *http.Request) {
	if !s.requireSQLLedger(w) {
		return
	}
	fromStr := r.URL.Query().Get("from")
	if fromStr == "" {
		writeError(w, http.StatusBadRequest, "from required")
		return
	}
	from, err := parseTime(fromStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid from: "+err.Error())
		return
	}
	toStr := r.URL.Query().Get("to")
	if toStr == "" {
		writeError(w, http.StatusBadRequest, "to required")
		return
	}
	to, err := parseTime(toStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid to: "+err.Error())
		return
	}

	q := luca.SearchQuery{
		FromTime: &from,
		ToTime:   &to,
	}
	movements, err := s.sqlLedger.SearchMovements(q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	events := make([]MovementEvent, len(movements))
	for i, m := range movements {
		events[i] = MovementEvent{
			ID:            m.ID,
			BatchID:       m.BatchID,
			FromAccountID: m.FromAccountID,
			ToAccountID:   m.ToAccountID,
			FromPath:      m.FromPath,
			ToPath:        m.ToPath,
			Amount:        m.Amount,
			ValueTime:     m.ValueTime,
			KnowledgeTime: m.KnowledgeTime,
			Description:   m.Description,
		}
	}
	writeJSON(w, http.StatusOK, events)
}

func (s *Server) handleBalance(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("account_id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "account_id required")
		return
	}
	bal, err := s.ledger.Balance(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, balanceResp{Balance: bal})
}

func (s *Server) handleBalanceAt(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("account_id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "account_id required")
		return
	}
	at, err := parseTime(r.URL.Query().Get("at"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid at: "+err.Error())
		return
	}
	bal, err := s.ledger.BalanceAt(id, at)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, balanceResp{Balance: bal})
}

func (s *Server) handleBalanceAsOf(w http.ResponseWriter, r *http.Request) {
	if !s.requireSQLLedger(w) {
		return
	}
	id := r.URL.Query().Get("account_id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "account_id required")
		return
	}
	vtStr := r.URL.Query().Get("value_time")
	if vtStr == "" {
		writeError(w, http.StatusBadRequest, "value_time required")
		return
	}
	vt, err := parseTime(vtStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid value_time: "+err.Error())
		return
	}
	ktStr := r.URL.Query().Get("knowledge_time")
	if ktStr == "" {
		writeError(w, http.StatusBadRequest, "knowledge_time required")
		return
	}
	kt, err := parseTime(ktStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid knowledge_time: "+err.Error())
		return
	}
	bal, err := s.sqlLedger.BalanceAsOf(id, vt, kt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, balanceResp{Balance: bal})
}

func (s *Server) handleDailyBalances(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("account_id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "account_id required")
		return
	}
	from, err := parseTime(r.URL.Query().Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid from: "+err.Error())
		return
	}
	to, err := parseTime(r.URL.Query().Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid to: "+err.Error())
		return
	}
	dailies, err := s.ledger.DailyBalances(id, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dailies)
}

func (s *Server) handleFirstTime(w http.ResponseWriter, r *http.Request) {
	if !s.requireSQLLedger(w) {
		return
	}
	accountID := r.URL.Query().Get("account_id")
	t, err := s.sqlLedger.FirstMovementTime(accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, timeResp{Time: t})
}

func (s *Server) handleLastTime(w http.ResponseWriter, r *http.Request) {
	if !s.requireSQLLedger(w) {
		return
	}
	accountID := r.URL.Query().Get("account_id")
	t, err := s.sqlLedger.LastMovementTime(accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, timeResp{Time: t})
}

// --- directive handlers ---

func (s *Server) handleListOptions(w http.ResponseWriter, r *http.Request) {
	if !s.requireSQLLedger(w) {
		return
	}
	opts, err := s.sqlLedger.ListOptions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, opts)
}

func (s *Server) handleUpsertOption(w http.ResponseWriter, r *http.Request) {
	if !s.requireSQLLedger(w) {
		return
	}
	var req upsertOptionReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Key == "" {
		writeError(w, http.StatusBadRequest, "key required")
		return
	}
	if err := s.sqlLedger.UpsertOption(req.Key, req.Value); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListCommodities(w http.ResponseWriter, r *http.Request) {
	if !s.requireSQLLedger(w) {
		return
	}
	commodities, err := s.sqlLedger.ListCommodities()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, commodities)
}

func (s *Server) handleListAliases(w http.ResponseWriter, r *http.Request) {
	if !s.requireSQLLedger(w) {
		return
	}
	aliases, err := s.sqlLedger.ListAliases()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, aliases)
}

func (s *Server) handleListCustomers(w http.ResponseWriter, r *http.Request) {
	if !s.requireSQLLedger(w) {
		return
	}
	customers, err := s.sqlLedger.ListCustomers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, customers)
}

func (s *Server) handleListDataPoints(w http.ResponseWriter, r *http.Request) {
	if !s.requireSQLLedger(w) {
		return
	}
	paramName := r.URL.Query().Get("param_name")
	if paramName != "" {
		// Filter by param_name: use DataPointRange with very wide time bounds
		// or list all and filter. ListDataPoints returns all; filter in Go.
		all, err := s.sqlLedger.ListDataPoints()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		var filtered []luca.DBDataPoint
		for _, dp := range all {
			if dp.ParamName == paramName {
				filtered = append(filtered, dp)
			}
		}
		writeJSON(w, http.StatusOK, filtered)
		return
	}
	dps, err := s.sqlLedger.ListDataPoints()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dps)
}

func (s *Server) handleSetDataPoint(w http.ResponseWriter, r *http.Request) {
	if !s.requireSQLLedger(w) {
		return
	}
	var req setDataPointReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.ParamName == "" {
		writeError(w, http.StatusBadRequest, "param_name required")
		return
	}
	vt, err := parseTime(req.ValueTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid value_time: "+err.Error())
		return
	}
	value := luca.InferDataPointType(req.Value)
	if err := s.sqlLedger.SetDataPoint(req.ParamName, vt, nil, value); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}

func (s *Server) handleGetDataPointAt(w http.ResponseWriter, r *http.Request) {
	if !s.requireSQLLedger(w) {
		return
	}
	paramName := r.URL.Query().Get("param_name")
	if paramName == "" {
		writeError(w, http.StatusBadRequest, "param_name required")
		return
	}
	atStr := r.URL.Query().Get("at")
	if atStr == "" {
		writeError(w, http.StatusBadRequest, "at required")
		return
	}
	at, err := parseTime(atStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid at: "+err.Error())
		return
	}
	dp, err := s.sqlLedger.GetDataPoint(paramName, at)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if dp == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("no data point for %q at %s", paramName, atStr))
		return
	}
	writeJSON(w, http.StatusOK, dp)
}

// --- import/export handlers ---

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	if !s.requireSQLLedger(w) {
		return
	}
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	if err := s.sqlLedger.ImportString(string(body), nil); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	if !s.requireSQLLedger(w) {
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := s.sqlLedger.Export(w); err != nil {
		// Headers already sent; best we can do is log/write error text
		fmt.Fprintf(w, "\n\nERROR: %v\n", err)
	}
}
