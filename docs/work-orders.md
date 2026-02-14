# Work Orders — pgspectre

## WO-01: Project Scaffold

**Goal:** Create Go project structure matching Spectre family conventions.

### Steps
1. `go mod init github.com/ppiankov/pgspectre`
2. Create `cmd/pgspectre/main.go` — minimal, delegates to `internal/cli`
3. Create `internal/cli/root.go` — Cobra root with version, `--db-url` persistent flag
4. Create `Makefile` — build, test, lint, fmt, vet, clean (copy pattern from kafkaspectre)
5. Add `.github/workflows/ci.yml` and `release.yml` from claude-skills templates
6. Add `.gitignore` matching other spectre tools

### Acceptance
- `make build` produces `bin/pgspectre`
- `./bin/pgspectre version` prints version
- `make test` passes (even with no tests yet)

---

## WO-02: PostgreSQL Inspector

**Goal:** Connect to PostgreSQL and fetch schema metadata + usage statistics.

### Details
Create `internal/postgres/` package:
- `inspector.go` — connect via pgx/v5, query catalog
- `types.go` — TableInfo, IndexInfo, ColumnInfo, Config structs

### Catalog Queries (all read-only, no superuser)
- Tables: `information_schema.tables` (name, type, row estimate from `pg_class`)
- Columns: `information_schema.columns` (name, type, nullable, default)
- Indexes: `pg_indexes` + `pg_stat_user_indexes` (name, definition, idx_scan count, size)
- Table stats: `pg_stat_user_tables` (seq_scan, idx_scan, n_live_tup, last_vacuum, last_analyze)
- Constraints: `pg_constraint` (primary keys, foreign keys, unique)

### Acceptance
- Connects to a local Postgres with `--db-url`
- Fetches metadata without requiring superuser
- Handles connection errors gracefully

---

## WO-03: Audit Command

**Goal:** Cluster-only analysis — find problems without code scanning.

### Detections
- **Unused tables**: `seq_scan = 0 AND idx_scan = 0` (no reads at all)
- **Unused indexes**: `idx_scan = 0` with size > threshold
- **Bloated indexes**: index size > table size
- **Missing vacuum**: `last_autovacuum IS NULL` or older than 30 days on active tables
- **Tables without primary key**
- **Duplicate indexes**: same definition on same table

### Steps
1. Create `internal/cli/audit.go` — Cobra `audit` subcommand
2. Create `internal/analyzer/audit.go` — detection logic
3. Risk scoring: high (missing tables), medium (unused indexes > 100MB), low (missing vacuum)
4. Reporter: JSON and text output

### Acceptance
- `pgspectre audit --db-url postgres://...` produces report
- `--format json|text` flag
- Exit code reflects severity
- `make test` passes with -race

---

## WO-04: Code Scanner

**Goal:** Scan code repo for SQL table/column references.

### Details
Create `internal/scanner/` package:
- `sql_scanner.go` — regex extraction of table names from raw SQL strings
- `orm_scanner.go` — detect ORM patterns (SQLAlchemy, Django, GORM, Prisma)
- `migration_scanner.go` — parse CREATE TABLE/ALTER TABLE from migration files

### Extracts
- Table name, columns referenced, file + line, context (SELECT/INSERT/UPDATE/DELETE)

### Acceptance
- Scans a Go/Python/JS project and finds table references
- Handles false positives gracefully (exclude comments, strings)
- `make test` passes with -race

---

## WO-05: Check Command (Code + Cluster Diff)

**Goal:** Compare code repo references against live PostgreSQL schema.

### Detections
- **MISSING_TABLE**: referenced in code, doesn't exist in DB
- **UNUSED_TABLE**: exists in DB, not referenced in code, no recent scans
- **SCHEMA_DRIFT**: column mismatch (code expects column DB doesn't have)
- **UNINDEXED_QUERY**: code queries on columns without matching index
- **OK**: everything matches

### Steps
1. Create `internal/cli/check.go` — Cobra `check` subcommand
2. Create `internal/analyzer/diff.go` — comparison engine
3. Merge audit findings with diff findings
4. Add `--repo`, `--fail-on-missing`, `--fail-on-drift` flags

### Acceptance
- `pgspectre check --repo ./app --db-url postgres://...` produces report
- JSON output compatible with spectrehub contract

---

## WO-06: Tests and Release v0.1.0

**Goal:** Full test suite and tagged release.

### Steps
1. Unit tests for inspector (mock pgx pool), analyzer, scanner, reporter
2. Integration test with dockerized Postgres (optional, CI-only)
3. Coverage >85% on analyzer, scanner, reporter
4. GoReleaser config — linux/darwin/windows, amd64/arm64
5. README: description, install, usage, architecture, license
6. Tag v0.1.0

### Acceptance
- `make test` passes with -race
- `make lint` clean
- `gh release list` shows v0.1.0
- spectrehub can ingest pgspectre JSON output

---

## WO-07: Column-level drift detection

**Goal:** Extend code scanner and check command to detect column-level drift, not just table-level.

Scanner already extracts table names. This adds column extraction from `SELECT col`, `WHERE col =`, `ORDER BY col`, `INSERT INTO t (col)`, `UPDATE t SET col =` and compares against `information_schema.columns`.

### Detections
- **MISSING_COLUMN**: code references `users.email` but column doesn't exist
- **UNUSED_COLUMN**: column exists in DB, never referenced in code
- **TYPE_MISMATCH**: code casts suggest different type than schema (stretch goal)

### Steps
1. Extend `internal/scanner/sql_scanner.go` — extract column names with table context
2. Extend `internal/scanner/orm_scanner.go` — extract field mappings from ORM models
3. Add `internal/analyzer/column_diff.go` — compare code columns vs `information_schema.columns`
4. Integrate into `check` command output

### Acceptance
- `pgspectre check` reports column-level drift alongside table-level
- JSON output includes column findings with file + line references
- `make test` passes with -race

---

## WO-08: Config file (.pgspectre.yml)

**Goal:** Support a config file for custom thresholds, ignore patterns, and schema filters.

Every production use needs to ignore certain tables (migrations, Django admin, audit logs) and tune thresholds (vacuum days, bloat size, scan count).

### Config shape
```yaml
thresholds:
  unused_index_min_size: 100MB
  vacuum_stale_days: 30
  bloat_ratio: 2.0
ignore:
  tables:
    - django_migrations
    - django_admin_log
    - "audit_*"
  schemas:
    - pg_catalog
    - information_schema
schemas:
  - public
  - payments
```

### Steps
1. Create `internal/config/config.go` — YAML parsing, defaults, validation
2. Create `internal/config/config_test.go`
3. Add `--config` flag to root command, default lookup: `.pgspectre.yml`, `~/.pgspectre.yml`
4. Wire thresholds into analyzer, ignore patterns into scanner and inspector

### Acceptance
- `pgspectre audit --config .pgspectre.yml` respects overrides
- Missing config file uses sensible defaults (not an error)
- `make test` passes with -race

---

## Non-Goals

- No schema migrations
- No query optimization
- No connection pooling
- No write operations
- No web UI
