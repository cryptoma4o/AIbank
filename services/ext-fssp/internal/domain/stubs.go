package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
)

// SyntheticByINN детерминированно строит набор исполнительных производств по ИНН.
//
// Стратегия:
//   - примерно 5% ИНН имеют ≥1 активное производство (по hash%20 == 0);
//   - количество производств 1..5;
//   - сумма каждого 1_000..5_000_000 рублей (в копейках).
func SyntheticByINN(inn string) ProceedingsResult {
	subject := "INN:" + inn
	seed := hashSeed("fssp:inn:" + inn)
	if seed%20 != 0 { // ~5%
		return ProceedingsResult{Subject: subject, Count: 0, Items: []Proceeding{}}
	}
	count := int((seed/3)%5) + 1
	items := make([]Proceeding, 0, count)
	var total int64
	debtor := synthesizeDebtor(inn, seed)

	for i := 0; i < count; i++ {
		s := hashSeed(fmt.Sprintf("fssp:inn:%s:%d", inn, i))
		p := buildProceeding(i, s, debtor)
		items = append(items, p)
		total += p.SumKopecks
	}
	return ProceedingsResult{
		Subject:         subject,
		Count:           count,
		TotalDebtKopecks: total,
		Items:           items,
	}
}

// SyntheticByPerson — то же для физлица (ключ — FullName + BirthDate).
func SyntheticByPerson(fullName, birthDate string) ProceedingsResult {
	subject := "PERSON:" + fullName + "|" + birthDate
	key := strings.ToLower(strings.TrimSpace(fullName)) + "|" + birthDate
	seed := hashSeed("fssp:person:" + key)
	if seed%20 != 0 { // ~5%
		return ProceedingsResult{Subject: subject, Count: 0, Items: []Proceeding{}}
	}
	count := int((seed/3)%4) + 1
	items := make([]Proceeding, 0, count)
	var total int64
	for i := 0; i < count; i++ {
		s := hashSeed(fmt.Sprintf("fssp:person:%s:%d", key, i))
		p := buildProceeding(i, s, fullName)
		items = append(items, p)
		total += p.SumKopecks
	}
	return ProceedingsResult{
		Subject:         subject,
		Count:           count,
		TotalDebtKopecks: total,
		Items:           items,
	}
}

func buildProceeding(idx int, seed uint64, debtor string) Proceeding {
	docTypes := []string{"Исполнительный лист", "Судебный приказ", "Постановление о взыскании"}
	courts := []string{
		"Арбитражный суд г. Москвы",
		"Тверской районный суд г. Москвы",
		"Арбитражный суд Санкт-Петербурга и Ленинградской области",
		"Мировой судья судебного участка №42",
		"Замоскворецкий районный суд г. Москвы",
	}
	bailiffs := []string{
		"ОСП по ЦАО №1 УФССП России по г. Москве",
		"Тверское РОСП УФССП России по г. Москве",
		"Калининское РОСП УФССП России по СПб",
	}
	statuses := []string{"active", "active", "active", "active", "closed"} // ~80% активных

	// Сумма 1_000..5_000_000 руб в копейках = 100_000..500_000_000 коп
	rub := int64((seed/7)%5_000_000) + 1_000
	sum := rub * 100

	year := 2019 + int((seed/11)%6) // 2019..2024
	month := 1 + int((seed/13)%12)
	day := 1 + int((seed/17)%28)

	return Proceeding{
		ID:            fmt.Sprintf("%d/%d-ИП", 100000+(seed/19)%900000, year),
		Debtor:        debtor,
		SumKopecks:    sum,
		Currency:      "RUB",
		StartedAt:     fmt.Sprintf("%04d-%02d-%02d", year, month, day),
		DocumentType:  docTypes[(seed/23)%uint64(len(docTypes))],
		Court:         courts[(seed/29)%uint64(len(courts))],
		Status:        statuses[(seed/31)%uint64(len(statuses))],
		BailiffOffice: bailiffs[(seed/37)%uint64(len(bailiffs))],
	}
}

func synthesizeDebtor(inn string, seed uint64) string {
	if len(inn) == 12 {
		// ИП
		return fmt.Sprintf("ИП Тестов А.А. (ИНН %s)", inn)
	}
	pool := []string{"Альфа", "Бета", "Гамма", "Дельта", "Сигма", "Омега"}
	return fmt.Sprintf("ООО «%s-%s» (ИНН %s)", pool[seed%uint64(len(pool))], lastN(inn, 4), inn)
}

func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func hashSeed(s string) uint64 {
	h := sha256.Sum256([]byte(s))
	return binary.BigEndian.Uint64(h[:8])
}
