//go:build contract

// Package contract — golden-fixture-based contract tests for abs-adapter-cft.
//
// Эти тесты — pre-integration слой между abs-adapter-cft и реальным ЦФТ
// (Group A блокер пилота, см. ADR-0006 § 5 «golden-tests + mock-CFT»).
// Они выполняют две независимые проверки:
//
//  1. Canonical round-trip: POST /v1/execute с каноническим ABSCommand
//     должен вернуть детерминированный ABSResponse, побайтово совпадающий
//     с golden-файлом *.expected.json. Это проверяет, что adapter
//     правильно обрабатывает контракт abs-connector ↔ adapter.
//
//  2. Canonical → ЦФТ XML translation: для тех же canonical-команд
//     генерируется XML-payload, который должен побайтово (после
//     normalization) совпасть с *.cft.request.xml; и обратно — pre-canned
//     ЦФТ-ответ из *.cft.response.xml парсится в canonical ABSResponse.Data
//     и сравнивается с *.expected.json.
//
//     Translation-helpers (buildCFTRequest / parseCFTResponse) живут в
//     ЭТОМ же тест-пакете, потому что production-handler пока не
//     эмитирует/парсит XML — он отдаёт SHA-256-stub. Когда ADR-0006 § 5
//     Phase 2 закроется и реальный SOAP/MQ-клиент попадёт в
//     internal/handler, translation-логика переедет в production-код,
//     а эти тесты будут импортировать её как нормальный пакет.
//
// Запуск:
//
//	go test -tags=contract -v ./test/contract/...
package contract

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aibank/abs-adapter-cft/internal/domain"
	"aibank/abs-adapter-cft/internal/handler"
)

// adapterVersion — фиксированная версия для тестов; не зависит от VERSION-файла,
// потому что contract-тесты проверяют translation-инвариант, а не версию.
const adapterVersion = "1.0.0-contract-test"

// fixturesDir — каталог с golden-фикстурами относительно этого тест-файла.
const fixturesDir = "fixtures"

// readFixture читает файл из fixtures/, возвращая trimmed-байты, чтобы
// сравнения JSON/XML не ломались на trailing newline'ах от редакторов.
func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join(fixturesDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return bytes.TrimSpace(data)
}

// startAdapter запускает adapter HTTP-server через httptest.
// Возвращает baseURL и cleanup. Stateless handler — параллельные t.Run
// допустимы, но мы их не делаем, чтобы фиксировать порядок логов.
func startAdapter(t *testing.T) (string, func()) {
	t.Helper()
	h := handler.NewHandler(adapterVersion)
	srv := httptest.NewServer(h.Router())
	return srv.URL, srv.Close
}

// postExecute — POST /v1/execute с raw JSON-body (как пришло из fixture-файла,
// чтобы не терять inputs при marshal/unmarshal round-trip).
func postExecute(t *testing.T, baseURL string, body []byte) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Post(baseURL+"/v1/execute", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/execute: %v", err)
	}
	defer resp.Body.Close()

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp, bytes.TrimSpace(buf.Bytes())
}

// jsonEqual сравнивает две JSON-строки как map[string]any, игнорируя
// форматирование/порядок ключей.
func jsonEqual(t *testing.T, got, want []byte) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("unmarshal got: %v\nbody=%s", err, string(got))
	}
	if err := json.Unmarshal(want, &w); err != nil {
		t.Fatalf("unmarshal want: %v\nbody=%s", err, string(want))
	}
	gNorm, _ := json.Marshal(g)
	wNorm, _ := json.Marshal(w)
	if string(gNorm) != string(wNorm) {
		t.Fatalf("JSON mismatch:\n  got:  %s\n  want: %s", gNorm, wNorm)
	}
}

// ------------------------------------------------------------------
// 1. Canonical round-trip tests
// ------------------------------------------------------------------

