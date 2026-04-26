# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /bff-admin ./cmd/server

# Runtime stage
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /bff-admin /bff-admin

EXPOSE 8095

USER nonroot:nonroot

ENTRYPOINT ["/bff-admin"]
