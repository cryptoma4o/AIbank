FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /db-migrator ./cmd/db-migrator

FROM gcr.io/distroless/static:nonroot

# Migrations expected to be mounted at /migrations or baked in via downstream image.
COPY --from=builder /db-migrator /db-migrator

ENTRYPOINT ["/db-migrator"]
