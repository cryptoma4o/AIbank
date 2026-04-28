package gost2012

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestGenerateStub_RoundTripSignVerify(t *testing.T) {
	t.Parallel()
	s, err := GenerateStub()
	if err != nil {
		t.Fatalf("GenerateStub: %v", err)
	}
	digest := []byte("audit-event-digest-32-bytes-data")
	sig, err := s.Sign(digest)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if len(sig) != SignatureSize {
		t.Errorf("signature size = %d, want %d (matches gost-2012-256 length)",
			len(sig), SignatureSize)
	}
	if err := s.Verify(digest, sig); err != nil {
		t.Errorf("Verify own signature: %v", err)
	}
}

func TestVerify_RejectsTamperedDigest(t *testing.T) {
	t.Parallel()
	s, _ := GenerateStub()
	digest := []byte("original-digest")
	sig, _ := s.Sign(digest)

	tampered := []byte("tampered-digest")
	if err := s.Verify(tampered, sig); err == nil {
		t.Errorf("expected verification failure on tampered digest")
	}
}

func TestVerify_RejectsBadSignature(t *testing.T) {
	t.Parallel()
	s, _ := GenerateStub()
	digest := []byte("any-digest")

	// Неправильная длина — отвергается синтаксической проверкой.
	if err := s.Verify(digest, []byte("too-short")); err == nil {
		t.Errorf("expected error on too-short signature")
	}

	// Правильная длина, но рандом — recompute mismatch отвергает.
	bad := make([]byte, SignatureSize)
	if err := s.Verify(digest, bad); err == nil {
		t.Errorf("expected verification failure on random signature")
	}
}

func TestFromStubKey_Deterministic(t *testing.T) {
	t.Parallel()
	priv := make([]byte, KeySize)
	for i := range priv {
		priv[i] = byte(i)
	}
	s1, err := FromStubKey(priv)
	if err != nil {
		t.Fatalf("FromStubKey: %v", err)
	}
	s2, _ := FromStubKey(priv)

	if s1.KeyID() != s2.KeyID() {
		t.Errorf("KeyID mismatch — same priv-key must produce same KeyID")
	}
	if s1.PublicKeyBase64() != s2.PublicKeyBase64() {
		t.Errorf("PublicKey mismatch")
	}

	// Подпись stub'а детерминирована (pure SHA-256(priv||digest)).
	digest := []byte("deterministic-test")
	sig1, _ := s1.Sign(digest)
	sig2, _ := s2.Sign(digest)
	if string(sig1) != string(sig2) {
		t.Errorf("signature not deterministic — что-то с реализацией stub'а")
	}
}

func TestFromBase64Stub_RoundTrip(t *testing.T) {
	t.Parallel()
	priv := make([]byte, KeySize)
	for i := range priv {
		priv[i] = byte(0xa0 + i)
	}
	encoded := base64.StdEncoding.EncodeToString(priv)

	s, err := FromBase64Stub(encoded)
	if err != nil {
		t.Fatalf("FromBase64Stub: %v", err)
	}
	direct, _ := FromStubKey(priv)
	if s.KeyID() != direct.KeyID() {
		t.Errorf("KeyID mismatch between Base64 and direct key")
	}
}

func TestFromBase64Stub_BadInput(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
	}{
		{"not base64", "!!!not-base64!!!"},
		{"too short", base64.StdEncoding.EncodeToString([]byte{1, 2, 3})},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if _, err := FromBase64Stub(c.in); err == nil {
				t.Errorf("expected error for %q", c.in)
			}
		})
	}
}

func TestFromStubKey_BadSize(t *testing.T) {
	t.Parallel()
	if _, err := FromStubKey([]byte{1, 2, 3}); err == nil {
		t.Errorf("expected error on too-short priv key")
	}
}

func TestGenerateStub_Uniqueness(t *testing.T) {
	t.Parallel()
	// Два независимых GenerateStub должны дать разные KeyID
	// (вероятность коллизии sha256(rand 32 bytes) пренебрежимо мала).
	s1, _ := GenerateStub()
	s2, _ := GenerateStub()
	if s1.KeyID() == s2.KeyID() {
		t.Errorf("two independent GenerateStub produced same KeyID — RNG broken?")
	}
}

