// Package handler — HTTP-обработчики имитатора SMEV3-эндпоинта ФНС ЕГРЮЛ/ЕГРИП.
//
// Mock-сервер отдаёт XML, повторяющий структуру реального ФНС-ответа (см.
// services/ext-egrul/internal/provider/xml_mapper.go и xml_mapper_test.go).
// Используется для pre-integration сценариев: LiveProvider → SOAP/HTTP → mock
// сервер → XML mapper → LegalEntity, без реального доступа к ФНС.
//
// Поведение:
//   - GET /by-inn/{inn}   — XML карточка ЮЛ/ИП (детерминированно от хэша ИНН).
//   - GET /by-ogrn/{ogrn} — XML карточка по ОГРН (детерминированно).
//   - GET /by-inn/9999999999 — специально 404 (для теста not-found).
//   - GET /healthz        — liveness.
package handler

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// NotFoundINN — специальный ИНН, для которого mock возвращает 404. Используется
// в тестах ext-egrul для проверки маппинга «нет данных» → ErrNotFound.
const NotFoundINN = "9999999999"

// New собирает chi-роутер с handler'ами mock-smev.
func New(logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &Handler{logger: logger}

	r := chi.NewRouter()
	r.Get("/healthz", h.healthz)
	r.Get("/by-inn/{inn}", h.byINN)
	r.Get("/by-ogrn/{ogrn}", h.byOGRN)
	return r
}

// Handler — состояние mock-сервера (только логгер, in-memory без БД).
type Handler struct {
	logger *slog.Logger
}

func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok","service":"mock-smev"}`))
}

// byINN отвечает XML-карточкой по ИНН. Для NotFoundINN отдаёт 404.
func (h *Handler) byINN(w http.ResponseWriter, r *http.Request) {
	inn := chi.URLParam(r, "inn")
	if err := validateINN(inn); err != nil {
		respondPlain(w, http.StatusBadRequest, err.Error())
		return
	}
	if inn == NotFoundINN {
		h.logger.Info("mock-smev: not-found inn", "inn", inn)
		respondPlain(w, http.StatusNotFound, "egrul record not found")
		return
	}

	xml := buildEgrulXML(inn, "")
	h.logger.Info("mock-smev: by-inn served", "inn", inn, "bytes", len(xml))
	respondXML(w, http.StatusOK, xml)
}

// byOGRN отвечает XML-карточкой по ОГРН. Для пустых/неверных формата — 400.
func (h *Handler) byOGRN(w http.ResponseWriter, r *http.Request) {
	ogrn := chi.URLParam(r, "ogrn")
	if err := validateOGRN(ogrn); err != nil {
		respondPlain(w, http.StatusBadRequest, err.Error())
		return
	}

	// По ОГРН синтезируем псевдо-ИНН (длина 10 для 13-знака, 12 для 15-знака).
	innLen := 10
	if len(ogrn) == 15 {
		innLen = 12
	}
	inn := synthesizeINN("ogrn:"+ogrn, innLen)
	xml := buildEgrulXML(inn, ogrn)
	h.logger.Info("mock-smev: by-ogrn served", "ogrn", ogrn, "inn", inn, "bytes", len(xml))
	respondXML(w, http.StatusOK, xml)
}

// --- XML builder ---

