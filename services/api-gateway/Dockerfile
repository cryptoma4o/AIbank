# Stage 1: build
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /api-gateway ./cmd/server

# Stage 2: minimal runtime image
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /api-gateway /api-gateway

EXPOSE 8000

USER nonroot:nonroot

ENTRYPOINT ["/api-gateway"]
