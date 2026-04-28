package storage

import (
	"bytes"
	"context"
	"io"
	"sync"
)

// InMemoryStorage — потокобезопасная in-memory реализация Storage.
// Используется в unit-тестах и dev-режиме (STORAGE_BACKEND=memory).
type InMemoryStorage struct {
	mu      sync.RWMutex
	objects map[string][]byte
	prefix  string
}

// NewInMemoryStorage создаёт пустое in-memory хранилище.
func NewInMemoryStorage() *InMemoryStorage {
	return &InMemoryStorage{
		objects: make(map[string][]byte),
		prefix:  "memory://documents/",
	}
}

// Put читает content в []byte и сохраняет под ключом key.
// contentType игнорируется (in-memory backend не хранит метаданные объекта).
func (s *InMemoryStorage) Put(ctx context.Context, key string, content io.Reader, contentType string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	buf, err := io.ReadAll(content)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// Копируем срез, чтобы исходный буфер не мутировался снаружи.
	stored := make([]byte, len(buf))
	copy(stored, buf)
	s.objects[key] = stored
	return s.prefix + key, nil
}

// Get возвращает поток чтения по ключу.
func (s *InMemoryStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.objects[key]
	if !ok {
		return nil, ErrObjectNotFound
	}
	// Возвращаем копию данных, чтобы дальнейшие Put по ключу не аффектили читателя.
	out := make([]byte, len(data))
	copy(out, data)
	return io.NopCloser(bytes.NewReader(out)), nil
}

// Exists проверяет наличие ключа.
func (s *InMemoryStorage) Exists(ctx context.Context, key string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.objects[key]
	return ok, nil
}
