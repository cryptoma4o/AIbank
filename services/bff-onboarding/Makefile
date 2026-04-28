.PHONY: build test tidy gqlgen run

# Сборка и тесты — стандартный поток.
build:
	go build ./...

test:
	go test ./...

tidy:
	go mod tidy

# gqlgen codegen (см. ADR-0003).  Запускается вручную при изменении схемы.
# Текущий runtime — graphql-go/graphql; gqlgen сохранён как контрактный
# slot для перехода на schema-first codegen, когда команде понадобится
# полная типизация резолверов и dataloaders.
gqlgen:
	go run github.com/99designs/gqlgen generate

run:
	go run ./cmd/server