// canonicalCases — таблица canonical-фикстур. Каждый кейс — пара
// (request.json → expected.json) + ожидаемый HTTP-status.
var canonicalCases = []struct {
	name           string
	requestFile    string
	expectedFile   string
	wantHTTPStatus int
}{
	{
		name:           "OpenAccount_LLC",
		requestFile:    "open_account_llc.request.json",
		expectedFile:   "open_account_llc.expected.json",
		wantHTTPStatus: http.StatusOK,
	},
	{
		name:           "GetBalance",
		requestFile:    "get_balance.request.json",
		expectedFile:   "get_balance.expected.json",
		wantHTTPStatus: http.StatusOK,
	},
	{
		name:           "CloseAccount",
		requestFile:    "close_account.request.json",
		expectedFile:   "close_account.expected.json",
		wantHTTPStatus: http.StatusOK,
	},
	{
		name:           "RejectUnknownCommand",
		requestFile:    "reject_unknown_command.request.json",
		expectedFile:   "reject_unknown_command.expected.json",
		wantHTTPStatus: http.StatusOK, // unknown команда — бизнес-failure, не HTTP-ошибка
	},
}

// TestContract_CanonicalRoundTrip — основной golden-тест. Шлёт canonical
// request, проверяет что body == golden-expected.
func TestContract_CanonicalRoundTrip(t *testing.T) {
	baseURL, stop := startAdapter(t)
	defer stop()

	for _, tc := range canonicalCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := readFixture(t, tc.requestFile)
			want := readFixture(t, tc.expectedFile)

			resp, got := postExecute(t, baseURL, req)
			if resp.StatusCode != tc.wantHTTPStatus {
				t.Fatalf("status: got %d, want %d (body=%s)",
					resp.StatusCode, tc.wantHTTPStatus, string(got))
			}
			if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "application/json") {
				t.Fatalf("Content-Type: got %q, want application/json", got)
			}

			jsonEqual(t, got, want)
		})
	}
}

// TestContract_CanonicalDeterministic — повторный POST с тем же
// idempotency_key должен вернуть бит-в-бит тот же ответ. Это критично
// для golden-fixture-стратегии: если детерминизм сломается, нужно
// перегенерить fixture, а не молча ловить flaky-тесты.
func TestContract_CanonicalDeterministic(t *testing.T) {
	baseURL, stop := startAdapter(t)
	defer stop()

	req := readFixture(t, "open_account_llc.request.json")

	_, first := postExecute(t, baseURL, req)
	_, second := postExecute(t, baseURL, req)
	if !bytes.Equal(first, second) {
		t.Fatalf("non-deterministic response:\n  first:  %s\n  second: %s", first, second)
	}
}

// ------------------------------------------------------------------
// 2. Canonical ↔ ЦФТ XML translation contract
// ------------------------------------------------------------------

// cftTranslationCases — фикстуры, для которых есть пара cft.request.xml /
// cft.response.xml. RejectUnknown сюда не входит — для неизвестной команды
// adapter не должен пытаться строить ЦФТ-запрос.
var cftTranslationCases = []struct {
	name             string
	canonicalRequest string
	canonicalExpect  string
	cftRequestXML    string
	cftResponseXML   string
}{
	{
		name:             "OpenAccount_LLC",
		canonicalRequest: "open_account_llc.request.json",
		canonicalExpect:  "open_account_llc.expected.json",
		cftRequestXML:    "open_account_llc.cft.request.xml",
		cftResponseXML:   "open_account_llc.cft.response.xml",
	},
	{
		name:             "GetBalance",
		canonicalRequest: "get_balance.request.json",
		canonicalExpect:  "get_balance.expected.json",
		cftRequestXML:    "get_balance.cft.request.xml",
		cftResponseXML:   "get_balance.cft.response.xml",
	},
	{
		name:             "CloseAccount",
		canonicalRequest: "close_account.request.json",
		canonicalExpect:  "close_account.expected.json",
		cftRequestXML:    "close_account.cft.request.xml",
		cftResponseXML:   "close_account.cft.response.xml",
	},
}

// TestContract_CFTRequestTranslation — canonical → ЦФТ XML.
// Для каждой команды берём canonical request.json, строим ЦФТ-payload
// через buildCFTRequest, и сравниваем с golden cft.request.xml после
// normalization (whitespace-insensitive).
func TestContract_CFTRequestTranslation(t *testing.T) {
	for _, tc := range cftTranslationCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			reqRaw := readFixture(t, tc.canonicalRequest)
			var canonical domain.ABSCommand
			if err := json.Unmarshal(reqRaw, &canonical); err != nil {
				t.Fatalf("decode canonical request: %v", err)
			}

			gotXML, err := buildCFTRequest(canonical)
			if err != nil {
				t.Fatalf("buildCFTRequest: %v", err)
			}
			wantXML := readFixture(t, tc.cftRequestXML)

			if normalizeXML(gotXML) != normalizeXML(wantXML) {
				t.Fatalf("CFT request XML mismatch:\n  got:\n%s\n\n  want:\n%s",
					string(gotXML), string(wantXML))
			}
		})
	}
}

