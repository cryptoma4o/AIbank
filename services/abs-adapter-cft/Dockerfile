FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /abs-adapter-cft ./cmd/server

FROM gcr.io/distroless/static:nonroot

COPY --from=builder /abs-adapter-cft /abs-adapter-cft

EXPOSE 9001

ENTRYPOINT ["/abs-adapter-cft"]
