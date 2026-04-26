FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /abs-adapter-rs-bank ./cmd/server

FROM gcr.io/distroless/static:nonroot

COPY --from=builder /abs-adapter-rs-bank /abs-adapter-rs-bank

EXPOSE 9003

ENTRYPOINT ["/abs-adapter-rs-bank"]
