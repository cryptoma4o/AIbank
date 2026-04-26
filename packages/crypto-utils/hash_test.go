package cryptoutils_test

import (
	"testing"

	cryptoutils "aibank/crypto-utils"
)

func TestStreebog256Length(t *testing.T) {
	h := cryptoutils.Streebog256([]byte("hello"))
	if len(h) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(h))
	}
}

func TestStreebog512Length(t *testing.T) {
	h := cryptoutils.Streebog512([]byte("hello"))
	if len(h) != 64 {
		t.Errorf("expected 64 bytes, got %d", len(h))
	}
}

func TestHMACStreebog256EmptyKey(t *testing.T) {
	_, err := cryptoutils.HMACStreebog256(nil, []byte("data"))
	if err == nil {
		t.Error("expected error on empty key")
	}
}

func TestToFromHex(t *testing.T) {
	original := []byte{0xde, 0xad, 0xbe, 0xef}
	encoded := cryptoutils.ToHex(original)
	decoded, err := cryptoutils.FromHex(encoded)
	if err != nil {
		t.Fatalf("FromHex error: %v", err)
	}
	if string(decoded) != string(original) {
		t.Errorf("round-trip mismatch: %x != %x", decoded, original)
	}
}

func TestMaskSecret(t *testing.T) {
	masked := cryptoutils.MaskSecret("my-secret-token-abc")
	if masked == "my-secret-token-abc" {
		t.Error("secret was not masked")
	}
	if len(masked) != len("my-secret-token-abc") {
		t.Errorf("masked length changed: %d vs %d", len(masked), len("my-secret-token-abc"))
	}
}

func TestStubSigner(t *testing.T) {
	signer := cryptoutils.NewSigner()
	_, err := signer.Sign([]byte("digest"))
	if err != cryptoutils.ErrNotImplemented {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}
