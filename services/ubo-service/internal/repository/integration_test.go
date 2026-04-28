//go:build integration

// Integration tests for ubo-service repository layer.
//
// Boots a real PostgreSQL 16 container, provisions:
//   - schema `platform` with a minimal `platform.tenants` registry (parallel
//     to the production layout maintained by tenant-service).
//   - schema `tnt_test`  — primary fixture for the per-tenant ubo_graphs
//     table; migrations from migrations/tenant/*.sql are applied to it.
//   - schemas `tnt_a`, `tnt_b` — used by the cross-tenant isolation test.
//
// Then exercises the postgres-backed PostgresUBOGraphRepository against
// the real database — no mocks below the SQL layer:
//
//   1. Create + GetByID round trip (all JSONB + scalar fields preserved)
//   2. GetByID returns ErrNotFound for missing rows
//   3. GetLatestByLegalEntity returns the row with highest version
//   4. VersionIncrement: 3 successive Creates auto-increment 1 -> 2 -> 3
//   5. VersionIncrement_Concurrent: 10 goroutines racing on the same
//      legal_entity_id end up with exactly 10 unique versions written;
//      collisions on the UNIQUE(legal_entity_id, version) constraint are
//      retried by the test harness, simulating the retry the calling
//      orchestrator must implement around Create().
//   6. TenantIsolation: rows in tnt_a remain invisible from tnt_b
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
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"aibank/ubo-service/internal/domain"

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

	// Provision tnt_test, tnt_a, tnt_b — apply tenant migrations to each.
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

func resetGraphs(t *testing.T) {
	t.Helper()
	for _, id := range []string{primaryTenantID, tenantAID, tenantBID} {
		if _, err := testDB.Exec(
			`TRUNCATE TABLE tnt_` + id + `.ubo_graphs RESTART IDENTITY CASCADE`,
		); err != nil {
			t.Fatalf("truncate tnt_%s.ubo_graphs: %v", id, err)
		}
	}
}

func newTestGraph(tenantID, legalEntityID string) *domain.UBOGraph {
	return &domain.UBOGraph{
		ID:                 uuid.NewString(),
		TenantID:           tenantID,
		LegalEntityID:      legalEntityID,
		Nodes:              json.RawMessage(`[{"id":"per_1","type":"person","name":"Иван"}]`),
		Edges:              json.RawMessage(`[{"from":"per_1","to":"le_1","share_percent":75}]`),
		UBOs:               json.RawMessage(`[{"person_id":"per_1","name":"Иван","effective_share_percent":75,"control_basis":"ownership","paths":[["per_1","le_1"]]}]`),
		Confidence:         0.93,
		UnresolvedBranches: json.RawMessage(`[]`),
		ComputedBy:         "agent-ubo-tracing@v1",
	}
}

