// Package esia реализует OIDC-интеграцию с ЕСИА (Госуслуги).
//
// ЕСИА (Единая система идентификации и аутентификации) — государственный
// провайдер OIDC для граждан и организаций РФ. Интеграция требует:
//   - регистрацию заявки relying-party через portal-uslug.gosuslugi.ru;
//   - сертификат ГОСТ-2012 для подписи запросов JWS (см. § 3.1
//     docs/security-architecture.md);
//   - тестовую среду ESIA-tech для приёмки;
//   - mTLS на token/userinfo endpoint.
//
// В коде есть два провайдера:
//
//   - StubProvider  — детерминированная заглушка для unit-тестов и dev,
//     ничего не вызывает по сети.
//   - LiveProvider — каркас под реальный OIDC; методы возвращают
//     ErrNotImplemented с TODO-checklist'ом, что нужно сделать перед
//     включением. Активируется через ESIA_LIVE=true.
//
// Provider-абстракция повторяет паттерн ext-* сервисов (kontur-*, fns-*),
// чтобы handler-слой не зависел от транспорта.
package esia

import (
	"context"
	"errors"
	"time"
)

// ErrNotImplemented — sentinel-ошибка, которой LiveProvider помечает методы,
// требующие реальной интеграции с ЕСИА. Возвращается до момента, пока
// не пройдены все TODO в live.go.
var ErrNotImplemented = errors.New("esia: live provider is not implemented; ESIA_LIVE требует prod-grade OIDC настройки")

// UserInfo — данные субъекта, полученные от ЕСИА после exchange + userinfo.
//
// Subject — стабильный OID гражданина в системе ЕСИА; не меняется при смене
// паспорта/имени. ЛЮБОЙ другой identifier (INN, SNILS, телефон) может
// меняться, поэтому привязка applicant'а делается ПО Subject.
//
// Поля INN/Phone/Email опциональны: их наполнение зависит от scope
// (см. https://digital.gov.ru/ru/documents/6186/) и от того, заполнил ли
// гражданин данные в личном кабинете.
type UserInfo struct {
	Subject   string    `json:"subject"`            // OID, e.g. "1000234567"
	FullName  string    `json:"full_name"`          // ФИО
	INN       string    `json:"inn,omitempty"`      // 12 цифр, физлицо
	Phone     string    `json:"phone,omitempty"`    // E.164
	Email     string    `json:"email,omitempty"`    // нормализованный
	BirthDate string    `json:"birth_date,omitempty"`
	IssuedAt  time.Time `json:"issued_at"`          // время exchange
}

// Provider — общий интерфейс для всех реализаций ЕСИА (Stub + Live).
//
// BuildAuthURL формирует URL authorize-эндпоинта с параметрами state/nonce
// и redirect_uri. Сам state/nonce генерирует caller (handler) — провайдеру
// они нужны только для встраивания в URL.
//
// Exchange меняет authorization-code на UserInfo. Реализация может
// внутренне вызвать token-endpoint и userinfo-endpoint — для caller'а это
// прозрачно: на вход — code, на выход — нормализованный UserInfo.
//
// Контекст обязателен: token/userinfo вызовы должны соблюдать таймаут
// вышестоящего HTTP-запроса (по умолчанию 30s в main.go).
type Provider interface {
	BuildAuthURL(state, nonce, redirectURI string) string
	Exchange(ctx context.Context, code, redirectURI string) (UserInfo, error)
}
