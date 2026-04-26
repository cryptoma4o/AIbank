FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /abs-connector ./cmd/server

FROM gcr.io/distroless/static:nonroot

COPY --from=builder /abs-connector /abs-connector

EXPOSE 8093

ENTRYPOINT ["/abs-connector"]
