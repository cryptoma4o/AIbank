//go:build e2e

// End-to-end интеграционный тест полной цепочки signing → storage → verification.
//
// ЦЕЛЬ:
//   * Поднять реальный PostgreSQL 16 через testcontainers-go.
//   * Применить миграции 001 (events) и 002 (signature columns) audit-service.
//   * Запустить in-process HTTP-сервер с handler'ом, у которого включён
//     Ed25519 signer (фактический NewEventHandlerWithSigner — тот же конструктор,
//     что в cmd/server/main.go).
//   * Записать N=10 событий через POST /v1/events/.
//   * Запустить tools/audit-verifier как subprocess с --source=db
//     и --pubkey=<base64 ed25519 pubkey>; распарсить JSON и убедиться, что
//     chain.valid=true И signatures.valid=true.
//   * Имитировать tampering — модифицировать payload одного row напрямую в БД
//     (с временным DISABLE TRIGGER, т.к. append-only-trigger из миграции 001
//     блокирует UPDATE) — и убедиться, что verifier завершается с exit code 1
//     и MismatchReason="hash_mismatch".
//
// Запуск:
//
//	go test -tags=e2e -v ./test/e2e/...
//
// Требования: запущенный Docker daemon. Без него — t.Skip с понятным сообщением.
package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	ed25519signer "github.com/aibank/platform/packages/signature/ed25519"
	"github.com/aibank/platform/services/audit-service/internal/handler"
	"github.com/aibank/platform/services/audit-service/internal/repository"
)

const (
	tenantID  = "tnt_e2e"
	numEvents = 10
)

// TestE2E_SignedChain_VerifierAcceptsThenDetectsTampering — полный сценарий:
//   - 10 валидных подписанных событий → verifier OK
//   - подмена одного payload → verifier FAIL с hash_mismatch
//
// Это контрактный тест над тремя независимыми компонентами:
//
//	audit-service handler ──► PostgreSQL audit.events ──► audit-verifier CLI
//
// Если любой из них дрейфует (поле в hash, формат signature, парсинг JSON)
// — тест ловит это до production'а.
func TestE2E_SignedChain_VerifierAcceptsThenDetectsTampering(t *testing.T) {
	skipIfNoDocker(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 1. PostgreSQL container.
	db, dsn, terminate := startPostgres(ctx, t)
	defer terminate()

	// 2. Apply migrations 001 + 002 from disk (real audit-service migrations).
	migrationsDir := absPathFromTest(t, "..", "..", "migrations")
	if err := applyMigrations(db, migrationsDir); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	// 3. Build audit-verifier CLI binary once.
	verifierBin := buildAuditVerifier(t)

	// 4. Spin up in-process HTTP server with signer-enabled handler.
	signer, err := ed25519signer.Generate()
	if err != nil {
		t.Fatalf("generate ed25519 keypair: %v", err)
	}
	pubB64 := signer.PublicKeyBase64()

	repo := repository.NewPostgresAuditRepository(db)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	eventHandler := handler.NewEventHandlerWithSigner(repo, log, signer, ed25519signer.Algorithm)

	r := chi.NewRouter()
	r.Mount("/v1/events", eventHandler.Routes())
	srv := httptest.NewServer(r)
	defer srv.Close()

	// 5. Submit 10 events via HTTP. Each event is hash-chained AND signed.
	createdIDs := make([]string, 0, numEvents)
	for i := 0; i < numEvents; i++ {
		id := submitEvent(t, srv.URL, fmt.Sprintf("step_%02d", i), i)
		createdIDs = append(createdIDs, id)
	}
	if got := countEvents(t, db); got != numEvents {
		t.Fatalf("rows in audit.events = %d, want %d", got, numEvents)
	}

	// 6. Verifier #1: chain valid + all 10 signatures valid.
	out := runVerifier(t, verifierBin, dsn, pubB64)
	if !out.Chain.Valid {
		t.Fatalf("verifier reported chain invalid: reason=%q expected=%q actual=%q",
			out.Chain.MismatchReason, out.Chain.ExpectedHash, out.Chain.ActualHash)
	}
	if out.Chain.EventCount != numEvents {
		t.Fatalf("verifier event_count = %d, want %d", out.Chain.EventCount, numEvents)
	}
	if out.Signatures == nil {
		t.Fatalf("verifier did not run signature check (output.signatures == nil)")
	}
	if !out.Signatures.Valid {
		t.Fatalf("verifier reported signatures invalid: reason=%q", out.Signatures.InvalidReason)
	}
	if out.Signatures.SignedCount != numEvents {
		t.Fatalf("signed_count = %d, want %d", out.Signatures.SignedCount, numEvents)
	}
	if out.Signatures.UnsignedCount != 0 {
		t.Fatalf("unsigned_count = %d, want 0", out.Signatures.UnsignedCount)
	}

	// 7. Tamper one event (modify payload directly in DB).
	//    Append-only-trigger из миграции 001 блокирует UPDATE — для имитации
	//    tampering временно отключаем триггер на этом сеансе. Реальный атакер
	//    с DBA-rights делает то же самое, и именно это verifier ДОЛЖЕН ловить.
	victim := createdIDs[len(createdIDs)/2]
	tamperEventPayload(t, db, victim, `{"tampered":true}`)

	// 8. Verifier #2: must fail with non-zero exit + hash_mismatch.
	out2, exitCode := runVerifierExpectFailure(t, verifierBin, dsn, pubB64)
	if exitCode == 0 {
		t.Fatalf("verifier exit code = 0 after tampering, want non-zero")
	}
	if out2.Chain.Valid {
		t.Fatalf("verifier reported chain valid after tampering — DETECTION FAILED")
	}
	if out2.Chain.MismatchReason != "hash_mismatch" {
		t.Fatalf("MismatchReason = %q, want %q", out2.Chain.MismatchReason, "hash_mismatch")
	}
	if out2.Chain.FirstMismatch == nil || out2.Chain.FirstMismatch.ID != victim {
		var gotID string
		if out2.Chain.FirstMismatch != nil {
			gotID = out2.Chain.FirstMismatch.ID
		}
		t.Fatalf("FirstMismatch.ID = %q, want %q", gotID, victim)
	}
}

// ----- helpers -------------------------------------------------------------

func skipIfNoDocker(t *testing.T) {
	t.Helper()
	// Quick probe: testcontainers will attempt this anyway; doing it upfront
	// gives a clear skip message instead of a 30-second timeout.
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker binary not in PATH — skipping e2e test")
	}
	cmd := exec.Command("docker", "info")
	if err := cmd.Run(); err != nil {
		t.Skip("docker daemon not available (`docker info` failed) — skipping e2e test")
	}
}

