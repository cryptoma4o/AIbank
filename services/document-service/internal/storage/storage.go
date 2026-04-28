// Package storage инкапсулирует объектное хранилище документов.
//
// На один тенант — отдельный bucket (см. ADR-0002, раздел Layout кластера),
// формат ключа объекта: tenants/{tenant_id}/applications/{application_id}/{file_uuid}-{filename}.
package storage

import (
	"context"
	"errors"
	"io"
)

// ErrObjectNotFound возвращается, когда объект отсутствует в хранилище.
var ErrObjectNotFound = errors.New("storage: object not found")

// Storage — порт объектного хранилища документов.
type Storage interface {
	// Put сохраняет content по ключу key и возвращает абсолютный storagePath
	// (вид зависит от backend: "memory://..." или "s3://bucket/key").
	Put(ctx context.Context, key string, content io.Reader, contentType string) (storagePath string, err error)

	// Get открывает поток на чтение объекта по ключу. Вызывающий обязан
	// закрыть полученный io.ReadCloser. Если объект не найден — ErrObjectNotFound.
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Exists проверяет наличие объекта по ключу.
	Exists(ctx context.Context, key string) (bool, error)
}
