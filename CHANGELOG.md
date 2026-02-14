# Changelog

## [0.1.0] - 2026-02-14

### Added
- `audit` command — cluster-only analysis (unused tables/indexes, bloated indexes, missing vacuum, no primary key, duplicate indexes)
- `check` command — code repo + cluster diff (missing tables, unreferenced tables, code matches)
- PostgreSQL catalog inspector (tables, columns, indexes, stats, constraints)
- Code scanner for SQL, ORM, and migration patterns (Go, Python, JS/TS, Java, Ruby, Rust, Prisma)
- Text and JSON report output
- Exit codes based on finding severity (high=2, medium=1, low/info=0)
