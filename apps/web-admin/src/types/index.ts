// types/index.ts — доменные типы web-admin.
//
// Поля и набор типов соответствуют bff-admin/graph/schema.graphqls.

import type { ApplicationState } from "@/lib/application-states";

export type RiskCategory = "LOW" | "MEDIUM" | "HIGH";

export type DecisionKind =
  | "APPROVED"
  | "APPROVED_WITH_EDD"
  | "DECLINED"
  | "ESCALATED";

export interface Tenant {
  id: string;
  name: string;
  bik: string;
  inn: string;
  status: string;
  deploymentMode: string;
}

export interface RiskAssessment {
  id: string;
  applicationId?: string;
  score: number;
  category: RiskCategory;
  recommendation: string;
  computedAt: string;
  /** factors — массив произвольных JSON-объектов от risk-scoring-service. */
  factors?: RiskFactor[] | null;
}

/**
 * RiskFactor — слабо-типизированный объект.  bff-admin отдаёт factors как
 * JSON-scalar; на стороне UI мы лояльно достаём поля weight/code/label.
 */
export interface RiskFactor {
  code?: string;
  label?: string;
  weight?: number;
  direction?: "positive" | "negative";
  details?: string;
  [key: string]: unknown;
}

export interface Decision {
  id: string;
  applicationId: string;
  decision: DecisionKind;
  reasoning: string;
  decidedAt: string;
}

export interface ApplicationSummary {
  id: string;
  tenantId: string;
  applicantId?: string | null;
  state: ApplicationState;
  legalEntityType: string;
  channel: string;
  productCodes: string[];
  createdAt: string;
  updatedAt: string;
  riskAssessment?: RiskAssessment | null;
  decision?: Decision | null;
}

export interface ApplicationDetail extends ApplicationSummary {
  // bff-admin v0.1 не отдаёт applicant и documents отдельно через
  // graph (см. ADR-0003 — только тонкая обёртка над orchestrator).
  // Когда добавим — расширим тип; компоненты уже готовы (см. DocumentList).
}

export interface AuditEvent {
  id: string;
  tenantId: string;
  actorType: string;
  actorId: string;
  action: string;
  subjectType: string;
  subjectId: string;
  occurredAt: string;
  data?: Record<string, unknown> | null;
}

export interface TenantConfig {
  tenantId: string;
  schemaVersion: string;
  rawConfig: string;
  updatedAt: string;
}
