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

## WO-07: Column-level drift detection ✅

**Goal:** Extend code scanner and check command to detect column-level drift, not just table-level.

### Implementation
- Added `ColumnRef` type and `ScanLineColumns()` to scanner with 5 column extraction patterns: dotted refs, SELECT columns, WHERE/AND/OR conditions, ORDER/GROUP BY, INSERT column lists
- Added `FindingMissingColumn` finding type (medium severity)
- Extended `Diff()` to compare column references against `snap.Columns`
- Rejects false positives: SQL keywords, functions, uppercase method names (e.g., `fmt.Println`)
- 10 new scanner tests, 3 new analyzer tests

### Files
- `internal/scanner/types.go` — added ColumnRef, extended ScanResult
- `internal/scanner/patterns.go` — column patterns, ScanLineColumns(), isValidColumnName()
- `internal/scanner/scanner.go` — wired column scanning, uniqueColumns()
- `internal/analyzer/types.go` — FindingMissingColumn
- `internal/analyzer/diff.go` — column drift detection

---

## WO-08: Config file (.pgspectre.yml) ✅

**Goal:** Support a config file for custom thresholds, ignore patterns, and defaults.

### Implementation
- Created `internal/config/` package following mongospectre pattern
- YAML config with `go.yaml.in/yaml/v3`: db_url, thresholds, exclude, defaults
- `Load(dir)` tries `.pgspectre.yml` in CWD, then `~/.pgspectre.yml`, falls back to `DefaultConfig()`
- Added `AuditOptions` struct to analyzer for threshold/exclusion passthrough
- `Audit()` and `Diff()` accept `AuditOptions` with configurable vacuum days, bloat threshold, table/schema exclusions
- CLI loads config in `PersistentPreRunE`, applies db_url/format/timeout defaults

### Config shape
```yaml
db_url: "postgres://localhost:5432/mydb"
thresholds:
  vacuum_days: 14
  bloat_min_bytes: 2097152
exclude:
  tables: [migrations, schema_versions]
  schemas: [pg_catalog]
defaults:
  format: json
  timeout: "60s"
```

### Files
- `internal/config/config.go` — Config, Load(), DefaultConfig(), TimeoutDuration()
- `internal/config/config_test.go` — 100% coverage
- `internal/analyzer/types.go` — AuditOptions, DefaultAuditOptions()
- `internal/analyzer/audit.go` — configurable thresholds and exclusions
- `internal/cli/root.go` — config loading, auditOptsFromConfig()

---

## WO-09: Multi-line SQL buffering

**Goal:** Scanner misses SQL split across lines. Buffer between SQL markers and scan assembled blocks.

### Context
Current scanner is line-by-line (`bufio.Scanner`). A `SELECT * FROM\n  users` is invisible — the FROM pattern never matches because `users` is on the next line. This is the #1 false-negative source.

### Approach
Not a full SQL parser. Context-aware line joining:
- Detect SQL block start: backtick strings (Go), triple-quotes (Python), heredocs, `.sql` file boundaries
- Buffer lines until block end
- Join and run existing `ScanLine()` + `ScanLineColumns()` on the assembled block
- Preserve original line number for the block start (for finding locations)
- Single-line mode still works as fallback for non-block SQL

### Files
- `internal/scanner/buffer.go` — `SQLBuffer` with `Feed(line)` / `Flush()` / `InBlock() bool`
- `internal/scanner/scanner.go` — integrate buffer into `ScanFile()`
- `internal/scanner/buffer_test.go`

### Acceptance
- Multi-line `SELECT ... FROM users` across 3 lines detects `users` table
- Go backtick strings, Python triple-quotes, raw `.sql` files all work
- Existing single-line tests still pass
- `make test` passes with -race

---

## WO-10: `scan` subcommand (offline mode)

**Goal:** Scan code without a live database connection. Enables CI pre-commit hooks and spectrehub integration.

### Context
Both `audit` and `check` require `--db-url`. Most CI pipelines don't have DB access. Teams need `pgspectre scan --repo .` to extract table/column references offline.

### Steps
1. Create `internal/cli/scan.go` — Cobra `scan` subcommand
2. `--repo` flag (required), `--format text|json` flag
3. Runs scanner only, outputs `ScanResult` as structured report
4. Exit code 0 always (no severity without DB comparison)
5. JSON output compatible with spectrehub ingest contract

### Acceptance
- `pgspectre scan --repo ./app` produces table/column reference report
- `--format json` output is machine-parseable
- Works without `--db-url`
- `make test` passes with -race

---

## WO-11: Baseline mode

**Goal:** First run produces N findings. Team triages. Next run flags only new findings. Without this, tool is noisy on day 1 and disabled on day 2.

### Steps
1. Create `internal/baseline/baseline.go` — `Baseline` struct with finding fingerprints
2. Fingerprint: hash of (finding type + schema + table + column + index)
3. `pgspectre check --baseline .pgspectre-baseline.json` reads baseline, suppresses known findings
4. `pgspectre check --update-baseline .pgspectre-baseline.json` writes current findings as new baseline
5. Reporter shows "N findings (M suppressed by baseline)" in summary
6. `--baseline` works with both `audit` and `check` commands

### Files
- `internal/baseline/baseline.go` — Load(), Save(), Contains(), fingerprint logic
- `internal/baseline/baseline_test.go`
- `internal/cli/audit.go`, `internal/cli/check.go` — wire `--baseline`, `--update-baseline` flags

