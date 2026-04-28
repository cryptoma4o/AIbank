// Package codegen contains a single tiny Go program that imports the
// generated AIbank proto package as a smoke test: if the import path or
// module declaration drifts, `go build ./packages/proto/codegen/...` fails
// loudly in CI.
//
// This file deliberately does NOT wire the generated stubs into the
// abs-connector service — that is a Phase 2 migration per ADR-0006. The only
// goal here is to prove that the generated Go module links cleanly and is
// reachable from the rest of the monorepo via `go.work`.
//
// Build tag `protocodegen_smoke` keeps the file out of normal `go build` /
// `go vet` walks of the surrounding tree; CI flips the tag to exercise it.
//go:build protocodegen_smoke

package codegen

import (
	// Underscore-import: we don't reference any symbol because, until
	// `buf generate` runs, the generated package only contains the doc.go
	// placeholder. Once codegen runs, this same import resolves to the full
	// surface (tenant.TenantServiceClient, identity.IdentityServiceClient,
	// etc.).
	_ "github.com/aibank/platform/packages/proto/generated/go"
)

// SmokeImport is a no-op exported to silence "imported and not used"
// complaints in tools that don't honour the underscore-import convention.
func SmokeImport() {}
