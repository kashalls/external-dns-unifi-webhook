# Contributing Guide

Thank you for your interest in contributing to external-dns-unifi-webhook! This document provides guidelines and instructions for contributing to the project.

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](.github/CODE_OF_CONDUCT.md). Be respectful and constructive in all interactions. We welcome contributions from everyone regardless of experience level.

## Getting Started

### Prerequisites

- Go 1.24 or later
- Git
- Docker or Podman
- A UniFi controller for testing (optional but recommended)

### Fork and Clone

1. Fork the repository on GitHub
2. Clone your fork locally:

```bash
git clone https://github.com/YOUR_USERNAME/external-dns-unifi-webhook.git
cd external-dns-unifi-webhook
```

3. Add upstream remote:

```bash
git remote add upstream https://github.com/kashalls/external-dns-unifi-webhook.git
```

### Development Workflow

1. Create a feature branch:

```bash
git checkout -b feature/your-feature-name
```

2. Make your changes
3. Run tests and linters:

```bash
go test ./...
golangci-lint run
```

4. Commit your changes:

```bash
git add .
git commit -s -m "feat: add your feature description"
```

Note: Use `-s` flag to sign off your commits (required).

5. Push to your fork:

```bash
git push origin feature/your-feature-name
```

6. Open a Pull Request on GitHub

## Commit Message Guidelines

We follow [Semantic Commit Messages](https://www.conventionalcommits.org/) format:

```text
<type>(<scope>): <subject>

<body>

<footer>
```

### Types

- `feat`: New features
- `fix`: Bug fixes
- `docs`: Documentation changes
- `style`: Code style changes (formatting, whitespace)
- `refactor`: Code refactoring without functionality changes
- `test`: Adding or updating tests
- `chore`: Maintenance tasks, dependencies
- `ci`: CI/CD pipeline changes
- `perf`: Performance improvements
- `build`: Build system changes

### Examples

```text
feat(webhook): add support for SRV records

Implement SRV record handling in the webhook provider.
SRV records require priority, weight, and port fields.

Signed-off-by: Your Name <your.email@example.com>
```

```text
fix(client): handle rate limiting from UniFi API

Add exponential backoff retry logic when UniFi API
returns 429 status code.

Closes #123

Signed-off-by: Your Name <your.email@example.com>
```

### Commit Signing

All commits must be signed off. Add the `-s` flag when committing:

```bash
git commit -s -m "your commit message"
```

This adds a `Signed-off-by` line certifying that you have the right to submit the work under the project's license.

## Code Style

### Go Code Style

- Follow [Effective Go](https://go.dev/doc/effective_go) guidelines
- Use `gofmt` for formatting (enforced by CI)
- Run `golangci-lint` before submitting (enforced by CI)
- Keep functions focused and testable
- Write clear variable and function names

### Error Handling

- Always check and handle errors explicitly
- Use custom error types from `internal/unifi/errors.go`
- Wrap errors with context using `errors.Wrap()` or `errors.Wrapf()`
- Never ignore errors with `_` unless absolutely necessary

Example:

```go
// Good
data, err := fetchData()
if err != nil {
    return errors.Wrap(err, "failed to fetch data")
}

// Bad
data, _ := fetchData()
```

### Testing

- Write unit tests for new functionality
- Maintain or improve code coverage
- Test error cases and edge cases
- Use table-driven tests where appropriate

Example:

```go
func TestParseRecord(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    Record
        wantErr bool
    }{
        {
            name:  "valid A record",
            input: "example.com A 192.168.1.1",
            want:  Record{Name: "example.com", Type: "A", Value: "192.168.1.1"},
        },
        {
            name:    "invalid format",
            input:   "invalid",
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ParseRecord(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("ParseRecord() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
                t.Errorf("ParseRecord() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## Pull Request Process

### Before Submitting

1. **Update your branch** with latest upstream changes:

```bash
git fetch upstream
git rebase upstream/main
```

2. **Run all checks**:

```bash
# Tests
go test ./...

# Linting
golangci-lint run

# Build
go build ./cmd/webhook
```

3. **Update documentation** if needed (README.md, TESTING.md, etc.)

### PR Description

Provide a clear description of your changes:

- **What** does this PR do?
- **Why** is this change needed?
- **How** does it work?
- Link related issues with `Closes #123` or `Fixes #123`

### Review Process

- Maintainers will review your PR
- Address review comments by pushing new commits
- Once approved, a maintainer will merge your PR
- PRs are typically squash-merged to maintain clean history

## Project Structure

```text
.
├── cmd/webhook/        # Main application entry point
│   └── init/           # Initialization packages
├── internal/           # Private application code
│   └── unifi/          # UniFi client and provider logic
├── pkg/                # Public packages
│   ├── metrics/        # Prometheus metrics
│   └── webhook/        # Webhook HTTP handlers
├── .github/            # GitHub Actions workflows
├── Containerfile       # Container build definition
└── README.md           # Project documentation
```

## Adding New Features

### API Changes

If adding new API endpoints or changing existing ones:

1. Update the webhook handlers in `pkg/webhook/`
2. Add tests for new endpoints
3. Update API documentation in README.md
4. Consider backward compatibility

### UniFi Integration Changes

If modifying UniFi controller integration:

1. Update client code in `internal/unifi/client.go`
2. Test against real UniFi controller if possible
3. Document UniFi version requirements
4. Consider rate limiting and error handling

### Metrics

When adding new metrics:

1. Define metrics in `pkg/metrics/metrics.go`
2. Follow Prometheus naming conventions
3. Add appropriate labels (e.g., `provider`, `operation`, `status`)
4. Document metrics in code comments

## Reporting Issues

### Bug Reports

Include:

- Clear description of the issue
- Steps to reproduce
- Expected vs actual behavior
- UniFi controller version
- Webhook provider version
- Relevant logs (use `LOG_LEVEL=debug`)

### Feature Requests

Include:

- Clear description of the feature
- Use case and motivation
- Proposed implementation (if any)
- Potential impact on existing functionality

## Questions and Support

- Open a GitHub Discussion for questions
- Join the [Home Operations Discord](https://discord.gg/home-operations)
- Check existing issues and pull requests

## License

By contributing, you agree that your contributions will be licensed under the project's license.

## Recognition

Contributors will be recognized in:

- Git commit history
- GitHub contributors page
- Release notes for significant contributions

Thank you for contributing to external-dns-unifi-webhook!