// 1. Create + GetByID — round trip, including JSONB payloads.
func TestUBORepo_CreateAndGetByID(t *testing.T) {
	resetGraphs(t)
	repo := NewPostgresUBOGraphRepository(testDB)
	ctx := context.Background()

	want := newTestGraph(primaryTenantID, "le_create")
	if err := repo.Create(ctx, want); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if want.Version != 1 {
		t.Fatalf("first Create must produce version=1, got %d", want.Version)
	}

	got, err := repo.GetByID(ctx, primaryTenantID, want.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != want.ID || got.LegalEntityID != want.LegalEntityID ||
		got.Version != want.Version || got.ComputedBy != want.ComputedBy {
		t.Fatalf("scalar mismatch:\n got=%+v\nwant=%+v", got, want)
	}
	if got.Confidence < 0.92 || got.Confidence > 0.94 {
		t.Fatalf("confidence not preserved: got %v", got.Confidence)
	}
	if got.ComputedAt.IsZero() {
		t.Fatalf("computed_at not stamped")
	}
	// JSONB payloads — Postgres normalises whitespace; check structurally
	// rather than byte-equal.
	var nodes []domain.GraphNode
	if err := json.Unmarshal(got.Nodes, &nodes); err != nil {
		t.Fatalf("nodes JSON not valid: %v", err)
	}
	if len(nodes) != 1 || nodes[0].ID != "per_1" {
		t.Fatalf("nodes payload corrupted: %+v", nodes)
	}
	var ubos []domain.UBO
	if err := json.Unmarshal(got.UBOs, &ubos); err != nil {
		t.Fatalf("ubos JSON not valid: %v", err)
	}
	if len(ubos) != 1 || ubos[0].ControlBasis != "ownership" {
		t.Fatalf("ubos payload corrupted: %+v", ubos)
	}
}

// 2. GetByID returns ErrNotFound for missing rows.
func TestUBORepo_GetByID_NotFound(t *testing.T) {
	resetGraphs(t)
	repo := NewPostgresUBOGraphRepository(testDB)
	_, err := repo.GetByID(context.Background(), primaryTenantID, "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// 3. GetLatestByLegalEntity returns the row with the highest version.
func TestUBORepo_GetLatestByLegalEntity(t *testing.T) {
	resetGraphs(t)
	repo := NewPostgresUBOGraphRepository(testDB)
	ctx := context.Background()

	const le = "le_latest"
	var lastID string
	for i := 0; i < 4; i++ {
		g := newTestGraph(primaryTenantID, le)
		if err := repo.Create(ctx, g); err != nil {
			t.Fatalf("Create #%d: %v", i, err)
		}
		lastID = g.ID
	}
	// Insert one row for a different LE — GetLatestByLegalEntity must NOT
	// pick it up.
	other := newTestGraph(primaryTenantID, "le_other")
	if err := repo.Create(ctx, other); err != nil {
		t.Fatalf("Create other LE: %v", err)
	}

	got, err := repo.GetLatestByLegalEntity(ctx, primaryTenantID, le)
	if err != nil {
		t.Fatalf("GetLatestByLegalEntity: %v", err)
	}
	if got.ID != lastID {
		t.Fatalf("latest id mismatch: got %q want %q", got.ID, lastID)
	}
	if got.Version != 4 {
		t.Fatalf("latest version: got %d, want 4", got.Version)
	}
	if got.LegalEntityID != le {
		t.Fatalf("latest LE crossed: got %q", got.LegalEntityID)
	}

	// Missing LE → ErrNotFound.
	if _, err := repo.GetLatestByLegalEntity(ctx, primaryTenantID, "le_unknown"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing LE: want ErrNotFound, got %v", err)
	}
}

// 4. Three sequential Creates against the same legal_entity_id auto-
//    increment version 1 → 2 → 3.
func TestUBORepo_VersionIncrement(t *testing.T) {
	resetGraphs(t)
	repo := NewPostgresUBOGraphRepository(testDB)
	ctx := context.Background()

	const le = "le_incr"
	for i := 1; i <= 3; i++ {
		g := newTestGraph(primaryTenantID, le)
		if err := repo.Create(ctx, g); err != nil {
			t.Fatalf("Create #%d: %v", i, err)
		}
		if g.Version != i {
			t.Fatalf("auto-increment broken: Create #%d set Version=%d, want %d", i, g.Version, i)
		}
	}

	// All versions queryable via ListByLegalEntity DESC.
	rows, err := repo.ListByLegalEntity(ctx, primaryTenantID, le, 100, 0)
	if err != nil {
		t.Fatalf("ListByLegalEntity: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("want 3 versions, got %d", len(rows))
	}
	for i, r := range rows {
		wantVer := 3 - i
		if r.Version != wantVer {
			t.Fatalf("row %d version: got %d, want %d (DESC order)", i, r.Version, wantVer)
		}
	}
}

// 5. Concurrent Create calls on the same legal_entity_id. Repo computes
//    version = MAX(version)+1 within a transaction without explicit
//    locking — under contention some inserts will collide on the
//    UNIQUE(legal_entity_id, version) constraint. The repository surfaces
//    this as a Postgres error; a real caller (orchestrator) is expected
//    to retry. This test wraps Create() in a bounded retry to model that
//    contract and asserts that exactly N unique versions land in the
//    table after N goroutines.
//
// NOTE: The retry loop lives in the test, not in the repository — the
// production code path expects the orchestrator to retry on conflict.
// See `TODOs` at the bottom of the report for upstream follow-up.
func TestUBORepo_VersionIncrement_Concurrent(t *testing.T) {
	resetGraphs(t)
	repo := NewPostgresUBOGraphRepository(testDB)
	ctx := context.Background()

	const (
		le        = "le_concurrent"
		workers   = 10
		retryCap  = 50 // generous; collisions on a fresh container are bounded
		retrySleep = 5 * time.Millisecond
	)

	var (
		wg          sync.WaitGroup
		errs        = make(chan error, workers)
		retryCounts = make(chan int, workers)
	)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			retries := 0
			for {
				g := newTestGraph(primaryTenantID, le)
				err := repo.Create(ctx, g)
				if err == nil {
					retryCounts <- retries
					return
				}
				// Postgres surfaces UNIQUE violations as messages mentioning
				// "duplicate key" / "unique constraint". Anything else is fatal.
				msg := err.Error()
				if !strings.Contains(msg, "duplicate key") &&
					!strings.Contains(msg, "unique constraint") {
					errs <- err
					return
				}
				retries++
				if retries > retryCap {
					errs <- err
					return
				}
				time.Sleep(retrySleep)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	close(retryCounts)

	for err := range errs {
		t.Fatalf("worker error: %v", err)
	}

	// After all retries succeed, exactly 10 rows must exist with
	// versions 1..10, no gaps, no duplicates.
	rows, err := repo.ListByLegalEntity(ctx, primaryTenantID, le, 100, 0)
	if err != nil {
		t.Fatalf("ListByLegalEntity: %v", err)
	}
	if len(rows) != workers {
		t.Fatalf("want %d versions, got %d", workers, len(rows))
	}
	seen := map[int]struct{}{}
	for _, r := range rows {
		if _, dup := seen[r.Version]; dup {
			t.Fatalf("duplicate version %d slipped past UNIQUE constraint", r.Version)
		}
		seen[r.Version] = struct{}{}
	}
	for v := 1; v <= workers; v++ {
		if _, ok := seen[v]; !ok {
			t.Fatalf("version %d missing — gap in sequence: %v", v, seen)
		}
	}
}

// 6. Schema-per-tenant isolation: a row in tnt_a must not be visible
//    when querying via the tnt_b schema. Proves that withTenantTx's
//    SET LOCAL search_path is doing its job (ADR-0002).
func TestUBORepo_TenantIsolation(t *testing.T) {
	resetGraphs(t)
	repo := NewPostgresUBOGraphRepository(testDB)
	ctx := context.Background()

	graphA := newTestGraph(tenantAID, "le_iso")
	graphB := newTestGraph(tenantBID, "le_iso") // same LE id — different tenant
	if err := repo.Create(ctx, graphA); err != nil {
		t.Fatalf("Create A: %v", err)
	}
	if err := repo.Create(ctx, graphB); err != nil {
		t.Fatalf("Create B: %v", err)
	}

	// A is visible from A only.
	gotA, err := repo.GetByID(ctx, tenantAID, graphA.ID)
	if err != nil {
		t.Fatalf("GetByID A from tenant A: %v", err)
	}
	if gotA.ID != graphA.ID {
		t.Fatalf("wrong row from tenant A: %+v", gotA)
	}
	if _, err := repo.GetByID(ctx, tenantBID, graphA.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tenant B sees tenant A's row: err=%v", err)
	}

	// B is visible from B only.
	gotB, err := repo.GetByID(ctx, tenantBID, graphB.ID)
	if err != nil {
		t.Fatalf("GetByID B from tenant B: %v", err)
	}
	if gotB.ID != graphB.ID {
		t.Fatalf("wrong row from tenant B: %+v", gotB)
	}
	if _, err := repo.GetByID(ctx, tenantAID, graphB.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tenant A sees tenant B's row: err=%v", err)
	}

	// And GetLatestByLegalEntity must respect schema scope: same LE id in
	// both tenants returns the tenant-local row only.
	latestA, err := repo.GetLatestByLegalEntity(ctx, tenantAID, "le_iso")
	if err != nil {
		t.Fatalf("Latest A: %v", err)
	}
	latestB, err := repo.GetLatestByLegalEntity(ctx, tenantBID, "le_iso")
	if err != nil {
		t.Fatalf("Latest B: %v", err)
	}
	if latestA.ID == latestB.ID {
		t.Fatalf("Latest crossed tenants: same id %q in A and B", latestA.ID)
	}

	// Sanity: each schema physically has exactly one row.
	for _, id := range []string{tenantAID, tenantBID} {
		var count int
		if err := testDB.QueryRowContext(ctx,
			`SELECT count(*) FROM tnt_`+id+`.ubo_graphs`).Scan(&count); err != nil {
			t.Fatalf("count tnt_%s: %v", id, err)
		}
		if count != 1 {
			t.Fatalf("tnt_%s row count: got %d, want 1", id, count)
		}
	}
}
