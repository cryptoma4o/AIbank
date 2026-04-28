//go:build integration

// Integration tests for document-service repository layer.
//
// Boots a real PostgreSQL 16 container, provisions:
//   - schema `platform` with a minimal `platform.tenants` row for `test`
//     (document-service migrations don't need it directly, but staying
//     consistent with the production layout helps when future migrations
//     reference the registry).
//   - schema `tnt_test` from migrations/tenant/001_create_documents.sql
//
// Then exercises:
//   - Create + GetByID round trip
//   - ListByApplication filter
//   - MarkParsed transitions state to "parsed"
//   - MarkRejected transitions state to "rejected" with a reason payload
//   - search_path isolation: rows in tnt_test are invisible from a
//     different schema (tnt_other), proving schema-per-tenant isolation
//     described in ADR-0002.
//
// Run with:
//
//	go test -tags=integration -v ./internal/repository/...
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/aibank/platform/services/document-service/internal/domain"
	"github.com/google/uuid"
)

var testDB *sql.DB

const testTenantID = "test"

func TestMain(m *testing.M) {
	db, teardown, err := startPostgres()
	if err != nil {
		log.Fatalf("startPostgres: %v", err)
	}
	defer teardown()

	// Mimic platform layout (no FK is required by document migrations,
	// but having a registry table mirrors production and supports future
	// cross-tenant tests).
	if _, err := db.Exec(`
		CREATE SCHEMA IF NOT EXISTS platform;
		CREATE TABLE IF NOT EXISTS platform.tenants (
			id   TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT ''
		);
		INSERT INTO platform.tenants (id, name) VALUES ($1, $1)
			ON CONFLICT DO NOTHING;
	`, testTenantID); err != nil {
		log.Fatalf("bootstrap platform.tenants: %v", err)
	}

	// Provision tnt_test and apply documents tenant migrations to it.
	tenantSchema := "tnt_" + testTenantID
	if _, err := db.Exec("CREATE SCHEMA IF NOT EXISTS " + tenantSchema); err != nil {
		log.Fatalf("create %s: %v", tenantSchema, err)
	}
	if _, err := db.Exec("SET search_path TO " + tenantSchema + ", public"); err != nil {
		log.Fatalf("set search_path: %v", err)
	}
	if err := applyMigrationsFromDir(db, filepath.Join("..", "..", "migrations", "tenant")); err != nil {
		log.Fatalf("apply tenant migrations: %v", err)
	}

	// Provision an isolation-counterpart schema for the search_path test.
	if _, err := db.Exec(`SET search_path TO public`); err != nil {
		log.Fatalf("reset search_path: %v", err)
	}
	if _, err := db.Exec(`CREATE SCHEMA IF NOT EXISTS tnt_other`); err != nil {
		log.Fatalf("create tnt_other: %v", err)
	}
	if _, err := db.Exec(`SET search_path TO tnt_other, public`); err != nil {
		log.Fatalf("set search_path tnt_other: %v", err)
	}
	if err := applyMigrationsFromDir(db, filepath.Join("..", "..", "migrations", "tenant")); err != nil {
		log.Fatalf("apply tenant migrations to tnt_other: %v", err)
	}
	if _, err := db.Exec(`SET search_path TO public`); err != nil {
		log.Fatalf("reset search_path: %v", err)
	}

	testDB = db
	os.Exit(m.Run())
}

func resetDocs(t *testing.T) {
	t.Helper()
	if _, err := testDB.Exec(`TRUNCATE TABLE tnt_test.documents RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate tnt_test.documents: %v", err)
	}
	if _, err := testDB.Exec(`TRUNCATE TABLE tnt_other.documents RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate tnt_other.documents: %v", err)
	}
}

func newDoc(applicationID string) *domain.Document {
	return &domain.Document{
		ID:            uuid.NewString(),
		TenantID:      testTenantID,
		ApplicationID: applicationID,
		Type:          domain.DocumentTypePassport,
		State:         domain.DocumentStateUploaded,
		FileID:        uuid.NewString(),
		Filename:      "passport.pdf",
		MimeType:      "application/pdf",
		SizeBytes:     1024,
		SHA256:        "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		StoragePath:   "minio://docs/test/passport.pdf",
		SourceType:    domain.SourceClientUpload,
		SourceActorID: "applicant_1",
	}
}

