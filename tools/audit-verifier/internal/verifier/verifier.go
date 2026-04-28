// Package verifier пересчитывает hash-цепочку audit-events для тенанта
// и сравнивает с записанным значением. Используется CLI и (через
// EventStore-интерфейс) интеграционными тестами.
//
// См. docs/runbooks/audit-log-integrity.md и
// services/audit-service/internal/domain/event.go (источник истины
// для ComputeHash).
package verifier

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"regexp"
	"time"
)

// tenantIDPattern — тот же whitelist, что у tenant-service /
// audit-service: лат. lower-case + цифры + дефис/подчёркивание.
// Защищает CLI от инъекции в DSN-mode SQL и API-mode URL.
var tenantIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)

// ValidateTenantID — единая точка валидации tenant_id для CLI.
func ValidateTenantID(id string) error {
	if id == "" {
		return errors.New("tenant_id is required")
	}
	if !tenantIDPattern.MatchString(id) {
		return fmt.Errorf("tenant_id %q invalid (allowed: ^[a-z][a-z0-9_-]{1,63}$)", id)
	}
	return nil
}

// EventStore поставляет события для верификации. Стрим (iter.Seq2)
// нужен, чтобы tool'у не приходилось грузить миллионы событий в память
// для крупного тенанта. Реализации обязаны выдавать события в порядке
// возрастания CreatedAt (хронологически — порядок цепочки).
type EventStore interface {
	ListEvents(ctx context.Context, tenantID string, from, to *time.Time) iter.Seq2[Event, error]
}

// VerificationResult — итог проверки одной цепочки.
type VerificationResult struct {
	TenantID      string     `json:"tenant_id"`
	Valid         bool       `json:"valid"`
	EventCount    int        `json:"event_count"`
	FirstAt       *time.Time `json:"first_at,omitempty"`
	LastAt        *time.Time `json:"last_at,omitempty"`
	LastHash      string     `json:"last_hash,omitempty"`
	FirstMismatch *Event     `json:"first_mismatch,omitempty"`
	// ExpectedHash — что ДОЛЖНО было быть записано в .Hash при честной цепочке.
	ExpectedHash string `json:"expected_hash,omitempty"`
	// ActualHash — что фактически записано в Event.Hash (потенциально подменённое).
	ActualHash string `json:"actual_hash,omitempty"`
	// MismatchReason — текстовый разбор: 'hash_mismatch' либо 'prev_hash_mismatch'.
	MismatchReason string `json:"mismatch_reason,omitempty"`
}

// Verifier — основной тип, переиспользуемый между CLI-командами.
type Verifier struct {
	store EventStore
}

// NewVerifier создаёт verifier с заданным источником событий.
func NewVerifier(store EventStore) *Verifier {
	return &Verifier{store: store}
}

// VerifyChain проходит события тенанта в хронологическом порядке,
// перевычисляет ComputeHash(prev_hash) и сравнивает с e.Hash.
//
// При первом mismatch'е — возврат с FirstMismatch != nil. Дальнейшие
// события не проверяются: разрыв цепочки = инцидент, дальнейший анализ
// — задача оператора (см. runbook §3 INVESTIGATE).
//
// Пустая цепочка считается валидной (vacuously true): тенант мог быть
// только что создан и не успеть записать события.
func (v *Verifier) VerifyChain(ctx context.Context, tenantID string, from, to *time.Time) (VerificationResult, error) {
	if err := ValidateTenantID(tenantID); err != nil {
		return VerificationResult{}, err
	}

	res := VerificationResult{TenantID: tenantID, Valid: true}
	var prevHash string
	var firstAt *time.Time

	for ev, err := range v.store.ListEvents(ctx, tenantID, from, to) {
		if err != nil {
			return VerificationResult{}, fmt.Errorf("list events: %w", err)
		}
		// Каждый шаг — возможность отменить через ctx (например,
		// timeout у крупного тенанта).
		if cerr := ctx.Err(); cerr != nil {
			return VerificationResult{}, cerr
		}

		res.EventCount++
		if firstAt == nil {
			t := ev.CreatedAt
			firstAt = &t
			res.FirstAt = firstAt
		}
		lastAt := ev.CreatedAt
		res.LastAt = &lastAt

		// 1. prev_hash в записанном событии должен указывать на хэш
		//    предыдущего события. Это ловит «удалённое из середины»
		//    событие — следующая запись будет ссылаться на
		//    несуществующий prev_hash.
		if ev.PreviousHash != prevHash {
			cp := ev
			res.Valid = false
			res.FirstMismatch = &cp
			res.ExpectedHash = prevHash
			res.ActualHash = ev.PreviousHash
			res.MismatchReason = "prev_hash_mismatch"
			return res, nil
		}

		// 2. ComputeHash(prev) должен совпадать с записанным Hash.
		//    Это ловит изменение payload / actor / любого immutable-поля.
		expected := ev.ComputeHash(prevHash)
		if expected != ev.Hash {
			cp := ev
			res.Valid = false
			res.FirstMismatch = &cp
			res.ExpectedHash = expected
			res.ActualHash = ev.Hash
			res.MismatchReason = "hash_mismatch"
			return res, nil
		}

		prevHash = ev.Hash
	}
	res.LastHash = prevHash
	return res, nil
}

