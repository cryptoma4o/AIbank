package esia

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"time"
)

// StubAuthorizeURL — фиктивный authorize-endpoint для тестов и dev.
const StubAuthorizeURL = "https://esia-stub.local/authorize"

// StubProvider — детерминированная заглушка ЕСИА. Никаких сетевых вызовов;
// все ответы вычисляются как функция от code, чтобы тесты были
// воспроизводимы. Используется по умолчанию (ESIA_LIVE != "true").
//
// Контракт детерминизма: при одинаковом code Exchange возвращает
// идентичный UserInfo, кроме поля IssuedAt — оно всегда now().
type StubProvider struct {
	now func() time.Time
}

// NewStubProvider возвращает StubProvider с time.Now в качестве часов.
func NewStubProvider() *StubProvider {
	return &StubProvider{now: time.Now}
}

// BuildAuthURL формирует URL вида
//
//	https://esia-stub.local/authorize?state=…&nonce=…&redirect_uri=…
//
// Точно такой же набор query-параметров, что у боевого ЕСИА — это
// позволяет handler-тестам опираться на единый формат.
func (s *StubProvider) BuildAuthURL(state, nonce, redirectURI string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", "esia-stub-client")
	q.Set("scope", "openid fullname email mobile inn")
	q.Set("state", state)
	q.Set("nonce", nonce)
	q.Set("redirect_uri", redirectURI)
	return StubAuthorizeURL + "?" + q.Encode()
}

// Exchange детерминированно строит UserInfo из code:
//   - subject = "esia-stub-" + sha256(code)[:12]
//   - INN     = первые 12 цифр от того же хеша
//   - email/full_name — синтетические значения для тестов.
func (s *StubProvider) Exchange(_ context.Context, code, _ string) (UserInfo, error) {
	if code == "" {
		return UserInfo{}, fmt.Errorf("esia/stub: пустой authorization code")
	}
	sum := sha256.Sum256([]byte(code))
	hexSum := hex.EncodeToString(sum[:])
	subject := "esia-stub-" + hexSum[:12]
	inn := digitsFromHex(hexSum, 12)
	return UserInfo{
		Subject:   subject,
		FullName:  "Стабов Стаб Стабович",
		INN:       inn,
		Phone:     "+7900" + digitsFromHex(hexSum, 7),
		Email:     subject + "@esia-stub.local",
		BirthDate: "1990-01-01",
		IssuedAt:  s.now().UTC(),
	}, nil
}

// digitsFromHex берёт первые n цифровых символов из hex-строки;
// если их меньше n — добивает нулями. Используется для синтетического INN
// и хвоста телефона, чтобы регулярки на стороне handler'а проходили.
func digitsFromHex(hexStr string, n int) string {
	out := make([]byte, 0, n)
	for i := 0; i < len(hexStr) && len(out) < n; i++ {
		c := hexStr[i]
		if c >= '0' && c <= '9' {
			out = append(out, c)
		}
	}
	for len(out) < n {
		out = append(out, '0')
	}
	return string(out)
}
