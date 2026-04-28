FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /tenant-cli ./cmd/tenant-cli

# Distroless static — no shell, no package manager.  config-validate requires
# python3 + pyyaml + jsonschema; for that path use the python:3.12-slim image
# in CI instead of this distroless image, or shell out via a separate sidecar.
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /tenant-cli /tenant-cli

ENTRYPOINT ["/tenant-cli"]
