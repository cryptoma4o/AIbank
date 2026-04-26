# syntax=docker/dockerfile:1

# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /identity-service ./cmd/server

# Final stage
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /identity-service /identity-service

EXPOSE 8086

USER nonroot:nonroot

ENTRYPOINT ["/identity-service"]
