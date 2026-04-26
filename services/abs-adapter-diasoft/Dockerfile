FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /abs-adapter-diasoft ./cmd/server

FROM gcr.io/distroless/static:nonroot

COPY --from=builder /abs-adapter-diasoft /abs-adapter-diasoft

EXPOSE 9002

ENTRYPOINT ["/abs-adapter-diasoft"]
