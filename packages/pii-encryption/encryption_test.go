package piiencryption

import (
	"context"
	"strings"
	"testing"
)

// Контрактные тесты MockPIIEncryptor — фиксируют поведение, которое должны
// соблюдать все реализации PIIEncryptor.

func TestMock_RoundTrip_AllFields(t *testing.T) {
	enc := NewMockPIIEncryptor()
	ctx := context.Background()

	cases := []struct {
		field     FieldType
		plaintext string
	}{
		{FieldPassportNumber, "1234 567890"},
		{FieldPassportIssuer, "ОВД Замоскворецкого района г. Москвы"},
		{FieldSNILS, "123-456-789 01"},
		{FieldPersonalINN, "123456789012"},
		{FieldBankAccount, "40817810099910004312"},
		{FieldPhone, "+7 (916) 123-45-67"},
	}

	for _, tc := range cases {
		t.Run(string(tc.field), func(t *testing.T) {
			ct, err := enc.Encrypt(ctx, tc.field, tc.plaintext)
			if err != nil {
				t.Fatalf("Encrypt(%s): %v", tc.field, err)
			}
			if ct == tc.plaintext {
				t.Fatalf("ciphertext == plaintext (XOR не применился)")
			}
			if !strings.HasPrefix(ct, mockPrefix) {
				t.Fatalf("expected mock prefix, got %q", ct)
			}

			pt, err := enc.Decrypt(ctx, tc.field, ct)
			if err != nil {
				t.Fatalf("Decrypt(%s): %v", tc.field, err)
			}
			if pt != tc.plaintext {
				t.Fatalf("round-trip mismatch: got %q, want %q", pt, tc.plaintext)
			}
		})
	}
}

func TestMock_Deterministic(t *testing.T) {
	enc := NewMockPIIEncryptor()
	ctx := context.Background()

	ct1, err := enc.Encrypt(ctx, FieldSNILS, "123-456-789 01")
	if err != nil {
		t.Fatalf("Encrypt 1: %v", err)
	}
	ct2, err := enc.Encrypt(ctx, FieldSNILS, "123-456-789 01")
	if err != nil {
		t.Fatalf("Encrypt 2: %v", err)
	}
	if ct1 != ct2 {
		t.Fatalf("mock encrypt non-deterministic: %q != %q", ct1, ct2)
	}
}

func TestMock_PerFieldIsolation(t *testing.T) {
	// Один и тот же plaintext под разными FieldType должен давать разные
	// ciphertext'ы (per-field key isolation).
	enc := NewMockPIIEncryptor()
	ctx := context.Background()

	plain := "1234567890"
	ctSnils, _ := enc.Encrypt(ctx, FieldSNILS, plain)
	ctInn, _ := enc.Encrypt(ctx, FieldPersonalINN, plain)
	ctPhone, _ := enc.Encrypt(ctx, FieldPhone, plain)

	if ctSnils == ctInn || ctSnils == ctPhone || ctInn == ctPhone {
		t.Fatalf("per-field isolation нарушено: snils=%q inn=%q phone=%q", ctSnils, ctInn, ctPhone)
	}
}

func TestMock_EmptyPlaintext_NoCall(t *testing.T) {
	enc := NewMockPIIEncryptor()
	ctx := context.Background()

	ct, err := enc.Encrypt(ctx, FieldSNILS, "")
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}
	if ct != "" {
		t.Fatalf("empty plaintext должен возвращать пустой ciphertext, got %q", ct)
	}
	if enc.CallCount() != 0 {
		t.Fatalf("empty plaintext не должен инкрементить CallCount, got %d", enc.CallCount())
	}

	pt, err := enc.Decrypt(ctx, FieldSNILS, "")
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}
	if pt != "" {
		t.Fatalf("empty ciphertext → empty plaintext, got %q", pt)
	}
}

