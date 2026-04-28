//go:build integration

// Integration tests for identity-service repository layer.
//
// Boots a real PostgreSQL 16 container, provisions:
//   - schema `platform` with a minimal `platform.tenants` table (FK target
//     of platform.users.tenant_id) — the real tenant table lives in
//     tenant-service migrations, so we recreate the bare minimum here.
//   - schema `platform.users` from migrations/001_create_users.sql
//   - schema `tnt_test` with applicants table from migrations/tenant/*.sql
//
// Then exercises:
//   - PostgresUserRepository: Create + GetByEmail + GetByID round trip
//   - PostgresApplicantRepository: Create + GetByID + GetByINN with
//     search_path set inside the connection's transaction
//   - validTenantID rejection of garbage tenant identifiers
//
// Run with:
//
//	go test -tags=integration -v ./internal/repository/...
package repository

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"path/filepath"
	"testing"

	"aibank/identity-service/internal/domain"
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

	// Bootstrap a minimal platform schema so users.tenant_id FK can resolve.
	// In production this DDL lives in tenant-service migrations; replicating
	// the FK target only — the test does not depend on the real shape.
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

	// Apply identity migrations targeting the platform schema.
	if err := applyMigrationsFromDir(db, filepath.Join("..", "..", "migrations")); err != nil {
		log.Fatalf("apply platform migrations: %v", err)
	}

	// Provision the per-tenant schema for applicants table.
	tenantSchemaName := "tnt_" + testTenantID
	if _, err := db.Exec("CREATE SCHEMA IF NOT EXISTS " + tenantSchemaName); err != nil {
		log.Fatalf("create tenant schema: %v", err)
	}
	if _, err := db.Exec("SET search_path TO " + tenantSchemaName + ", public"); err != nil {
		log.Fatalf("set search_path: %v", err)
	}
	if err := applyMigrationsFromDir(db, filepath.Join("..", "..", "migrations", "tenant")); err != nil {
		log.Fatalf("apply tenant migrations: %v", err)
	}
	// Reset the connection's search_path so test code uses the postgres-default.
	if _, err := db.Exec("SET search_path TO public"); err != nil {
		log.Fatalf("reset search_path: %v", err)
	}

	testDB = db
	os.Exit(m.Run())
}

func resetUsers(t *testing.T) {
	t.Helper()
	if _, err := testDB.Exec(`TRUNCATE TABLE platform.users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate users: %v", err)
	}
}

func resetApplicants(t *testing.T) {
	t.Helper()
	if _, err := testDB.Exec(`TRUNCATE TABLE tnt_test.applicants RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate applicants: %v", err)
	}
}

// --- Users (platform schema) --------------------------------------------------

func TestPostgresUserRepository_CreateAndLookup(t *testing.T) {
	resetUsers(t)
	repo := NewPostgresUserRepository(testDB)
	ctx := context.Background()

	tenantID := testTenantID
	user := &domain.User{
		ID:           uuid.NewString(),
		TenantID:     &tenantID,
		Email:        "operator@bank.test",
		PasswordHash: "$argon2id$v=19$m=65536,t=3,p=4$dGVzdHRlc3Q$abcdef",
		Role:         domain.RoleBankOperator,
		IsActive:     true,
	}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	byEmail, err := repo.GetByEmail(ctx, "operator@bank.test")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if byEmail.ID != user.ID || byEmail.Role != user.Role || !byEmail.IsActive {
		t.Fatalf("GetByEmail mismatch: %+v", byEmail)
	}
	if byEmail.TenantID == nil || *byEmail.TenantID != testTenantID {
		t.Fatalf("tenant_id round-trip failed: %v", byEmail.TenantID)
	}

	byID, err := repo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if byID.Email != user.Email {
		t.Fatalf("GetByID email mismatch: %q vs %q", byID.Email, user.Email)
	}
}

func TestPostgresUserRepository_PlatformAdmin_NoTenant(t *testing.T) {
	resetUsers(t)
	repo := NewPostgresUserRepository(testDB)
	ctx := context.Background()

	admin := &domain.User{
		ID:           uuid.NewString(),
		TenantID:     nil,
		Email:        "admin@platform.test",
		PasswordHash: "x",
		Role:         domain.RolePlatformAdmin,
		IsActive:     true,
	}
	if err := repo.Create(ctx, admin); err != nil {
		t.Fatalf("Create platform admin: %v", err)
	}
	got, err := repo.GetByEmail(ctx, "admin@platform.test")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if got.TenantID != nil {
		t.Fatalf("platform admin must have nil tenant_id, got %v", got.TenantID)
	}
}

func TestPostgresUserRepository_GetByEmail_NotFound(t *testing.T) {
	resetUsers(t)
	repo := NewPostgresUserRepository(testDB)
	_, err := repo.GetByEmail(context.Background(), "ghost@test")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// --- Applicants (tenant schema via search_path) -------------------------------

func TestPostgresApplicantRepository_CreateAndGet(t *testing.T) {
	resetApplicants(t)
	repo := NewPostgresApplicantRepository(testDB, nil)
	ctx := context.Background()

	a := &domain.Applicant{
		ID:           uuid.NewString(),
		TenantID:     testTenantID,
		INN:          "123456789012",
		Phone:        "+79991234567",
		FullName:     "Иванов Иван Иванович",
		ESIAVerified: true,
		ESIASubject:  "esia-sub-001",
	}
	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("Create: %v", err)
	}

	byID, err := repo.GetByID(ctx, testTenantID, a.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if byID.INN != a.INN || byID.FullName != a.FullName || !byID.ESIAVerified {
		t.Fatalf("round-trip mismatch: %+v", byID)
	}
	if byID.ESIASubject != "esia-sub-001" {
		t.Fatalf("esia_subject not preserved: got %q", byID.ESIASubject)
	}

	byINN, err := repo.GetByINN(ctx, testTenantID, a.INN)
	if err != nil {
		t.Fatalf("GetByINN: %v", err)
	}
	if byINN.ID != a.ID {
		t.Fatalf("GetByINN returned wrong row: %+v", byINN)
	}
}

func TestPostgresApplicantRepository_GetByID_NotFound(t *testing.T) {
	resetApplicants(t)
	repo := NewPostgresApplicantRepository(testDB, nil)
	_, err := repo.GetByID(context.Background(), testTenantID, "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPostgresApplicantRepository_RejectsBadTenantID(t *testing.T) {
	repo := NewPostgresApplicantRepository(testDB, nil)
	ctx := context.Background()

	bad := []string{
		"",
		"1bad",
		"BadName",
		"bad-name",
		`evil"; DROP TABLE applicants; --`,
		"кир",
	}
	for _, id := range bad {
		_, err := repo.GetByID(ctx, id, "anything")
		if err == nil {
			t.Fatalf("GetByID(%q) accepted invalid tenant id", id)
		}
		// Sanity: error must NOT be ErrNotFound — it must come from the
		// validation layer (returns "invalid tenant id" via fmt.Errorf).
		if errors.Is(err, ErrNotFound) {
			t.Fatalf("GetByID(%q) returned ErrNotFound instead of validation error", id)
		}
	}
}
