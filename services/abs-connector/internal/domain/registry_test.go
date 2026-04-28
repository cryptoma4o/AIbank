package domain

import (
	"errors"
	"testing"
)

func TestAdapterRegistry_RegisterAndLookup(t *testing.T) {
	t.Parallel()

	r := NewAdapterRegistry()
	if err := r.Register(AdapterEntry{
		TenantID:    "bank-alpha",
		AdapterName: "cft",
		URL:         "http://abs-adapter-cft:9001",
		Version:     "1.4.2",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := r.Lookup("bank-alpha")
	if err != nil {
		t.Fatalf("Lookup hit: %v", err)
	}
	if got.AdapterName != "cft" || got.Version != "1.4.2" {
		t.Fatalf("unexpected entry: %+v", got)
	}
}

func TestAdapterRegistry_LookupMiss(t *testing.T) {
	t.Parallel()

	r := NewAdapterRegistry()
	_, err := r.Lookup("unknown-tenant")
	if !errors.Is(err, ErrTenantNotConfigured) {
		t.Fatalf("expected ErrTenantNotConfigured, got %v", err)
	}
}

func TestAdapterRegistry_RegisterRejectsInvalid(t *testing.T) {
	t.Parallel()

	r := NewAdapterRegistry()

	cases := []struct {
		name  string
		entry AdapterEntry
	}{
		{"missing tenant_id", AdapterEntry{AdapterName: "cft", URL: "http://x", Version: "1"}},
		{"invalid tenant_id", AdapterEntry{TenantID: "Bank Alpha", AdapterName: "cft", URL: "http://x", Version: "1"}},
		{"missing url", AdapterEntry{TenantID: "alpha", AdapterName: "cft", Version: "1"}},
		{"missing version", AdapterEntry{TenantID: "alpha", AdapterName: "cft", URL: "http://x"}},
		{"missing adapter_name", AdapterEntry{TenantID: "alpha", URL: "http://x", Version: "1"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := r.Register(tc.entry); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestAdapterRegistry_LoadFromYAML(t *testing.T) {
	t.Parallel()

	yaml := []byte(`
# leading comment
adapters:
  - tenant_id: bank-alpha
    adapter_name: cft
    url: http://abs-adapter-cft:9001
    version: "1.4.2"
  - tenant_id: demo
    adapter_name: cft
    url: memory:mock
    version: "0.0.0"
`)
	r := NewAdapterRegistry()
	if err := r.LoadFromYAML(yaml); err != nil {
		t.Fatalf("LoadFromYAML: %v", err)
	}

	alpha, err := r.Lookup("bank-alpha")
	if err != nil {
		t.Fatalf("Lookup bank-alpha: %v", err)
	}
	if alpha.URL != "http://abs-adapter-cft:9001" || alpha.Version != "1.4.2" {
		t.Fatalf("alpha entry: %+v", alpha)
	}

	demo, err := r.Lookup("demo")
	if err != nil {
		t.Fatalf("Lookup demo: %v", err)
	}
	if demo.URL != MockAdapterURL {
		t.Fatalf("demo url: %s", demo.URL)
	}
	if got := len(r.All()); got != 2 {
		t.Fatalf("expected 2 entries, got %d", got)
	}
}

func TestAdapterRegistry_LoadFromEnvOverridesYAML(t *testing.T) {
	t.Parallel()

	r := NewAdapterRegistry()
	if err := r.LoadFromYAML([]byte(`
adapters:
  - tenant_id: bank-alpha
    adapter_name: cft
    url: http://old:9001
    version: "1.0.0"
`)); err != nil {
		t.Fatalf("LoadFromYAML: %v", err)
	}

	env := []string{
		"PATH=/usr/bin",
		"ABS_ADAPTER_BANK-ALPHA=cft,http://new:9001,2.0.0", // dash в env-key — невалидно для bash, скипнут
		"ABS_ADAPTER_GAMMA=diasoft,http://abs-adapter-diasoft:9001,0.9.5",
	}
	if err := r.LoadFromEnv(env); err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}

	gamma, err := r.Lookup("gamma")
	if err != nil {
		t.Fatalf("Lookup gamma: %v", err)
	}
	if gamma.AdapterName != "diasoft" || gamma.Version != "0.9.5" {
		t.Fatalf("gamma entry: %+v", gamma)
	}
}

func TestAdapterRegistry_LoadFromEnvBadValueErrors(t *testing.T) {
	t.Parallel()

	r := NewAdapterRegistry()
	err := r.LoadFromEnv([]string{
		"ABS_ADAPTER_ALPHA=missing-comma-value",
	})
	if err == nil {
		t.Fatal("expected error on malformed env value")
	}
}

func TestValidateTenantID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want bool
	}{
		{"alpha", true},
		{"bank-alpha", true},
		{"bank_alpha", true},
		{"alpha42", true},
		{"", false},
		{"Alpha", false},
		{"bank alpha", false},
		{"-alpha", false},
		{"alpha;DROP", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			err := ValidateTenantID(tc.in)
			if tc.want && err != nil {
				t.Fatalf("expected ok for %q, got %v", tc.in, err)
			}
			if !tc.want && err == nil {
				t.Fatalf("expected err for %q, got nil", tc.in)
			}
		})
	}
}
