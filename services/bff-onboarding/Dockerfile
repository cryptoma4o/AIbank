FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /bff-onboarding ./cmd/server

FROM gcr.io/distroless/static:nonroot

COPY --from=builder /bff-onboarding /bff-onboarding

EXPOSE 8094

USER nonroot:nonroot

ENTRYPOINT ["/bff-onboarding"]
