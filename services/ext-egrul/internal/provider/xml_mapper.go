package provider

// xml_mapper.go — парсинг XML-ответа ФНС ЕГРЮЛ/ЕГРИП в domain.LegalEntity.
//
// ФНС через СМЭВ-3 отдаёт XML с данными по запросам. Точная схема варьируется
// между методами getCompanyByInn / getEntrepreneurByOgrnip / getFounders, но
// набор полей стабилен. Этот mapper нормализует XML в нашу domain-модель и
// готов к использованию в LiveProvider после получения ФНС-договора (см. live.go).
//
// Mapper — pure-функция, без I/O: принимает []byte XML, возвращает LegalEntity.
// Это позволяет независимо тестировать разбор без HTTP-клиента ФНС.

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"aibank/ext-egrul/internal/domain"
)

// ErrEmptyXML возвращается, когда payload XML пустой или whitespace-only.
var ErrEmptyXML = errors.New("egrul xml mapper: empty payload")

// ErrInvalidXML — XML не парсится / структура не распознана.
var ErrInvalidXML = errors.New("egrul xml mapper: invalid xml structure")

// egrulXMLEnvelope — корневая структура. ФНС может отдавать как
// <egrul-record> (юрлицо) так и <egrip-record> (ИП). Разбираем оба варианта
// через единую структуру с XMLName-fallback.
type egrulXMLEnvelope struct {
	XMLName        xml.Name      `xml:""`
	INN            string        `xml:"inn"`
	OGRN           string        `xml:"ogrn"`
	KPP            string        `xml:"kpp,omitempty"`
	FullName       string        `xml:"full-name"`
	ShortName      string        `xml:"short-name,omitempty"`
	OPFCode        string        `xml:"opf-code,omitempty"`
	OPFName        string        `xml:"opf-name,omitempty"`
	OKVED          string        `xml:"okved,omitempty"`
	OKVEDDesc      string        `xml:"okved-description,omitempty"`
	Address        string        `xml:"address,omitempty"`
	CEO            string        `xml:"ceo,omitempty"`
	CEOPosition    string        `xml:"ceo-position,omitempty"`
	StatusRaw      string        `xml:"status,omitempty"`
	RegisteredAt   string        `xml:"registered-at,omitempty"`
	CharterCapital string        `xml:"charter-capital-rub,omitempty"`
	Founders       founderListXML `xml:"founders,omitempty"`
}

type founderListXML struct {
	Items []founderXML `xml:"founder"`
}

type founderXML struct {
	Type           string `xml:"type,attr"` // "person" | "legal_entity"
	INN            string `xml:"inn,omitempty"`
	OGRN           string `xml:"ogrn,omitempty"`
	FullName       string `xml:"full-name"`
	SharePercent   string `xml:"share-percent,omitempty"`
	ShareKopecks   string `xml:"share-kopecks,omitempty"`
	IsRussianResid string `xml:"is-russian-resident,omitempty"`
}

// MapXMLToLegalEntity парсит XML-ответ ФНС ЕГРЮЛ/ЕГРИП в domain.LegalEntity.
//
// Особенности:
//   - IsIndividual определяется по длине ИНН (10 → юрлицо, 12 → ИП).
//   - StatusRaw нормализуется в enum {"active","liquidated","reorganizing"}.
//   - CharterCapital ожидается в рублях с дробной частью ("12345.67"); конвертируется в копейки.
//   - Дата RegisteredAt поддерживает DD.MM.YYYY (ФНС-формат) и YYYY-MM-DD; нормализуется в YYYY-MM-DD.
//   - OPF предпочитает OPFCode (короткий: ООО, АО, ИП), fallback на OPFName.
//   - Все строки тримятся; пустые опциональные поля сохраняются пустыми.
func MapXMLToLegalEntity(payload []byte) (domain.LegalEntity, error) {
	trimmed := strings.TrimSpace(string(payload))
	if trimmed == "" {
		return domain.LegalEntity{}, ErrEmptyXML
	}

	var env egrulXMLEnvelope
	if err := xml.Unmarshal([]byte(trimmed), &env); err != nil {
		return domain.LegalEntity{}, fmt.Errorf("%w: %v", ErrInvalidXML, err)
	}

	if env.INN == "" || env.OGRN == "" {
		return domain.LegalEntity{}, fmt.Errorf("%w: inn/ogrn required", ErrInvalidXML)
	}

	le := domain.LegalEntity{
		INN:              strings.TrimSpace(env.INN),
		OGRN:             strings.TrimSpace(env.OGRN),
		KPP:              strings.TrimSpace(env.KPP),
		FullName:         strings.TrimSpace(env.FullName),
		ShortName:        strings.TrimSpace(env.ShortName),
		OPF:              normalizeOPF(env.OPFCode, env.OPFName),
		OKVED:            strings.TrimSpace(env.OKVED),
		OKVEDDescription: strings.TrimSpace(env.OKVEDDesc),
		Address:          strings.TrimSpace(env.Address),
		CEO:              strings.TrimSpace(env.CEO),
		CEOPosition:      strings.TrimSpace(env.CEOPosition),
		Status:           normalizeStatus(env.StatusRaw),
		RegisteredAt:     normalizeDate(env.RegisteredAt),
		CharterCapital:   parseRublesToKopecks(env.CharterCapital),
		IsIndividual:     len(env.INN) == 12,
	}

	if len(env.Founders.Items) > 0 {
		le.Founders = make([]domain.Founder, 0, len(env.Founders.Items))
		for _, f := range env.Founders.Items {
			le.Founders = append(le.Founders, mapFounder(f))
		}
	}

	return le, nil
}