### Acceptance
- First run with `--update-baseline` saves findings
- Second run with `--baseline` suppresses previously seen findings
- New findings still reported
- `make test` passes with -race

---

## WO-12: SARIF output

**Goal:** GitHub Security tab, GitLab SAST, and VS Code all consume SARIF. One format unlocks three integration points.

### Steps
1. Create `internal/reporter/sarif.go` — SARIF 2.1.0 writer
2. Map finding types to SARIF rule IDs (`pgspectre/MISSING_TABLE`, etc.)
3. Map severity to SARIF level (high→error, medium→warning, low→note)
4. Include artifact locations with file path + line number (for code-side findings)
5. Add `sarif` to `--format` flag in all commands
6. Schema-only findings (audit) use DB URL as artifact location

### Acceptance
- `pgspectre check --format sarif` produces valid SARIF 2.1.0
- Output validates against SARIF JSON schema
- GitHub Security tab ingests it via `upload-sarif` action
- `make test` passes with -race

---

## WO-13: Finding suppression

**Goal:** Teams will have false positives. If they can't silence them, they'll silence the whole tool.

### Mechanisms
1. Inline: `// pgspectre:ignore` comment on the line with the table/column reference
2. File-level: `.pgspectre-ignore.yml` with table/finding-type suppressions
3. Config-level: extend `.pgspectre.yml` `exclude.findings` list

### Ignore file format
```yaml
# .pgspectre-ignore.yml
suppressions:
  - table: legacy_audit_log
    reason: "Intentionally unused, retained for compliance"
  - table: temp_migration_*
    type: UNUSED_TABLE
    reason: "Migration tables cleaned up monthly"
```

### Steps
1. Create `internal/suppress/suppress.go` — load ignore file, check inline comments
2. Scanner: detect `pgspectre:ignore` comments and mark refs as suppressed
3. Analyzer: filter suppressed findings before reporting
4. Reporter: show "N suppressed" count in summary

### Acceptance
- Inline `// pgspectre:ignore` suppresses the finding for that line
- `.pgspectre-ignore.yml` suppressions work with glob patterns
- Suppressed count shown in summary
- `make test` passes with -race

---

## WO-14: `--fail-on` granularity

**Goal:** CI needs `--fail-on MISSING_TABLE,MISSING_COLUMN` not just `--fail-on-missing`.

### Steps
1. Add `--fail-on` flag accepting comma-separated finding types
2. Finding types: `MISSING_TABLE`, `MISSING_COLUMN`, `UNUSED_TABLE`, `UNUSED_INDEX`, `BLOATED_INDEX`, `MISSING_VACUUM`, `NO_PRIMARY_KEY`, `DUPLICATE_INDEX`, `SCHEMA_DRIFT`
3. Exit code 2 if any matching finding exists, regardless of severity
4. Deprecate `--fail-on-missing` (keep working, alias to `--fail-on MISSING_TABLE`)
5. `--fail-on high` as shorthand for "any high-severity finding"

### Acceptance
- `pgspectre check --fail-on MISSING_TABLE,MISSING_COLUMN` exits 2 on match
- `--fail-on-missing` still works (backward compatible)
- `make test` passes with -race

---

## WO-15: Index advisor

**Goal:** Cross-reference WHERE/JOIN columns from code against database indexes. pgspectre's killer feature — it knows both your code and your database.

### Context
Inspector already fetches index definitions (`pg_indexes`) and column stats (`pg_stat_user_indexes`). Scanner extracts column references with query context (SELECT/WHERE/JOIN). Cross-referencing them finds unindexed query patterns.

### Detection
- For each column referenced in WHERE/JOIN/ORDER BY context in code, check if a matching index exists
- New finding type: `UNINDEXED_QUERY` (medium severity)
- Message: "Column `orders.user_id` referenced in WHERE clause (14 locations) but has no index"
- Include reference count to prioritize high-traffic columns

### Steps
1. Extend `internal/scanner/types.go` — add query context to `ColumnRef` (WHERE vs SELECT vs ORDER BY)
2. Create `internal/analyzer/index_advisor.go` — cross-reference column refs against index definitions
3. Filter: only flag columns in WHERE/JOIN/ORDER BY context (not SELECT-only)
4. Composite index awareness: `(user_id, created_at)` covers `user_id` alone
5. Wire into `check` command output

### Acceptance
- Detects unindexed WHERE columns
- Composite indexes recognized as covering leftmost columns
- `make test` passes with -race

---

## WO-16: Parallel file scanning

**Goal:** `filepath.WalkDir` is sequential. Fan out to N goroutines for large repos.

### Steps
1. Walk directory tree, collect file paths
2. Fan out to `runtime.NumCPU()` goroutines via buffered channel
3. Each goroutine runs `ScanFile()`, sends results to collector
4. Merge results after all goroutines complete
5. `--parallel N` flag (default: NumCPU, `--parallel 1` for sequential)

### Files
- `internal/scanner/parallel.go` — worker pool, path channel, result collector
- `internal/scanner/parallel_test.go`

### Acceptance
- Same results as sequential scan (deterministic)
- Measurable speedup on repos with 1000+ files
- `make test` passes with -race
- No data races under `go test -race`

---

## Non-Goals

- No full SQL parser / AST — regex with multi-line buffering covers 80%+
- No schema migrations
- No SSL/IAM config flags — pgx handles via URL params
- No watch mode — CI runs on push
- No plugin system — add patterns directly
- No connection pooling config — read-only catalog queries take <1s
- No write operations
- No web UI
