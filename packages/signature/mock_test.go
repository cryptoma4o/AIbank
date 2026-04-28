package signature

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMockSignatureProvider_HappyPath(t *testing.T) {
	t.Parallel()
	m := NewMockSignatureProvider()
	ctx := context.Background()

	signed, err := m.Sign(ctx, []byte("hello"), "mock-cert-001")
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if signed.HashAlgorithm != "sha256" {
		t.Errorf("HashAlgorithm = %q", signed.HashAlgorithm)
	}
	if signed.Provider != "mock" {
		t.Errorf("Provider = %q", signed.Provider)
	}
	if len(signed.Signature) != 32 { // SHA-256 = 32 bytes
		t.Errorf("Signature len = %d, want 32", len(signed.Signature))
	}
	if signed.PayloadHash == "" || len(signed.PayloadHash) != 64 {
		t.Errorf("PayloadHash format wrong: %q", signed.PayloadHash)
	}

	// Verify должен сказать valid для свежей подписи.
	res, err := m.Verify(ctx, signed)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Valid {
		t.Errorf("Verify.Valid = false, want true (cert valid for 1 year)")
	}
	if res.IsExpired {
		t.Errorf("IsExpired = true, want false")
	}
	if res.SignerName == "" {
		t.Errorf("SignerName empty")
	}
}

func TestMockSignatureProvider_Deterministic(t *testing.T) {
	t.Parallel()
	m := NewMockSignatureProvider()
	m.SetFixedNow(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	ctx := context.Background()

	s1, _ := m.Sign(ctx, []byte("payload"), "mock-cert-001")
	s2, _ := m.Sign(ctx, []byte("payload"), "mock-cert-001")

	if s1.PayloadHash != s2.PayloadHash {
		t.Errorf("PayloadHash mismatch — Mock должен быть детерминированный")
	}
	if !s1.SignedAt.Equal(s2.SignedAt) {
		t.Errorf("SignedAt mismatch при fixed now")
	}
}

func TestMockSignatureProvider_DifferentPayloadDifferentSignature(t *testing.T) {
	t.Parallel()
	m := NewMockSignatureProvider()
	ctx := context.Background()
	s1, _ := m.Sign(ctx, []byte("payload-A"), "mock-cert-001")
	s2, _ := m.Sign(ctx, []byte("payload-B"), "mock-cert-001")
	if s1.PayloadHash == s2.PayloadHash {
		t.Errorf("разные payloads должны давать разные хеши")
	}
}

func TestMockSignatureProvider_CertNotFound(t *testing.T) {
	t.Parallel()
	m := NewMockSignatureProvider()
	_, err := m.Sign(context.Background(), []byte("x"), "nonexistent-cert")
	if !errors.Is(err, ErrCertificateNotFound) {
		t.Errorf("err = %v, want ErrCertificateNotFound", err)
	}
}

func TestMockSignatureProvider_ExpiredCert(t *testing.T) {
	t.Parallel()
	m := NewMockSignatureProvider()
	expired := &Certificate{
		ID:           "expired-cert",
		Subject:      "CN=Expired,O=Test",
		ValidFrom:    time.Now().AddDate(-2, 0, 0),
		ValidTo:      time.Now().AddDate(-1, 0, 0), // истёк год назад
		AlgorithmOID: "1.2.643.7.1.1.1.1",
		IsExpired:    true,
	}
	m.AddCertificate(expired)

	_, err := m.Sign(context.Background(), []byte("x"), "expired-cert")
	if !errors.Is(err, ErrCertificateExpired) {
		t.Errorf("Sign: err = %v, want ErrCertificateExpired", err)
	}
}

func TestMockSignatureProvider_ListAndGet(t *testing.T) {
	t.Parallel()
	m := NewMockSignatureProvider()
	m.AddCertificate(&Certificate{
		ID: "second-cert", Subject: "CN=Second", ValidTo: time.Now().AddDate(1, 0, 0),
	})

	list, err := m.ListCertificates(context.Background())
	if err != nil {
		t.Fatalf("ListCertificates: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("count = %d, want 2 (default + second)", len(list))
	}

	c, err := m.GetCertificate(context.Background(), "second-cert")
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if c.Subject != "CN=Second" {
		t.Errorf("Subject = %q", c.Subject)
	}

	// Mutating returned copy не должно затрагивать internal state.
	c.Subject = "MUTATED"
	c2, _ := m.GetCertificate(context.Background(), "second-cert")
	if c2.Subject == "MUTATED" {
		t.Errorf("internal state mutable — нужны копии")
	}
}

func TestMockSignatureProvider_VerifyExpired(t *testing.T) {
	t.Parallel()
	m := NewMockSignatureProvider()
	ctx := context.Background()

	// Подписываем сейчас (сертификат валиден).
	signed, err := m.Sign(ctx, []byte("test"), "mock-cert-001")
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	// Прокручиваем время вперёд — сертификат истёк.
	m.SetFixedNow(time.Now().AddDate(2, 0, 0))

	res, err := m.Verify(ctx, signed)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.Valid {
		t.Errorf("Valid = true, want false (cert expired)")
	}
	if !res.IsExpired {
		t.Errorf("IsExpired = false, want true")
	}
	if res.ErrorMessage == "" {
		t.Errorf("ErrorMessage empty")
	}
}

func TestMockSignatureProvider_VerifyNilOrEmpty(t *testing.T) {
	t.Parallel()
	m := NewMockSignatureProvider()
	ctx := context.Background()

	res, _ := m.Verify(ctx, nil)
	if res.Valid {
		t.Errorf("nil signed should not be valid")
	}

	res, _ = m.Verify(ctx, &SignedPayload{})
	if res.Valid {
		t.Errorf("empty signed should not be valid")
	}
}

func TestMockSignatureProvider_Validation(t *testing.T) {
	t.Parallel()
	m := NewMockSignatureProvider()
	ctx := context.Background()

	if _, err := m.Sign(ctx, []byte("x"), ""); err == nil {
		t.Errorf("expected error on empty certID")
	}
	if _, err := m.Sign(ctx, nil, "mock-cert-001"); err == nil {
		t.Errorf("expected error on empty payload")
	}
}

// Compile-time check: MockSignatureProvider реализует SignatureProvider.
var _ SignatureProvider = (*MockSignatureProvider)(nil)
