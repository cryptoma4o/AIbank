package provider

import (
	"strings"
	"testing"
)

// sampleEgrulXML — синтетический XML, повторяющий structure ФНС-ответа
// с покрытием всех ключевых полей. Используется в нескольких тестах.
const sampleEgrulXML = `<egrul-record>
  <inn>7707083893</inn>
  <ogrn>1027700132195</ogrn>
  <kpp>770701001</kpp>
  <full-name>Общество с ограниченной ответственностью "ДЕМО-БАНК"</full-name>
  <short-name>ООО "ДЕМО-БАНК"</short-name>
  <opf-code>12300</opf-code>
  <opf-name>Общества с ограниченной ответственностью</opf-name>
  <okved>64.19</okved>
  <okved-description>Прочее денежное посредничество</okved-description>
  <address>123022, г. Москва, ул. Демонстрационная, 1</address>
  <ceo>Иванов Иван Иванович</ceo>
  <ceo-position>Генеральный директор</ceo-position>
  <status>Действующее</status>
  <registered-at>15.03.2010</registered-at>
  <charter-capital-rub>10000000.50</charter-capital-rub>
  <founders>
    <founder type="person">
      <inn>770700000001</inn>
      <full-name>Петров Пётр Петрович</full-name>
      <share-percent>50.00</share-percent>
      <share-kopecks>500000050</share-kopecks>
      <is-russian-resident>true</is-russian-resident>
    </founder>
    <founder type="legal_entity">
      <inn>9909000111</inn>
      <ogrn>1027700111111</ogrn>
      <full-name>ООО "Партнёр"</full-name>
      <share-percent>50.00</share-percent>
      <share-kopecks>500000050</share-kopecks>
      <is-russian-resident>true</is-russian-resident>
    </founder>
  </founders>
</egrul-record>`

func TestMapXMLToLegalEntity_HappyPath(t *testing.T) {
	t.Parallel()
	le, err := MapXMLToLegalEntity([]byte(sampleEgrulXML))
	if err != nil {
		t.Fatalf("Map: %v", err)
	}
	if le.INN != "7707083893" {
		t.Errorf("INN = %q", le.INN)
	}
	if le.OGRN != "1027700132195" {
		t.Errorf("OGRN = %q", le.OGRN)
	}
	if le.KPP != "770701001" {
		t.Errorf("KPP = %q", le.KPP)
	}
	if le.OPF != "ООО" {
		t.Errorf("OPF = %q, want ООО (через OPFCode маппинг)", le.OPF)
	}
	if le.Status != "active" {
		t.Errorf("Status = %q, want active (нормализация \"Действующее\")", le.Status)
	}
	if le.RegisteredAt != "2010-03-15" {
		t.Errorf("RegisteredAt = %q, want 2010-03-15 (DD.MM.YYYY → ISO)", le.RegisteredAt)
	}
	if le.CharterCapital != 1000000050 {
		t.Errorf("CharterCapital = %d kopecks, want 1000000050 (10000000.50 руб)", le.CharterCapital)
	}
	if le.IsIndividual {
		t.Errorf("IsIndividual = true, want false (10-знаков ИНН — юрлицо)")
	}
	if len(le.Founders) != 2 {
		t.Fatalf("Founders count = %d, want 2", len(le.Founders))
	}
	if le.Founders[0].Type != "person" || le.Founders[1].Type != "legal_entity" {
		t.Errorf("Founder types = (%q, %q)", le.Founders[0].Type, le.Founders[1].Type)
	}
	if le.Founders[1].OGRN != "1027700111111" {
		t.Errorf("Founder[1].OGRN = %q", le.Founders[1].OGRN)
	}
}

func TestMapXMLToLegalEntity_IndividualEntrepreneur(t *testing.T) {
	t.Parallel()
	xml := `<egrip-record>
		<inn>770700000012</inn>
		<ogrn>304770000000001</ogrn>
		<full-name>ИП Сидоров Сидор Сидорович</full-name>
		<opf-code>50102</opf-code>
		<status>Действующее</status>
		<registered-at>2015-06-20</registered-at>
	</egrip-record>`
	le, err := MapXMLToLegalEntity([]byte(xml))
	if err != nil {
		t.Fatalf("Map: %v", err)
	}
	if !le.IsIndividual {
		t.Errorf("IsIndividual = false, want true (12-знаков ИНН)")
	}
	if le.OPF != "ИП" {
		t.Errorf("OPF = %q, want ИП", le.OPF)
	}
	if le.RegisteredAt != "2015-06-20" {
		t.Errorf("RegisteredAt = %q (YYYY-MM-DD как есть)", le.RegisteredAt)
	}
	if len(le.Founders) != 0 {
		t.Errorf("ИП не должен иметь founders, got %d", len(le.Founders))
	}
}

