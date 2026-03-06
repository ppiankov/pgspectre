# pgspectre

[![CI](https://github.com/ppiankov/pgspectre/actions/workflows/ci.yml/badge.svg)](https://github.com/ppiankov/pgspectre/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/ppiankov/pgspectre)](https://goreportcard.com/report/github.com/ppiankov/pgspectre)
[![ANCC](https://img.shields.io/badge/ANCC-compliant-brightgreen)](https://ancc.dev)

**pgspectre** — PostgreSQL schema and usage auditor. Part of [SpectreHub](https://github.com/ppiankov/spectrehub).

## What it is

- Connects to PostgreSQL and fetches schema metadata and usage statistics from pg_catalog
- Scans code repositories for SQL table references across Go, Python, JS/TS, Java, Ruby, Rust, Prisma
- Compares code references against live database to find drift, unused indexes, and missing tables
- Produces deterministic output for CI/CD gating
- Outputs text, JSON, SARIF, and SpectreHub formats

## What it is NOT

- Not a PostgreSQL monitoring tool — use pg_stat_monitor for that
- Not a migration tool or query optimizer
- Not a backup or replication tool
- Does not modify any data — strictly read-only

## Quick start

### Homebrew

```sh
brew tap ppiankov/tap
brew install pgspectre
```

### From source

```sh
git clone https://github.com/ppiankov/pgspectre.git
cd pgspectre
make build
```

### Usage

```sh
pgspectre audit --dsn "postgres://localhost:5432/mydb"
```

## CLI commands

| Command | Description |
|---------|-------------|
| `pgspectre audit` | Audit PostgreSQL for unused indexes and schema drift |
| `pgspectre check` | Compare code references against live database |
| `pgspectre version` | Print version |

## SpectreHub integration

pgspectre feeds PostgreSQL drift findings into [SpectreHub](https://github.com/ppiankov/spectrehub) for unified visibility across your infrastructure.

```sh
spectrehub collect --tool pgspectre
```

## Safety

pgspectre operates in **read-only mode**. It inspects and reports — never modifies, deletes, or alters your data.

## Documentation

| Document | Contents |
|----------|----------|
| [CLI Reference](docs/cli-reference.md) | Full command reference, flags, and configuration |

## License

MIT — see [LICENSE](LICENSE).

---

Built by [Obsta Labs](https://obstalabs.dev)
