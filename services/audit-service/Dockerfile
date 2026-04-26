FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /audit-service ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=builder /audit-service /audit-service
EXPOSE 8081
ENTRYPOINT ["/audit-service"]
