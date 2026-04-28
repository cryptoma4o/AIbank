package ed25519

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"
)

func TestGenerate_RoundTripSignVerify(t *testing.T) {
	t.Parallel()
	s, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	digest := []byte("audit-event-digest-32-bytes-data")
	sig, err := s.Sign(digest)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if len(sig) != ed25519.SignatureSize {
		t.Errorf("signature size = %d, want %d", len(sig), ed25519.SignatureSize)
	}
	if err := s.Verify(digest, sig); err != nil {
		t.Errorf("Verify own signature: %v", err)
	}
}

func TestVerify_RejectsTamperedDigest(t *testing.T) {
	t.Parallel()
	s, _ := Generate()
	digest := []byte("original-digest")
	sig, _ := s.Sign(digest)

	tampered := []byte("tampered-digest")
	if err := s.Verify(tampered, sig); err == nil {
		t.Errorf("expected verification failure on tampered digest")
	}
}

func TestVerify_RejectsBadSignature(t *testing.T) {
	t.Parallel()
	s, _ := Generate()
	digest := []byte("any-digest")

	// Неправильная длина — должна отвергаться синтаксической проверкой.
	if err := s.Verify(digest, []byte("too-short")); err == nil {
		t.Errorf("expected error on too-short signature")
	}

	// Правильная длина, но рандом — крипто-проверка отвергает.
	bad := make([]byte, ed25519.SignatureSize)
	if err := s.Verify(digest, bad); err == nil {
		t.Errorf("expected verification failure on random signature")
	}
}

func TestFromSeed_Deterministic(t *testing.T) {
	t.Parallel()
	seed := make([]byte, SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	s1, err := FromSeed(seed)
	if err != nil {
		t.Fatalf("FromSeed: %v", err)
	}
	s2, _ := FromSeed(seed)

	if s1.KeyID() != s2.KeyID() {
		t.Errorf("KeyID mismatch — same seed must produce same key")
	}
	if s1.PublicKeyBase64() != s2.PublicKeyBase64() {
		t.Errorf("PublicKey mismatch")
	}
	// Подпись детерминирована для ed25519 (RFC 8032).
	digest := []byte("deterministic-test")
	sig1, _ := s1.Sign(digest)
	sig2, _ := s2.Sign(digest)
	if string(sig1) != string(sig2) {
		t.Errorf("signature not deterministic — что-то с реализацией")
	}
}

func TestFromBase64Seed_RoundTrip(t *testing.T) {
	t.Parallel()
	seed := make([]byte, SeedSize)
	for i := range seed {
		seed[i] = byte(0xa0 + i)
	}
	encoded := base64.StdEncoding.EncodeToString(seed)

	s, err := FromBase64Seed(encoded)
	if err != nil {
		t.Fatalf("FromBase64Seed: %v", err)
	}

	// KeyID consistent с FromSeed напрямую.
	direct, _ := FromSeed(seed)
	if s.KeyID() != direct.KeyID() {
		t.Errorf("KeyID mismatch between Base64 and direct seed")
	}
}

func TestFromBase64Seed_BadInput(t *testing.T) {
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
			if _, err := FromBase64Seed(c.in); err == nil {
				t.Errorf("expected error for %q", c.in)
			}
		})
	}
}

// Note: t.Setenv несовместим с t.Parallel в одном тесте — поэтому
// FromEnv-тесты идут последовательно.

func TestFromEnv_Empty(t *testing.T) {
	t.Setenv("AUDIT_SIGNING_KEY_TEST_EMPTY", "")
	s, err := FromEnv("AUDIT_SIGNING_KEY_TEST_EMPTY")
	if err != nil {
		t.Errorf("err = %v, want nil for empty env", err)
	}
	if s != nil {
		t.Errorf("expected nil signer for empty env")
	}
}

func TestFromEnv_Valid(t *testing.T) {
	seed := make([]byte, SeedSize)
	for i := range seed {
		seed[i] = byte(0xbb)
	}
	t.Setenv("AUDIT_SIGNING_KEY_TEST_VALID", base64.StdEncoding.EncodeToString(seed))
	s, err := FromEnv("AUDIT_SIGNING_KEY_TEST_VALID")
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if s == nil {
		t.Fatalf("expected non-nil signer")
	}
}

func TestKeyID_StableHexFingerprint(t *testing.T) {
	t.Parallel()
	s, _ := Generate()
	keyID := s.KeyID()
	if len(keyID) != 64 {
		t.Errorf("KeyID len = %d, want 64 (hex SHA-256)", len(keyID))
	}
	for _, c := range keyID {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("KeyID has non-hex char: %c", c)
		}
	}
}

func TestVerifyWithPublicKey_StandAlone(t *testing.T) {
	t.Parallel()
	s, _ := Generate()
	digest := []byte("audit-digest")
	sig, _ := s.Sign(digest)

	// Stand-alone verification (как сделает audit-verifier CLI).
	if err := VerifyWithPublicKey(s.PublicKey(), digest, sig); err != nil {
		t.Errorf("VerifyWithPublicKey: %v", err)
	}

	// Через base64 round-trip (как через CLI flag).
	pubB64 := s.PublicKeyBase64()
	pub, err := PublicKeyFromBase64(pubB64)
	if err != nil {
		t.Fatalf("PublicKeyFromBase64: %v", err)
	}
	if err := VerifyWithPublicKey(pub, digest, sig); err != nil {
		t.Errorf("verify after base64 round-trip: %v", err)
	}

	// KeyID consistency.
	if KeyIDFromPublicKey(pub) != s.KeyID() {
		t.Errorf("KeyIDFromPublicKey != Signer.KeyID")
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
