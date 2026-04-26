package domain

type EGRULRecord struct {
	INN      string `json:"inn"`
	OGRN     string `json:"ogrn"`
	FullName string `json:"full_name"`
	OKVED    string `json:"okved"`
	Address  string `json:"address"`
	CEO      string `json:"ceo"`
	Status   string `json:"status"` // "active" | "liquidated" | "reorganizing"
}
