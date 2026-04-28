package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	const pw = "correct horse battery staple"
	h, err := Hash(pw)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if h == pw {
		t.Fatal("hash should not equal plaintext")
	}
	if !strings.HasPrefix(h, "$2a$") && !strings.HasPrefix(h, "$2b$") && !strings.HasPrefix(h, "$2y$") {
		t.Fatalf("expected bcrypt hash prefix, got %s", h[:4])
	}
	if err := Verify(h, pw); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerify_WrongPassword(t *testing.T) {
	h, err := Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	err = Verify(h, "wrong password!!!")
	if !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword, got %v", err)
	}
}

func TestVerify_InvalidHash(t *testing.T) {
	if err := Verify("not-a-bcrypt-hash", "anything"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword for malformed hash, got %v", err)
	}
}

func TestHash_RejectsTooShort(t *testing.T) {
	if _, err := Hash("short"); err == nil {
		t.Fatal("expected error for password shorter than 8 chars")
	}
}

func TestHash_RejectsTooLong(t *testing.T) {
	pw := strings.Repeat("a", 73)
	if _, err := Hash(pw); err == nil {
		t.Fatal("expected error for password longer than 72 chars")
	}
}
