// Package domain — модель UBOGraph.
//
// Граф владения хранится как версионированный snapshot: orchestrator после
// прогона agent-ubo-tracing (см. ai/agent-ubo-tracing/models/schemas.py)
// присылает результат — и мы материализуем его одной строкой в БД с
// инкрементом version. История версий сохраняется (никаких UPDATE);
// «текущий» граф — MAX(version) для (tenant_id, legal_entity_id).
//
// Schema нод/рёбер/UBO копирует контракт agent-ubo-tracing один к одному,
// чтобы payload приходил без преобразования. Мы храним эти три коллекции
// как JSONB в БД и как json.RawMessage в Go — при PATCH/анализе не нужно
// разворачивать структуру здесь, доменный анализ — в risk-engine.
package domain

import (
	"encoding/json"
	"time"
)

// GraphNode — узел графа владения.
// type: "person" | "legal_entity" (StrEnum в python).
type GraphNode struct {
	ID   string  `json:"id"`
	Type string  `json:"type"`
	Name string  `json:"name"`
	INN  *string `json:"inn,omitempty"`
}

// GraphEdge — направленное ребро владения.
// agent-ubo-tracing сериализует ребро с алиасами from/to, не from_id/to_id
// (см. schemas.py: GraphEdge.model_config{"populate_by_name": True}).
// Поэтому здесь именно "from"/"to" — без нижнего подчёркивания.
type GraphEdge struct {
	From         string  `json:"from"`
	To           string  `json:"to"`
	SharePercent float64 `json:"share_percent"`
}

// UBO — конечный бенефициар, выведенный агентом.
// control_basis: "ownership" | "voting" | "appointment".
type UBO struct {
	PersonID              string     `json:"person_id"`
	Name                  string     `json:"name"`
	EffectiveSharePercent float64    `json:"effective_share_percent"`
	ControlBasis          string     `json:"control_basis"`
	Paths                 [][]string `json:"paths"`
}

// UBOGraph — материализованный snapshot графа владения для одного юрлица.
//
// Версионирование: на каждое (tenant_id, legal_entity_id) допускается N
// строк, каждая со своим version (UNIQUE индекс гарантирует отсутствие
// дублей). «Текущий граф» = MAX(version). Старые версии удерживаются как
// audit-trail (что показывали комплаенсу в момент Decision).
//
// Поля nodes/edges/ubos/unresolved_branches приходят из agent-ubo-tracing
// «as-is» и хранятся в БД как JSONB. На стороне Go это json.RawMessage —
// мы не валидируем глубину структуры в репозитории, это контракт уровня
// schema-validation на edge'е (HTTP handler).
type UBOGraph struct {
	ID                  string          `json:"id"`
	TenantID            string          `json:"tenant_id"`
	LegalEntityID       string          `json:"legal_entity_id"`
	Version             int             `json:"version"`
	Nodes               json.RawMessage `json:"nodes"`
	Edges               json.RawMessage `json:"edges"`
	UBOs                json.RawMessage `json:"ubos"`
	Confidence          float64         `json:"confidence"`
	UnresolvedBranches  json.RawMessage `json:"unresolved_branches"`
	ComputedAt          time.Time       `json:"computed_at"`
	ComputedBy          string          `json:"computed_by"`
}
