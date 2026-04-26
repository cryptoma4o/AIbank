# Stage 1: build
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /ubo-service ./cmd/server

# Stage 2: minimal runtime
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /ubo-service /ubo-service

EXPOSE 8088

USER nonroot:nonroot

ENTRYPOINT ["/ubo-service"]
