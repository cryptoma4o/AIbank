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

// ── Этап 3 — AML-сведения о деятельности ───────────────────────────────

export interface MoneyAmount {
  amount: number;
  currency: string;
}

export interface Counterparty {
  name: string;
  inn?: string | null;
  country: string;
  sharePercent: number;
  relationshipType: string;
}

export interface OperationalModel {
  geography?: string[] | null;
  monthlyTurnoverPlanned?: MoneyAmount | null;
  annualTurnoverPlanned?: MoneyAmount | null;
  cashSharePercent?: number | null;
  foreignEconomicActivity: boolean;
  foreignCountries?: string[] | null;
  currencyOperations?: string[] | null;
}

export interface FundsSource {
  category: string;
  description?: string | null;
}

export interface ApplicationActivity {
  id: string;
  applicationId: string;
  businessDescription: string;
  businessCategory: string;
  topSuppliers?: Counterparty[] | null;
  topBuyers?: Counterparty[] | null;
  operationalModel?: OperationalModel | null;
  fundsSource: FundsSource;
  createdAt: string;
  updatedAt: string;
}

// ── Этап 4 — ЕИО и представители ───────────────────────────────────────

export interface IDDocument {
  docType: string;
  series?: string | null;
  number: string;
  issueDate?: string | null;
  expiryDate?: string | null;
  issuedBy?: string | null;
  departmentCode?: string | null;
}

export interface AuthorityInfo {
  position: string;
  authorityBasis: string;
  authorityDocNumber?: string | null;
  authorityDocDate?: string | null;
}

export interface PDLDeclaration {
  isPdl: boolean;
  category?: string | null;
  position?: string | null;
  relation?: string | null;
}

export interface Representative {
  id: string;
  applicationId: string;
  legalEntityId: string;
  lastName: string;
  firstName: string;
  middleName?: string | null;
  birthDate: string;
  birthPlace?: string | null;
  citizenship?: string[] | null;
  inn?: string | null;
  snils?: string | null;
  idDocument: IDDocument;
  authority: AuthorityInfo;
  pdlDeclaration?: PDLDeclaration | null;
  isPrimary: boolean;
  isSignatory: boolean;
  createdAt: string;
  updatedAt: string;
}
