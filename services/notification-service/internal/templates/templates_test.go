package templates

import (
	"errors"
	"strings"
	"testing"
)

func TestRegistry_AllBuiltinsRender(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	cases := map[string]map[string]string{
		"applicant_welcome": {
			"full_name": "Иван Иванов", "bank_name": "Альфа-Банк",
		},
		"application_approved": {
			"full_name": "Иван Иванов", "bank_name": "Альфа-Банк",
			"account_number": "40702810100000123456",
		},
		"application_declined": {
			"full_name": "Иван Иванов", "bank_name": "Альфа-Банк",
			"reason": "не пройдена верификация",
		},
		"document_request": {
			"full_name": "Иван Иванов", "bank_name": "Альфа-Банк",
			"document_type": "Устав",
		},
		"otp": {
			"code": "1234", "ttl_minutes": "5",
		},
	}
	for id, vars := range cases {
		out, err := r.Render(id, vars)
		if err != nil {
			t.Errorf("%s: render: %v", id, err)
			continue
		}
		if out.Subject == "" || out.Body == "" {
			t.Errorf("%s: empty subject/body", id)
		}
	}
}

func TestRegistry_RenderSubstitutes(t *testing.T) {
	r, _ := NewRegistry()
	out, err := r.Render("applicant_welcome", map[string]string{
		"full_name": "Иван Иванов", "bank_name": "Альфа-Банк",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out.Subject, "Иван Иванов") {
		t.Errorf("subject missing name: %q", out.Subject)
	}
	if !strings.Contains(out.Body, "Альфа-Банк") {
		t.Errorf("body missing bank: %q", out.Body)
	}
}

func TestRegistry_MissingVarErrors(t *testing.T) {
	r, _ := NewRegistry()
	_, err := r.Render("applicant_welcome", map[string]string{"bank_name": "X"})
	if err == nil {
		t.Fatal("expected error when full_name missing")
	}
}

func TestRegistry_UnknownTemplate(t *testing.T) {
	r, _ := NewRegistry()
	_, err := r.Render("does_not_exist", nil)
	if !errors.Is(err, ErrUnknownTemplate) {
		t.Fatalf("expected ErrUnknownTemplate, got %v", err)
	}
}

func TestRegistry_IDsContainsBuiltins(t *testing.T) {
	r, _ := NewRegistry()
	ids := r.IDs()
	want := []string{"applicant_welcome", "application_approved", "application_declined", "document_request", "otp"}
	got := make(map[string]bool, len(ids))
	for _, id := range ids {
		got[id] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing built-in template %s", w)
		}
	}
}
