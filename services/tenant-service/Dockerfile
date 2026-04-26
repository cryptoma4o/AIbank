FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /tenant-service ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=builder /tenant-service /tenant-service
EXPOSE 8080
ENTRYPOINT ["/tenant-service"]
