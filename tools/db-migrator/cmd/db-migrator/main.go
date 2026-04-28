// db-migrator — CLI для применения SQL-миграций к схемам platform и tenant
// согласно ADR-0005.
//
// Команды:
//
//	db-migrator platform   --service <name> --source <dir>
//	db-migrator tenant     --tenant-id <id> --service <name> --source <dir>
//	db-migrator tenant-all --service <name> --source <dir>
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"

	"github.com/aibank/platform/tools/db-migrator/internal/runner"
	"github.com/aibank/platform/tools/db-migrator/internal/source"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	switch cmd {
	case "platform":
		runPlatformCmd(args, log)
	case "tenant":
		runTenantCmd(args, log)
	case "tenant-all":
		runTenantAllCmd(args, log)
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %q\n\n", cmd)
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `db-migrator — apply DB migrations per ADR-0005

USAGE:
  db-migrator platform   --service <name> --source <dir> [--dsn <url>]
  db-migrator tenant     --tenant-id <id> --service <name> --source <dir> [--dsn <url>]
  db-migrator tenant-all --service <name> --source <dir> [--dsn <url>]

GLOBAL FLAGS:
  --dsn         postgres DSN (default: $DATABASE_URL)
  --run-id      run identifier (default: manual-<timestamp>)

ENV:
  DATABASE_URL  postgres DSN (overridden by --dsn)
`)
}

func mustOpenDB(dsn string, log *slog.Logger) *sql.DB {
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		log.Error("DSN required (--dsn or DATABASE_URL)")
		os.Exit(2)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Error("open db", "err", err)
		os.Exit(1)
	}
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		log.Error("ping db", "err", err)
		os.Exit(1)
	}
	return db
}

func runPlatformCmd(args []string, log *slog.Logger) {
	fs := flag.NewFlagSet("platform", flag.ExitOnError)
	service := fs.String("service", "", "service name (for logs only)")
	srcDir := fs.String("source", "", "directory with platform migrations")
	dsn := fs.String("dsn", "", "postgres DSN")
	_ = fs.Parse(args)

	if *srcDir == "" {
		log.Error("--source is required")
		os.Exit(2)
	}

	db := mustOpenDB(*dsn, log)
	defer db.Close()

	migs, err := source.Discover(*srcDir)
	if err != nil {
		log.Error("discover migrations", "err", err)
		os.Exit(1)
	}
	log.Info("discovered platform migrations", "service", *service, "count", len(migs))

	r := runner.NewPlatformRunner(db, log)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := r.Apply(ctx, migs); err != nil {
		log.Error("apply", "err", err)
		os.Exit(1)
	}
	log.Info("platform migrations done", "service", *service)
}

func runTenantCmd(args []string, log *slog.Logger) {
	fs := flag.NewFlagSet("tenant", flag.ExitOnError)
	tenantID := fs.String("tenant-id", "", "tenant ID without tnt_ prefix (e.g. 'alfa')")
	service := fs.String("service", "", "service name (recorded in platform.tenant_migrations)")
	srcDir := fs.String("source", "", "directory with tenant migrations")
	dsn := fs.String("dsn", "", "postgres DSN")
	runID := fs.String("run-id", fmt.Sprintf("manual-%d", time.Now().Unix()),
		"run identifier (Temporal workflow ID or 'manual-...')")
	_ = fs.Parse(args)

	if *tenantID == "" || *service == "" || *srcDir == "" {
		log.Error("--tenant-id, --service, --source are required")
		os.Exit(2)
	}

	db := mustOpenDB(*dsn, log)
	defer db.Close()

	migs, err := source.Discover(*srcDir)
	if err != nil {
		log.Error("discover", "err", err)
		os.Exit(1)
	}
	log.Info("discovered tenant migrations",
		"tenant", *tenantID, "service", *service, "count", len(migs))

	r := runner.NewTenantRunner(db, log, *service, *runID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := r.Apply(ctx, *tenantID, migs); err != nil {
		log.Error("apply", "tenant", *tenantID, "err", err)
		os.Exit(1)
	}
	log.Info("tenant migrations done", "tenant", *tenantID)
}

func runTenantAllCmd(args []string, log *slog.Logger) {
	fs := flag.NewFlagSet("tenant-all", flag.ExitOnError)
	service := fs.String("service", "", "service name")
	srcDir := fs.String("source", "", "directory with tenant migrations")
	dsn := fs.String("dsn", "", "postgres DSN")
	runID := fs.String("run-id", fmt.Sprintf("manual-%d", time.Now().Unix()), "run identifier")
	stopOnError := fs.Bool("stop-on-error", false,
		"stop on first failed tenant (default: continue and report at end)")
	_ = fs.Parse(args)

	if *service == "" || *srcDir == "" {
		log.Error("--service and --source are required")
		os.Exit(2)
	}

	db := mustOpenDB(*dsn, log)
	defer db.Close()

	migs, err := source.Discover(*srcDir)
	if err != nil {
		log.Error("discover", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	tenants, err := runner.ListActiveTenants(ctx, db)
	if err != nil {
		log.Error("list tenants", "err", err)
		os.Exit(1)
	}
	log.Info("processing tenants", "count", len(tenants), "service", *service)

	r := runner.NewTenantRunner(db, log, *service, *runID)
	var failures []string
	for _, tid := range tenants {
		if err := r.Apply(ctx, tid, migs); err != nil {
			log.Error("tenant migration failed", "tenant", tid, "err", err)
			failures = append(failures, tid)
			if *stopOnError {
				break
			}
		}
	}
	if len(failures) > 0 {
		log.Error("some tenants failed", "tenants", strings.Join(failures, ","))
		os.Exit(1)
	}
	log.Info("all tenant migrations applied", "service", *service)
}
