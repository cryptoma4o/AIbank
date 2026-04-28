package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// SyntheticLegalEntity детерминированно строит синтетическую запись ЮЛ/ИП по ИНН.
//
// Один и тот же ИНН всегда даст один и тот же результат — это ключевое
// свойство для стабильных интеграционных тестов и кросс-сервисной согласованности.
//
// Не имеет отношения к реальным юрлицам: имена, адреса и КЕО синтезируются из
// фиксированных пулов по хэшу от ИНН.
func SyntheticLegalEntity(inn string) LegalEntity {
	seed := hashSeed(inn)
	isIndividual := len(inn) == 12

	opfPool := []string{"ООО", "АО", "ПАО", "ООО"}
	statusPool := []string{"active", "active", "active", "active", "active", "liquidated", "reorganizing"}
	okvedPool := []string{"62.01", "47.11", "70.22", "46.90", "68.20", "01.41", "10.71", "41.20"}
	okvedDescr := map[string]string{
		"62.01": "Разработка компьютерного программного обеспечения",
		"47.11": "Торговля розничная преимущественно пищевыми продуктами",
		"70.22": "Консультирование по вопросам коммерческой деятельности и управления",
		"46.90": "Торговля оптовая неспециализированная",
		"68.20": "Аренда и управление собственным или арендованным недвижимым имуществом",
		"01.41": "Разведение молочного крупного рогатого скота",
		"10.71": "Производство хлеба и мучных кондитерских изделий",
		"41.20": "Строительство жилых и нежилых зданий",
	}
	cityPool := []string{"г. Москва", "г. Санкт-Петербург", "г. Казань", "г. Новосибирск", "г. Екатеринбург", "г. Краснодар"}
	streetPool := []string{"ул. Ленина", "пр-т Мира", "ул. Гагарина", "ул. Советская", "пер. Лесной", "наб. Реки Фонтанки"}
	ceoLastPool := []string{"Иванов", "Петров", "Сидоров", "Кузнецов", "Смирнов", "Волков", "Морозов", "Соколов"}
	ceoFirstPool := []string{"А.А.", "И.И.", "С.В.", "П.Н.", "Е.М.", "Д.А.", "В.С.", "М.И."}
	namePool := []string{"Альфа", "Бета", "Гамма", "Дельта", "Сигма", "Омега", "Прогресс", "Восход", "Технология", "Развитие"}

	opf := opfPool[seed%uint64(len(opfPool))]
	status := statusPool[(seed/7)%uint64(len(statusPool))]
	okved := okvedPool[(seed/11)%uint64(len(okvedPool))]
	city := cityPool[(seed/13)%uint64(len(cityPool))]
	street := streetPool[(seed/17)%uint64(len(streetPool))]
	houseNum := (seed/19)%200 + 1
	namePart := namePool[(seed/23)%uint64(len(namePool))]
	ceo := ceoLastPool[(seed/29)%uint64(len(ceoLastPool))] + " " + ceoFirstPool[(seed/31)%uint64(len(ceoFirstPool))]

	var fullName, shortName, ogrn, kpp string
	if isIndividual {
		// ИП — индивидуальный предприниматель
		opf = "ИП"
		fullName = fmt.Sprintf("Индивидуальный предприниматель %s", ceo)
		shortName = fmt.Sprintf("ИП %s", ceo)
		ogrn = synthesizeOGRN(inn, true)
	} else {
		fullName = fmt.Sprintf("%s «%s-%s»", opf, namePart, lastN(inn, 4))
		shortName = fmt.Sprintf("%s %s", opf, namePart)
		ogrn = synthesizeOGRN(inn, false)
		kpp = synthesizeKPP(inn)
	}

	// Дата регистрации: 2005..2024
	regYear := 2005 + int((seed/37)%20)
	regMonth := 1 + int((seed/41)%12)
	regDay := 1 + int((seed/43)%28)
	regAt := fmt.Sprintf("%04d-%02d-%02d", regYear, regMonth, regDay)

	// Уставный капитал в копейках: 10_000 руб..10_000_000 руб
	charter := int64(((seed / 47) % 999) * 100000) // 0..99,800,000 копеек
	if charter < 1000000 {
		charter = 1000000 // не меньше 10к рублей
	}

	return LegalEntity{
		INN:              inn,
		OGRN:             ogrn,
		KPP:              kpp,
		FullName:         fullName,
		ShortName:        shortName,
		OPF:              opf,
		OKVED:            okved,
		OKVEDDescription: okvedDescr[okved],
		Address:          fmt.Sprintf("%s, %s, д. %d", city, street, houseNum),
		CEO:              ceo,
		CEOPosition:      pickCEOPosition(opf),
		Status:           status,
		RegisteredAt:     regAt,
		CharterCapital:   charter,
		IsIndividual:     isIndividual,
	}
}

