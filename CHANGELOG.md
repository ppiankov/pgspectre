# Changelog

## [0.1.0] - 2026-02-16

### Added
- `audit` command — cluster-only analysis (unused tables/indexes, bloated indexes, missing vacuum, no primary key, duplicate indexes)
- `check` command — code repo + cluster diff (missing tables/columns, unreferenced tables, unindexed queries)
- `scan` command — offline code scanning without database connection
- PostgreSQL catalog inspector (tables, columns, indexes, stats, constraints)
- Code scanner for SQL, ORM, and migration patterns (Go, Python, JS/TS, Java, Ruby, Rust, Prisma)
- Multi-line SQL buffering across backtick blocks, triple-quote strings, and .sql files
- Column-level drift detection (5 extraction patterns)
- Index advisor — detects unindexed WHERE/ORDER BY columns
- Parallel file scanning with `--parallel` flag
- Text, JSON, and SARIF report output
- Baseline mode (`--baseline`, `--update-baseline`) for incremental adoption
- Finding suppression via inline `// pgspectre:ignore`, `.pgspectre-ignore.yml`, and config
- Config file (`.pgspectre.yml`) for thresholds, exclusions, and defaults
- `--fail-on` flag for granular CI failure control (by type or severity)
- `--min-severity` and `--type` report filters
- `--schema` flag for multi-schema databases
- Enriched findings with contextual detail (sizes, scan counts, vacuum dates)
- Structured logging via slog (`--verbose` flag)
- Connection resilience with exponential backoff retry
- Grouped text output with severity indicators and ANSI color
- Exit codes based on finding severity (high=2, medium=1, low/info=0)
- Docker image (`ghcr.io/ppiankov/pgspectre`)
- Homebrew formula (`brew install ppiankov/tap/pgspectre`)
- GitHub Action (`ppiankov/pgspectre-action@v1`)
- First-run experience with summary headers and helpful empty-state messages
