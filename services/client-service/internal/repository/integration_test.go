//go:build integration

// Integration tests for client-service repository layer.
//
// Boots a real PostgreSQL 16 container, provisions:
//   - schema `platform` with a minimal `platform.tenants` registry (parallel
//     to the production layout maintained by tenant-service).
//   - schema `tnt_test`  — primary fixture for the per-tenant tables;
//     migrations from migrations/tenant/*.sql are applied to it.
//   - schemas `tnt_a`, `tnt_b` — used by the cross-tenant isolation test.
//
// Then exercises the postgres-backed PostgresClientRepository and
// PostgresClientHistoryRepository against the real database — no mocks
// below the SQL layer:
//
//   1. Create + GetByID round trip (all fields preserved)
//   2. GetByID returns ErrNotFound for missing rows
//   3. ListByTenant pagination (limit/offset, order DESC by created_at)
//   4. UpdateStatus changes status + history append within a tenant
//   5. client_history append-only trigger blocks UPDATE
//   6. client_history append-only trigger blocks DELETE
//   7. Schema-per-tenant isolation: rows in tnt_a are invisible from tnt_b
//
// Run with:
//
//	go test -tags=integration -v ./internal/repository/...
//
// Requires a running Docker daemon. Without `-tags=integration` the file
// is excluded from the build so plain `go test ./...` stays fast and
// hermetic.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aibank/client-service/internal/domain"

	"github.com/google/uuid"
)

var (
	testDB              *sql.DB
	tenantMigrationsDir string
)

const (
	primaryTenantID = "test"
	tenantAID       = "a"
	tenantBID       = "b"
)

func TestMain(m *testing.M) {
	db, teardown, err := startPostgres()
	if err != nil {
		log.Fatalf("startPostgres: %v", err)
	}
	defer teardown()

	tenantMigrationsDir = filepath.Join("..", "..", "migrations", "tenant")

	// Bootstrap minimal platform.tenants registry (mirrors production).
	if _, err := db.Exec(`
		CREATE SCHEMA IF NOT EXISTS platform;
		CREATE TABLE IF NOT EXISTS platform.tenants (
			id   TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT ''
		);
	`); err != nil {
		log.Fatalf("bootstrap platform schema: %v", err)
	}
	for _, id := range []string{primaryTenantID, tenantAID, tenantBID} {
		if _, err := db.Exec(
			`INSERT INTO platform.tenants (id, name) VALUES ($1, $1)
			 ON CONFLICT DO NOTHING`, id); err != nil {
			log.Fatalf("seed platform.tenants(%s): %v", id, err)
		}
	}

	// Provision tnt_test, tnt_a, tnt_b. createTenantSchema mutates
	// search_path during application and resets it to public afterwards.
	for _, id := range []string{primaryTenantID, tenantAID, tenantBID} {
		schema := "tnt_" + id
		if _, err := db.Exec(`CREATE SCHEMA IF NOT EXISTS ` + schema); err != nil {
			log.Fatalf("create %s: %v", schema, err)
		}
		if _, err := db.Exec(`SET search_path TO ` + schema + `, public`); err != nil {
			log.Fatalf("set search_path %s: %v", schema, err)
		}
		if err := applyMigrationsFromDir(db, tenantMigrationsDir); err != nil {
			log.Fatalf("apply tenant migrations to %s: %v", schema, err)
		}
	}
	if _, err := db.Exec(`SET search_path TO public`); err != nil {
		log.Fatalf("reset search_path: %v", err)
	}

	testDB = db
	os.Exit(m.Run())
}

func resetTenantData(t *testing.T) {
	t.Helper()
	for _, id := range []string{primaryTenantID, tenantAID, tenantBID} {
		// Truncate via TRUNCATE — bypasses row-level triggers, so the
		// append-only guard on client_history doesn't block test cleanup.
		if _, err := testDB.Exec(
			`TRUNCATE TABLE tnt_` + id + `.client_history,
			               tnt_` + id + `.clients RESTART IDENTITY CASCADE`,
		); err != nil {
			t.Fatalf("truncate tnt_%s: %v", id, err)
		}
	}
}

func newTestClient(tenantID, applicantSuffix string) *domain.Client {
	return &domain.Client{
		ID:            uuid.NewString(),
		TenantID:      tenantID,
		ApplicantID:   "applicant_" + applicantSuffix,
		LegalEntityID: "le_" + applicantSuffix,
		Status:        domain.ClientStatusOnboarding,
		RiskCategory:  domain.RiskCategoryLow,
	}
}

func newTestHistory(clientID, eventType string) *domain.ClientHistory {
	return &domain.ClientHistory{
		ID:        uuid.NewString(),
		ClientID:  clientID,
		EventType: eventType,
		Summary:   "test summary",
		Source:    "audit_event_xyz",
	}
}

