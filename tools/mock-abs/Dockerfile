FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o mock-abs ./main.go

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /app/mock-abs /mock-abs
EXPOSE 9000
ENTRYPOINT ["/mock-abs"]
