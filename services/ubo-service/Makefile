.PHONY: build test migrate lint

build:
	go build ./...

test:
	go test ./... -race

# Применяется к КАЖДОЙ схеме тенанта (ADR-0002 + ADR-0005). Production —
# через db-migrator job, здесь только локальный smoke.
migrate:
	goose -dir migrations/tenant postgres "$(DATABASE_URL)" up

lint:
	golangci-lint run ./...
