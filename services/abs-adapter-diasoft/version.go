// Package adapterdiasoft — корневой пакет модуля aibank/abs-adapter-diasoft.
//
// Существует исключительно для того, чтобы дать `go:embed` возможность
// «увидеть» VERSION-файл в корне сервиса (см. ADR-0006 § Implementation Notes:
// `services/abs-adapter-diasoft/VERSION` — единственный источник правды).
//
// `go:embed`-паттерн работает относительно директории Go-файла и не может
// смотреть «вверх» по дереву каталогов; поэтому файл, объявляющий
// embedded VERSION, обязан лежать в корне модуля, а не в `cmd/server/`.
package adapterdiasoft

import _ "embed"

//go:embed VERSION
var rawVersion string

// Version возвращает строку semver, прочитанную из embedded VERSION
// при сборке бинаря. Используется cmd/server/main.go и handler'ом для
// /version-эндпоинта.
func Version() string {
	return rawVersion
}
