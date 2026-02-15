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

## WO-09: Multi-line SQL buffering ✅

**Goal:** Scanner misses SQL split across lines. Buffer between SQL markers and scan assembled blocks.

### Implementation
- Created `internal/scanner/buffer.go` with `sqlBuffer` struct: two-mode buffering (SQL files vs code files)
- SQL files (.sql): buffer lines between semicolons, `splitOnSemicolons()` respects single-quoted strings
- Code files: detect backtick blocks (Go/JS/TS) and triple-quote blocks (Python/Java), buffer until closing delimiter
- `normalize()` joins buffered lines and collapses whitespace to single space
- Modified `scanFile()` in scanner.go to use buffer: lines inside blocks are NOT scanned individually
- 22 new buffer tests + 1 integration test covering multi-line .sql, Go backtick, Python triple-quote

### Files
- `internal/scanner/buffer.go` — sqlBuffer with feedSQL(), feedCode(), normalize(), splitOnSemicolons()
- `internal/scanner/buffer_test.go` — 22 unit tests
- `internal/scanner/scanner.go` — integrated buffer into scanFile()
- `internal/scanner/scanner_test.go` — TestScan_MultiLineSQL integration test

---

## WO-10: `scan` subcommand (offline mode) ✅

**Goal:** Scan code without a live database connection. Enables CI pre-commit hooks and spectrehub integration.

### Implementation
- Created `internal/cli/scan.go` — Cobra `scan` subcommand with `--repo` and `--format text|json` flags
- Text output: tables list, columns list, references with file:line locations, summary
- JSON output: marshals `ScanResult` directly (already has JSON tags)
- Exit code 0 always (no severity without DB comparison)
- 6 tests in `internal/cli/scan_test.go`: text/JSON output, missing repo error, empty dir, formatters

### Files
- `internal/cli/scan.go` — newScanCmd(), writeScanResult(), writeScanResultText()
- `internal/cli/scan_test.go` — 6 tests
- `internal/cli/root.go` — wired newScanCmd()

---

## WO-11: Baseline mode ✅

**Goal:** First run produces N findings. Team triages. Next run flags only new findings. Without this, tool is noisy on day 1 and disabled on day 2.

### Implementation
- Created `internal/baseline/baseline.go` — SHA-256 fingerprints from `type|schema|table|column|index`
- `Load()` reads baseline file (returns empty baseline if missing), `Save()` deduplicates and sorts
- `Filter()` removes baselined findings, returns filtered list and suppressed count
- `--baseline` and `--update-baseline` flags on both `audit` and `check` commands
- Suppressed count printed to stderr when baseline active
- Added `Column` field to `Finding` struct for proper MISSING_COLUMN fingerprinting
- 9 tests at 94.9% coverage

### Files
- `internal/baseline/baseline.go` — Load(), Save(), Contains(), Filter(), Fingerprint()
- `internal/baseline/baseline_test.go` — 9 tests
- `internal/analyzer/types.go` — added Column field to Finding
- `internal/analyzer/diff.go` — set Column on MISSING_COLUMN findings
- `internal/cli/root.go` — wired --baseline and --update-baseline flags

---

## WO-12: SARIF output ✅

**Goal:** GitHub Security tab, GitLab SAST, and VS Code all consume SARIF. One format unlocks three integration points.

### Implementation
- Created `internal/reporter/sarif.go` — SARIF 2.1.0 writer with minimal type subset
- Rule IDs: `pgspectre/MISSING_TABLE`, `pgspectre/UNUSED_INDEX`, etc.
- Severity mapping: high→error, medium→warning, low/info→note
- Logical locations with schema.table.column FQN
- `--format sarif` added to audit, check, and scan commands
- 4 tests: valid structure, empty report, column FQN, severity mapping

### Files
- `internal/reporter/sarif.go` — writeSARIF(), SARIF 2.1.0 types
- `internal/reporter/sarif_test.go` — 4 tests
- `internal/reporter/reporter.go` — added FormatSARIF constant
- `internal/cli/root.go` — updated format help text
- `internal/cli/scan.go` — updated format help text

---

## WO-13: Finding suppression ✅

**Goal:** Teams will have false positives. If they can't silence them, they'll silence the whole tool.

### Implementation
- Created `internal/suppress/suppress.go` — three suppression mechanisms:
  1. Inline `// pgspectre:ignore` comment marks refs as suppressed during scanning
  2. `.pgspectre-ignore.yml` file with table/type glob patterns
  3. Config-level `exclude.findings` list by finding type
- Scanner marks `TableRef.Suppressed` and `ColumnRef.Suppressed` on lines with inline ignore
- CLI `filterFindings()` helper applies baseline + suppress rules to findings
- Glob patterns: `temp_migration_*` matches `temp_migration_001`, etc.
- 12 tests at 94.3% coverage

### Files
- `internal/suppress/suppress.go` — LoadRules(), IsSuppressed(), Filter(), HasInlineIgnore()
- `internal/suppress/suppress_test.go` — 12 tests
- `internal/scanner/types.go` — added Suppressed field to TableRef and ColumnRef
- `internal/scanner/scanner.go` — inline pgspectre:ignore detection
- `internal/config/config.go` — added Findings to Exclude struct
- `internal/cli/root.go` — filterFindings() helper, wired suppress into commands

---

## WO-14: `--fail-on` granularity ✅

**Goal:** CI needs `--fail-on MISSING_TABLE,MISSING_COLUMN` not just `--fail-on-missing`.

### Implementation
- Added `--fail-on` flag to both `audit` and `check` commands
- Accepts comma-separated finding types (MISSING_TABLE,MISSING_COLUMN) or severity levels (high,medium)
- `shouldFailOn()` helper: case-insensitive matching, distinguishes types from severities
- `--fail-on-missing` kept as backward-compatible alias (maps to `--fail-on MISSING_TABLE`)
- 7 tests: by type, by severity, comma-separated, mixed, empty, case-insensitive, no findings

### Files
- `internal/cli/root.go` — shouldFailOn(), --fail-on flag on audit and check
- `internal/cli/failon_test.go` — 7 tests

---

## WO-15: Index advisor ✅

**Status:** Complete — `ecfc25d`

**Implementation:**
- Added `ContextWhere` and `ContextOrderBy` to `scanner/types.go`; updated `extractConditionColumn` and `extractByColumn` in `patterns.go`
- Created `internal/analyzer/index_advisor.go` — `DetectUnindexedQueries()`, `buildIndexedColumns()`, `parseIndexColumns()`, `isIndexableContext()`
- Parses `CREATE INDEX ... (col1, col2)` definitions via regex, builds `schema.table.column` lookup
- Composite index awareness: all columns in a composite index are individually covered
- Filters: only WHERE/ORDER BY contexts flagged; unknown tables skipped; SELECT-only columns ignored
- Finding type: `UNINDEXED_QUERY` (medium severity) with reference count
- Wired into `Diff()` in `diff.go`
- Created `internal/analyzer/index_advisor_test.go` — 6 test functions covering parse, basic detection, index exists, composite, ORDER BY, unknown table, buildIndexedColumns
- 149 tests pass, lint clean, analyzer coverage 95.3%

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
