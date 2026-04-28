package domain

// LegalEntity (alias EGRULRecord) описывает запись ФНС ЕГРЮЛ/ЕГРИП.
//
// Используется как для юрлиц (ИНН 10 знаков, ОГРН 13 знаков),
// так и для индивидуальных предпринимателей (ИНН 12 знаков, ОГРНИП 15 знаков).
type LegalEntity struct {
	INN              string    `json:"inn"`
	OGRN             string    `json:"ogrn"`
	KPP              string    `json:"kpp,omitempty"`
	FullName         string    `json:"full_name"`
	ShortName        string    `json:"short_name,omitempty"`
	OPF              string    `json:"opf"` // организационно-правовая форма
	OKVED            string    `json:"okved"`
	OKVEDDescription string    `json:"okved_description,omitempty"`
	Address          string    `json:"address"`
	CEO              string    `json:"ceo"`
	CEOPosition      string    `json:"ceo_position,omitempty"`
	Status           string    `json:"status"` // "active" | "liquidated" | "reorganizing"
	RegisteredAt     string    `json:"registered_at"`
	CharterCapital   int64     `json:"charter_capital_kopecks,omitempty"`
	IsIndividual     bool      `json:"is_individual"`
	Founders         []Founder `json:"founders,omitempty"`
}

// EGRULRecord — backward-compatible alias на LegalEntity.
type EGRULRecord = LegalEntity

// Founder представляет учредителя/участника ЮЛ. Может быть как ФЛ, так и ЮЛ.
type Founder struct {
	Type           string `json:"type"` // "person" | "legal_entity"
	INN            string `json:"inn,omitempty"`
	OGRN           string `json:"ogrn,omitempty"`
	FullName       string `json:"full_name"`
	SharePercent   string `json:"share_percent"`             // "50.00" — храним строкой для точности
	ShareKopecks   int64  `json:"share_kopecks,omitempty"`   // номинальная стоимость доли
	IsRussianResid bool   `json:"is_russian_resident"`
}

// FoundersResult — ответ /v1/egrul/founders/{inn}.
type FoundersResult struct {
	INN      string    `json:"inn"`
	Count    int       `json:"count"`
	Founders []Founder `json:"founders"`
}
