package domain

import (
	"encoding/json"
	"time"
)

// CommandType — каноническое имя команды, передаваемое на адаптер.
// Состав совпадает с adapter-side enum (services/abs-adapter-cft/internal/domain),
// но физически живёт в connector — adapter может иметь свой расширенный список,
// connector не должен его знать (ADR-0006: транспарентный router).
type CommandType string

const (
	CmdOpenAccount    CommandType = "OpenAccount"
	CmdCloseAccount   CommandType = "CloseAccount"
	CmdGetAccountInfo CommandType = "GetAccountInfo"
	CmdCreateClient   CommandType = "CreateClient"
)

// CommandMetadata переносит контекст инициирования команды.
// Поля попадают в audit log на стороне adapter и в billing events
// (см. ADR-0010), connector обязан их прокидывать без потерь.
type CommandMetadata struct {
	InitiatedBy   string `json:"initiated_by,omitempty"`
	TraceID       string `json:"trace_id,omitempty"`
	ApplicationID string `json:"application_id,omitempty"`
}

// CanonicalCommand — каноническая команда, которую connector принимает
// от внешних вызывающих (Temporal activities, web-bff) и маршрутизирует
// в адаптер банка. Структура совместима по wire-формату с
// services/abs-adapter-cft.domain.ABSCommand: дополнительные поля
// (id, metadata, created_at) сохраняются на стороне connector,
// в адаптер прокидывается ровно тот набор, который адаптер ожидает.
type CanonicalCommand struct {
	// ID — внутренний идентификатор записи в журнале команд.
	// Не используется для дедупликации.
	ID string `json:"id,omitempty"`

	IdempotencyKey string `json:"idempotency_key"`
	TenantID       string `json:"tenant_id"`

	Command CommandType     `json:"command"`
	Payload json.RawMessage `json:"payload"`

	Metadata  CommandMetadata `json:"metadata,omitempty"`
	CreatedAt time.Time       `json:"created_at,omitempty"`
}

// CanonicalResponse — канонический ответ от адаптера, обёрнутый connector'ом.
// Расширенные поля (CompletedAt) проставляются connector'ом для
// последующего билинга и audit-tracking.
type CanonicalResponse struct {
	IdempotencyKey string          `json:"idempotency_key"`
	Success        bool            `json:"success"`
	Error          string          `json:"error,omitempty"`
	Data           json.RawMessage `json:"data,omitempty"`

	// AdapterUsed — логическое имя адаптера ("cft" / "diasoft" / "rs-bank").
	AdapterUsed string `json:"adapter_used"`

	// AdapterVersion — semver, прочитанный из конфигурации registry.
	// Согласно ADR-0006 адаптер сам сообщает версию через Capabilities();
	// в HTTP-MVP версия читается из registry и пишется здесь для audit.
	AdapterVersion string `json:"adapter_version,omitempty"`

	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// AdapterRequest — payload, который мы фактически отправляем в adapter HTTP-API.
// Совпадает с services/abs-adapter-cft.domain.ABSCommand.
type AdapterRequest struct {
	IdempotencyKey string          `json:"idempotency_key"`
	TenantID       string          `json:"tenant_id"`
	Command        CommandType     `json:"command"`
	Payload        json.RawMessage `json:"payload"`
}

// AdapterResponse — payload, который мы получаем от adapter HTTP-API.
type AdapterResponse struct {
	IdempotencyKey string          `json:"idempotency_key"`
	Success        bool            `json:"success"`
	Data           json.RawMessage `json:"data,omitempty"`
	Error          string          `json:"error,omitempty"`
	AdapterUsed    string          `json:"adapter_used"`
}
