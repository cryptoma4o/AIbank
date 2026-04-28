package repository

import "testing"

func TestTenantSchemaName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    string
		want     string
		wantErr  bool
	}{
		{"valid simple", "alfa", "tnt_alfa", false},
		{"valid with digits", "bank42", "tnt_bank42", false},
		{"valid with underscore", "alfa_bank", "tnt_alfa_bank", false},
		{"empty", "", "", true},
		{"too short", "a", "", true},
		{"starts with digit", "1alfa", "", true},
		{"uppercase rejected", "Alfa", "", true},
		{"hyphen rejected", "alfa-bank", "", true},
		{"sql injection attempt", "alfa\"; DROP SCHEMA public; --", "", true},
		{"too long", "alfa_bank_with_a_really_really_really_long_name", "", true},
		{"unicode rejected", "альфа", "", true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := tenantSchemaName(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %q", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
