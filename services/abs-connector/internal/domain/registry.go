package domain

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
)

// MockAdapterURL — специальный псевдо-URL для тестов / demo-tenant.
// Connector интерпретирует его как «не делать настоящий HTTP-вызов,
// возвращать deterministic-ответ» — реализация в clients/adapter.go.
const MockAdapterURL = "memory:mock"

// AdapterEntry — одна запись в реестре, одна на тенанта.
//
// Согласно ADR-0006:
//   - URL — endpoint конкретного адаптера-сервиса в кластере.
//   - Version — semver MAJOR.MINOR.PATCH, читается build-системой адаптера
//     из VERSION-файла. Connector хранит его как непрозрачную строку.
//   - AdapterName — логическое имя ("cft" / "diasoft" / "rs-bank"),
//     попадает в audit / billing events.
type AdapterEntry struct {
	TenantID    string `json:"tenant_id"     yaml:"tenant_id"`
	AdapterName string `json:"adapter_name"  yaml:"adapter_name"`
	URL         string `json:"url"           yaml:"url"`
	Version     string `json:"version"       yaml:"version"`
}

// ErrTenantNotConfigured возвращается, когда у тенанта нет адаптера.
// Handler должен мапить это в HTTP 503 (тенант существует в системе,
// но интеграция с АБС ещё не настроена).
var ErrTenantNotConfigured = errors.New("tenant has no adapter configured")

// validTenantIDForRegistry допускает чуть более широкий набор символов,
// чем schema-id в tenant-service: дефис тоже разрешаем, потому что
// здесь мы не используем имя для построения SQL-идентификатора.
// Защита от prompt-injection / wild input всё равно нужна:
// tenantID попадает в Redis-ключ и log-string.
var validTenantIDForRegistry = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// ValidateTenantID — публичная функция для использования handler'ом.
func ValidateTenantID(id string) error {
	if !validTenantIDForRegistry.MatchString(id) {
		return fmt.Errorf("invalid tenant_id %q", id)
	}
	return nil
}

// AdapterRegistry — потокобезопасный in-memory реестр.
// На прод-инстансе он строится один раз при старте (configs/adapters.yaml
// и/или env override) и затем читается под RLock'ом на каждом запросе.
//
// Hot-reload не поддерживаем здесь: согласно ADR-0006 changes к версионной
// матрице в on-prem прокатываются rolling-restart'ом connector pod'ов,
// а не runtime-reload'ом.
type AdapterRegistry struct {
	mu      sync.RWMutex
	entries map[string]AdapterEntry
}

func NewAdapterRegistry() *AdapterRegistry {
	return &AdapterRegistry{entries: make(map[string]AdapterEntry)}
}

// Lookup возвращает запись для тенанта или ErrTenantNotConfigured.
// Поведение «default»-fallback'а намеренно НЕ реализовано:
// согласно ADR-0006, явная конфигурация на тенант — это контрактное
// требование, и неявный fallback скрыл бы misconfiguration.
func (r *AdapterRegistry) Lookup(tenantID string) (AdapterEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[tenantID]
	if !ok {
		return AdapterEntry{}, ErrTenantNotConfigured
	}
	return e, nil
}

// Register добавляет/перезаписывает запись. Используется загрузчиком
// конфигурации; не предназначено для вызова из handler'а.
func (r *AdapterRegistry) Register(e AdapterEntry) error {
	if err := ValidateTenantID(e.TenantID); err != nil {
		return err
	}
	if e.URL == "" {
		return fmt.Errorf("adapter url is required for tenant %q", e.TenantID)
	}
	if e.Version == "" {
		return fmt.Errorf("adapter version is required for tenant %q", e.TenantID)
	}
	if e.AdapterName == "" {
		// Допускаем пустое имя — но лучше fail-fast: tenant-config обязан задавать.
		return fmt.Errorf("adapter_name is required for tenant %q", e.TenantID)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[e.TenantID] = e
	return nil
}

// All возвращает копию всех записей — для health/debug-эндпоинтов.
func (r *AdapterRegistry) All() []AdapterEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]AdapterEntry, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e)
	}
	return out
}