func TestMapXMLToLegalEntity_StatusNormalization(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw  string
		want string
	}{
		{"Действующее", "active"},
		{"ДЕЙСТВУЮЩЕЕ ЮЛ", "active"},
		{"active", "active"},
		{"Ликвидировано", "liquidated"},
		{"Прекратило деятельность", "liquidated"},
		{"liquidated", "liquidated"},
		{"В процессе реорганизации", "reorganizing"},
		{"reorganizing", "reorganizing"},
		{"", "active"}, // дефолт
		{"неизвестный_статус", "active"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.raw, func(t *testing.T) {
			t.Parallel()
			if got := normalizeStatus(c.raw); got != c.want {
				t.Errorf("normalizeStatus(%q) = %q, want %q", c.raw, got, c.want)
			}
		})
	}
}

func TestParseRublesToKopecks(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want int64
	}{
		{"", 0},
		{"   ", 0},
		{"100", 10000},
		{"100.5", 10050},
		{"100.50", 10050},
		{"100,50", 10050},      // запятая как separator (ФНС-формат)
		{"10000000.50", 1000000050},
		{"0.99", 99},
		{"0", 0},
		{"-100.50", -10050},
		{"abc", 0}, // невалидное → 0, не паника
		{"100.5678", 10056}, // больше 2 знаков — труncate
	}
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()
			if got := parseRublesToKopecks(c.in); got != c.want {
				t.Errorf("parseRublesToKopecks(%q) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

func TestNormalizeDate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"15.03.2010", "2010-03-15"},
		{"2010-03-15", "2010-03-15"},
		{"01.01.2024", "2024-01-01"},
		{"неизвестная_дата", "неизвестная_дата"}, // не теряем данные
	}
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()
			if got := normalizeDate(c.in); got != c.want {
				t.Errorf("normalizeDate(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestMapXMLToLegalEntity_Errors(t *testing.T) {
	t.Parallel()

	t.Run("empty payload", func(t *testing.T) {
		_, err := MapXMLToLegalEntity([]byte(""))
		if err != ErrEmptyXML {
			t.Errorf("err = %v, want ErrEmptyXML", err)
		}
	})

	t.Run("whitespace only", func(t *testing.T) {
		_, err := MapXMLToLegalEntity([]byte("   \n\t  "))
		if err != ErrEmptyXML {
			t.Errorf("err = %v, want ErrEmptyXML", err)
		}
	})

	t.Run("malformed xml", func(t *testing.T) {
		_, err := MapXMLToLegalEntity([]byte("<not-closed>"))
		if err == nil || !strings.Contains(err.Error(), "egrul xml mapper") {
			t.Errorf("err = %v, want ErrInvalidXML wrap", err)
		}
	})

	t.Run("missing inn", func(t *testing.T) {
		xml := `<egrul-record><ogrn>1234567890123</ogrn></egrul-record>`
		_, err := MapXMLToLegalEntity([]byte(xml))
		if err == nil || !strings.Contains(err.Error(), "inn/ogrn required") {
			t.Errorf("err = %v, want inn/ogrn required", err)
		}
	})
}

func TestMapXMLToFounders(t *testing.T) {
	t.Parallel()
	founders, err := MapXMLToFounders([]byte(sampleEgrulXML))
	if err != nil {
		t.Fatalf("MapXMLToFounders: %v", err)
	}
	if len(founders) != 2 {
		t.Fatalf("count = %d, want 2", len(founders))
	}
	if founders[0].SharePercent != "50.00" || founders[1].SharePercent != "50.00" {
		t.Errorf("share percents = (%q, %q)", founders[0].SharePercent, founders[1].SharePercent)
	}
}

func TestMapFounder_TypeInference(t *testing.T) {
	t.Parallel()
	// Учредитель без явного type, но с OGRN → legal_entity.
	xml := `<egrul-record>
		<inn>1234567890</inn>
		<ogrn>1027700000001</ogrn>
		<full-name>Test</full-name>
		<founders>
			<founder>
				<ogrn>1027700111222</ogrn>
				<full-name>Без типа но с ОГРН</full-name>
				<share-percent>100.00</share-percent>
			</founder>
		</founders>
	</egrul-record>`
	le, err := MapXMLToLegalEntity([]byte(xml))
	if err != nil {
		t.Fatalf("Map: %v", err)
	}
	if le.Founders[0].Type != "legal_entity" {
		t.Errorf("Founder.Type = %q, want legal_entity (inferred from OGRN)", le.Founders[0].Type)
	}
}
