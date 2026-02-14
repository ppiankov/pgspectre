# pgspectre

PostgreSQL schema and usage auditor. Scans codebases for table/column/query references, compares with live Postgres schema and statistics, detects drift.

## What This Is

CLI tool that connects to PostgreSQL, fetches schema metadata and usage statistics from `pg_catalog`/`information_schema`, optionally scans a code repo for SQL references, and produces audit reports. Part of the Spectre family (code-vs-reality drift detection).

## What This Is NOT

- Not a PostgreSQL monitoring tool (use pg_stat_monitor for that)
- Not a migration tool
- Not a query optimizer
- Not a backup or replication tool

## Structure

```
cmd/pgspectre/main.go      — CLI entry point (Cobra)
internal/cli/              — Cobra commands (audit, check)
internal/postgres/         — pg_catalog inspector, stats collector
internal/scanner/          — code repo SQL reference scanner
internal/analyzer/         — diff engine (repo refs vs live schema)
internal/reporter/         — JSON/text report output
```

## Subcommands

- `audit` — cluster-only analysis: unused tables, unused indexes, missing stats
- `check` — code repo + cluster: missing tables, schema drift, unindexed queries

## Code Style

- Go with pgx/v5 (pure Go Postgres driver, no CGO)
- Cobra for CLI
- All queries use read-only catalog access (no superuser required)
- Conventional commits: feat:, fix:, docs:, test:, refactor:, chore:

## Testing

- `make test` (includes -race)
- `make lint` (golangci-lint)
- Target: >85% coverage

## Anti-Patterns

- NEVER modify database schema — read-only catalog queries only
- NEVER auto-drop tables or indexes — report and recommend only
- NEVER require superuser — all queries must work with read-only access
- NEVER store credentials — use connection string or env vars only
