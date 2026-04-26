package cryptoutils

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// ToHex encodes bytes as lowercase hex string.
func ToHex(b []byte) string {
	return hex.EncodeToString(b)
}

// FromHex decodes a hex string. Returns error on invalid input.
func FromHex(s string) ([]byte, error) {
	s = strings.TrimPrefix(strings.ToLower(s), "0x")
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("crypto-utils: invalid hex: %w", err)
	}
	return b, nil
}

// MaskSecret masks all but the first and last 2 characters of a secret string for logging.
func MaskSecret(s string) string {
	if len(s) <= 6 {
		return "***"
	}
	return s[:2] + strings.Repeat("*", len(s)-4) + s[len(s)-2:]
}