func TestKeyID_StubPrefix(t *testing.T) {
	t.Parallel()
	s, _ := GenerateStub()
	keyID := s.KeyID()
	if !strings.HasPrefix(keyID, "STUB-") {
		t.Errorf("KeyID = %q, must start with STUB- (visibility for SOC grep)", keyID)
	}
	// "STUB-" + 32 hex chars = 5 + 32 = 37
	if len(keyID) != 37 {
		t.Errorf("KeyID len = %d, want 37 (STUB- + 32 hex chars)", len(keyID))
	}
}

func TestKeyIDFromPublicKey_Consistency(t *testing.T) {
	t.Parallel()
	s, _ := GenerateStub()
	pub := s.PublicKey()
	if KeyIDFromPublicKey(pub) != s.KeyID() {
		t.Errorf("KeyIDFromPublicKey != Signer.KeyID — verifier и signer разойдутся")
	}
}

func TestPublicKeyFromBase64_RoundTrip(t *testing.T) {
	t.Parallel()
	s, _ := GenerateStub()
	pubB64 := s.PublicKeyBase64()
	pub, err := PublicKeyFromBase64(pubB64)
	if err != nil {
		t.Fatalf("PublicKeyFromBase64: %v", err)
	}
	if KeyIDFromPublicKey(pub) != s.KeyID() {
		t.Errorf("KeyID mismatch after base64 round-trip")
	}
}

func TestPublicKeyFromBase64_BadInput(t *testing.T) {
	t.Parallel()
	if _, err := PublicKeyFromBase64("!!not-base64!!"); err == nil {
		t.Errorf("expected error on bad base64")
	}
	if _, err := PublicKeyFromBase64(base64.StdEncoding.EncodeToString([]byte{1, 2})); err == nil {
		t.Errorf("expected error on too-short pubkey")
	}
}

func TestAlgorithm_StubMarker(t *testing.T) {
	t.Parallel()
	// Алгоритм-маркер должен явно отличаться от "gost-2012-256",
	// иначе при production rollout stub-данные могут быть приняты verifier'ом
	// за реальную ГОСТ-подпись.
	if Algorithm == "gost-2012-256" {
		t.Errorf("Algorithm constant must NOT equal real gost-2012-256")
	}
	if !strings.HasSuffix(Algorithm, "-stub") {
		t.Errorf("Algorithm = %q, must end with -stub", Algorithm)
	}
	if Algorithm != "gost-2012-256-stub" {
		t.Errorf("Algorithm = %q, want exactly 'gost-2012-256-stub'", Algorithm)
	}
}

// FromEnv: t.Setenv несовместим с t.Parallel — запускаем последовательно.

func TestFromEnv_Empty(t *testing.T) {
	t.Setenv("AUDIT_GOST_STUB_KEY_TEST_EMPTY", "")
	s, err := FromEnv("AUDIT_GOST_STUB_KEY_TEST_EMPTY")
	if err != nil {
		t.Errorf("err = %v, want nil for empty env", err)
	}
	if s != nil {
		t.Errorf("expected nil signer for empty env (no-signer mode)")
	}
}

func TestFromEnv_Valid(t *testing.T) {
	priv := make([]byte, KeySize)
	for i := range priv {
		priv[i] = byte(0xbb)
	}
	t.Setenv("AUDIT_GOST_STUB_KEY_TEST_VALID", base64.StdEncoding.EncodeToString(priv))
	s, err := FromEnv("AUDIT_GOST_STUB_KEY_TEST_VALID")
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if s == nil {
		t.Fatalf("expected non-nil signer")
	}
	if !strings.HasPrefix(s.KeyID(), "STUB-") {
		t.Errorf("KeyID = %q, must have STUB- prefix", s.KeyID())
	}
}

func TestNilSigner_ReturnsErr(t *testing.T) {
	t.Parallel()
	var s *StubSigner
	if _, err := s.Sign([]byte("x")); err == nil {
		t.Errorf("expected error on nil-signer Sign")
	}
	if err := s.Verify([]byte("x"), make([]byte, SignatureSize)); err == nil {
		t.Errorf("expected error on nil-signer Verify")
	}
}
