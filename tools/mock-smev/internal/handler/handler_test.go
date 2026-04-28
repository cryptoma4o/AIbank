package handler

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(New(slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(srv.Close)
	return srv
}

func TestByINN_HappyPath_ReturnsXML(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/by-inn/7707083893")
	if err != nil {
		t.Fatalf("GET by-inn: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "xml") {
		t.Errorf("Content-Type = %q, want xml", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	xml := string(body)

	for _, want := range []string{
		"<egrul-record>",
		"<inn>7707083893</inn>",
		"<ogrn>",
		"<full-name>",
		"<status>",
		"<registered-at>",
		"<founders>",
	} {
		if !strings.Contains(xml, want) {
			t.Errorf("XML не содержит %q\n--\n%s", want, xml)
		}
	}
}

func TestByINN_NotFound_ForReservedINN(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/by-inn/" + NotFoundINN)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 для INN %s", resp.StatusCode, NotFoundINN)
	}
}

func TestByINN_DifferentINNs_ProduceDifferentXML(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)

	a := mustGet(t, srv.URL+"/by-inn/7707083893")
	b := mustGet(t, srv.URL+"/by-inn/5077746001") // другой ИНН (10 цифр)
	if a == b {
		t.Errorf("ожидался разный XML для разных INN, получили одинаковый")
	}
	// и оба валидно содержат свой INN
	if !strings.Contains(a, "<inn>7707083893</inn>") {
		t.Errorf("XML для 7707083893 не содержит соответствующего <inn>")
	}
}

func TestByINN_DeterministicForSameINN(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)

	a := mustGet(t, srv.URL+"/by-inn/7707083893")
	b := mustGet(t, srv.URL+"/by-inn/7707083893")
	if a != b {
		t.Errorf("ожидался идентичный XML для одного INN (детерминизм), получили разные")
	}
}

func TestByINN_IndividualEntrepreneur_12DigitsINN(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)

	xml := mustGet(t, srv.URL+"/by-inn/770700000012")
	if !strings.Contains(xml, "<egrip-record>") {
		t.Errorf("12-знач INN должен давать egrip-record\n%s", xml)
	}
	if !strings.Contains(xml, "<opf-code>50102</opf-code>") {
		t.Errorf("ИП ожидался opf-code 50102")
	}
	// ИП не имеет founders.
	if strings.Contains(xml, "<founders>") {
		t.Errorf("ИП не должен иметь <founders>\n%s", xml)
	}
}

func TestByINN_BadFormat_400(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)

	cases := []string{
		"/by-inn/abc",        // не цифры
		"/by-inn/123",        // короткий
		"/by-inn/1234567890123", // длиннее 12
	}
	for _, p := range cases {
		p := p
		t.Run(p, func(t *testing.T) {
			t.Parallel()
			resp, err := http.Get(srv.URL + p)
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("%s: status = %d, want 400", p, resp.StatusCode)
			}
		})
	}
}

func TestByOGRN_HappyPath(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/by-ogrn/1027700132195")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	xml := string(body)
	if !strings.Contains(xml, "<ogrn>1027700132195</ogrn>") {
		t.Errorf("ответ должен сохранять запрошенный ОГРН: \n%s", xml)
	}
}

func TestHealthz(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("healthz status = %d, want 200", resp.StatusCode)
	}
}

// --- helpers ---

func mustGet(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(body)
}
