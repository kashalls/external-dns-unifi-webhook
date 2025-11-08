# Testing Guide

This document describes how to test the external-dns-unifi-webhook provider.

## Prerequisites

- Go 1.24 or later
- Docker or Podman (for container builds)
- [act](https://github.com/nektos/act) (optional, for local CI testing)
- Access to a UniFi controller for integration testing

## Running Unit Tests

Run all unit tests:

```bash
go test ./...
```

Run tests with coverage:

```bash
go test -cover ./...
```

Generate detailed coverage report:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

Run tests for a specific package:

```bash
go test ./pkg/webhook/
go test ./internal/unifi/
```

## Running Linters

This project uses [golangci-lint](https://golangci-lint.run/) for code quality checks.

Install golangci-lint:

```bash
# Binary will be installed at $(go env GOPATH)/bin/golangci-lint
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
```

Run all configured linters:

```bash
golangci-lint run
```

Run specific linters:

```bash
# Error handling linters
golangci-lint run --enable err113,errcheck,errchkjson,wrapcheck

# Code quality linters
golangci-lint run --enable staticcheck,gosimple,unused
```

Fix auto-fixable issues:

```bash
golangci-lint run --fix
```

## Linting Documentation

Check markdown documentation files:

```bash
npx markdownlint-cli2 "**/*.md"
```

Auto-fix markdown issues:

```bash
npx markdownlint-cli2 "**/*.md" --fix
```

Configuration is in `.markdownlint.yaml`.

## Building the Binary

Build for your current platform:

```bash
go build -o external-dns-unifi-webhook ./cmd/webhook
```

Build with version information:

```bash
VERSION=$(git describe --tags --always --dirty)
REVISION=$(git rev-parse HEAD)
go build -ldflags "-X main.Version=${VERSION} -X main.Revision=${REVISION}" -o external-dns-unifi-webhook ./cmd/webhook
```

Cross-compile for different platforms:

```bash
# Linux AMD64
GOOS=linux GOARCH=amd64 go build -o external-dns-unifi-webhook-linux-amd64 ./cmd/webhook

# Linux ARM64
GOOS=linux GOARCH=arm64 go build -o external-dns-unifi-webhook-linux-arm64 ./cmd/webhook
```

## Building Container Images

Build with Docker:

```bash
docker build -t external-dns-unifi-webhook:local -f Containerfile .
```

Build with Podman:

```bash
podman build -t external-dns-unifi-webhook:local -f Containerfile .
```

Build for multiple platforms:

```bash
docker buildx build --platform linux/amd64,linux/arm64 -t external-dns-unifi-webhook:local -f Containerfile .
```

## Testing Container Images

Run the container locally:

```bash
docker run --rm -p 8888:8888 \
  -e UNIFI_HOST=https://your-unifi-controller \
  -e UNIFI_API_KEY=your-api-key \
  external-dns-unifi-webhook:local
```

Test health endpoints:

```bash
# Health check
curl http://localhost:8080/healthz

# Readiness check
curl http://localhost:8080/readyz

# Metrics
curl http://localhost:8080/metrics
```

## Local CI Testing with act

Test GitHub Actions workflows locally using [act](https://github.com/nektos/act):

```bash
# Install act (macOS)
brew install act

# Run PR checks
act pull_request --workflows .github/workflows/pr-check.yaml

# Run specific job
act pull_request --workflows .github/workflows/pr-check.yaml --job lint

# Use specific runner image
act pull_request --workflows .github/workflows/pr-check.yaml --container-architecture linux/amd64
```

## Integration Testing

### Manual Testing with UniFi Controller

1. Set up environment variables:

```bash
export UNIFI_HOST=https://192.168.1.1
export UNIFI_API_KEY=your-api-key
export UNIFI_SKIP_TLS_VERIFY=true
export LOG_LEVEL=debug
```

2. Run the webhook provider:

```bash
go run ./cmd/webhook
```

3. Test the endpoints:

```bash
# Negotiate (get domain filter)
curl -H "Accept: application/external.dns.webhook+json;version=1" \
  http://localhost:8888/

# Get records
curl -H "Accept: application/external.dns.webhook+json;version=1" \
  http://localhost:8888/records

# Apply changes
curl -X POST \
  -H "Content-Type: application/external.dns.webhook+json;version=1" \
  -H "Accept: application/external.dns.webhook+json;version=1" \
  -d '{"Create":[{"dnsName":"test.example.com","recordType":"A","targets":["192.168.1.100"]}]}' \
  http://localhost:8888/records
```

### Testing with external-dns

Deploy the webhook provider alongside external-dns in a test Kubernetes cluster to verify end-to-end functionality.

## Troubleshooting

### Debug Logging

Enable debug logging for detailed output:

```bash
export LOG_LEVEL=debug
go run ./cmd/webhook
```

### Common Issues

**TLS Certificate Errors:**

```bash
export UNIFI_SKIP_TLS_VERIFY=true
```

**Authentication Failures:**

- Verify API key is correct
- Check that user has Site Admin permissions
- Ensure controller is accessible from your network

**Build Failures:**

```bash
# Clean go cache
go clean -cache -modcache

# Reinstall dependencies
go mod download
go mod tidy
```

## Performance Testing

Test webhook performance under load:

```bash
# Install hey (HTTP load testing tool)
go install github.com/rakyll/hey@latest

# Load test the records endpoint
hey -n 1000 -c 10 -H "Accept: application/external.dns.webhook+json;version=1" \
  http://localhost:8888/records
```

## Code Coverage Goals

- Overall coverage: >70%
- Critical packages (webhook, unifi): >80%
- New code: 100% coverage required

Check current coverage:

```bash
go test -cover ./... | grep -E "coverage:|total:"
```
