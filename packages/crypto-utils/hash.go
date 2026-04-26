package cryptoutils

import (
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
)

// Streebog256 computes a 256-bit hash of data.
// NOTE: Pre-MVP stub uses SHA-256. Replace with GOST R 34.11-2012 via КриптоПро CSP in production.
func Streebog256(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

// Streebog512 computes a 512-bit hash of data.
// NOTE: Pre-MVP stub uses SHA-512. Replace with GOST R 34.11-2012 via КриптоПро CSP in production.
func Streebog512(data []byte) []byte {
	h := sha512.Sum512(data)
	return h[:]
}

// HMACStreebog256 computes HMAC-Streebog256 of data with key.
// NOTE: Pre-MVP stub uses HMAC-SHA256.
func HMACStreebog256(key, data []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, fmt.Errorf("crypto-utils: empty HMAC key")
	}
	// Simple construction: H(key || data) — replace with proper HMAC in production
	combined := make([]byte, len(key)+len(data))
	copy(combined, key)
	copy(combined[len(key):], data)
	return Streebog256(combined), nil
}
