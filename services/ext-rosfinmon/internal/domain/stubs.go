package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// SyntheticList — детерминированный синтетический перечень 115-ФЗ.
//
// Состав фиксирован, имена/ИНН синтетические. ~50 записей разных типов списка.
// Используется для list-info и как «БД» для маловероятных совпадений по ИНН.
//
// Любые совпадения с реальными лицами — случайны.
func SyntheticList() []SanctionedEntry {
	if cachedList != nil {
		return cachedList
	}
	out := make([]SanctionedEntry, 0, 50)
	listTypes := []string{"terrorist", "extremist", "extremist", "proliferation"}
	lastNames := []string{"Тестов", "Иванов", "Петров", "Сидоров", "Кузнецов", "Соколов", "Морозов", "Волков", "Лебедев", "Новиков"}
	firstNames := []string{"Александр", "Иван", "Сергей", "Михаил", "Дмитрий", "Андрей", "Алексей", "Николай"}
	patrs := []string{"Александрович", "Иванович", "Сергеевич", "Михайлович", "Дмитриевич", "Петрович"}
	leNames := []string{"Альфа-Тест", "Заглушка", "Синтетика", "Прокси-Стаб", "Тест-Холдинг"}

	for i := 0; i < 35; i++ {
		s := hashSeed(fmt.Sprintf("rfm:person:%d", i))
		ln := lastNames[s%uint64(len(lastNames))]
		fn := firstNames[(s/3)%uint64(len(firstNames))]
		pt := patrs[(s/5)%uint64(len(patrs))]
		year := 1960 + int((s/7)%50)
		month := 1 + int((s/11)%12)
		day := 1 + int((s/13)%28)
		out = append(out, SanctionedEntry{
			ID:        fmt.Sprintf("RFM-P-%05d", i),
			INN:       synthesizeINN(fmt.Sprintf("rfm:p:%d", i), 12),
			FullName:  fmt.Sprintf("%s %s %s", ln, fn, pt),
			BirthDate: fmt.Sprintf("%04d-%02d-%02d", year, month, day),
			ListType:  listTypes[s%uint64(len(listTypes))],
			AddedAt:   "2023-01-15",
		})
	}
	for i := 0; i < 15; i++ {
		s := hashSeed(fmt.Sprintf("rfm:le:%d", i))
		nm := leNames[s%uint64(len(leNames))]
		out = append(out, SanctionedEntry{
			ID:       fmt.Sprintf("RFM-L-%05d", i),
			INN:      synthesizeINN(fmt.Sprintf("rfm:le:%d", i), 10),
			FullName: fmt.Sprintf("ООО «%s-%d»", nm, i),
			ListType: listTypes[s%uint64(len(listTypes))],
			AddedAt:  "2023-06-20",
		})
	}
	cachedList = out
	return out
}

var cachedList []SanctionedEntry

// CurrentSnapshot возвращает метаданные перечня.
func CurrentSnapshot() ListSnapshot {
	list := SyntheticList()
	return ListSnapshot{
		LastUpdated: time.Date(2025, 1, 15, 6, 0, 0, 0, time.UTC),
		ListCount:   len(list),
		Source:      "stub:115-FZ",
		Version:     "2025-01-15",
	}
}

// Screen применяет детерминированную логику матчинга.
//
// Стратегия:
//  1. Если ИНН субъекта присутствует в перечне — точное совпадение, confidence=1.0.
//  2. Иначе — псевдо-фуззи по хэшу (FullName+BirthDate).
//     ~1% запросов вернут «matched=true» с confidence ~0.85, чтобы интеграционные
//     тесты ловили обработку false-positive case.
//  3. Иначе — matched=false.
func Screen(req ScreeningRequest) ScreeningResult {
	now := time.Now().UTC()

	list := SyntheticList()

	// 1) Прямой матч по ИНН
	if req.Identifiers.INN != "" {
		for _, e := range list {
			if e.INN == req.Identifiers.INN {
				return ScreeningResult{
					Matched:         true,
					ListName:        "115-FZ:" + e.ListType,
					MatchConfidence: 1.0,
					SourceRecord:    entryToMap(e),
					CheckedAt:       now,
				}
			}
		}
	}

	// 2) Стохастический псевдо-матч по 1% (детерминированно от входа)
	key := strings.ToLower(strings.TrimSpace(req.Identifiers.FullName)) +
		"|" + req.Identifiers.BirthDate +
		"|" + req.Identifiers.INN
	h := hashSeed("screen:" + key)
	// 1% от 0..99 = ровно одно значение → matched
	if h%100 == 7 {
		// «Фантомный» источник — ближайшая запись из перечня по индексу
		idx := int((h / 100) % uint64(len(list)))
		return ScreeningResult{
			Matched:         true,
			ListName:        "115-FZ:" + list[idx].ListType,
			MatchConfidence: 0.85,
			SourceRecord:    entryToMap(list[idx]),
			CheckedAt:       now,
		}
	}

	return ScreeningResult{
		Matched:         false,
		MatchConfidence: 0.0,
		CheckedAt:       now,
	}
}

func entryToMap(e SanctionedEntry) map[string]interface{} {
	m := map[string]interface{}{
		"id":        e.ID,
		"full_name": e.FullName,
		"list_type": e.ListType,
		"added_at":  e.AddedAt,
	}
	if e.INN != "" {
		m["inn"] = e.INN
	}
	if e.BirthDate != "" {
		m["birth_date"] = e.BirthDate
	}
	return m
}

func hashSeed(s string) uint64 {
	h := sha256.Sum256([]byte(s))
	return binary.BigEndian.Uint64(h[:8])
}

func synthesizeINN(seedStr string, length int) string {
	seed := hashSeed(seedStr)
	digits := fmt.Sprintf("%020d", seed)
	if len(digits) >= length {
		return digits[:length]
	}
	return digits + strings.Repeat("0", length-len(digits))
}
