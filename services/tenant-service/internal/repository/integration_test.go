//go:build integration

// Integration tests for tenant-service repository layer.
//
// These tests boot a real PostgreSQL 16 container via testcontainers-go,
// apply ALL platform-level migrations from ../../migrations/*.sql, and
// exercise the postgres-backed implementations of TenantRepository,
// TenantConfigRepository, and SchemaProvisioner end-to-end against the
// real database — no mocks below the SQL layer.
//
// Run with:
//
//	go test -tags=integration -v ./internal/repository/...
//
// Requires a running Docker daemon. Without `-tags=integration` this file
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
	"testing"

	"github.com/aibank/platform/services/tenant-service/internal/domain"
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

func resetTenants(t *testing.T) {
	t.Helper()
	if _, err := testDB.Exec(`TRUNCATE TABLE platform.tenant_configs, platform.tenants RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("reset tenants: %v", err)
	}
}

func newTestTenant(id string) *domain.Tenant {
	return &domain.Tenant{
		ID:             id,
		Name:           "Test Bank " + id,
		BIK:            "044525225",
		INN:            "7707083893",
		Status:         domain.TenantStatusTrial,
		DeploymentMode: domain.DeploymentModeSaaS,
	}
}

// --- TenantRepository ---------------------------------------------------------

func TestPostgresTenantRepository_CreateAndGetByID(t *testing.T) {
	resetTenants(t)
	repo := NewPostgresTenantRepository(testDB)
	ctx := context.Background()

	want := newTestTenant("alfa")
	if err := repo.Create(ctx, want); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetByID(ctx, "alfa")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != want.ID || got.Name != want.Name || got.BIK != want.BIK ||
		got.INN != want.INN || got.Status != want.Status ||
		got.DeploymentMode != want.DeploymentMode {
		t.Fatalf("round-trip mismatch:\n got=%+v\nwant=%+v", got, want)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not populated: %+v", got)
	}
}

func TestPostgresTenantRepository_GetByID_NotFound(t *testing.T) {
	resetTenants(t)
	repo := NewPostgresTenantRepository(testDB)
	_, err := repo.GetByID(context.Background(), "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPostgresTenantRepository_List_OrderedByCreatedAtDesc(t *testing.T) {
	resetTenants(t)
	repo := NewPostgresTenantRepository(testDB)
	ctx := context.Background()

	for _, id := range []string{"first", "second", "third"} {
		if err := repo.Create(ctx, newTestTenant(id)); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
	}
	got, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 tenants, got %d", len(got))
	}
	for i := 0; i < len(got)-1; i++ {
		if got[i].CreatedAt.Before(got[i+1].CreatedAt) {
			t.Fatalf("List not ordered by created_at DESC: pos %d (%v) before pos %d (%v)",
				i, got[i].CreatedAt, i+1, got[i+1].CreatedAt)
		}
	}
}

func TestPostgresTenantRepository_Update_ChangesStatus(t *testing.T) {
	resetTenants(t)
	repo := NewPostgresTenantRepository(testDB)
	ctx := context.Background()

	tn := newTestTenant("toupdate")
	if err := repo.Create(ctx, tn); err != nil {
		t.Fatalf("Create: %v", err)
	}
	tn.Status = domain.TenantStatusActive
	tn.Name = "Updated Bank"
	if err := repo.Update(ctx, tn); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := repo.GetByID(ctx, "toupdate")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != domain.TenantStatusActive {
		t.Fatalf("status not updated: got %s", got.Status)
	}
	if got.Name != "Updated Bank" {
		t.Fatalf("name not updated: got %s", got.Name)
	}
}

func TestPostgresTenantRepository_Update_NotFound(t *testing.T) {
	resetTenants(t)
	repo := NewPostgresTenantRepository(testDB)
	err := repo.Update(context.Background(), newTestTenant("ghost"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound on missing row, got %v", err)
	}
}

// --- TenantConfigRepository ---------------------------------------------------

func TestPostgresTenantConfigRepository_SaveAndGet_Idempotent(t *testing.T) {
	resetTenants(t)
	tenantRepo := NewPostgresTenantRepository(testDB)
	cfgRepo := NewPostgresTenantConfigRepository(testDB)
	ctx := context.Background()

	if err := tenantRepo.Create(ctx, newTestTenant("cfgtest")); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}

	cfg := &domain.TenantConfig{
		TenantID:      "cfgtest",
		RawConfig:     []byte(`{"theme":"dark"}`),
		SchemaVersion: "v1",
	}
	if err := cfgRepo.SaveConfig(ctx, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	cfg.SchemaVersion = "v2"
	cfg.RawConfig = []byte(`{"theme":"light"}`)
	if err := cfgRepo.SaveConfig(ctx, cfg); err != nil {
		t.Fatalf("SaveConfig (upsert): %v", err)
	}

	got, err := cfgRepo.GetConfig(ctx, "cfgtest")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if got.SchemaVersion != "v2" {
		t.Fatalf("upsert did not overwrite schema_version: got %s", got.SchemaVersion)
	}
	if string(got.RawConfig) != `{"theme":"light"}` {
		t.Fatalf("upsert did not overwrite raw_config: got %s", got.RawConfig)
	}
}

func TestPostgresTenantConfigRepository_GetConfig_NotFound(t *testing.T) {
	resetTenants(t)
	cfgRepo := NewPostgresTenantConfigRepository(testDB)
	_, err := cfgRepo.GetConfig(context.Background(), "nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// --- SchemaProvisioner --------------------------------------------------------

func TestPostgresSchemaProvisioner_CreatesSchema(t *testing.T) {
	resetTenants(t)
	prov := NewPostgresSchemaProvisioner(testDB)
	ctx := context.Background()

	tenantID := "provtest"
	exists, err := prov.SchemaExists(ctx, tenantID)
	if err != nil {
		t.Fatalf("SchemaExists pre: %v", err)
	}
	if exists {
		t.Fatalf("schema unexpectedly exists before Provision")
	}
	if err := prov.Provision(ctx, tenantID); err != nil {
		t.Fatalf("Provision: %v", err)
	}
	exists, err = prov.SchemaExists(ctx, tenantID)
	if err != nil {
		t.Fatalf("SchemaExists post: %v", err)
	}
	if !exists {
		t.Fatalf("schema tnt_%s not created", tenantID)
	}

	if err := prov.Provision(ctx, tenantID); err != nil {
		t.Fatalf("Provision (second call) must be idempotent, got: %v", err)
	}

	t.Cleanup(func() {
		_, _ = testDB.Exec(`DROP SCHEMA IF EXISTS tnt_provtest CASCADE`)
		_, _ = testDB.Exec(`DROP ROLE IF EXISTS tnt_provtest_app`)
	})
}

func TestPostgresSchemaProvisioner_RejectsBadTenantID(t *testing.T) {
	prov := NewPostgresSchemaProvisioner(testDB)
	ctx := context.Background()

	bad := []string{"", "1bad", "Bad-Bank", `evil"; DROP TABLE x;--`, "верх"}
	for _, id := range bad {
		if err := prov.Provision(ctx, id); err == nil {
			t.Fatalf("Provision(%q) accepted invalid id (SQL-injection guard breached)", id)
		}
	}
}