func TestPostgresDocumentRepository_CreateAndGetByID(t *testing.T) {
	resetDocs(t)
	repo := NewPostgresDocumentRepository(testDB)
	ctx := context.Background()

	doc := newDoc("app_1")
	doc.ParsedData = json.RawMessage(`{"first_name":"Иван"}`)
	if err := repo.Create(ctx, doc); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, testTenantID, doc.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Filename != doc.Filename || got.SHA256 != doc.SHA256 ||
		got.Type != doc.Type || got.State != domain.DocumentStateUploaded {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if string(got.ParsedData) == "" {
		t.Fatalf("parsed_data not preserved")
	}
}

func TestPostgresDocumentRepository_GetByID_NotFound(t *testing.T) {
	resetDocs(t)
	repo := NewPostgresDocumentRepository(testDB)
	_, err := repo.GetByID(context.Background(), testTenantID, "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPostgresDocumentRepository_ListByApplication(t *testing.T) {
	resetDocs(t)
	repo := NewPostgresDocumentRepository(testDB)
	ctx := context.Background()

	for _, app := range []string{"app_a", "app_b", "app_a"} {
		if err := repo.Create(ctx, newDoc(app)); err != nil {
			t.Fatalf("Create %s: %v", app, err)
		}
	}
	got, err := repo.ListByApplication(ctx, testTenantID, "app_a")
	if err != nil {
		t.Fatalf("ListByApplication: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("filter failed: got %d docs, want 2", len(got))
	}
	for _, d := range got {
		if d.ApplicationID != "app_a" {
			t.Fatalf("filter leaked application_id %q", d.ApplicationID)
		}
	}
}

func TestPostgresDocumentRepository_MarkParsed(t *testing.T) {
	resetDocs(t)
	repo := NewPostgresDocumentRepository(testDB)
	ctx := context.Background()

	doc := newDoc("app_parse")
	if err := repo.Create(ctx, doc); err != nil {
		t.Fatalf("Create: %v", err)
	}
	parsed := json.RawMessage(`{"series":"1234","number":"567890"}`)
	if err := repo.MarkParsed(ctx, testTenantID, doc.ID, parsed); err != nil {
		t.Fatalf("MarkParsed: %v", err)
	}
	got, err := repo.GetByID(ctx, testTenantID, doc.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.State != domain.DocumentStateParsed {
		t.Fatalf("state not transitioned: got %s", got.State)
	}
	if string(got.ParsedData) == "" {
		t.Fatalf("parsed_data not stored")
	}
}

func TestPostgresDocumentRepository_MarkParsed_NotFound(t *testing.T) {
	resetDocs(t)
	repo := NewPostgresDocumentRepository(testDB)
	err := repo.MarkParsed(context.Background(), testTenantID, "missing", json.RawMessage(`{}`))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPostgresDocumentRepository_MarkRejected(t *testing.T) {
	resetDocs(t)
	repo := NewPostgresDocumentRepository(testDB)
	ctx := context.Background()

	doc := newDoc("app_reject")
	if err := repo.Create(ctx, doc); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.MarkRejected(ctx, testTenantID, doc.ID, "blurry image"); err != nil {
		t.Fatalf("MarkRejected: %v", err)
	}
	got, err := repo.GetByID(ctx, testTenantID, doc.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.State != domain.DocumentStateRejected {
		t.Fatalf("state not transitioned: got %s", got.State)
	}
	var payload map[string]string
	if err := json.Unmarshal(got.ParsedData, &payload); err != nil {
		t.Fatalf("parsed_data not valid json: %v", err)
	}
	if payload["reason"] != "blurry image" {
		t.Fatalf("reason not stored: got %v", payload)
	}
}

func TestPostgresDocumentRepository_RejectsBadTenantID(t *testing.T) {
	repo := NewPostgresDocumentRepository(testDB)
	ctx := context.Background()
	for _, id := range []string{"", "1abc", "Bad", "bad-name", `evil";--`} {
		_, err := repo.GetByID(ctx, id, "x")
		if !errors.Is(err, ErrInvalidTenant) {
			t.Fatalf("GetByID(%q) want ErrInvalidTenant, got %v", id, err)
		}
	}
}

func TestPostgresDocumentRepository_SearchPathIsolation(t *testing.T) {
	resetDocs(t)
	repo := NewPostgresDocumentRepository(testDB)
	ctx := context.Background()

	// Insert into tnt_test via the repository (which sets search_path
	// transparently through SET LOCAL).
	doc := newDoc("app_iso")
	if err := repo.Create(ctx, doc); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Direct query into tnt_other.documents must yield zero rows —
	// proving the row landed only in tnt_test, isolated by schema.
	var count int
	if err := testDB.QueryRowContext(ctx,
		`SELECT count(*) FROM tnt_other.documents WHERE id = $1`, doc.ID,
	).Scan(&count); err != nil {
		t.Fatalf("query tnt_other: %v", err)
	}
	if count != 0 {
		t.Fatalf("schema isolation breached: tnt_test row visible from tnt_other (count=%d)", count)
	}

	// Sanity: the row IS in tnt_test.
	if err := testDB.QueryRowContext(ctx,
		`SELECT count(*) FROM tnt_test.documents WHERE id = $1`, doc.ID,
	).Scan(&count); err != nil {
		t.Fatalf("query tnt_test: %v", err)
	}
	if count != 1 {
		t.Fatalf("row not present in tnt_test: count=%d", count)
	}
}
