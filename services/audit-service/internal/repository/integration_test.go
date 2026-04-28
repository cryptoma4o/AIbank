//go:build integration

// Integration tests for audit-service repository layer.
//
// Boots a real PostgreSQL 16 container, applies the audit schema migration,
// and exercises the append-only contract: Append + LatestHash round trip,
// trigger-enforced rejection of UPDATE/DELETE, and hash-chain integrity
// across multiple events.
//
// Run with:
//
//	go test -tags=integration -v ./internal/repository/...
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aibank/platform/services/audit-service/internal/domain"
	"github.com/google/uuid"
)

var testDB *sql.DB

func TestMain(m *testing.M) {
	db, teardown, err := startPostgres()
	if err != nil {
		log.Fatalf("startPostgres: %v", err)
	}
	defer teardown()

	migrationsDir := filepath.Join("..", "..", "migrations")
	if err := applyMigrationsFromDir(db, migrationsDir); err != nil {
		log.Fatalf("apply migrations: %v", err)
	}
	testDB = db
	os.Exit(m.Run())
}

func resetEvents(t *testing.T) {
	t.Helper()
	// Append-only triggers block DELETE; use a privileged session_replication_role
	// hack OR drop+recreate. Easiest: TRUNCATE bypasses row-level triggers.
	if _, err := testDB.Exec(`TRUNCATE TABLE audit.events`); err != nil {
		t.Fatalf("truncate audit.events: %v", err)
	}
}

func newEvent(tenantID, entityID, eventType, prevHash string) *domain.AuditEvent {
	e := &domain.AuditEvent{
		ID:         uuid.NewString(),
		TenantID:   tenantID,
		EntityType: "application",
		EntityID:   entityID,
		EventType:  eventType,
		ActorID:    "actor_1",
		ActorType:  domain.ActorTypeUser,
		Payload:    json.RawMessage(`{}`),
		CreatedAt:  time.Now().UTC(),
	}
	e.Hash = e.ComputeHash(prevHash)
	return e
}

func TestPostgresAuditRepository_Append_RoundTrip(t *testing.T) {
	resetEvents(t)
	repo := NewPostgresAuditRepository(testDB)
	ctx := context.Background()

	prev, err := repo.LatestHash(ctx, "tnt_demo")
	if err != nil {
		t.Fatalf("LatestHash (genesis): %v", err)
	}
	if prev != "" {
		t.Fatalf("genesis tenant must have empty latest hash, got %q", prev)
	}

	evt := newEvent("tnt_demo", "app_1", "submitted", prev)
	if err := repo.Append(ctx, evt); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, err := repo.LatestHash(ctx, "tnt_demo")
	if err != nil {
		t.Fatalf("LatestHash post-append: %v", err)
	}
	if got != evt.Hash {
		t.Fatalf("LatestHash mismatch: got %q want %q", got, evt.Hash)
	}
}

func TestPostgresAuditRepository_AppendOnly_BlocksUpdate(t *testing.T) {
	resetEvents(t)
	repo := NewPostgresAuditRepository(testDB)
	ctx := context.Background()

	evt := newEvent("tnt_demo", "app_1", "submitted", "")
	if err := repo.Append(ctx, evt); err != nil {
		t.Fatalf("Append: %v", err)
	}

	_, err := testDB.Exec(`UPDATE audit.events SET event_type='tampered' WHERE id=$1`, evt.ID)
	if err == nil {
		t.Fatalf("UPDATE on audit.events succeeded — append-only trigger missing")
	}
	if !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("expected error containing 'append-only', got: %v", err)
	}
}

func TestPostgresAuditRepository_AppendOnly_BlocksDelete(t *testing.T) {
	resetEvents(t)
	repo := NewPostgresAuditRepository(testDB)
	ctx := context.Background()

	evt := newEvent("tnt_demo", "app_1", "submitted", "")
	if err := repo.Append(ctx, evt); err != nil {
		t.Fatalf("Append: %v", err)
	}

	_, err := testDB.Exec(`DELETE FROM audit.events WHERE id=$1`, evt.ID)
	if err == nil {
		t.Fatalf("DELETE on audit.events succeeded — append-only trigger missing")
	}
	if !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("expected error containing 'append-only', got: %v", err)
	}
}

func TestPostgresAuditRepository_HashChain_Integrity(t *testing.T) {
	resetEvents(t)
	repo := NewPostgresAuditRepository(testDB)
	ctx := context.Background()

	tenantID := "tnt_chain"
	prev := ""
	var events []*domain.AuditEvent
	for i := 0; i < 3; i++ {
		// Spread created_at so ORDER BY created_at DESC LIMIT 1 is unambiguous.
		evt := newEvent(tenantID, "app_chain", "step", prev)
		evt.CreatedAt = time.Now().UTC().Add(time.Duration(i) * time.Millisecond)
		evt.Hash = evt.ComputeHash(prev)
		if err := repo.Append(ctx, evt); err != nil {
			t.Fatalf("Append #%d: %v", i, err)
		}
		events = append(events, evt)
		prev = evt.Hash
	}

	got, err := repo.LatestHash(ctx, tenantID)
	if err != nil {
		t.Fatalf("LatestHash: %v", err)
	}
	want := events[2].Hash
	if got != want {
		t.Fatalf("LatestHash != 3rd event hash: got %q want %q", got, want)
	}

	// Each subsequent event must reference its predecessor's hash.
	for i := 1; i < len(events); i++ {
		if events[i].PreviousHash != events[i-1].Hash {
			t.Fatalf("chain broken at i=%d: prev=%q want=%q",
				i, events[i].PreviousHash, events[i-1].Hash)
		}
	}
}

func TestPostgresAuditRepository_List_FiltersByEntity(t *testing.T) {
	resetEvents(t)
	repo := NewPostgresAuditRepository(testDB)
	ctx := context.Background()

	tenantID := "tnt_list"
	for _, id := range []string{"app_a", "app_b", "app_a"} {
		evt := newEvent(tenantID, id, "step", "")
		if err := repo.Append(ctx, evt); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	got, err := repo.List(ctx, tenantID, "application", "app_a", 100)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("filter by entity_id failed: got %d events, want 2", len(got))
	}
	for _, e := range got {
		if e.EntityID != "app_a" {
			t.Fatalf("filter leaked entity_id %q", e.EntityID)
		}
	}
}
