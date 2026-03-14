package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/drummonds/go-luca"
)

// Server wraps a Ledger and exposes it as HTTP/JSON endpoints.
type Server struct {
	ledger luca.Ledger
	mux    *http.ServeMux
}

// NewServer creates a new API server backed by the given Ledger.
func NewServer(l luca.Ledger) *Server {
	s := &Server{ledger: l, mux: http.NewServeMux()}
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

	s.mux.HandleFunc("POST /movements/record", s.handleRecordMovement)
	s.mux.HandleFunc("POST /movements/record-linked", s.handleRecordLinkedMovements)

	s.mux.HandleFunc("GET /balances/balance", s.handleBalance)
	s.mux.HandleFunc("GET /balances/balance-at", s.handleBalanceAt)
	s.mux.HandleFunc("GET /balances/daily", s.handleDailyBalances)
}

// --- request/response types ---

type createAccountReq struct {
	FullPath           string  `json:"full_path"`
	Currency           string  `json:"currency"`
	Exponent           int     `json:"exponent"`
	AnnualInterestRate float64 `json:"annual_interest_rate"`
}

type recordMovementReq struct {
	FromAccountID int64       `json:"from_account_id"`
	ToAccountID   int64       `json:"to_account_id"`
	Amount        luca.Amount `json:"amount"`
	ValueTime     string      `json:"value_time"` // RFC3339
	Description   string      `json:"description"`
}

type recordLinkedReq struct {
	Movements []luca.MovementInput `json:"movements"`
	ValueTime string               `json:"value_time"` // RFC3339
}

type batchIDResp struct {
	BatchID int64 `json:"batch_id"`
}

type balanceResp struct {
	Balance luca.Amount `json:"balance"`
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

// --- handlers ---

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var req createAccountReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	acct, err := s.ledger.CreateAccount(req.FullPath, req.Currency, req.Exponent, req.AnnualInterestRate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, acct)
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
	var id int64
	if _, err := fmt.Sscan(r.URL.Query().Get("id"), &id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	acct, err := s.ledger.GetAccountByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if acct == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("account %d not found", id))
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
	mov, err := s.ledger.RecordMovement(req.FromAccountID, req.ToAccountID, req.Amount, vt, req.Description)
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

func (s *Server) handleBalance(w http.ResponseWriter, r *http.Request) {
	var id int64
	if _, err := fmt.Sscan(r.URL.Query().Get("account_id"), &id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid account_id")
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
	var id int64
	if _, err := fmt.Sscan(r.URL.Query().Get("account_id"), &id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid account_id")
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

func (s *Server) handleDailyBalances(w http.ResponseWriter, r *http.Request) {
	var id int64
	if _, err := fmt.Sscan(r.URL.Query().Get("account_id"), &id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid account_id")
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
