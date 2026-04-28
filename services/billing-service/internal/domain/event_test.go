package domain

import "testing"

func TestDeriveID_DeterministicSameInput(t *testing.T) {
	t.Parallel()

	a := DeriveID("bank-alpha", EventTypeAccountOpenedLLC, "app-123")
	b := DeriveID("bank-alpha", EventTypeAccountOpenedLLC, "app-123")
	if a != b {
		t.Fatalf("expected identical UUID v5 for identical input, got %q vs %q", a, b)
	}
	if a == "" {
		t.Fatalf("expected non-empty UUID")
	}
}

func TestDeriveID_DifferentSourceEvent(t *testing.T) {
	t.Parallel()

	a := DeriveID("bank-alpha", EventTypeAccountOpenedLLC, "app-123")
	b := DeriveID("bank-alpha", EventTypeAccountOpenedLLC, "app-124")
	if a == b {
		t.Fatalf("expected different UUIDs for different source_event_id, got %q == %q", a, b)
	}
}

func TestDeriveID_DifferentTenant(t *testing.T) {
	t.Parallel()

	a := DeriveID("bank-alpha", EventTypeUBOCheckExecuted, "evt-1")
	b := DeriveID("bank-beta", EventTypeUBOCheckExecuted, "evt-1")
	if a == b {
		t.Fatalf("expected different UUIDs for different tenant, got %q == %q", a, b)
	}
}

func TestDeriveID_DifferentEventType(t *testing.T) {
	t.Parallel()

	a := DeriveID("bank-alpha", EventTypeAccountOpenedIP, "evt-1")
	b := DeriveID("bank-alpha", EventTypeAccountOpenedLLC, "evt-1")
	if a == b {
		t.Fatalf("expected different UUIDs for different event_type, got %q == %q", a, b)
	}
}

func TestComputeTotal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		quantity  int
		unitPrice int64
		want      int64
	}{
		{"single unit", 1, 80000, 80000},
		{"five units", 5, 20000, 100000},
		{"zero quantity defaults to 1", 0, 50000, 50000},
		{"negative quantity defaults to 1", -3, 50000, 50000},
		{"zero price", 4, 0, 0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ComputeTotal(tc.quantity, tc.unitPrice)
			if got != tc.want {
				t.Errorf("ComputeTotal(%d, %d) = %d, want %d",
					tc.quantity, tc.unitPrice, got, tc.want)
			}
		})
	}
}
