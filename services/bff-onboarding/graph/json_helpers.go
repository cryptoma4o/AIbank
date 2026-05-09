// Package graph — helpers для резолверов, читающих snake_case-ключи из
// map[string]any. Источник данных — orchestrator REST, который возвращает
// JSON напрямую с тегами struct'а (snake_case). GraphQL-схема BFF
// использует camelCase, поэтому каждое поле в schema-builder'е ставит
// Resolve: jsonField("snake_case_name").
package graph

import (
	"time"

	"github.com/graphql-go/graphql"
)

// jsonField возвращает резолвер, который из p.Source (ожидается
// map[string]interface{}) достаёт значение по snake_case-ключу.
// Если Source — nil или не map, возвращает (nil, nil).
func jsonField(key string) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		if p.Source == nil {
			return nil, nil
		}
		m, ok := p.Source.(map[string]interface{})
		if !ok {
			return nil, nil
		}
		v, ok := m[key]
		if !ok {
			return nil, nil
		}
		return v, nil
	}
}

// jsonObjectField — частный случай jsonField для полей, где значение —
// nested-объект; нужен только в исключительных случаях, иначе jsonField
// достаточно (graphql-go сам пройдётся по nested map).
//
//nolint:revive // унифицированный API с jsonField/jsonTimeField
func jsonObjectField(key string, _ *graphql.Object) graphql.FieldResolveFn {
	return jsonField(key)
}

// jsonTimeField парсит RFC3339 строку из orchestrator'а в time.Time, как
// того ждёт graphql.DateTime. Если значение уже time.Time — отдаёт как есть.
func jsonTimeField(key string) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		if p.Source == nil {
			return nil, nil
		}
		m, ok := p.Source.(map[string]interface{})
		if !ok {
			return nil, nil
		}
		raw, ok := m[key]
		if !ok || raw == nil {
			return nil, nil
		}
		switch v := raw.(type) {
		case time.Time:
			return v, nil
		case string:
			if v == "" {
				return nil, nil
			}
			if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
				return t, nil
			}
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				return t, nil
			}
			// Дата без времени (YYYY-MM-DD) — orchestrator такое тоже возвращает.
			if t, err := time.Parse("2006-01-02", v); err == nil {
				return t, nil
			}
			return v, nil // graphql отдаст как есть строкой
		}
		return raw, nil
	}
}
