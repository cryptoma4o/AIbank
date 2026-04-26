package auth

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// OTPStore is an in-memory OTP store (replace with Redis in production).
type OTPStore struct {
	mu      sync.Mutex
	entries map[string]otpEntry
}

type otpEntry struct {
	code      string
	expiresAt time.Time
}

func NewOTPStore() *OTPStore {
	return &OTPStore{entries: make(map[string]otpEntry)}
}

func (s *OTPStore) Generate(phone string) (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return "", fmt.Errorf("otp: generate: %w", err)
	}
	code := fmt.Sprintf("%06d", n.Int64()+100000)
	s.mu.Lock()
	s.entries[phone] = otpEntry{code: code, expiresAt: time.Now().Add(5 * time.Minute)}
	s.mu.Unlock()
	return code, nil
}

func (s *OTPStore) Verify(phone, code string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[phone]
	if !ok || time.Now().After(entry.expiresAt) {
		return false
	}
	if entry.code != code {
		return false
	}
	delete(s.entries, phone)
	return true
}