// 1. Create + GetByID — round trip, all fields preserved.
func TestClientRepo_CreateAndGetByID(t *testing.T) {
	resetTenantData(t)
	repo := NewPostgresClientRepository(testDB)
	ctx := context.Background()

	want := newTestClient(primaryTenantID, "1")
	want.RiskCategory = domain.RiskCategoryMedium
	if err := repo.Create(ctx, want); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetByID(ctx, primaryTenantID, want.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != want.ID || got.ApplicantID != want.ApplicantID ||
		got.LegalEntityID != want.LegalEntityID ||
		got.Status != want.Status || got.RiskCategory != want.RiskCategory {
		t.Fatalf("round-trip mismatch:\n got=%+v\nwant=%+v", got, want)
	}
	if got.TenantID != primaryTenantID {
		t.Fatalf("tenant_id not propagated: got %q", got.TenantID)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not populated: %+v", got)
	}
}

// 2. GetByID returns ErrNotFound for missing rows.
func TestClientRepo_GetByID_NotFound(t *testing.T) {
	resetTenantData(t)
	repo := NewPostgresClientRepository(testDB)
	_, err := repo.GetByID(context.Background(), primaryTenantID, "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// 3. ListByTenant pagination — limit/offset honored, ORDER BY created_at DESC.
func TestClientRepo_ListByTenant(t *testing.T) {
	resetTenantData(t)
	repo := NewPostgresClientRepository(testDB)
	ctx := context.Background()

	// Insert 5 clients with strictly increasing created_at so the DESC
	// ordering is deterministic.
	ids := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		c := newTestClient(primaryTenantID, "p"+string(rune('0'+i)))
		if err := repo.Create(ctx, c); err != nil {
			t.Fatalf("Create #%d: %v", i, err)
		}
		ids = append(ids, c.ID)
		// Ensure stable monotonic created_at — Create stamps it from
		// time.Now(); a small sleep guarantees ordering on fast hosts.
		time.Sleep(2 * time.Millisecond)
	}

	// First page: 2 most recent.
	page1, err := repo.ListByTenant(ctx, primaryTenantID, 2, 0)
	if err != nil {
		t.Fatalf("ListByTenant page 1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page 1: got %d, want 2", len(page1))
	}
	// Most recent insert should come first.
	if page1[0].ID != ids[len(ids)-1] {
		t.Fatalf("page 1 not DESC by created_at: head=%q want=%q",
			page1[0].ID, ids[len(ids)-1])
	}
	for i := 0; i < len(page1)-1; i++ {
		if page1[i].CreatedAt.Before(page1[i+1].CreatedAt) {
			t.Fatalf("page 1 ordering broken at %d", i)
		}
	}

	// Second page: skip 2, take 2.
	page2, err := repo.ListByTenant(ctx, primaryTenantID, 2, 2)
	if err != nil {
		t.Fatalf("ListByTenant page 2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("page 2: got %d, want 2", len(page2))
	}

	// Third page: 1 remaining.
	page3, err := repo.ListByTenant(ctx, primaryTenantID, 2, 4)
	if err != nil {
		t.Fatalf("ListByTenant page 3: %v", err)
	}
	if len(page3) != 1 {
		t.Fatalf("page 3: got %d, want 1", len(page3))
	}

	// No overlap across pages.
	seen := map[string]struct{}{}
	for _, c := range append(append(page1, page2...), page3...) {
		if _, dup := seen[c.ID]; dup {
			t.Fatalf("pagination yielded duplicate id %q", c.ID)
		}
		seen[c.ID] = struct{}{}
	}
	if len(seen) != 5 {
		t.Fatalf("pagination lost rows: covered %d, want 5", len(seen))
	}
}

// 4. UpdateStatus changes status; verify history append also works (both
//    are independent repos but together model the typical "status flip +
//    audit row" workflow).
func TestClientRepo_UpdateStatus(t *testing.T) {
	resetTenantData(t)
	clients := NewPostgresClientRepository(testDB)
	history := NewPostgresClientHistoryRepository(testDB)
	ctx := context.Background()

	c := newTestClient(primaryTenantID, "upd")
	if err := clients.Create(ctx, c); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := clients.UpdateStatus(ctx, primaryTenantID, c.ID, domain.ClientStatusActive); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, err := clients.GetByID(ctx, primaryTenantID, c.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != domain.ClientStatusActive {
		t.Fatalf("status not updated: got %s", got.Status)
	}
	if !got.UpdatedAt.After(got.CreatedAt) && !got.UpdatedAt.Equal(got.CreatedAt) {
		t.Fatalf("updated_at not progressed: created=%v updated=%v", got.CreatedAt, got.UpdatedAt)
	}

	// Append a history record reflecting the transition.
	h := newTestHistory(c.ID, "status_changed")
	if err := history.Append(ctx, h, primaryTenantID); err != nil {
		t.Fatalf("history.Append: %v", err)
	}
	rows, err := history.ListByClient(ctx, primaryTenantID, c.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListByClient: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("history rows: got %d, want 1", len(rows))
	}
	if rows[0].EventType != "status_changed" || rows[0].Source != "audit_event_xyz" {
		t.Fatalf("history payload mismatch: %+v", rows[0])
	}

	// UpdateStatus on a missing id yields ErrNotFound.
	if err := clients.UpdateStatus(ctx, primaryTenantID, "ghost", domain.ClientStatusSuspended); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateStatus(missing) want ErrNotFound, got %v", err)
	}
}

// 5. client_history append-only: direct UPDATE must be rejected by trigger.
func TestClientHistoryRepo_AppendOnly(t *testing.T) {
	resetTenantData(t)
	clients := NewPostgresClientRepository(testDB)
	history := NewPostgresClientHistoryRepository(testDB)
	ctx := context.Background()

	c := newTestClient(primaryTenantID, "ao")
	if err := clients.Create(ctx, c); err != nil {
		t.Fatalf("Create client: %v", err)
	}
	h := newTestHistory(c.ID, "submitted")
	if err := history.Append(ctx, h, primaryTenantID); err != nil {
		t.Fatalf("history.Append: %v", err)
	}

	_, err := testDB.Exec(
		`UPDATE tnt_test.client_history SET event_type = 'tampered' WHERE id = $1`, h.ID)
	if err == nil {
		t.Fatalf("UPDATE on client_history succeeded — append-only trigger missing")
	}
	if !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("expected error containing 'append-only', got: %v", err)
	}
}

// 6. client_history append-only: direct DELETE must be rejected by trigger.
func TestClientHistoryRepo_AppendOnly_Delete(t *testing.T) {
	resetTenantData(t)
	clients := NewPostgresClientRepository(testDB)
	history := NewPostgresClientHistoryRepository(testDB)
	ctx := context.Background()

	c := newTestClient(primaryTenantID, "del")
	if err := clients.Create(ctx, c); err != nil {
		t.Fatalf("Create client: %v", err)
	}
	h := newTestHistory(c.ID, "submitted")
	if err := history.Append(ctx, h, primaryTenantID); err != nil {
		t.Fatalf("history.Append: %v", err)
	}

	_, err := testDB.Exec(
		`DELETE FROM tnt_test.client_history WHERE id = $1`, h.ID)
	if err == nil {
		t.Fatalf("DELETE on client_history succeeded — append-only trigger missing")
	}
	if !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("expected error containing 'append-only', got: %v", err)
	}
}

// 7. Schema-per-tenant isolation: a row in tnt_a must not be visible
//    when querying via the tnt_b schema. Proves that withTenantTx's
//    SET LOCAL search_path is doing its job (ADR-0002).
func TestClientRepo_TenantIsolation(t *testing.T) {
	resetTenantData(t)
	repo := NewPostgresClientRepository(testDB)
	ctx := context.Background()

	clientA := newTestClient(tenantAID, "iso_a")
	clientB := newTestClient(tenantBID, "iso_b")

	if err := repo.Create(ctx, clientA); err != nil {
		t.Fatalf("Create tenant A: %v", err)
	}
	if err := repo.Create(ctx, clientB); err != nil {
		t.Fatalf("Create tenant B: %v", err)
	}

	// A is visible from A only.
	gotA, err := repo.GetByID(ctx, tenantAID, clientA.ID)
	if err != nil {
		t.Fatalf("GetByID A from tenant A: %v", err)
	}
	if gotA.ID != clientA.ID {
		t.Fatalf("wrong row from tenant A: %+v", gotA)
	}
	if _, err := repo.GetByID(ctx, tenantBID, clientA.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tenant B sees tenant A's row: err=%v", err)
	}

	// B is visible from B only.
	gotB, err := repo.GetByID(ctx, tenantBID, clientB.ID)
	if err != nil {
		t.Fatalf("GetByID B from tenant B: %v", err)
	}
	if gotB.ID != clientB.ID {
		t.Fatalf("wrong row from tenant B: %+v", gotB)
	}
	if _, err := repo.GetByID(ctx, tenantAID, clientB.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tenant A sees tenant B's row: err=%v", err)
	}

	// Sanity: each schema physically has exactly one row.
	for _, id := range []string{tenantAID, tenantBID} {
		var count int
		if err := testDB.QueryRowContext(ctx,
			`SELECT count(*) FROM tnt_`+id+`.clients`).Scan(&count); err != nil {
			t.Fatalf("count tnt_%s: %v", id, err)
		}
		if count != 1 {
			t.Fatalf("tnt_%s row count: got %d, want 1", id, count)
		}
	}
}
