FROM golang:1.23-alpine AS builder

WORKDIR /app
# Сначала копируем общий пакет — replace в go.mod указывает на ../../packages/outbox.
# Это требует, чтобы build context был корнем монорепо (см. README).
COPY packages/outbox /workspace/packages/outbox
COPY tools/outbox-cleanup /workspace/tools/outbox-cleanup

WORKDIR /workspace/tools/outbox-cleanup
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /outbox-cleanup ./cmd/outbox-cleanup

FROM gcr.io/distroless/static:nonroot

COPY --from=builder /outbox-cleanup /outbox-cleanup

ENTRYPOINT ["/outbox-cleanup"]
