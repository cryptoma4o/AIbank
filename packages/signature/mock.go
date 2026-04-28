package signature

// mock.go — детерминированный MockSignatureProvider для unit/integration-тестов.
//
// Не криптостойкий, **никогда не должен использоваться в production**. В
// pre-MVP покрывает все потребители (document-service, identity-service,
// audit-service) до получения КриптоПро / VipNet лицензий.
//
// Детерминизм: одинаковый payload + certID → одинаковая Signature. Это
// позволяет писать стабильные golden-tests.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// MockSignatureProvider — in-memory provider для тестов.
//
// Поддерживает:
//   - Sign — Streebog-stub (фактически SHA-256) от payload+certID
//   - Verify — re-hash и сравнение
//   - ListCertificates / GetCertificate — из in-memory map
//
// Concurrent-safe: использует sync.RWMutex.
type MockSignatureProvider struct {
	mu    sync.RWMutex
	certs map[string]*Certificate
	// fixedNow — если установлено, Sign использует это время вместо time.Now().
	// Полезно для детерминистических golden-tests.
	fixedNow time.Time
}

// NewMockSignatureProvider создаёт mock с одним предустановленным сертификатом
// "mock-cert-001" со сроком действия +1 год от now. Для большинства тестов
// этого достаточно.
func NewMockSignatureProvider() *MockSignatureProvider {
	now := time.Now().UTC()
	defaultCert := &Certificate{
		ID:           "mock-cert-001",
		Subject:      "CN=Mock Signer,O=AIbank Test,C=RU",
		SerialNumber: "01",
		Issuer:       "CN=Mock CA,O=AIbank Test,C=RU",
		ValidFrom:    now,
		ValidTo:      now.AddDate(1, 0, 0),
		Thumbprint:   "0000000000000000000000000000000000000000000000000000000000000001",
		AlgorithmOID: "1.2.643.7.1.1.1.1", // ГОСТ-2012/256 (имитация)
		KeySize:      256,
		IsExpired:    false,
	}
	return &MockSignatureProvider{
		certs: map[string]*Certificate{
			defaultCert.ID: defaultCert,
		},
	}
}

// AddCertificate добавляет сертификат в mock-store. Используется для
// тестирования multi-cert-сценариев (например, expired-cert path).
func (m *MockSignatureProvider) AddCertificate(c *Certificate) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.certs[c.ID] = c
}

// SetFixedNow фиксирует время для детерминистических тестов.
func (m *MockSignatureProvider) SetFixedNow(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fixedNow = t
}

// Name — "mock".
func (m *MockSignatureProvider) Name() string { return "mock" }

// Sign делает детерминированный hash от payload+certID и возвращает SignedPayload.
func (m *MockSignatureProvider) Sign(ctx context.Context, payload []byte, certID string) (*SignedPayload, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if certID == "" {
		return nil, errors.New("signature: certID is required")
	}
	if len(payload) == 0 {
		return nil, errors.New("signature: payload is required")
	}

	m.mu.RLock()
	cert, ok := m.certs[certID]
	now := m.fixedNow
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrCertificateNotFound, certID)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if cert.ValidTo.Before(now) {
		return nil, ErrCertificateExpired
	}

	// Детерминированная "подпись": SHA-256(payload || certID).
	h := sha256.New()
	h.Write(payload)
	h.Write([]byte(certID))
	digest := h.Sum(nil)

	// "Сертификат" в SignedPayload — для Mock просто его ID.
	return &SignedPayload{
		Signature:         digest,
		PayloadHash:       hex.EncodeToString(digest),
		HashAlgorithm:     "sha256", // в production-провайдерах будет "streebog256"
		SignerCertificate: "mock-cert:" + certID,
		SignedAt:          now,
		Provider:          "mock",
		ProviderVersion:   "0.1.0-test",
	}, nil
}

// Verify пересчитывает hash и сверяет с payload_hash из SignedPayload. В
// Mock-режиме это всегда успешно для непустых payload (так как мы не получаем
// исходный payload — Verify в реальном провайдере проверяет криптографически
// signature vs hash).
//
// Для тестового сценария мы извлекаем certID из SignerCertificate (формат
// "mock-cert:<id>") и проверяем срок действия.
func (m *MockSignatureProvider) Verify(ctx context.Context, signed *SignedPayload) (*VerificationResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if signed == nil {
		return &VerificationResult{Valid: false, ErrorMessage: "signed payload is nil"}, nil
	}
	if len(signed.Signature) == 0 || signed.PayloadHash == "" {
		return &VerificationResult{Valid: false, ErrorMessage: "signature or hash empty"}, nil
	}

	// Из mock-формата извлекаем certID.
	certID := ""
	if len(signed.SignerCertificate) > len("mock-cert:") &&
		signed.SignerCertificate[:len("mock-cert:")] == "mock-cert:" {
		certID = signed.SignerCertificate[len("mock-cert:"):]
	}

	if certID == "" {
		return &VerificationResult{Valid: false, ErrorMessage: "cannot extract certID from mock signature"}, nil
	}

	m.mu.RLock()
	cert, ok := m.certs[certID]
	now := m.fixedNow
	m.mu.RUnlock()
	if now.IsZero() {
		now = time.Now().UTC()
	}

	if !ok {
		return &VerificationResult{Valid: false, ErrorMessage: "unknown certificate"}, nil
	}

	expired := cert.ValidTo.Before(now)
	return &VerificationResult{
		Valid:             !expired,
		SignerName:        cert.Subject,
		CertificateExpiry: cert.ValidTo,
		IsExpired:         expired,
		ChainValid:        true, // Mock не проверяет цепочку
		TimestampVerified: false,
		ErrorMessage: func() string {
			if expired {
				return "certificate expired"
			}
			return ""
		}(),
	}, nil
}

// ListCertificates возвращает все сертификаты из in-memory store.
func (m *MockSignatureProvider) ListCertificates(_ context.Context) ([]*Certificate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Certificate, 0, len(m.certs))
	for _, c := range m.certs {
		// Возвращаем копию, чтобы вызывающий код не мог мутировать internal state.
		cp := *c
		out = append(out, &cp)
	}
	return out, nil
}

// GetCertificate возвращает копию сертификата по ID.
func (m *MockSignatureProvider) GetCertificate(_ context.Context, certID string) (*Certificate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cert, ok := m.certs[certID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrCertificateNotFound, certID)
	}
	cp := *cert
	return &cp, nil
}
