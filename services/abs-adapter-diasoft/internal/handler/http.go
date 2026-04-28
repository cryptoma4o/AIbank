// Package handler — HTTP-обвязка адаптера Diasoft FA#.
//
// Согласно ADR-0006 целевой контракт между abs-connector и адаптером — gRPC.
// До миграции на gRPC адаптер экспонирует HTTP/JSON-API, совместимый с
// services/abs-connector/internal/clients/adapter.go: один POST /v1/execute,
// принимающий канонический ABSCommand и отдающий канонический ABSResponse.
//
// Ответы адаптера в текущей итерации — детерминированные «заглушки»:
// настоящего MQ/SDK-клиента к Diasoft ещё нет (golden-tests + mock-Diasoft —
// Phase 2, см. ADR-0006 § 5). Детерминизм нужен, чтобы тесты были стабильными
// и чтобы повторный вызов с тем же idempotency_key возвращал тот же результат
// (имитация идемпотентности на стороне адаптера).
package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"aibank/abs-adapter-diasoft/internal/domain"
)

// AdapterName — логическое имя адаптера в audit/billing-логах.
// Должно совпадать со значением в configs/tenants/<bank>/integrations/abs.yaml.
const AdapterName = "diasoft"

// AccountPrefix — фиксированный префикс «расчётный счёт юр.лица в рублях».
// Diasoft-инстанс отличается reserved-меткой DIA в составе номера, чтобы
// в логах и в тестовых сценариях легко различать banking origin.
const AccountPrefix = "40702810"

// AccountReservedTag — метка, отделяющая diasoft-номер от cft-номера в
// stub-режиме. На реальном Diasoft FA# 20-значный номер будет считаться
// по алгоритму ЦБ (Положение 579-П), и эта метка исчезнет.
const AccountReservedTag = "DIA"

// BIK — пример БИК банка-владельца Diasoft-инстанса (статический для stub'а).
const BIK = "044585219"

// Handler — HTTP-handler Diasoft-адаптера. Stateless, безопасен для
// конкурентного использования (детерминизм через sha256, без shared state).
type Handler struct {
	version  string
	commands []domain.CommandType
}

// NewHandler — фабрика. Версия читается build-системой из VERSION-файла
// и прокидывается через cmd/server/main.go.
func NewHandler(version string) *Handler {
	return &Handler{
		version: version,
		commands: []domain.CommandType{
			domain.CmdOpenAccount,
			domain.CmdCreateClient,
			domain.CmdGetAccountInfo,
			domain.CmdCloseAccount,
		},
	}
}

// Router — http.Handler с тремя публичными ручками: /healthz (k8s-probe),
// /version (capability-discovery до миграции на gRPC.Capabilities) и
// POST /v1/execute (канонический транспорт adapter ↔ connector).
func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", h.healthz)
	r.Get("/version", h.versionInfo)
	r.Post("/v1/execute", h.execute)

	return r
}

func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// versionInfo возвращает {"adapter", "version", "commands"}. Используется
// abs-connector'ом для проверки совместимости (упрощённый аналог
// gRPC Capabilities() из ADR-0006 § 2).
func (h *Handler) versionInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"adapter":  AdapterName,
		"version":  h.version,
		"commands": h.commands,
	})
}

func (h *Handler) execute(w http.ResponseWriter, r *http.Request) {
	var cmd domain.ABSCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		writeJSON(w, http.StatusBadRequest, domain.ABSResponse{
			IdempotencyKey: "",
			Success:        false,
			Error:          "invalid request body",
			AdapterUsed:    "abs-adapter-diasoft",
		})
		return
	}

	resp := h.stub(cmd)
	writeJSON(w, http.StatusOK, resp)
}

// stub строит канонический ABSResponse для одной из четырёх поддерживаемых
// канонических команд. Все идентификаторы (account_number, client_id)
// детерминированы относительно (command, idempotency_key) — это гарантирует
// стабильность golden-tests и имитирует idempotency.
func (h *Handler) stub(cmd domain.ABSCommand) domain.ABSResponse {
	base := domain.ABSResponse{
		IdempotencyKey: cmd.IdempotencyKey,
		AdapterUsed:    "abs-adapter-diasoft",
	}

	switch cmd.Command {
	case domain.CmdOpenAccount:
		// 20-символьный номер: 8 цифр AccountPrefix + 3 буквы reserved-метки
		// + 9 цифр из sha256. Не пур-числовой формат намеренно — это
		// stub-маркер, отсутствующий в реальной Diasoft-нумерации.
		number := AccountPrefix + AccountReservedTag + digestDigits(cmd, 9)
		base.Success = true
		base.Data = map[string]any{
			"account_number": number,
			"bik":            BIK,
		}

	case domain.CmdCreateClient:
		base.Success = true
		base.Data = map[string]any{
			"client_id": "diasoft_" + digestHex(cmd, 16),
		}

	case domain.CmdGetAccountInfo:
		base.Success = true
		base.Data = map[string]any{
			"balance_kopecks": 0,
			"status":          "active",
			"abs":             "diasoft",
		}

	case domain.CmdCloseAccount:
		base.Success = true
		base.Data = map[string]any{
			"status": "closed",
			"abs":    "diasoft",
		}

	default:
		base.Success = false
		base.Error = "unknown command: " + string(cmd.Command)
	}

	return base
}

// digestHex — детерминированный n-символьный hex-суффикс на основе
// (command, idempotency_key). Используется для client_id.
func digestHex(cmd domain.ABSCommand, n int) string {
	sum := sha256.Sum256([]byte("diasoft|" + string(cmd.Command) + "|" + cmd.IdempotencyKey))
	encoded := hex.EncodeToString(sum[:])
	if n > len(encoded) {
		n = len(encoded)
	}
	return encoded[:n]
}

// digestDigits — детерминированный n-значный десятичный суффикс на основе
// (command, idempotency_key). Криптостойкость не требуется (это не secret),
// важна только однородность распределения.
func digestDigits(cmd domain.ABSCommand, n int) string {
	sum := sha256.Sum256([]byte("diasoft|" + string(cmd.Command) + "|" + cmd.IdempotencyKey))
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		out[i] = '0' + (sum[i%len(sum)] % 10)
	}
	return string(out)
}

// HasReservedTag — экспортирован для тестов: проверяет, что stub-номер
// действительно содержит метку DIA в нужной позиции (после AccountPrefix).
func HasReservedTag(accountNumber string) bool {
	return strings.HasPrefix(accountNumber, AccountPrefix+AccountReservedTag)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
