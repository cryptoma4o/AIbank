FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /ext-egrul ./cmd/server

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /ext-egrul /ext-egrul
EXPOSE 8090
ENTRYPOINT ["/ext-egrul"]
