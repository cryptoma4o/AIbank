package cryptoutils

import "errors"

// ErrNotImplemented is returned by HSM-dependent operations in Pre-MVP mode.
var ErrNotImplemented = errors.New("crypto-utils: operation requires КриптоПро CSP HSM integration (not available in Pre-MVP)")

// Signer is the interface for GOST R 34.10-2012 digital signatures.
// Production implementation wraps КриптоПро CSP or VipNet CSP via cgo.
type Signer interface {
	// Sign signs digest with the private key, returns DER-encoded signature.
	Sign(digest []byte) ([]byte, error)
	// Verify verifies a DER-encoded GOST signature.
	Verify(digest, signature, publicKey []byte) (bool, error)
	// CertificateThumbprint returns the ГОСТ-сертификат thumbprint.
	CertificateThumbprint() ([]byte, error)
}

// StubSigner is a Pre-MVP placeholder that always returns ErrNotImplemented.
// Replace with KriproPro or VipNet signer before production.
type StubSigner struct{}

func (s *StubSigner) Sign(_ []byte) ([]byte, error) {
	return nil, ErrNotImplemented
}

func (s *StubSigner) Verify(_, _, _ []byte) (bool, error) {
	return false, ErrNotImplemented
}

func (s *StubSigner) CertificateThumbprint() ([]byte, error) {
	return nil, ErrNotImplemented
}

// NewSigner returns the active Signer implementation.
// In Pre-MVP, returns StubSigner. Configure via BUILD tags in production.
func NewSigner() Signer {
	return &StubSigner{}
}
