.PHONY: build test lint migrate migrate-down tidy

BINARY   := notification-service
PKG      := ./...
MIGRATE  := goose
DB_DSN   ?= postgres://aibank:aibank_dev@localhost:5432/aibank?sslmode=disable
MIGRATIONS_DIR := migrations

build:
	CGO_ENABLED=0 go build -o bin/$(BINARY) ./cmd/server

test:
	go test -race -count=1 $(PKG)

lint:
	golangci-lint run $(PKG)

tidy:
	go mod tidy

migrate:
	$(MIGRATE) -dir $(MIGRATIONS_DIR) postgres "$(DB_DSN)" up

migrate-down:
	$(MIGRATE) -dir $(MIGRATIONS_DIR) postgres "$(DB_DSN)" down
