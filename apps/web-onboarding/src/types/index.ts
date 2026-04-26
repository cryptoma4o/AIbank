export type CompanyType = "ip" | "ooo" | "ao";

export interface CompanyInfo {
  inn: string;
  ogrn: string;
  fullName: string;
  type: CompanyType;
}

export interface ApplicationStatus {
  id: string;
  status: string;
  inn: string;
  tenantId: string;
}

export interface ChatMessage {
  role: "user" | "assistant";
  content: string;
}
