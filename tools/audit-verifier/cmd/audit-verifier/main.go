// audit-verifier — CLI для проверки целостности hash-цепочки audit-events.
//
// См. docs/runbooks/audit-log-integrity.md (когда и как использовать)
// и services/audit-service/internal/domain/event.go (источник истины
// для ComputeHash, который этот tool воспроизводит локально).
//
// Команды:
//
//	audit-verifier verify --tenant-id <id> [--source api|db] [--format text|json]
//	audit-verifier latest --tenant-id <id> [--source api|db]
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	_ "github.com/lib/pq"

	"github.com/aibank/platform/tools/audit-verifier/internal/verifier"
)

const (
	defaultAuditURL = "http://audit-service:8081"
	exitOK          = 0
	exitChainBroken = 1
	exitUsage       = 2
	exitInternal    = 3
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(exitUsage)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	switch cmd := os.Args[1]; cmd {
	case "verify":
		os.Exit(runVerify(os.Args[2:], log))
	case "latest":
		os.Exit(runLatest(os.Args[2:], log))
	case "-h", "--help", "help":
		printUsage()
		os.Exit(exitOK)
	default:
		fmt.Fprintf(os.Stderr, "Неизвестная команда: %q\n\n", cmd)
		printUsage()
		os.Exit(exitUsage)
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `audit-verifier — проверка целостности hash-цепочки audit-events

ИСПОЛЬЗОВАНИЕ:
  audit-verifier verify --tenant-id <id> [флаги]
  audit-verifier latest --tenant-id <id> [флаги]

ФЛАГИ verify:
  --tenant-id     обязательный, идентификатор тенанта
  --source        api|db (по умолчанию api)
  --audit-url     URL audit-service для api-режима (по умолчанию http://audit-service:8081)
  --dsn           postgres DSN для db-режима (или переменная DATABASE_URL)
  --from          ISO8601, нижняя граница CreatedAt
  --to            ISO8601, верхняя граница CreatedAt
  --format        text|json (по умолчанию text)

ФЛАГИ latest:
  --tenant-id     обязательный
  --source        api|db (по умолчанию api)
  --audit-url, --dsn — как у verify

КОДЫ ВЫХОДА:
  0  цепочка целая
  1  обнаружен разрыв (P0 инцидент — см. runbook audit-log-integrity)
  2  ошибка использования (неверные флаги)
  3  внутренняя ошибка (БД недоступна, audit-service не отвечает и т.п.)
`)
}

// commonFlags — флаги, общие для verify/latest.
type commonFlags struct {
	tenantID string
	source   string
	auditURL string
	dsn      string
}

func bindCommon(fs *flag.FlagSet) *commonFlags {
	c := &commonFlags{}
	fs.StringVar(&c.tenantID, "tenant-id", "", "tenant ID")
	fs.StringVar(&c.source, "source", "api", "источник событий: api или db")
	fs.StringVar(&c.auditURL, "audit-url", defaultAuditURL, "URL audit-service для api-режима")
	fs.StringVar(&c.dsn, "dsn", "", "postgres DSN для db-режима (или $DATABASE_URL)")
	return c
}

// buildStore конструирует EventStore + cleanup-функцию по флагам source.
func buildStore(c *commonFlags, log *slog.Logger) (verifier.EventStore, func(), error) {
	switch c.source {
	case "api":
		log.Info("using API source", "url", c.auditURL)
		return verifier.NewAPIStore(c.auditURL), func() {}, nil
	case "db":
		dsn := c.dsn
		if dsn == "" {
			dsn = os.Getenv("DATABASE_URL")
		}
		if dsn == "" {
			return nil, nil, errors.New("db-режим требует --dsn или переменную DATABASE_URL")
		}
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			return nil, nil, fmt.Errorf("открыть БД: %w", err)
		}
		pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := db.PingContext(pingCtx); err != nil {
			db.Close()
			return nil, nil, fmt.Errorf("ping БД: %w", err)
		}
		log.Info("using DB source")
		return verifier.NewDBStore(db), func() { _ = db.Close() }, nil
	default:
		return nil, nil, fmt.Errorf("--source: ожидается api|db, получено %q", c.source)
	}
}

