//go:build integration

// Package repository: shared test helpers for integration tests.
//
// These helpers boot a real PostgreSQL container via testcontainers-go,
// apply *.sql migrations from a directory (parsing goose-style sections),
// and provide cleanup utilities. All test files in this package that use
// the helpers MUST carry the `//go:build integration` tag — otherwise
// `go test ./...` (without `-tags=integration`) will try to spin up
// Docker for every package, which is undesirable for fast unit-test runs.
//
// Run integration tests:
//
//	go test -tags=integration -v ./internal/repository/...
//
// Requires: a running Docker daemon on the host.
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// startPostgres boots a postgres:16-alpine container, returns the connected
// *sql.DB and a teardown closure. Returns errors instead of calling Fatal so
// it can be used from TestMain (where *testing.T is unavailable).
func startPostgres() (*sql.DB, func(), error) {
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("aibank_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("start postgres container: %w", err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, nil, fmt.Errorf("connection string: %w", err)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, nil, fmt.Errorf("sql.Open: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for {
		if err := db.PingContext(pingCtx); err == nil {
			break
		}
		select {
		case <-pingCtx.Done():
			_ = db.Close()
			_ = container.Terminate(ctx)
			return nil, nil, fmt.Errorf("ping postgres: %w", pingCtx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}

	_ = os.Setenv("DATABASE_URL", dsn)

	teardown := func() {
		_ = db.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = container.Terminate(shutdownCtx)
	}
	return db, teardown, nil
}

// applyMigrationsFromDir reads *.sql files under `dir` (sorted lexically),
// parses the goose `-- +goose Up` block from each, and executes the SQL.
// Down sections are ignored.
func applyMigrationsFromDir(db *sql.DB, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", dir, err)
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, f := range files {
		path := filepath.Join(dir, f)
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		upSQL := extractGooseUp(string(raw))
		if strings.TrimSpace(upSQL) == "" {
			return fmt.Errorf("migration %s has no `+goose Up` content", f)
		}
		if _, err := db.Exec(upSQL); err != nil {
			return fmt.Errorf("apply migration %s: %w", f, err)
		}
	}
	return nil
}

// extractGooseUp returns the SQL between `-- +goose Up` and the optional
// `-- +goose Down` marker. If neither marker is present the whole content
// is returned (treated as a plain SQL file).
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

// truncateTable wipes a table for inter-test isolation.
func truncateTable(t *testing.T, db *sql.DB, qualifiedTable string) {
	t.Helper()
	if _, err := db.Exec(fmt.Sprintf(`TRUNCATE TABLE %s RESTART IDENTITY CASCADE`, qualifiedTable)); err != nil {
		t.Fatalf("truncate %s: %v", qualifiedTable, err)
	}
}
