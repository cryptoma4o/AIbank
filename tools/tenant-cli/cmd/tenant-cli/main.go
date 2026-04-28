// tenant-cli — administrative CLI for the AIbank tenant-service.
//
// Subcommands:
//
//	tenant-cli create          --id --name --bik --inn [--deployment-mode]
//	tenant-cli list            [--status] [--limit] [--format table|json]
//	tenant-cli get             --id [--format table|json]
//	tenant-cli config-validate --path
//	tenant-cli config-apply    --id --path [--schema-version]
//
// Global:
//
//	--tenant-service-url   default http://tenant-service:8080
//
// The CLI is intentionally thin: it delegates HTTP work to internal/client
// and JSON-Schema validation to internal/validate (which shells out to the
// canonical packages/tenant-config-schema/validate.py).
package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/aibank/platform/tools/tenant-cli/internal/client"
	"github.com/aibank/platform/tools/tenant-cli/internal/format"
	"github.com/aibank/platform/tools/tenant-cli/internal/validate"
)

const defaultURL = "http://tenant-service:8080"

func main() {
	exit := run(os.Args[1:], os.Stdout, os.Stderr)
	os.Exit(exit)
}

// run is the testable entry point.  Returns a process exit code.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		printUsage(stderr)
		return 2
	}
	cmd := args[0]
	rest := args[1:]
	switch cmd {
	case "create":
		return runCreate(rest, stdout, stderr)
	case "list":
		return runList(rest, stdout, stderr)
	case "get":
		return runGet(rest, stdout, stderr)
	case "config-validate":
		return runConfigValidate(rest, stdout, stderr)
	case "config-apply":
		return runConfigApply(rest, stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown subcommand: %q\n\n", cmd)
		printUsage(stderr)
		return 2
	}
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `tenant-cli — manage AIbank tenants via tenant-service

USAGE:
  tenant-cli create          --id --name --bik --inn [--deployment-mode saas]
  tenant-cli list            [--status active|trial|...] [--limit 50] [--format table|json]
  tenant-cli get             --id <id> [--format table|json]
  tenant-cli config-validate --path configs/tenants/<id>
  tenant-cli config-apply    --id <id> --path configs/tenants/<id>
                             [--schema-version 1.0]

GLOBAL FLAGS:
  --tenant-service-url   tenant-service base URL (default `+defaultURL+`)

ENV:
  TENANT_SERVICE_URL     overrides the default base URL
`)
}

// envURL returns $TENANT_SERVICE_URL or defaultURL.
func envURL() string {
	if v := os.Getenv("TENANT_SERVICE_URL"); v != "" {
		return v
	}
	return defaultURL
}

