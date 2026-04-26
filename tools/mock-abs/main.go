package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Account represents a bank account stored in memory.
type Account struct {
	AccountNumber string `json:"account_number"`
	BIK           string `json:"bik"`
	BankName      string `json:"bank_name"`
	OpenedAt      string `json:"opened_at"`
	ABSReference  string `json:"abs_reference"`
	Tenant        string `json:"tenant"`
	ClientID      string `json:"client_id"`
	AccountType   string `json:"account_type"`
}

// Client represents a client stored in memory.
type Client struct {
	ClientID  string `json:"client_id"`
	CreatedAt string `json:"created_at"`
	Tenant    string `json:"tenant"`
	INN       string `json:"inn"`
	FullName  string `json:"full_name"`
}

var (
	mu              sync.Mutex
	accounts        = make(map[string]Account) // key: account_number
	idempotencyKeys = make(map[string]string)  // key: idempotency_key -> account_number
	clients         = make(map[string]Client)  // key: client_id
	clientIdempKeys = make(map[string]string)  // key: idempotency_key -> client_id
)

func randomDigits(n int) string {
	digits := make([]byte, n)
	for i := range digits {
		num, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			panic(err)
		}
		digits[i] = byte('0' + num.Int64())
	}
	return string(digits)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)[:n]
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)
		log.Printf("[%s] %s %s -> %d (%s)",
			start.Format("2006-01-02T15:04:05Z07:00"),
			r.Method,
			r.URL.Path,
			ww.Status(),
			time.Since(start),
		)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// POST /v1/accounts/open
func openAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IdempotencyKey string `json:"idempotency_key"`
		Tenant         string `json:"tenant"`
		ClientID       string `json:"client_id"`
		AccountType    string `json:"account_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.IdempotencyKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "idempotency_key is required"})
		return
	}

	mu.Lock()
	defer mu.Unlock()

	// Idempotency check
	if accNum, ok := idempotencyKeys[req.IdempotencyKey]; ok {
		writeJSON(w, http.StatusOK, accounts[accNum])
		return
	}

	accountNumber := "40702810" + randomDigits(12)
	acc := Account{
		AccountNumber: accountNumber,
		BIK:           "044525225",
		BankName:      "Банк Тестовый",
		OpenedAt:      time.Now().UTC().Format(time.RFC3339),
		ABSReference:  "mock_" + req.IdempotencyKey,
		Tenant:        req.Tenant,
		ClientID:      req.ClientID,
		AccountType:   req.AccountType,
	}
	accounts[accountNumber] = acc
	idempotencyKeys[req.IdempotencyKey] = accountNumber

	writeJSON(w, http.StatusCreated, acc)
}

// GET /v1/accounts/{account_number}
func getAccount(w http.ResponseWriter, r *http.Request) {
	accountNumber := chi.URLParam(r, "account_number")

	mu.Lock()
	acc, ok := accounts[accountNumber]
	mu.Unlock()

	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "account not found"})
		return
	}
	writeJSON(w, http.StatusOK, acc)
}

// POST /v1/clients/create
func createClient(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IdempotencyKey string `json:"idempotency_key"`
		Tenant         string `json:"tenant"`
		INN            string `json:"inn"`
		FullName       string `json:"full_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.IdempotencyKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "idempotency_key is required"})
		return
	}

	mu.Lock()
	defer mu.Unlock()

	// Idempotency check
	if clientID, ok := clientIdempKeys[req.IdempotencyKey]; ok {
		writeJSON(w, http.StatusOK, clients[clientID])
		return
	}

	clientID := "mock_cli_" + randomHex(8)
	cli := Client{
		ClientID:  clientID,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Tenant:    req.Tenant,
		INN:       req.INN,
		FullName:  req.FullName,
	}
	clients[clientID] = cli
	clientIdempKeys[req.IdempotencyKey] = clientID

	writeJSON(w, http.StatusCreated, cli)
}

// GET /healthz
func healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	port := getEnv("PORT", "9000")

	r := chi.NewRouter()
	r.Use(loggingMiddleware)

	r.Post("/v1/accounts/open", openAccount)
	r.Get("/v1/accounts/{account_number}", getAccount)
	r.Post("/v1/clients/create", createClient)
	r.Get("/healthz", healthz)

	addr := fmt.Sprintf(":%s", port)
	log.Printf("mock-abs listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