// TestContract_CFTResponseTranslation — ЦФТ XML → canonical.
// Парсим pre-canned cft.response.xml, превращаем в canonical
// ABSResponse.Data, и сравниваем с *.expected.json (поле data).
func TestContract_CFTResponseTranslation(t *testing.T) {
	for _, tc := range cftTranslationCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cftRaw := readFixture(t, tc.cftResponseXML)
			expectedRaw := readFixture(t, tc.canonicalExpect)

			var expected domain.ABSResponse
			if err := json.Unmarshal(expectedRaw, &expected); err != nil {
				t.Fatalf("decode expected canonical: %v", err)
			}

			gotData, err := parseCFTResponse(cftRaw)
			if err != nil {
				t.Fatalf("parseCFTResponse: %v", err)
			}

			gotJSON, _ := json.Marshal(gotData)
			wantJSON, _ := json.Marshal(expected.Data)
			if string(gotJSON) != string(wantJSON) {
				t.Fatalf("canonical Data mismatch after CFT->canonical translation:\n  got:  %s\n  want: %s",
					gotJSON, wantJSON)
			}
		})
	}
}

// ------------------------------------------------------------------
// Translation helpers
//
// ВРЕМЕННО живут в тест-коде. Когда production-handler научится говорить
// на ЦФТ-XML (ADR-0006 § 5 Phase 2), эти функции переедут в
// internal/handler/cft_translator.go и тест станет тонкой обёрткой.
// ------------------------------------------------------------------

// cftRequest / cftResponse — XML-шейпы, повторяющие structure golden-фикстур.
// `xml:",chardata"` и omitempty используются для опциональных полей
// (KPP/CloseReason появляются только в части команд).
type cftRequest struct {
	XMLName xml.Name      `xml:"CFTRequest"`
	Header  cftReqHeader  `xml:"Header"`
	Body    cftReqBody    `xml:"Body"`
}

type cftReqHeader struct {
	IdempotencyKey string `xml:"IdempotencyKey"`
	TenantID       string `xml:"TenantID"`
	Operation      string `xml:"Operation"`
}

type cftReqBody struct {
	Account *cftReqAccount `xml:"Account,omitempty"`
	Client  *cftReqClient  `xml:"Client,omitempty"`
}

type cftReqAccount struct {
	Type          string `xml:"Type,omitempty"`
	BIK           string `xml:"BIK,omitempty"`
	ClientRef     string `xml:"ClientRef,omitempty"`
	AccountNumber string `xml:"AccountNumber,omitempty"`
	CloseReason   string `xml:"CloseReason,omitempty"`
}

type cftReqClient struct {
	LegalForm string `xml:"LegalForm,omitempty"`
	FullName  string `xml:"FullName,omitempty"`
	INN       string `xml:"INN,omitempty"`
	KPP       string `xml:"KPP,omitempty"`
}

type cftResponse struct {
	XMLName xml.Name       `xml:"CFTResponse"`
	Header  cftRespHeader  `xml:"Header"`
	Body    cftRespBody    `xml:"Body"`
}

type cftRespHeader struct {
	IdempotencyKey string `xml:"IdempotencyKey"`
	Operation      string `xml:"Operation"`
	Status         string `xml:"Status"`
}

type cftRespBody struct {
	Account *cftRespAccount `xml:"Account,omitempty"`
}

type cftRespAccount struct {
	AccountNumber  string `xml:"AccountNumber,omitempty"`
	BIK            string `xml:"BIK,omitempty"`
	BalanceKopecks *int64 `xml:"BalanceKopecks,omitempty"`
	State          string `xml:"State,omitempty"`
}

// canonicalToCFTOperation — единственная точка маппинга canonical-команды
// в ЦФТ Operation-string. Если ЦФТ-вендор поменяет нейминг, правка здесь.
func canonicalToCFTOperation(c domain.CommandType) (string, bool) {
	switch c {
	case domain.CmdOpenAccount:
		return "ACCOUNT_OPEN", true
	case domain.CmdGetAccountInfo:
		return "ACCOUNT_INFO", true
	case domain.CmdCloseAccount:
		return "ACCOUNT_CLOSE", true
	case domain.CmdCreateClient:
		return "CLIENT_CREATE", true
	default:
		return "", false
	}
}

