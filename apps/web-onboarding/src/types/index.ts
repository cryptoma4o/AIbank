// Доменные типы, разделяемые между Apollo-операциями и компонентами.

import type { ApplicationState } from "@/lib/application-states";

export type RiskCategory = "LOW" | "MEDIUM" | "HIGH";
export type DocumentType =
  | "PASSPORT"
  | "CHARTER"
  | "PROTOCOL"
  | "AGREEMENT"
  | "EGRUL_EXTRACT"
  | "POWER_OF_ATTORNEY"
  | "ACCOUNTING_REPORT"
  | "OTHER";

export interface Person {
  id: string;
  fullName: string;
  inn?: string | null;
  phone?: string | null;
  email?: string | null;
}

export interface DocumentRef {
  id: string;
  type: DocumentType;
  applicationId: string;
  filename: string;
  state: string;
  uploadedAt: string;
}

export interface RiskAssessment {
  id: string;
  score: number;
  category: RiskCategory;
  recommendation: string;
  computedAt: string;
}

export interface ApplicationSummary {
  id: string;
  tenantId: string;
  state: ApplicationState;
  legalEntityType: string;
  channel: string;
  productCodes: string[];
  createdAt: string;
  updatedAt: string;
}

export interface ApplicationDetail extends ApplicationSummary {
  applicant?: Person | null;
  documents: DocumentRef[];
  riskAssessment?: RiskAssessment | null;
}
