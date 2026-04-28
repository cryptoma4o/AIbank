FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /mock-smev ./cmd/server

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /mock-smev /mock-smev
EXPOSE 8500
ENTRYPOINT ["/mock-smev"]
