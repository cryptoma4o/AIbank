package domain

import (
	"encoding/json"
	"time"
)

// DocumentType — классификация документа.
// Соответствует docs/domain-model.md § 2.6 (упрощённый набор для MVP).
type DocumentType string

const (
	DocumentTypePassport        DocumentType = "passport"
	DocumentTypePassportForeign DocumentType = "passport_foreign"
	DocumentTypeCharter         DocumentType = "charter"
	DocumentTypeProtocol        DocumentType = "protocol"
	DocumentTypeExtract         DocumentType = "extract"
	DocumentTypeAgreement       DocumentType = "agreement"
	// Расширение из docs/onboarding-form-spec.md §6 (этап 6 формы).
	// Add-only: старые значения сохранены.
	DocumentTypeEgrulRecordSheet      DocumentType = "egrul_record_sheet"
	DocumentTypeEIOAppointmentProtocol DocumentType = "eio_appointment_protocol"
	DocumentTypeEIOOrder              DocumentType = "eio_order"
	DocumentTypeSignatureCard         DocumentType = "signature_card"
	DocumentTypeLicense               DocumentType = "license"
	DocumentTypeAddressConfirmation   DocumentType = "address_confirmation"
	DocumentTypeFinancialStatements   DocumentType = "financial_statements"
	DocumentTypeTaxDeclaration        DocumentType = "tax_declaration"
	DocumentTypeTaxClearanceCertificate DocumentType = "tax_clearance_certificate"
	DocumentTypeOwnershipChainDiagram DocumentType = "ownership_chain_diagram"
	DocumentTypeFATCAW8Form           DocumentType = "fatca_w8_form"
	DocumentTypeFATCAW9Form           DocumentType = "fatca_w9_form"
	DocumentTypeMigrationCard         DocumentType = "migration_card"
	DocumentTypeResidencePermit       DocumentType = "residence_permit"
	DocumentTypePowerOfAttorney       DocumentType = "power_of_attorney"
	DocumentTypeINNCertificate        DocumentType = "inn_certificate"
	DocumentTypeOGRNCertificate       DocumentType = "ogrn_certificate"
)

// IsValidDocumentType проверяет, входит ли значение в whitelist типов.
func IsValidDocumentType(t DocumentType) bool {
	switch t {
	case DocumentTypePassport, DocumentTypePassportForeign,
		DocumentTypeCharter, DocumentTypeProtocol,
		DocumentTypeExtract, DocumentTypeAgreement,
		DocumentTypeEgrulRecordSheet, DocumentTypeEIOAppointmentProtocol,
		DocumentTypeEIOOrder, DocumentTypeSignatureCard,
		DocumentTypeLicense, DocumentTypeAddressConfirmation,
		DocumentTypeFinancialStatements, DocumentTypeTaxDeclaration,
		DocumentTypeTaxClearanceCertificate, DocumentTypeOwnershipChainDiagram,
		DocumentTypeFATCAW8Form, DocumentTypeFATCAW9Form,
		DocumentTypeMigrationCard, DocumentTypeResidencePermit,
		DocumentTypePowerOfAttorney, DocumentTypeINNCertificate,
		DocumentTypeOGRNCertificate:
		return true
	}
	return false
}

// LegalEntityForm — юр.форма заявителя (определяет required-чеклист).
type LegalEntityForm string

const (
	LegalEntityIP  LegalEntityForm = "IP"  // Индивидуальный предприниматель
	LegalEntityLLC LegalEntityForm = "LLC" // Общество с ограниченной ответственностью
	LegalEntityJSC LegalEntityForm = "JSC" // Акционерное общество
	LegalEntityNPF LegalEntityForm = "NPF" // Некоммерческая партнёрство фонд
)

// RequiredDocumentTypes возвращает список обязательных типов документов
// для заявителя в зависимости от его юр.формы. Соответствует
// docs/onboarding-form-spec.md §6.
func RequiredDocumentTypes(form LegalEntityForm) []DocumentType {
	switch form {
	case LegalEntityIP:
		return []DocumentType{
			DocumentTypePassport,
			DocumentTypeINNCertificate,
			DocumentTypeOGRNCertificate, // ОГРНИП для ИП
			DocumentTypeSignatureCard,
		}
	case LegalEntityLLC, LegalEntityJSC:
		return []DocumentType{
			DocumentTypeCharter,
			DocumentTypeEgrulRecordSheet,
			DocumentTypeEIOAppointmentProtocol,
			DocumentTypeEIOOrder,
			DocumentTypeSignatureCard,
			DocumentTypeAddressConfirmation,
			DocumentTypeFinancialStatements,
			DocumentTypeTaxDeclaration,
		}
	case LegalEntityNPF:
		return []DocumentType{
			DocumentTypeCharter,
			DocumentTypeEgrulRecordSheet,
			DocumentTypeEIOAppointmentProtocol,
			DocumentTypeFinancialStatements,
		}
	}
	return nil
}

// IsValidLegalEntityForm — whitelist форм.
func IsValidLegalEntityForm(f LegalEntityForm) bool {
	switch f {
	case LegalEntityIP, LegalEntityLLC, LegalEntityJSC, LegalEntityNPF:
		return true
	}
	return false
}

// DocumentState — конечный автомат состояний документа.
type DocumentState string

const (
	DocumentStateUploaded  DocumentState = "uploaded"
	DocumentStateParsed    DocumentState = "parsed"
	DocumentStateValidated DocumentState = "validated"
	DocumentStateRejected  DocumentState = "rejected"
)

// SourceType — откуда документ попал в систему.
type SourceType string

const (
	SourceClientUpload SourceType = "client_upload"
	SourceEGRUL        SourceType = "egrul"
	SourceESIA         SourceType = "esia"
	SourceGenerated    SourceType = "generated"
)

// IsValidSourceType проверяет валидность источника.
func IsValidSourceType(s SourceType) bool {
	switch s {
	case SourceClientUpload, SourceEGRUL, SourceESIA, SourceGenerated:
		return true
	}
	return false
}

// Document — единица хранения документа в схеме тенанта.
//
// Файл-контент находится в объектном хранилище (см. internal/storage), здесь —
// только метаданные и ссылка StoragePath. Поле ParsedData — JSON, заполняется
// document-intake-agent после OCR/извлечения; в этом сервисе не интерпретируется.
type Document struct {
	ID            string          `json:"id"`
	TenantID      string          `json:"tenant_id"`
	ApplicationID string          `json:"application_id"`
	Type          DocumentType    `json:"type"`
	State         DocumentState   `json:"state"`
	FileID        string          `json:"file_id"`
	Filename      string          `json:"filename"`
	MimeType      string          `json:"mime_type"`
	SizeBytes     int64           `json:"size_bytes"`
	SHA256        string          `json:"sha256"`
	StoragePath   string          `json:"storage_path"`
	SourceType    SourceType      `json:"source_type"`
	SourceActorID string          `json:"source_actor_id,omitempty"`
	ParsedData    json.RawMessage `json:"parsed_data,omitempty"`
	UploadedAt    time.Time       `json:"uploaded_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}