// buildEgrulXML строит XML-карточку, совместимую с MapXMLToLegalEntity.
//
// Если overrideOGRN пуст — ОГРН синтезируется из ИНН; иначе используется
// переданный (для GetByOGRN, чтобы вернуть тот же ОГРН, что был запрошен).
func buildEgrulXML(inn, overrideOGRN string) string {
	seed := hashSeed(inn)
	isIP := len(inn) == 12

	rootTag := "egrul-record"
	if isIP {
		rootTag = "egrip-record"
	}

	ogrn := overrideOGRN
	if ogrn == "" {
		ogrn = synthesizeOGRN(inn, isIP)
	}

	opfPool := []string{"ООО", "АО", "ПАО"}
	statusPool := []string{"Действующее", "Действующее", "Действующее", "Ликвидировано", "В процессе реорганизации"}
	okvedPool := []string{"62.01", "47.11", "70.22", "46.90", "68.20"}
	okvedDescr := map[string]string{
		"62.01": "Разработка компьютерного программного обеспечения",
		"47.11": "Торговля розничная преимущественно пищевыми продуктами",
		"70.22": "Консультирование по вопросам коммерческой деятельности и управления",
		"46.90": "Торговля оптовая неспециализированная",
		"68.20": "Аренда и управление собственным или арендованным недвижимым имуществом",
	}
	cityPool := []string{"г. Москва", "г. Санкт-Петербург", "г. Казань"}
	streetPool := []string{"ул. Ленина", "пр-т Мира", "ул. Гагарина"}
	ceoLastPool := []string{"Иванов", "Петров", "Сидоров", "Кузнецов"}
	ceoFirstPool := []string{"Иван Иванович", "Сергей Петрович", "Михаил Андреевич"}
	namePool := []string{"Альфа", "Бета", "Гамма", "Прогресс"}

	opfShort := opfPool[seed%uint64(len(opfPool))]
	if isIP {
		opfShort = "ИП"
	}
	status := statusPool[(seed/7)%uint64(len(statusPool))]
	okved := okvedPool[(seed/11)%uint64(len(okvedPool))]
	city := cityPool[(seed/13)%uint64(len(cityPool))]
	street := streetPool[(seed/17)%uint64(len(streetPool))]
	houseNum := (seed/19)%200 + 1
	namePart := namePool[(seed/23)%uint64(len(namePool))]
	ceo := ceoLastPool[(seed/29)%uint64(len(ceoLastPool))] + " " + ceoFirstPool[(seed/31)%uint64(len(ceoFirstPool))]

	// Mapping ОПФ → opf-code (см. xml_mapper.normalizeOPF).
	opfCode := "12300" // ООО
	switch opfShort {
	case "АО":
		opfCode = "12200"
	case "ПАО":
		opfCode = "12247_ПАО"
	case "ИП":
		opfCode = "50102"
	}

	regYear := 2005 + int((seed/37)%20)
	regMonth := 1 + int((seed/41)%12)
	regDay := 1 + int((seed/43)%28)
	regAt := fmt.Sprintf("%02d.%02d.%04d", regDay, regMonth, regYear) // ФНС-формат DD.MM.YYYY

	var fullName, shortName, kpp, charterCapital string
	var foundersBlock string

	if isIP {
		fullName = fmt.Sprintf("Индивидуальный предприниматель %s", ceo)
		shortName = fmt.Sprintf("ИП %s", ceo)
	} else {
		fullName = fmt.Sprintf("%s «%s-%s»", opfShort, namePart, lastN(inn, 4))
		shortName = fmt.Sprintf("%s %s", opfShort, namePart)
		if len(inn) >= 4 {
			kpp = inn[:4] + "01001"
		}
		// Уставный капитал в рублях (xml_mapper парсит "12345.67" → копейки).
		rub := 100000 + int(seed/47)%9900000
		charterCapital = fmt.Sprintf("%d.00", rub)
		foundersBlock = buildFoundersXML(inn)
	}

	address := fmt.Sprintf("%s, %s, д. %d", city, street, houseNum)

	var b strings.Builder
	b.Grow(1024)
	fmt.Fprintf(&b, "<%s>\n", rootTag)
	fmt.Fprintf(&b, "  <inn>%s</inn>\n", xmlEscape(inn))
	fmt.Fprintf(&b, "  <ogrn>%s</ogrn>\n", xmlEscape(ogrn))
	if kpp != "" {
		fmt.Fprintf(&b, "  <kpp>%s</kpp>\n", xmlEscape(kpp))
	}
	fmt.Fprintf(&b, "  <full-name>%s</full-name>\n", xmlEscape(fullName))
	fmt.Fprintf(&b, "  <short-name>%s</short-name>\n", xmlEscape(shortName))
	fmt.Fprintf(&b, "  <opf-code>%s</opf-code>\n", xmlEscape(opfCode))
	fmt.Fprintf(&b, "  <opf-name>%s</opf-name>\n", xmlEscape(opfShort))
	fmt.Fprintf(&b, "  <okved>%s</okved>\n", xmlEscape(okved))
	fmt.Fprintf(&b, "  <okved-description>%s</okved-description>\n", xmlEscape(okvedDescr[okved]))
	fmt.Fprintf(&b, "  <address>%s</address>\n", xmlEscape(address))
	fmt.Fprintf(&b, "  <ceo>%s</ceo>\n", xmlEscape(ceo))
	fmt.Fprintf(&b, "  <ceo-position>%s</ceo-position>\n", xmlEscape(pickCEOPosition(opfShort)))
	fmt.Fprintf(&b, "  <status>%s</status>\n", xmlEscape(status))
	fmt.Fprintf(&b, "  <registered-at>%s</registered-at>\n", xmlEscape(regAt))
	if charterCapital != "" {
		fmt.Fprintf(&b, "  <charter-capital-rub>%s</charter-capital-rub>\n", xmlEscape(charterCapital))
	}
	if foundersBlock != "" {
		b.WriteString(foundersBlock)
	}
	fmt.Fprintf(&b, "</%s>", rootTag)
	return b.String()
}