// runCreate handles `tenant-cli create`.
func runCreate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	fs.SetOutput(stderr)
	id := fs.String("id", "", "tenant id (required, kebab-case)")
	name := fs.String("name", "", "tenant display name (required)")
	bik := fs.String("bik", "", "BIK — 9 digits (required)")
	inn := fs.String("inn", "", "INN — 10 digits for legal entity (required)")
	mode := fs.String("deployment-mode", "saas", "saas | on_prem | hybrid")
	url := fs.String("tenant-service-url", envURL(), "tenant-service base URL")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *id == "" || *name == "" || *bik == "" || *inn == "" {
		fmt.Fprintln(stderr, "error: --id, --name, --bik, --inn are required")
		return 2
	}

	c := client.New(*url)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t, err := c.Create(ctx, client.CreateRequest{
		ID:             *id,
		Name:           *name,
		BIK:            *bik,
		INN:            *inn,
		DeploymentMode: *mode,
	})
	if err != nil {
		fmt.Fprintf(stderr, "create failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "OK: tenant %s created (status=%s)\n", t.ID, t.Status)
	fmt.Fprintf(stdout, "Next step: db-migrator tenant --tenant-id %s --service tenant-service --source <path>\n", t.ID)
	return 0
}

// runList handles `tenant-cli list`.
func runList(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	statusFlag := fs.String("status", "", "filter by status (trial|active|suspended|archived|terminated)")
	limit := fs.Int("limit", 0, "max items (0 = no limit)")
	formatFlag := fs.String("format", format.FormatTable, "table|json")
	url := fs.String("tenant-service-url", envURL(), "tenant-service base URL")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	c := client.New(*url)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tenants, err := c.List(ctx, *statusFlag, *limit)
	if err != nil {
		fmt.Fprintf(stderr, "list failed: %v\n", err)
		return 1
	}
	if err := format.Tenants(stdout, tenants, *formatFlag); err != nil {
		fmt.Fprintf(stderr, "render failed: %v\n", err)
		return 1
	}
	return 0
}

// runGet handles `tenant-cli get`.
func runGet(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	fs.SetOutput(stderr)
	id := fs.String("id", "", "tenant id (required)")
	formatFlag := fs.String("format", format.FormatTable, "table|json")
	url := fs.String("tenant-service-url", envURL(), "tenant-service base URL")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *id == "" {
		fmt.Fprintln(stderr, "error: --id is required")
		return 2
	}
	c := client.New(*url)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	t, err := c.Get(ctx, *id)
	if err != nil {
		fmt.Fprintf(stderr, "get failed: %v\n", err)
		return 1
	}
	if err := format.SingleTenant(stdout, t, *formatFlag); err != nil {
		fmt.Fprintf(stderr, "render failed: %v\n", err)
		return 1
	}
	return 0
}

// runConfigValidate handles `tenant-cli config-validate`.
func runConfigValidate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("config-validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	path := fs.String("path", "", "path to a tenant config directory (required)")
	python := fs.String("python", "", "python interpreter (default python3)")
	verbose := fs.Bool("verbose", false, "verbose output (passes --verbose to validate.py)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *path == "" {
		fmt.Fprintln(stderr, "error: --path is required")
		return 2
	}
	abs, err := filepath.Abs(*path)
	if err != nil {
		fmt.Fprintf(stderr, "abs path: %v\n", err)
		return 2
	}
	validator, err := validate.FindValidator(abs)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}
	res, err := validate.Run(*python, validator, abs, *verbose)
	if err != nil {
		fmt.Fprintf(stderr, "validate: %v\n", err)
		return 1
	}
	if res.Stdout != "" {
		fmt.Fprint(stdout, res.Stdout)
	}
	if res.Stderr != "" {
		fmt.Fprint(stderr, res.Stderr)
	}
	if !res.Ok() {
		return res.ExitCode
	}
	return 0
}

// runConfigApply handles `tenant-cli config-apply`.
//
// It reads tenant.yaml from --path (treated as the canonical entry-point
// document), gzip-compresses the bytes, and PUTs them to
// /v1/tenants/{id}/config along with a schema_version.  The handler
// stores raw_config as bytea — base64 encoding happens automatically as
// part of JSON marshaling of `[]byte`.
func runConfigApply(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("config-apply", flag.ContinueOnError)
	fs.SetOutput(stderr)
	id := fs.String("id", "", "tenant id (required)")
	path := fs.String("path", "", "path to tenant config directory (required)")
	schemaVersion := fs.String("schema-version", "1.0", "config schema version")
	url := fs.String("tenant-service-url", envURL(), "tenant-service base URL")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *id == "" || *path == "" {
		fmt.Fprintln(stderr, "error: --id and --path are required")
		return 2
	}
	tenantYAML := filepath.Join(*path, "tenant.yaml")
	raw, err := os.ReadFile(tenantYAML)
	if err != nil {
		fmt.Fprintf(stderr, "read %s: %v\n", tenantYAML, err)
		return 1
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(raw); err != nil {
		fmt.Fprintf(stderr, "gzip: %v\n", err)
		return 1
	}
	if err := gz.Close(); err != nil {
		fmt.Fprintf(stderr, "gzip close: %v\n", err)
		return 1
	}

	c := client.New(*url)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.UpdateConfig(ctx, *id, client.UpdateConfigRequest{
		RawConfig:     buf.Bytes(),
		SchemaVersion: *schemaVersion,
	}); err != nil {
		fmt.Fprintf(stderr, "config-apply failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "OK: config applied to %s (schema_version=%s, %d bytes gzipped)\n",
		*id, *schemaVersion, buf.Len())
	return 0
}
