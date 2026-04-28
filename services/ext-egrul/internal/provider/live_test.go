package provider

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// helpers ------------------------------------------------------------

// quietLogger — slog в io.Discard, чтобы не засорять test output.
func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newLiveProvider(t *testing.T, endpoint string) *LiveProvider {
	t.Helper()
	return NewLiveProvider(LiveConfig{
		Endpoint: endpoint,
		Timeout:  2 * time.Second,
		Logger:   quietLogger(),
	})
}

// happyXML — минимальный XML, достаточный для MapXMLToLegalEntity.
const happyXML = `<egrul-record>
  <inn>7707083893</inn>
  <ogrn>1027700132195</ogrn>
  <kpp>770701001</kpp>
  <full-name>ООО Тест</full-name>
  <opf-code>12300</opf-code>
  <status>Действующее</status>
  <registered-at>15.03.2010</registered-at>
  <charter-capital-rub>10000.00</charter-capital-rub>
</egrul-record>`

// tests --------------------------------------------------------------

func TestLive_GetByINN_HappyPath(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/by-inn/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(happyXML))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newLiveProvider(t, srv.URL)
	le, err := p.GetByINN(context.Background(), "7707083893")
	if err != nil {
		t.Fatalf("GetByINN: %v", err)
	}
	if le.INN != "7707083893" || le.OGRN != "1027700132195" {
		t.Errorf("unexpected entity: %+v", le)
	}
	if le.OPF != "ООО" {
		t.Errorf("OPF = %q, want ООО (через xml_mapper нормализацию opf-code 12300)", le.OPF)
	}
	if le.Status != "active" {
		t.Errorf("Status = %q, want active", le.Status)
	}
}

func TestLive_GetByINN_404_MapsToErrNotFound(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	p := newLiveProvider(t, srv.URL)
	_, err := p.GetByINN(context.Background(), "9999999999")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestLive_GetByINN_5xxRetries_3Attempts(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := newLiveProvider(t, srv.URL)
	_, err := p.GetByINN(context.Background(), "7707083893")
	if err == nil {
		t.Fatalf("ожидалась ошибка после 3 попыток, получили nil")
	}
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want wraps ErrUpstream", err)
	}
	if got := calls.Load(); got != int32(liveRetryMaxAttempts) {
		t.Errorf("attempts = %d, want %d", got, liveRetryMaxAttempts)
	}
}

func TestLive_GetByINN_5xxThenSuccess_RetryRecovers(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		if n < 2 {
			http.Error(w, "boom", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(happyXML))
	}))
	defer srv.Close()

	p := newLiveProvider(t, srv.URL)
	le, err := p.GetByINN(context.Background(), "7707083893")
	if err != nil {
		t.Fatalf("ожидался успех на 2-й попытке, получили: %v", err)
	}
	if le.INN != "7707083893" {
		t.Errorf("INN = %q", le.INN)
	}
	if calls.Load() != 2 {
		t.Errorf("attempts = %d, want 2 (1 fail + 1 success)", calls.Load())
	}
}

func TestLive_CircuitBreaker_OpensAfter5ConsecutiveFailures(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := newLiveProvider(t, srv.URL)

	// Каждый GetByINN делает до 3 retry; за 2 запроса наберём 5 5xx
	// (3 + 2 = 5) и breaker откроется внутри второго запроса.
	for i := 0; i < 2; i++ {
		_, err := p.GetByINN(context.Background(), "7707083893")
		if err == nil {
			t.Fatalf("ожидалась ошибка на запросе #%d", i)
		}
	}

	// Третий запрос должен немедленно вернуть ErrCircuitOpen без вызова сервера.
	callsBefore := calls.Load()
	_, err := p.GetByINN(context.Background(), "7707083893")
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("err = %v, want ErrCircuitOpen", err)
	}
	if calls.Load() != callsBefore {
		t.Errorf("breaker не отрезал запрос: calls += %d", calls.Load()-callsBefore)
	}
}

func TestLive_CircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	t.Parallel()
	// Прямой unit-тест на breaker, чтобы не ждать 30s в HTTP-тесте.
	var b circuitBreaker
	now := time.Now()
	for i := 0; i < circuitFailureThreshold; i++ {
		b.onFailure(now)
	}
	if !b.isOpen(now) {
		t.Fatalf("breaker должен быть open после %d ошибок", circuitFailureThreshold)
	}
	// Через circuitOpenDuration должен пускать (half-open).
	if b.isOpen(now.Add(circuitOpenDuration + time.Millisecond)) {
		t.Errorf("breaker должен переходить в half-open после circuitOpenDuration")
	}
	// onSuccess сбрасывает.
	b.onSuccess()
	if b.isOpen(now) {
		t.Errorf("после onSuccess breaker должен быть closed")
	}
}

func TestLive_GetByINN_4xxNonNotFound_NoRetry(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	p := newLiveProvider(t, srv.URL)
	_, err := p.GetByINN(context.Background(), "7707083893")
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
	if calls.Load() != 1 {
		t.Errorf("на 4xx (не 404) не должно быть retry: calls = %d", calls.Load())
	}
}

func TestLive_GetByOGRN_HappyPath(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/by-ogrn/1027700132195" {
			http.Error(w, "wrong path "+r.URL.Path, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(happyXML))
	}))
	defer srv.Close()

	p := newLiveProvider(t, srv.URL)
	le, err := p.GetByOGRN(context.Background(), "1027700132195")
	if err != nil {
		t.Fatalf("GetByOGRN: %v", err)
	}
	if le.OGRN != "1027700132195" {
		t.Errorf("OGRN = %q", le.OGRN)
	}
}

func TestLive_GetFounders_ReusesByINN(t *testing.T) {
	t.Parallel()
	withFounders := `<egrul-record>
		<inn>7707083893</inn>
		<ogrn>1027700132195</ogrn>
		<full-name>ООО Тест</full-name>
		<opf-code>12300</opf-code>
		<status>Действующее</status>
		<registered-at>2010-03-15</registered-at>
		<founders>
			<founder type="person">
				<inn>770700000001</inn>
				<full-name>Петров Пётр Петрович</full-name>
				<share-percent>100.00</share-percent>
				<is-russian-resident>true</is-russian-resident>
			</founder>
		</founders>
	</egrul-record>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(withFounders))
	}))
	defer srv.Close()

	p := newLiveProvider(t, srv.URL)
	founders, err := p.GetFounders(context.Background(), "7707083893")
	if err != nil {
		t.Fatalf("GetFounders: %v", err)
	}
	if len(founders) != 1 || founders[0].FullName != "Петров Пётр Петрович" {
		t.Errorf("founders = %+v", founders)
	}
}

func TestLive_GetByINN_ContextCancelled(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Имитируем долгий запрос.
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := newLiveProvider(t, srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := p.GetByINN(ctx, "7707083893")
	if err == nil {
		t.Fatalf("ожидалась ошибка по cancelled context")
	}
}

func TestLive_GetByINN_InvalidXML_ReturnsErrUpstream(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte("<not-closed"))
	}))
	defer srv.Close()

	p := newLiveProvider(t, srv.URL)
	_, err := p.GetByINN(context.Background(), "7707083893")
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream wrap", err)
	}
}