// LoadFromEnv читает env-переменные вида:
//
//	ABS_ADAPTER_<TENANTID>=<adapter_name>,<url>,<version>
//
// например `ABS_ADAPTER_BANK_ALPHA=cft,http://abs-adapter-cft:9001,1.4.2`.
// Плюсы env-варианта — простота dev/test override без редактирования YAML.
// Несовпадающие префиксы игнорируем; невалидные значения — ошибка fail-fast.
//
// Преобразование ключа: после префикса `ABS_ADAPTER_` всё остальное берётся
// как tenant_id, lower-cased, с заменой `_` на `-` НЕ делается — оставляем
// underscore'ы (валидатор их принимает). Если кому-то нужен дефис — путь
// через YAML.
func (r *AdapterRegistry) LoadFromEnv(environ []string) error {
	const prefix = "ABS_ADAPTER_"
	for _, kv := range environ {
		if !strings.HasPrefix(kv, prefix) {
			continue
		}
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		key := kv[len(prefix):eq]
		val := kv[eq+1:]
		tenantID := strings.ToLower(key)

		parts := strings.SplitN(val, ",", 3)
		if len(parts) != 3 {
			return fmt.Errorf("env %s: expected 'adapter_name,url,version', got %q", kv[:eq], val)
		}
		entry := AdapterEntry{
			TenantID:    tenantID,
			AdapterName: strings.TrimSpace(parts[0]),
			URL:         strings.TrimSpace(parts[1]),
			Version:     strings.TrimSpace(parts[2]),
		}
		if err := r.Register(entry); err != nil {
			return fmt.Errorf("env %s: %w", kv[:eq], err)
		}
	}
	return nil
}

// LoadFromYAML парсит configs/adapters.yaml.
//
// Формат файла:
//
//	adapters:
//	  - tenant_id: bank-alpha
//	    adapter_name: cft
//	    url: http://abs-adapter-cft:9001
//	    version: "1.4.2"
//	  - tenant_id: demo
//	    adapter_name: cft
//	    url: memory:mock
//	    version: "0.0.0"
//
// Используем минималистичный самописный парсер «псевдо-YAML»: делаем это,
// чтобы не тащить yaml.v3 в зависимости MVP-сервиса. Формат поддерживает
// ровно такой layout, как в configs/adapters.yaml; полноценный YAML —
// предмет отдельной задачи (TODO: переход на gopkg.in/yaml.v3 при появлении
// первого реального YAML с anchors/aliases).
func (r *AdapterRegistry) LoadFromYAML(data []byte) error {
	lines := strings.Split(string(data), "\n")

	var (
		inList  bool
		current AdapterEntry
		hasItem bool
	)

	flush := func() error {
		if !hasItem {
			return nil
		}
		if err := r.Register(current); err != nil {
			return err
		}
		current = AdapterEntry{}
		hasItem = false
		return nil
	}

	for _, raw := range lines {
		line := raw
		// Снимаем комментарии после `#`.
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		if trim == "adapters:" {
			inList = true
			continue
		}
		if !inList {
			continue
		}
		if strings.HasPrefix(trim, "- ") {
			if err := flush(); err != nil {
				return err
			}
			hasItem = true
			trim = strings.TrimPrefix(trim, "- ")
		}
		// Парсим k: v.
		eq := strings.IndexByte(trim, ':')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(trim[:eq])
		val := strings.TrimSpace(trim[eq+1:])
		val = strings.Trim(val, `"'`)
		switch key {
		case "tenant_id":
			current.TenantID = val
		case "adapter_name":
			current.AdapterName = val
		case "url":
			current.URL = val
		case "version":
			current.Version = val
		}
	}
	return flush()
}

// LoadFromYAMLFile — convenience-обёртка для cmd/server/main.go.
func (r *AdapterRegistry) LoadFromYAMLFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read adapters yaml %s: %w", path, err)
	}
	return r.LoadFromYAML(data)
}
