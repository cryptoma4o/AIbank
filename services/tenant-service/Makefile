.PHONY: build test migrate lint

build:
	go build ./...

test:
	go test ./... -race

migrate:
	goose -dir migrations postgres "$(DATABASE_URL)" up

lint:
	golangci-lint run ./...