// payloadString — безопасное чтение string-поля из map[string]any.
func payloadString(p map[string]any, key string) string {
	if v, ok := p[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// buildCFTRequest строит ЦФТ-XML-запрос из canonical ABSCommand.
// Возвращает форматированный XML с pretty-print indent ("  "), как в
// golden-фикстурах, чтобы diff при mismatch был читаемым.
func buildCFTRequest(cmd domain.ABSCommand) ([]byte, error) {
	op, ok := canonicalToCFTOperation(cmd.Command)
	if !ok {
		return nil, errUnsupportedCommand{cmd: cmd.Command}
	}

	req := cftRequest{
		Header: cftReqHeader{
			IdempotencyKey: cmd.IdempotencyKey,
			TenantID:       cmd.TenantID,
			Operation:      op,
		},
	}

	switch cmd.Command {
	case domain.CmdOpenAccount:
		req.Body.Account = &cftReqAccount{
			Type:      "RUB_CURRENT",
			BIK:       handler.BIK,
			ClientRef: payloadString(cmd.Payload, "client_id"),
		}
		req.Body.Client = &cftReqClient{
			LegalForm: payloadString(cmd.Payload, "legal_form"),
			FullName:  payloadString(cmd.Payload, "full_name"),
			INN:       payloadString(cmd.Payload, "inn"),
			KPP:       payloadString(cmd.Payload, "kpp"),
		}

	case domain.CmdGetAccountInfo:
		req.Body.Account = &cftReqAccount{
			AccountNumber: payloadString(cmd.Payload, "account_number"),
		}

	case domain.CmdCloseAccount:
		req.Body.Account = &cftReqAccount{
			AccountNumber: payloadString(cmd.Payload, "account_number"),
			CloseReason:   payloadString(cmd.Payload, "reason"),
		}
	}

	out, err := xml.MarshalIndent(req, "", "  ")
	if err != nil {
		return nil, err
	}
	// Префиксуем XML-декларацией, как в golden-файлах.
	return []byte(xml.Header + string(out) + "\n"), nil
}

// parseCFTResponse декодирует ЦФТ-XML-ответ обратно в canonical Data-map.
// Возвращает значение, которое ожидается в ABSResponse.Data — те же ключи,
// которые сейчас формирует stub-handler (account_number / bik /
// balance_kopecks / status).
func parseCFTResponse(raw []byte) (map[string]any, error) {
	var r cftResponse
	if err := xml.Unmarshal(raw, &r); err != nil {
		return nil, err
	}

	data := make(map[string]any)
	if r.Body.Account == nil {
		return data, nil
	}
	a := r.Body.Account

	switch r.Header.Operation {
	case "ACCOUNT_OPEN":
		if a.AccountNumber != "" {
			data["account_number"] = a.AccountNumber
		}
		if a.BIK != "" {
			data["bik"] = a.BIK
		}

	case "ACCOUNT_INFO":
		if a.BalanceKopecks != nil {
			// JSON-numbers через encoding/json идут как float64, поэтому
			// приводим *int64 к float64 для побайтового совпадения с
			// expected.json (где 0 декодируется как float64(0)).
			data["balance_kopecks"] = float64(*a.BalanceKopecks)
		}
		if a.State != "" {
			data["status"] = strings.ToLower(a.State)
		}

	case "ACCOUNT_CLOSE":
		if a.State != "" {
			data["status"] = strings.ToLower(a.State)
		}
	}
	return data, nil
}

// errUnsupportedCommand — sentinel-ошибка для buildCFTRequest. Отдельный
// тип нужен, чтобы test'ы (если появятся) могли проверять errors.As.
type errUnsupportedCommand struct {
	cmd domain.CommandType
}

func (e errUnsupportedCommand) Error() string {
	return "cft: unsupported canonical command: " + string(e.cmd)
}

// normalizeXML приводит XML к whitespace-insensitive форме для сравнения:
// убирает leading/trailing whitespace и схлопывает любые run'ы whitespace
// между токенами в один пробел. Этого достаточно, чтобы golden-fixture не
// зависел от настроек редактора (CRLF/LF, indent 2 vs 4).
func normalizeXML(b []byte) string {
	s := strings.TrimSpace(string(b))
	var out strings.Builder
	out.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !prevSpace {
				out.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		out.WriteRune(r)
		prevSpace = false
	}
	return out.String()
}
