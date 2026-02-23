# pgspectre
[![CI](https://github.com/ppiankov/pgspectre/actions/workflows/ci.yml/badge.svg)](https://github.com/ppiankov/pgspectre/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![ANCC](https://img.shields.io/badge/ANCC-compliant-brightgreen)](https://ancc.dev)

PostgreSQL schema and usage auditor. Scans codebases for table/column/query references, compares with live Postgres schema and statistics, detects drift.

## What This Is

A CLI tool that:

- Connects to PostgreSQL and fetches schema metadata and usage statistics from `pg_catalog`/`information_schema`
- Scans code repositories for SQL table references (Go, Python, JS/TS, Java, Ruby, Rust, Prisma)
- Compares code references against live database to find missing tables, unused indexes, and drift
- Produces JSON or text audit reports

Part of the **Spectre** family — code-vs-reality drift detection tools.

## What This Is NOT

- Not a PostgreSQL monitoring tool (use `pg_stat_monitor` for that)
- Not a migration tool
- Not a query optimizer
- Not a backup or replication tool
- Does not modify any data — all queries are strictly read-only

## Quick Start

```bash
# Homebrew
brew install ppiankov/tap/pgspectre

# Or download latest release
curl -LO https://github.com/ppiankov/pgspectre/releases/latest/download/pgspectre_0.2.0_darwin_arm64.tar.gz
tar -xzf pgspectre_0.2.0_darwin_arm64.tar.gz
sudo mv pgspectre /usr/local/bin/

# Audit a cluster (no code scanning)
pgspectre audit --db-url "$DATABASE_URL"

# Check code repo against live cluster
pgspectre check --repo ./my-app --db-url "$DATABASE_URL"

# JSON output for CI pipelines
pgspectre audit --db-url "$DATABASE_URL" --format json
```

### Agent Integration

pgspectre is designed to be used by autonomous agents without plugins or SDKs. Single binary, deterministic output, structured JSON, bounded jobs.

Agents: read [`SKILL.md`](SKILL.md) for commands, flags, JSON output structure, and parsing examples.

Key pattern for agents: `pgspectre audit --db-url "$DATABASE_URL" --format json` then parse `.findings[]` for schema issues.

## Usage

### `audit` — Cluster-Only Analysis

Inspects PostgreSQL without code scanning. Detects:

| Finding | Severity | Description |
|---------|----------|-------------|
| `UNUSED_TABLE` | high | Table has zero sequential and index scans |
| `UNUSED_INDEX` | medium | Index has zero scans and is larger than 100 MB |
| `BLOATED_INDEX` | low | Index is larger than its table (with 1 MB floor) |
| `MISSING_VACUUM` | low | Active table never vacuumed or not vacuumed in 30+ days |
| `NO_PRIMARY_KEY` | medium | Table has no primary key constraint |
| `DUPLICATE_INDEX` | low | Two indexes with identical definitions |

```bash
pgspectre audit --db-url "$DATABASE_URL" [--format json|text]
```

### `check` — Code + Cluster Diff

Scans a code repository and compares table references against live PostgreSQL:

| Finding | Severity | Description |
|---------|----------|-------------|
| `MISSING_TABLE` | high | Referenced in code, doesn't exist in DB |
| `UNREFERENCED_TABLE` | low | Exists in DB with no activity, not in code |
| `CODE_MATCH` | info | Table exists and is referenced in code |

Also includes all `audit` findings for the cluster.

```bash
pgspectre check --repo ./app --db-url "$DATABASE_URL" [--format json|text] [--fail-on-missing]
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | No issues or low/info only |
| 1 | Medium severity findings |
| 2 | High severity findings |

## Architecture

```
cmd/pgspectre/main.go      — CLI entry point
internal/cli/              — Cobra commands (audit, check)
internal/postgres/         — pg_catalog inspector (read-only queries)
internal/scanner/          — Code repo SQL reference scanner
internal/analyzer/         — Detection engines (audit + diff)
internal/reporter/         — JSON/text report output
```

### Supported Languages

The code scanner detects SQL table references in:

- **SQL** — `SELECT FROM`, `JOIN`, `INSERT INTO`, `UPDATE`, `DELETE FROM`
- **Go** — GORM `TableName()`, `db.Table("x")`
- **Python** — SQLAlchemy `__tablename__`, Django `db_table`
- **JavaScript/TypeScript** — Prisma `@@map("x")`
- **Migrations** — `CREATE TABLE`, `ALTER TABLE`, `DROP TABLE`, `CREATE INDEX ON`

Schema-qualified references (`public.users`) are supported across all patterns.

## Building from Source

```bash
git clone https://github.com/ppiankov/pgspectre.git
cd pgspectre
make build    # produces bin/pgspectre
make test     # run tests with -race
make lint     # golangci-lint
```

## Known Limitations

- Table references using variables (`db.Query(tableName)`) are not detected
- Line-by-line scanning does not track multi-line SQL statements
- Column-level drift detection is not yet implemented
- Requires PostgreSQL 12+ (uses `pg_stat_user_tables` fields)
- No superuser required, but some stats may be limited without `pg_read_all_stats`

## Roadmap

- [ ] Column-level drift detection
- [ ] Configuration file for custom patterns and thresholds
- [ ] SpectreHub integration for centralized drift dashboards
- [ ] Multi-database scanning in a single run
- [ ] Watch mode for CI/CD integration

## License

[MIT](LICENSE)
