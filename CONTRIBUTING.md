# Contributing to pgspectre

## Prerequisites

- Go 1.22+
- PostgreSQL 14+ (for integration tests)
- [golangci-lint](https://golangci-lint.run/welcome/install/) v2+

## Quick Start

```bash
git clone https://github.com/ppiankov/pgspectre.git
cd pgspectre
make deps
make test
make build
```

## Development Commands

| Command | Description |
|---------|-------------|
| `make build` | Build binary to `bin/pgspectre` |
| `make test` | Run all tests with `-race` and coverage |
| `make lint` | Run golangci-lint |
| `make fmt` | Format code with `go fmt` |
| `make vet` | Run `go vet` |
| `make coverage` | Generate coverage report |
| `make coverage-html` | Open coverage in browser |

## Architecture

```
cmd/pgspectre/         CLI entry point (delegates to internal/cli)
internal/
  cli/                 Cobra commands: audit, check, scan
  postgres/            PostgreSQL catalog inspector (pgx/v5, read-only)
  scanner/             Code repo SQL reference scanner (regex-based)
  analyzer/            Diff engine — compares code refs vs live schema
  reporter/            Output formatters: text, JSON, SARIF
  baseline/            Fingerprint-based finding suppression
  suppress/            Rule-based finding suppression (.pgspectre-ignore.yml)
  config/              YAML configuration loader
  logging/             Structured logging (slog)
```

## Testing

Tests are mandatory for all new code. Target: >85% coverage.

```bash
make test                    # unit tests (no database required)
make test-integration        # requires local PostgreSQL
```

All tests use `-race`. Tests must be deterministic — no flaky or timing-dependent tests.

## Pull Request Conventions

1. **Conventional commits**: `feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `chore:`
2. **One commit message line**, max 72 chars, imperative mood
3. **All tests must pass**: `make test && make lint`
4. **Coverage must not decrease** for touched packages
5. **No superuser queries**: all SQL must work with read-only catalog access

## Code Style

- Minimal `main.go` delegating to `internal/`
- Package names: short, single-word (`cli`, `scanner`, `analyzer`)
- File names: `snake_case.go`
- Comments explain "why" not "what"
- No magic numbers — use named constants

## What Not to Do

- Never modify database schema — read-only catalog queries only
- Never auto-drop tables or indexes — report and recommend only
- Never require superuser — all queries must work with read-only access
- Never store credentials — use connection strings or environment variables