func absPathFromTest(t *testing.T, parts ...string) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	rel := filepath.Join(parts...)
	abs, err := filepath.Abs(filepath.Join(wd, rel))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	return abs
}

func startPostgres(ctx context.Context, t *testing.T) (*sql.DB, string, func()) {
	t.Helper()
	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("aibank_e2e"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("connection string: %v", err)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("sql.Open: %v", err)
	}

	pingCtx, cancelPing := context.WithTimeout(ctx, 30*time.Second)
	defer cancelPing()
	for {
		if err := db.PingContext(pingCtx); err == nil {
			break
		}
		select {
		case <-pingCtx.Done():
			_ = db.Close()
			_ = container.Terminate(ctx)
			t.Fatalf("ping postgres: %v", pingCtx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}

	teardown := func() {
		_ = db.Close()
		shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = container.Terminate(shutCtx)
	}
	return db, dsn, teardown
}

// applyMigrations parses goose-style `-- +goose Up` sections from .sql files
// in dir (sorted lexicographically) and executes the Up portion against db.
// Mirrors services/audit-service/internal/repository/testhelper_test.go logic.
func applyMigrations(db *sql.DB, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)

	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}
		up := extractGooseUp(string(raw))
		if strings.TrimSpace(up) == "" {
			return fmt.Errorf("migration %s has no `+goose Up` content", f)
		}
		if _, err := db.Exec(up); err != nil {
			return fmt.Errorf("apply %s: %w", f, err)
		}
	}
	return nil
}

func extractGooseUp(content string) string {
	upIdx := strings.Index(content, "-- +goose Up")
	if upIdx < 0 {
		return content
	}
	body := content[upIdx+len("-- +goose Up"):]
	if downIdx := strings.Index(body, "-- +goose Down"); downIdx >= 0 {
		body = body[:downIdx]
	}
	return body
}

// buildAuditVerifier compiles tools/audit-verifier into a temp binary.
// We compile once per test run; later subprocess invocations only execute
// the binary. Path returned is absolute.
func buildAuditVerifier(t *testing.T) string {
	t.Helper()
	verifierDir := absPathFromTest(t, "..", "..", "..", "..", "tools", "audit-verifier")
	if _, err := os.Stat(verifierDir); err != nil {
		t.Fatalf("audit-verifier dir not found at %s: %v", verifierDir, err)
	}
	binPath := filepath.Join(t.TempDir(), "audit-verifier")
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/audit-verifier")
	cmd.Dir = verifierDir
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build audit-verifier failed: %v\n%s", err, out)
	}
	return binPath
}