// MapXMLToFounders извлекает только список учредителей. Используется когда ФНС
// отвечает узким эндпоинтом /founders без полной info по ЮЛ.
func MapXMLToFounders(payload []byte) ([]domain.Founder, error) {
	le, err := MapXMLToLegalEntity(payload)
	if err != nil {
		return nil, err
	}
	return le.Founders, nil
}

// normalizeOPF — приоритет OPFCode (короткое имя из ФНС-классификатора), fallback OPFName.
// Дополнительно нормализуем известные коды ОКОПФ-2014 в короткие имена.
func normalizeOPF(code, name string) string {
	c := strings.TrimSpace(code)
	if c != "" {
		// Маппинг известных ОКОПФ-2014 кодов → короткие имена.
		switch c {
		case "12300", "12300_ООО":
			return "ООО"
		case "12200", "12247":
			return "АО"
		case "12247_ПАО":
			return "ПАО"
		case "50101", "50102":
			return "ИП"
		}
		return c
	}
	return strings.TrimSpace(name)
}

// normalizeStatus — ФНС возвращает статус в свободной форме («Действующее»,
// «Прекратило деятельность», «В процессе реорганизации»). Приводим к enum.
func normalizeStatus(raw string) string {
	r := strings.ToLower(strings.TrimSpace(raw))
	if r == "" {
		return "active"
	}
	switch {
	case strings.Contains(r, "действ"), strings.Contains(r, "active"):
		return "active"
	case strings.Contains(r, "ликвид"), strings.Contains(r, "прекрат"), strings.Contains(r, "liquidat"):
		return "liquidated"
	case strings.Contains(r, "реорганиз"), strings.Contains(r, "reorganiz"):
		return "reorganizing"
	}
	return "active" // fallback — не блокируем поток на неизвестном статусе
}

// normalizeDate — поддержка DD.MM.YYYY и YYYY-MM-DD, нормализуем в YYYY-MM-DD.
func normalizeDate(raw string) string {
	r := strings.TrimSpace(raw)
	if r == "" {
		return ""
	}
	// YYYY-MM-DD уже корректен.
	if t, err := time.Parse("2006-01-02", r); err == nil {
		return t.Format("2006-01-02")
	}
	// DD.MM.YYYY (ФНС-формат) → YYYY-MM-DD.
	if t, err := time.Parse("02.01.2006", r); err == nil {
		return t.Format("2006-01-02")
	}
	return r // если формат неизвестен — возвращаем как есть, чтобы не терять данные
}

// parseRublesToKopecks — конвертация "12345.67" → 1234567 (копейки).
// Принимает запятую как dec separator (ФНС-формат), точку, или integer без дробной.
// Возвращает 0 при пустой строке или невалидном формате.
func parseRublesToKopecks(raw string) int64 {
	r := strings.ReplaceAll(strings.TrimSpace(raw), ",", ".")
	if r == "" {
		return 0
	}
	// Разделяем на рубли и копейки.
	parts := strings.SplitN(r, ".", 2)
	rub, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0
	}
	var kop int64
	if len(parts) == 2 {
		// Дополняем до 2 знаков (например, "5" → "50" копеек).
		frac := parts[1]
		if len(frac) > 2 {
			frac = frac[:2]
		} else if len(frac) == 1 {
			frac += "0"
		}
		if v, err := strconv.ParseInt(frac, 10, 64); err == nil {
			kop = v
		}
	}
	if rub < 0 {
		return rub*100 - kop
	}
	return rub*100 + kop
}

func mapFounder(f founderXML) domain.Founder {
	t := strings.TrimSpace(f.Type)
	if t == "" {
		// Если тип не указан — определяем по наличию ОГРН.
		if strings.TrimSpace(f.OGRN) != "" {
			t = "legal_entity"
		} else {
			t = "person"
		}
	}
	share := strings.TrimSpace(f.SharePercent)
	if share == "" {
		share = "0.00"
	}

	var shareKop int64
	if k, err := strconv.ParseInt(strings.TrimSpace(f.ShareKopecks), 10, 64); err == nil {
		shareKop = k
	}

	isResident := true
	if v := strings.ToLower(strings.TrimSpace(f.IsRussianResid)); v != "" {
		isResident = v == "true" || v == "1" || v == "да"
	}

	return domain.Founder{
		Type:           t,
		INN:            strings.TrimSpace(f.INN),
		OGRN:           strings.TrimSpace(f.OGRN),
		FullName:       strings.TrimSpace(f.FullName),
		SharePercent:   share,
		ShareKopecks:   shareKop,
		IsRussianResid: isResident,
	}
}