func runVerify(args []string, log *slog.Logger) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	c := bindCommon(fs)
	fromStr := fs.String("from", "", "ISO8601 нижняя граница (опционально)")
	toStr := fs.String("to", "", "ISO8601 верхняя граница (опционально)")
	format := fs.String("format", "text", "text|json")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if err := verifier.ValidateTenantID(c.tenantID); err != nil {
		log.Error("validate tenant-id", "err", err)
		return exitUsage
	}

	from, err := parseTime(*fromStr)
	if err != nil {
		log.Error("--from", "err", err)
		return exitUsage
	}
	to, err := parseTime(*toStr)
	if err != nil {
		log.Error("--to", "err", err)
		return exitUsage
	}

	store, cleanup, err := buildStore(c, log)
	if err != nil {
		log.Error("init store", "err", err)
		return exitInternal
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	res, err := verifier.NewVerifier(store).VerifyChain(ctx, c.tenantID, from, to)
	if err != nil {
		log.Error("verify chain", "err", err)
		return exitInternal
	}

	switch *format {
	case "json":
		_ = json.NewEncoder(os.Stdout).Encode(res)
	case "text":
		printVerifyText(res)
	default:
		log.Error("--format: ожидается text|json", "got", *format)
		return exitUsage
	}
	if !res.Valid {
		return exitChainBroken
	}
	return exitOK
}

func runLatest(args []string, log *slog.Logger) int {
	fs := flag.NewFlagSet("latest", flag.ContinueOnError)
	c := bindCommon(fs)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if err := verifier.ValidateTenantID(c.tenantID); err != nil {
		log.Error("validate tenant-id", "err", err)
		return exitUsage
	}

	store, cleanup, err := buildStore(c, log)
	if err != nil {
		log.Error("init store", "err", err)
		return exitInternal
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	hash, err := verifier.NewVerifier(store).LatestHash(ctx, c.tenantID)
	if err != nil {
		log.Error("latest hash", "err", err)
		return exitInternal
	}
	if hash == "" {
		fmt.Println("(нет событий)")
	} else {
		fmt.Println(hash)
	}
	return exitOK
}

// parseTime — RFC3339 / ISO8601-совместимый парсер. Пустая строка → nil.
func parseTime(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, fmt.Errorf("ожидается формат RFC3339 (2026-04-26T12:00:00Z): %w", err)
	}
	return &t, nil
}

// printVerifyText — Russian-language отчёт для оператора в консоли.
func printVerifyText(r verifier.VerificationResult) {
	fmt.Printf("Тенант:        %s\n", r.TenantID)
	fmt.Printf("Событий:       %d\n", r.EventCount)
	if r.FirstAt != nil {
		fmt.Printf("Первое:        %s\n", r.FirstAt.Format(time.RFC3339))
	}
	if r.LastAt != nil {
		fmt.Printf("Последнее:     %s\n", r.LastAt.Format(time.RFC3339))
	}
	if r.Valid {
		fmt.Println("Цепочка:       ЦЕЛАЯ")
		if r.LastHash != "" {
			fmt.Printf("Хэш последнего: %s\n", r.LastHash)
		}
		return
	}
	fmt.Println("Цепочка:       РАЗОРВАНА — TAMPERING ALERT (P0)")
	if r.FirstMismatch != nil {
		fmt.Printf("Первая аномалия: event_id=%s created_at=%s\n",
			r.FirstMismatch.ID, r.FirstMismatch.CreatedAt.Format(time.RFC3339))
	}
	fmt.Printf("Причина:       %s\n", r.MismatchReason)
	fmt.Printf("Ожидался hash: %s\n", r.ExpectedHash)
	fmt.Printf("Фактический:   %s\n", r.ActualHash)
	fmt.Println("Действия:      см. docs/runbooks/audit-log-integrity.md §3 INVESTIGATE — НЕ модифицировать audit_events.")
}