func TestMock_UnknownField_Rejected(t *testing.T) {
	enc := NewMockPIIEncryptor()
	ctx := context.Background()

	if _, err := enc.Encrypt(ctx, FieldType("unknown-field"), "x"); err == nil {
		t.Fatalf("Encrypt с неизвестным FieldType должен отказывать")
	}
	if _, err := enc.Decrypt(ctx, FieldType("legal-inn"), "vault:v1:mock-AAA="); err == nil {
		t.Fatalf("Decrypt с не-whitelist FieldType должен отказывать (юрлицо ИНН не шифруется)")
	}
	if _, err := enc.EncryptHash(ctx, FieldType(""), "x"); err == nil {
		t.Fatalf("EncryptHash с пустым FieldType должен отказывать")
	}
}

func TestMock_EncryptHash_Deterministic(t *testing.T) {
	enc := NewMockPIIEncryptor()
	ctx := context.Background()

	h1, err := enc.EncryptHash(ctx, FieldSNILS, "123-456-789 01")
	if err != nil {
		t.Fatalf("EncryptHash 1: %v", err)
	}
	h2, err := enc.EncryptHash(ctx, FieldSNILS, "123-456-789 01")
	if err != nil {
		t.Fatalf("EncryptHash 2: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("EncryptHash не детерминирован: %q != %q", h1, h2)
	}
	if !strings.HasPrefix(h1, "h1:") {
		t.Fatalf("expected h1: prefix for forward-compat, got %q", h1)
	}
	// h1: + 64 hex chars (HMAC-SHA256 = 32 bytes = 64 hex).
	if len(h1) != 3+64 {
		t.Fatalf("expected length %d, got %d (%q)", 3+64, len(h1), h1)
	}
}

func TestMock_EncryptHash_PerFieldIsolation(t *testing.T) {
	// Hash одного и того же plaintext под разными FieldType различен.
	enc := NewMockPIIEncryptor()
	ctx := context.Background()

	plain := "123456789012"
	hSnils, _ := enc.EncryptHash(ctx, FieldSNILS, plain)
	hInn, _ := enc.EncryptHash(ctx, FieldPersonalINN, plain)
	if hSnils == hInn {
		t.Fatalf("hash коллизия между FieldType: %q", hSnils)
	}
}

func TestMock_EncryptHash_DifferentPlaintexts(t *testing.T) {
	enc := NewMockPIIEncryptor()
	ctx := context.Background()

	h1, _ := enc.EncryptHash(context.Background(), FieldSNILS, "111-111-111 11")
	h2, _ := enc.EncryptHash(ctx, FieldSNILS, "222-222-222 22")
	if h1 == h2 {
		t.Fatalf("разные plaintext'ы дали одинаковый hash: %q", h1)
	}
}

func TestSupportedFields_Whitelist(t *testing.T) {
	want := map[FieldType]bool{
		FieldPassportNumber: true,
		FieldPassportIssuer: true,
		FieldSNILS:          true,
		FieldPersonalINN:    true,
		FieldBankAccount:    true,
		FieldPhone:          true,
	}
	got := SupportedFields()
	if len(got) != len(want) {
		t.Fatalf("SupportedFields len mismatch: got %d, want %d", len(got), len(want))
	}
	for _, f := range got {
		if !want[f] {
			t.Errorf("unexpected field in whitelist: %q", f)
		}
		if !IsSupported(f) {
			t.Errorf("IsSupported(%q) = false, expected true", f)
		}
	}

	// Юрлицо ИНН (10 цифр, НЕ шифруется) не должно быть в whitelist'е.
	if IsSupported(FieldType("legal-inn")) {
		t.Errorf("legal-inn не должен шифроваться (публичный идентификатор)")
	}
}

func TestKeyName_Format(t *testing.T) {
	cases := map[FieldType]string{
		FieldPassportNumber: "pii-passport-number",
		FieldSNILS:          "pii-snils",
		FieldPersonalINN:    "pii-personal-inn",
		FieldPhone:          "pii-phone",
	}
	for f, want := range cases {
		if got := KeyName(f); got != want {
			t.Errorf("KeyName(%q) = %q; want %q", f, got, want)
		}
	}
}