// buildFoundersXML — детерминированный список из 1..2 учредителей.
// ИП не имеют учредителей (вызов не делается).
func buildFoundersXML(inn string) string {
	seed := hashSeed("founders:" + inn)
	count := int((seed % 2) + 1) // 1..2 — упрощённо для mock

	lastNamePool := []string{"Иванов", "Петров", "Сидоров", "Кузнецов"}
	firstNamePool := []string{"Александр", "Иван", "Сергей", "Михаил"}
	patrPool := []string{"Александрович", "Иванович", "Сергеевич", "Михайлович"}

	var b strings.Builder
	b.WriteString("  <founders>\n")
	remaining := 10000
	for i := 0; i < count; i++ {
		s := hashSeed(fmt.Sprintf("founder:%s:%d", inn, i))
		var bps int
		if i == count-1 {
			bps = remaining
		} else {
			bps = int(s%uint64(remaining/100/2)+1) * 100
			if bps > remaining-100 {
				bps = remaining - 100
			}
		}
		remaining -= bps
		isLE := (s % 5) == 0

		share := fmt.Sprintf("%d.%02d", bps/100, bps%100)
		shareKop := int64(bps) * 1000

		if isLE {
			leINN := synthesizeINN(fmt.Sprintf("le:%s:%d", inn, i), 10)
			leOGRN := synthesizeOGRN(leINN, false)
			ln := lastNamePool[(s/3)%uint64(len(lastNamePool))]
			fmt.Fprintf(&b, "    <founder type=\"legal_entity\">\n")
			fmt.Fprintf(&b, "      <inn>%s</inn>\n", xmlEscape(leINN))
			fmt.Fprintf(&b, "      <ogrn>%s</ogrn>\n", xmlEscape(leOGRN))
			fmt.Fprintf(&b, "      <full-name>ООО «%s-Холдинг»</full-name>\n", xmlEscape(ln))
			fmt.Fprintf(&b, "      <share-percent>%s</share-percent>\n", share)
			fmt.Fprintf(&b, "      <share-kopecks>%d</share-kopecks>\n", shareKop)
			fmt.Fprintf(&b, "      <is-russian-resident>true</is-russian-resident>\n")
			fmt.Fprintf(&b, "    </founder>\n")
		} else {
			ln := lastNamePool[(s/3)%uint64(len(lastNamePool))]
			fn := firstNamePool[(s/5)%uint64(len(firstNamePool))]
			pt := patrPool[(s/7)%uint64(len(patrPool))]
			pINN := synthesizeINN(fmt.Sprintf("p:%s:%d", inn, i), 12)
			fmt.Fprintf(&b, "    <founder type=\"person\">\n")
			fmt.Fprintf(&b, "      <inn>%s</inn>\n", xmlEscape(pINN))
			fmt.Fprintf(&b, "      <full-name>%s %s %s</full-name>\n", xmlEscape(ln), xmlEscape(fn), xmlEscape(pt))
			fmt.Fprintf(&b, "      <share-percent>%s</share-percent>\n", share)
			fmt.Fprintf(&b, "      <share-kopecks>%d</share-kopecks>\n", shareKop)
			fmt.Fprintf(&b, "      <is-russian-resident>true</is-russian-resident>\n")
			fmt.Fprintf(&b, "    </founder>\n")
		}
	}
	b.WriteString("  </founders>\n")
	return b.String()
}

// --- helpers ---

func validateINN(inn string) error {
	if len(inn) != 10 && len(inn) != 12 {
		return fmt.Errorf("invalid inn: expected 10 or 12 digits, got %d", len(inn))
	}
	for _, c := range inn {
		if c < '0' || c > '9' {
			return fmt.Errorf("invalid inn: non-digit character")
		}
	}
	return nil
}

func validateOGRN(ogrn string) error {
	if len(ogrn) != 13 && len(ogrn) != 15 {
		return fmt.Errorf("invalid ogrn: expected 13 or 15 digits, got %d", len(ogrn))
	}
	for _, c := range ogrn {
		if c < '0' || c > '9' {
			return fmt.Errorf("invalid ogrn: non-digit character")
		}
	}
	return nil
}

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

// xmlEscape — минимальный escape для XML-контекста.
func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}

func respondXML(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func respondPlain(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