// LatestHash возвращает hash последнего события тенанта (для оп-тулов
// и runbook §1 quick-check). Пустая цепочка → ("", nil).
func (v *Verifier) LatestHash(ctx context.Context, tenantID string) (string, error) {
	if err := ValidateTenantID(tenantID); err != nil {
		return "", err
	}
	var last string
	var lastTime time.Time
	for ev, err := range v.store.ListEvents(ctx, tenantID, nil, nil) {
		if err != nil {
			return "", fmt.Errorf("list events: %w", err)
		}
		if ev.CreatedAt.After(lastTime) || last == "" {
			last = ev.Hash
			lastTime = ev.CreatedAt
		}
	}
	return last, nil
}

// SignatureVerifyFunc — callback для проверки криптоподписи одного события.
//
// Реализации (типичные):
//   - ed25519.Verify(publicKeyForKeyID(ev.SignerKeyID), digest, ev.Signature)
//   - packages/signature.SignatureProvider.Verify (для ГОСТ через КриптоПро)
//
// Возвращает nil если подпись валидна, error — иначе. Verifier'у достаточно
// факта валидности; детали (expired cert, chain broken) — внутренний концерн
// callback'а и его места в audit-bundle.
type SignatureVerifyFunc func(ctx context.Context, ev *Event) error

// SignatureVerificationResult — итог проверки подписей в цепочке.
type SignatureVerificationResult struct {
	TenantID      string `json:"tenant_id"`
	EventCount    int    `json:"event_count"`
	SignedCount   int    `json:"signed_count"`   // сколько событий имеют (signature, alg, keyID)
	UnsignedCount int    `json:"unsigned_count"` // EventCount - SignedCount
	Valid         bool   `json:"valid"`
	// FirstInvalid — первое событие с невалидной подписью (nil если все ОК).
	FirstInvalid *Event `json:"first_invalid,omitempty"`
	// InvalidReason — текст ошибки от SignatureVerifyFunc.
	InvalidReason string `json:"invalid_reason,omitempty"`
}

// VerifySignatures проходит события тенанта и для каждого с HasSignature()=true
// вызывает verifyFn. Возвращает первую невалидную подпись или Valid=true если
// все подписанные события прошли проверку (включая случай SignedCount=0).
//
// Backwards compat: события без signature пропускаются (не считаются невалидными).
// Это позволяет внедрять signing инкрементально — старые события без подписи
// продолжают работать на одном hash chain.
//
// Этот метод НЕ дублирует VerifyChain — hash chain и подпись это два
// независимых аудита, и оба вызываются при полной проверке (см. CLI).
func (v *Verifier) VerifySignatures(ctx context.Context, tenantID string, verifyFn SignatureVerifyFunc) (SignatureVerificationResult, error) {
	if err := ValidateTenantID(tenantID); err != nil {
		return SignatureVerificationResult{}, err
	}
	if verifyFn == nil {
		return SignatureVerificationResult{}, errors.New("verifyFn is required")
	}

	res := SignatureVerificationResult{TenantID: tenantID, Valid: true}

	for ev, err := range v.store.ListEvents(ctx, tenantID, nil, nil) {
		if err != nil {
			return SignatureVerificationResult{}, fmt.Errorf("list events: %w", err)
		}
		if cerr := ctx.Err(); cerr != nil {
			return SignatureVerificationResult{}, cerr
		}

		res.EventCount++
		if !ev.HasSignature() {
			res.UnsignedCount++
			continue
		}
		res.SignedCount++

		if err := verifyFn(ctx, &ev); err != nil {
			cp := ev
			res.Valid = false
			res.FirstInvalid = &cp
			res.InvalidReason = err.Error()
			return res, nil
		}
	}
	return res, nil
}
