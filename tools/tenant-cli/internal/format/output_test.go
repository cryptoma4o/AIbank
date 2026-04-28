package format

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/aibank/platform/tools/tenant-cli/internal/client"
)

func TestTenants_Table(t *testing.T) {
	tenants := []client.Tenant{
		{ID: "demo", Name: "Demo Bank", BIK: "044525974", INN: "7700000000",
			Status: "trial", DeploymentMode: "saas",
			CreatedAt: time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)},
	}
	var buf bytes.Buffer
	if err := Tenants(&buf, tenants, FormatTable); err != nil {
		t.Fatalf("Tenants: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"ID", "NAME", "demo", "Demo Bank", "044525974", "trial", "saas", "2026-04-27"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q in:\n%s", want, out)
		}
	}
}

func TestTenants_JSON(t *testing.T) {
	tenants := []client.Tenant{{ID: "x", Status: "active"}}
	var buf bytes.Buffer
	if err := Tenants(&buf, tenants, FormatJSON); err != nil {
		t.Fatalf("Tenants: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"id": "x"`) || !strings.Contains(out, `"status": "active"`) {
		t.Errorf("json output unexpected: %s", out)
	}
}

func TestSingleTenant_Table(t *testing.T) {
	tenant := &client.Tenant{ID: "demo", Name: "Demo", BIK: "044525974",
		INN: "7700000000", Status: "trial", DeploymentMode: "saas"}
	var buf bytes.Buffer
	if err := SingleTenant(&buf, tenant, FormatTable); err != nil {
		t.Fatalf("SingleTenant: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"id", "demo", "deployment_mode", "saas"} {
		if !strings.Contains(out, want) {
			t.Errorf("kv missing %q in:\n%s", want, out)
		}
	}
}

func TestTenants_EmptyTable(t *testing.T) {
	var buf bytes.Buffer
	if err := Tenants(&buf, nil, FormatTable); err != nil {
		t.Fatalf("Tenants: %v", err)
	}
	if !strings.Contains(buf.String(), "(no tenants)") {
		t.Errorf("expected empty marker, got: %s", buf.String())
	}
}

func TestTenants_UnknownFormat(t *testing.T) {
	var buf bytes.Buffer
	if err := Tenants(&buf, nil, "csv"); err == nil {
		t.Fatal("expected error for unknown format")
	}
}