// SyntheticFounders генерирует список учредителей по ИНН.
//
// ИП не имеют учредителей. ЮЛ имеют 1..4 учредителей со 100% распределением.
func SyntheticFounders(inn string) []Founder {
	if len(inn) != 10 {
		// ИП и невалидные форматы — без учредителей
		return []Founder{}
	}

	seed := hashSeed("founders:" + inn)
	count := int((seed % 4) + 1) // 1..4

	founders := make([]Founder, 0, count)
	remaining := 10000 // 100.00% хранится как 10000 базисных пунктов

	lastNamePool := []string{"Иванов", "Петров", "Сидоров", "Кузнецов", "Смирнов", "Васильев", "Попов", "Соколов"}
	firstNamePool := []string{"Александр", "Иван", "Сергей", "Михаил", "Дмитрий", "Андрей", "Алексей"}
	patrPool := []string{"Александрович", "Иванович", "Сергеевич", "Михайлович", "Дмитриевич", "Андреевич"}

	for i := 0; i < count; i++ {
		s := hashSeed(fmt.Sprintf("founder:%s:%d", inn, i))

		// Последний учредитель забирает остаток
		var bps int
		if i == count-1 {
			bps = remaining
		} else {
			// Распределяем «справедливо» от текущего остатка
			maxShare := remaining - (count-i-1)*100 // оставить минимум 1% каждому
			if maxShare < 100 {
				maxShare = 100
			}
			bps = int((s%uint64(maxShare/100)+1)*100)
			if bps > remaining-100*(count-i-1) {
				bps = remaining - 100*(count-i-1)
			}
		}
		remaining -= bps

		isLE := (s % 5) == 0 // ~20% учредителей — юрлица
		f := Founder{
			SharePercent:   fmt.Sprintf("%d.%02d", bps/100, bps%100),
			ShareKopecks:   int64(bps) * 1000, // условная номинальная стоимость
			IsRussianResid: (s%17) != 0,       // ~94% резиденты
		}
		if isLE {
			f.Type = "legal_entity"
			leINN := synthesizeINN(fmt.Sprintf("le:%s:%d", inn, i), 10)
			f.INN = leINN
			f.OGRN = synthesizeOGRN(leINN, false)
			ln := lastNamePool[(s/3)%uint64(len(lastNamePool))]
			f.FullName = fmt.Sprintf("ООО «%s-Холдинг»", ln)
		} else {
			f.Type = "person"
			ln := lastNamePool[(s/3)%uint64(len(lastNamePool))]
			fn := firstNamePool[(s/5)%uint64(len(firstNamePool))]
			pt := patrPool[(s/7)%uint64(len(patrPool))]
			f.FullName = fmt.Sprintf("%s %s %s", ln, fn, pt)
			f.INN = synthesizeINN(fmt.Sprintf("p:%s:%d", inn, i), 12)
		}
		founders = append(founders, f)
	}
	return founders
}

// SyntheticByOGRN ищет/синтезирует запись по ОГРН.
//
// Для детерминизма: ОГРН → ИНН по обратному хэшу, потом стандартный синтез.
// В реальности это лукап в БД, у нас же по ОГРН восстанавливаем «псевдо-ИНН».
func SyntheticByOGRN(ogrn string) LegalEntity {
	innLen := 10
	if len(ogrn) == 15 {
		innLen = 12
	}
	inn := synthesizeINN("ogrn:"+ogrn, innLen)
	le := SyntheticLegalEntity(inn)
	// Уважаем входной ОГРН — пересохраним его, чтобы вызывающий получил то,
	// что искал.
	le.OGRN = ogrn
	return le
}

// hashSeed возвращает детерминированное uint64 от строки.
func hashSeed(s string) uint64 {
	h := sha256.Sum256([]byte(s))
	return binary.BigEndian.Uint64(h[:8])
}

func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func synthesizeOGRN(inn string, isIP bool) string {
	// 13 цифр для ЮЛ, 15 для ИП. Первая цифра «1» (основной реестр).
	target := 13
	prefix := "1"
	if isIP {
		target = 15
		prefix = "3"
	}
	seed := hashSeed("ogrn:" + inn)
	digits := fmt.Sprintf("%d", seed)
	body := digits
	for len(prefix+body) < target {
		body += digits
	}
	return (prefix + body)[:target]
}

func synthesizeKPP(inn string) string {
	if len(inn) < 4 {
		return "770101001"
	}
	return inn[:4] + "01001"
}

func synthesizeINN(seedStr string, length int) string {
	seed := hashSeed(seedStr)
	digits := fmt.Sprintf("%020d", seed)
	if len(digits) >= length {
		return digits[:length]
	}
	return digits + strings.Repeat("0", length-len(digits))
}

func pickCEOPosition(opf string) string {
	switch opf {
	case "ООО":
		return "Генеральный директор"
	case "АО", "ПАО":
		return "Председатель Правления"
	case "ИП":
		return "Индивидуальный предприниматель"
	default:
		return "Директор"
	}
}

// ParseRegisteredAt — утилита (не используется в стабах, но полезно потребителям).
func ParseRegisteredAt(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}