// submitEvent posts one event to /v1/events/ and returns the assigned ID.
func submitEvent(t *testing.T, baseURL, eventType string, idx int) string {
	t.Helper()
	body := map[string]any{
		"tenant_id":   tenantID,
		"entity_type": "application",
		"entity_id":   "app_e2e",
		"event_type":  eventType,
		"actor_id":    "per_e2e",
		"actor_type":  "user",
		"payload":     map[string]any{"i": idx, "channel": "web"},
	}
	buf, _ := json.Marshal(body)
	resp, err := http.Post(baseURL+"/v1/events/", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("POST /v1/events/: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST status = %d, body = %s", resp.StatusCode, raw)
	}
	var saved struct {
		ID                 string `json:"id"`
		Hash               string `json:"hash"`
		Signature          []byte `json:"signature,omitempty"`
		SignatureAlgorithm string `json:"signature_algorithm,omitempty"`
		SignerKeyID        string `json:"signer_key_id,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&saved); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if saved.ID == "" || saved.Hash == "" {
		t.Fatalf("event missing id/hash: %+v", saved)
	}
	if saved.SignatureAlgorithm != ed25519signer.Algorithm {
		t.Fatalf("event %d: signature_algorithm = %q, want %q",
			idx, saved.SignatureAlgorithm, ed25519signer.Algorithm)
	}
	if len(saved.Signature) == 0 || saved.SignerKeyID == "" {
		t.Fatalf("event %d: signature or signer_key_id empty", idx)
	}
	return saved.ID
}

func countEvents(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM audit.events WHERE tenant_id=$1`, tenantID).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

// tamperEventPayload directly mutates a single event's payload in the DB.
//
// The append-only trigger from migration 001 blocks UPDATE, so we DISABLE
// it for the duration of the UPDATE. This simulates a real DBA-level attack:
// any actor with sufficient DB privileges can disable triggers, mutate rows,
// and re-enable. The whole purpose of audit-verifier is to detect this
// off-band tampering, so reproducing the attack faithfully is essential.
func tamperEventPayload(t *testing.T, db *sql.DB, eventID, newPayload string) {
	t.Helper()
	mustExec := func(q string, args ...any) {
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	mustExec(`ALTER TABLE audit.events DISABLE TRIGGER no_update`)
	defer mustExec(`ALTER TABLE audit.events ENABLE TRIGGER no_update`)

	res, err := db.Exec(`UPDATE audit.events SET payload=$1::jsonb WHERE id=$2`,
		newPayload, eventID)
	if err != nil {
		t.Fatalf("tamper UPDATE: %v", err)
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		t.Fatalf("tamper UPDATE affected %d rows, want 1", n)
	}
}

// verifierJSON mirrors the JSON shape emitted by audit-verifier with --format=json.
// Only the fields we assert on are listed; any extras are ignored on decode.
type verifierJSON struct {
	Chain      verifierChainResult `json:"chain"`
	Signatures *verifierSigResult  `json:"signatures,omitempty"`
}

type verifierChainResult struct {
	TenantID       string                  `json:"tenant_id"`
	Valid          bool                    `json:"valid"`
	EventCount     int                     `json:"event_count"`
	LastHash       string                  `json:"last_hash,omitempty"`
	FirstMismatch  *verifierMismatchEvent  `json:"first_mismatch,omitempty"`
	ExpectedHash   string                  `json:"expected_hash,omitempty"`
	ActualHash     string                  `json:"actual_hash,omitempty"`
	MismatchReason string                  `json:"mismatch_reason,omitempty"`
}

type verifierMismatchEvent struct {
	ID string `json:"id"`
}

type verifierSigResult struct {
	TenantID      string `json:"tenant_id"`
	EventCount    int    `json:"event_count"`
	SignedCount   int    `json:"signed_count"`
	UnsignedCount int    `json:"unsigned_count"`
	Valid         bool   `json:"valid"`
	InvalidReason string `json:"invalid_reason,omitempty"`
}

// runVerifier runs audit-verifier expecting exit code 0.
func runVerifier(t *testing.T, bin, dsn, pubKeyB64 string) verifierJSON {
	t.Helper()
	out, code := runVerifierRaw(t, bin, dsn, pubKeyB64)
	if code != 0 {
		t.Fatalf("audit-verifier exited %d (expected 0). stdout/stderr:\n%s", code, out)
	}
	var v verifierJSON
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("decode verifier output: %v\nraw: %s", err, out)
	}
	return v
}

// runVerifierExpectFailure runs audit-verifier expecting non-zero exit (chain broken).
func runVerifierExpectFailure(t *testing.T, bin, dsn, pubKeyB64 string) (verifierJSON, int) {
	t.Helper()
	out, code := runVerifierRaw(t, bin, dsn, pubKeyB64)
	if code == 0 {
		t.Fatalf("audit-verifier exited 0 (expected non-zero). stdout/stderr:\n%s", out)
	}
	var v verifierJSON
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("decode verifier output: %v\nraw: %s", err, out)
	}
	return v, code
}

// runVerifierRaw runs audit-verifier and returns combined stdout/stderr + exit code.
// Sanity check on pubKeyB64 — this is the same encoding the CLI accepts via --pubkey.
func runVerifierRaw(t *testing.T, bin, dsn, pubKeyB64 string) ([]byte, int) {
	t.Helper()
	if _, err := base64.StdEncoding.DecodeString(pubKeyB64); err != nil {
		t.Fatalf("invalid base64 pubkey: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin,
		"verify",
		"--tenant-id", tenantID,
		"--source", "db",
		"--dsn", dsn,
		"--pubkey", pubKeyB64,
		"--format", "json",
	)
	// Verifier emits structured logs to stderr and the JSON payload to stdout.
	// Combining streams is fine for assertions because we json.Unmarshal which
	// will just match the JSON object — but cleaner: capture stdout separately.
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("run audit-verifier: %v\nstderr:\n%s", err, stderr.String())
		}
	}
	if t.Failed() || testing.Verbose() {
		t.Logf("audit-verifier stderr:\n%s", stderr.String())
	}
	return stdout.Bytes(), code
}
