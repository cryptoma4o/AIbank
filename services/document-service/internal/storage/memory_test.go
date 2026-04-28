package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestInMemoryStoragePutGetExists(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewInMemoryStorage()

	const key = "tenants/alfa/applications/app_1/uuid-passport.pdf"
	payload := []byte("hello, document")

	storagePath, err := s.Put(ctx, key, bytes.NewReader(payload), "application/pdf")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !strings.HasSuffix(storagePath, key) {
		t.Errorf("storagePath %q must end with key %q", storagePath, key)
	}

	exists, err := s.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Fatal("expected object to exist after Put")
	}

	rc, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("Get returned %q, want %q", got, payload)
	}
}

func TestInMemoryStorageMissingKey(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewInMemoryStorage()

	exists, err := s.Exists(ctx, "missing")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists {
		t.Fatal("expected non-existent key to be absent")
	}

	_, err = s.Get(ctx, "missing")
	if err == nil {
		t.Fatal("expected error for missing key")
	}
	if !errors.Is(err, ErrObjectNotFound) {
		t.Errorf("expected ErrObjectNotFound, got %v", err)
	}
}

func TestInMemoryStorageConcurrent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewInMemoryStorage()

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := "k" + string(rune('0'+i%10))
			_, err := s.Put(ctx, key, strings.NewReader("payload"), "text/plain")
			if err != nil {
				t.Errorf("concurrent Put: %v", err)
			}
		}()
	}
	wg.Wait()
}
