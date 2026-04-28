package esia

import (
	"context"
)

// Реальные endpoint'ы ЕСИА (продовая среда).
// Тестовая среда: https://esia-portal1.test.gosuslugi.ru/aas/oauth2/v3/...
//
// Эти URL — placeholders в коде; перед активацией LiveProvider они должны
// быть подтверждены актуальной документацией Минцифры
// (https://digital.gov.ru/ru/documents/6186/).
const (
	LiveAuthorizeEndpoint = "https://esia.gosuslugi.ru/aas/oauth2/v3/ac"
	LiveTokenEndpoint     = "https://esia.gosuslugi.ru/aas/oauth2/v3/te"
	// LiveUserinfoEndpoint требует подстановки {oid} субъекта; для
	// формирования URL'а используется userinfoURL(oid).
	LiveUserinfoBase = "https://esia.gosuslugi.ru/rs/prns/"

	// DefaultScope — минимальный набор для онбординга физлица/ИП.
	DefaultScope = "openid fullname email mobile inn"
)

// LiveProviderConfig — параметры конструктора LiveProvider.
//
// ClientSecretPEMPath — путь к ГОСТ-2012 (или RSA для тестовой среды)
// сертификату-приватному ключу, которым подписываются JWS-запросы к
// token-endpoint'у. Файл НИКОГДА не попадает в репозиторий и
// загружается через packages/secrets (Vault).
type LiveProviderConfig struct {
	ClientID            string
	ClientSecretPEMPath string
	Scope               string
}

// LiveProvider — каркас боевой интеграции с ЕСИА.
//
// На текущий момент BuildAuthURL формирует корректный URL (этот шаг
// безопасен — ничего секретного не используется), а Exchange возвращает
// ErrNotImplemented со списком TODO. Это позволяет включить ESIA_LIVE=true
// в smoke-тесте и сразу увидеть, что именно осталось сделать.
//
// TODO checklist (см. docs/security-architecture.md § 3.1, ADR-0010):
//
//  1. Получить статус relying-party через portal-uslug.gosuslugi.ru;
//     получить mnemonic и тестовый client_id.
//  2. Сгенерировать ГОСТ-2012 ключ и подписать CSR в УЦ
//     Минцифры; импортировать в Vault Transit (engine "esia-jws").
//  3. Реализовать JWS-подпись token-exchange запроса:
//     header alg=GOST3410_2012_256, body=client_secret_jwt
//     (RFC 7521) с claims: iss=client_id, sub=client_id,
//     aud=token_endpoint, jti, exp.
//  4. Реализовать HTTP-клиент с mTLS-trust-store; принять только
//     сертификаты CA Минцифры.
//  5. Userinfo-вызов: GET {LiveUserinfoBase}{oid} с access_token,
//     парсинг ответа в UserInfo (поля могут быть отдельными вызовами:
//     /contacts, /docs/RF_PASSPORT — оптимизировать через batch).
//  6. Прокинуть метрики/трейсы (esia.token.duration, esia.userinfo.duration)
//     в packages/observability.
//  7. Обработать ошибки ЕСИА: 400 invalid_grant, 503 service_unavailable
//     с retry-after; не ретраить 4xx.
//  8. Интегрировать GovKey для scope "юрлицо" (отдельный flow с
//     X-PROVIDED-FROM-MTH в заголовке и подписью от руководителя).
type LiveProvider struct {
	cfg LiveProviderConfig
}

// NewLiveProvider — конструктор, валидирует обязательные поля и
// дефолтит Scope. НЕ читает файл сертификата — это произойдёт при первом
// Exchange (lazy), чтобы non-prod окружения могли запустить сервис без
// сертификата на диске.
func NewLiveProvider(cfg LiveProviderConfig) *LiveProvider {
	if cfg.Scope == "" {
		cfg.Scope = DefaultScope
	}
	return &LiveProvider{cfg: cfg}
}

// BuildAuthURL формирует URL ЕСИА authorize-endpoint'а. Реализация
// безопасна (нет секретов, нет сетевых вызовов) и используется в smoke-
// тестах ESIA_LIVE=true ещё до полной реализации Exchange.
func (l *LiveProvider) BuildAuthURL(state, nonce, redirectURI string) string {
	// Минимально валидный URL: response_type, client_id, scope, state, nonce,
	// redirect_uri. Боевая реализация добавит timestamp, client_secret (JWS),
	// access_type=online — но базовая форма уже соответствует RFC 6749.
	return LiveAuthorizeEndpoint +
		"?response_type=code" +
		"&client_id=" + urlEscape(l.cfg.ClientID) +
		"&scope=" + urlEscape(l.cfg.Scope) +
		"&state=" + urlEscape(state) +
		"&nonce=" + urlEscape(nonce) +
		"&redirect_uri=" + urlEscape(redirectURI)
}

// Exchange — пока заглушка: возвращает ErrNotImplemented. При активации
// см. TODO checklist в docstring LiveProvider.
func (l *LiveProvider) Exchange(_ context.Context, _, _ string) (UserInfo, error) {
	return UserInfo{}, ErrNotImplemented
}

// userinfoURL формирует userinfo URL для конкретного oid. Используется
// будущей реализацией Exchange после получения access_token и oid.
func userinfoURL(oid string) string { return LiveUserinfoBase + oid }

// urlEscape — облегчённая версия url.QueryEscape, чтобы не тянуть
// импорты в build-формирователь URL'а. Сделано как минимально
// допустимая функция: побайтное экранирование непечатаемых/спец-
// символов; для ASCII-параметров (state/nonce/client_id) поведение
// совпадает с url.QueryEscape.
func urlEscape(s string) string {
	const hex = "0123456789ABCDEF"
	var b []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~' {
			b = append(b, c)
			continue
		}
		b = append(b, '%', hex[c>>4], hex[c&0x0F])
	}
	return string(b)
}
