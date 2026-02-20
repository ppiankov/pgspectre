---
name: pgspectre
description: PostgreSQL schema auditor — detects unused tables, orphaned indexes, schema drift between code and live database
user-invocable: false
metadata: {"requires":{"bins":["pgspectre"]}}
---

# pgspectre — PostgreSQL Schema Auditor

You have access to `pgspectre`, a tool that audits PostgreSQL clusters for unused tables, orphaned indexes, missing vacuum, and schema drift between code and live database.

## Install

```bash
brew install ppiankov/tap/pgspectre
```

## Commands

| Command | What it does |
|---------|-------------|
| `pgspectre audit --db-url <url>` | Cluster-only analysis (no code scanning) |
| `pgspectre check --db-url <url> --repo <path>` | Compare code references against live DB |
| `pgspectre scan --repo <path>` | Code-only scanning (no database needed) |
| `pgspectre version` | Print version |

## Key Flags

| Flag | Applies to | Description |
|------|-----------|-------------|
| `--db-url` | audit, check | PostgreSQL connection URL (env: `PGSPECTRE_DB_URL`) |
| `--repo` | check, scan | Path to code repository |
| `--format` / `-f` | audit, check, scan | Output: text, json, sarif |
| `--fail-on` | audit, check | Exit 2 if findings match (severity or types) |
| `--min-severity` | audit, check | Filter: high, medium, low, info |
| `--schema` | audit, check | Schemas to analyze (comma-separated) |
| `--baseline` | audit, check | Path to baseline file (suppress known findings) |
| `--update-baseline` | audit, check | Save current findings as new baseline |
| `--no-color` | audit, check | Disable ANSI colors |

## Agent Usage Pattern

```bash
pgspectre audit --db-url "$DATABASE_URL" --format json
```

### JSON Output Structure

```json
{
  "metadata": {
    "tool": "pgspectre",
    "version": "0.1.0",
    "command": "audit",
    "timestamp": "2026-02-20T12:00:00Z"
  },
  "findings": [
    {
      "type": "UNUSED_TABLE",
      "severity": "medium",
      "schema": "public",
      "table": "legacy_users",
      "message": "Table has no reads in 90 days"
    }
  ],
  "summary": {
    "total": 12,
    "high": 2,
    "medium": 5,
    "low": 3,
    "info": 2
  }
}
```

### Parsing Examples

```bash
# Get high-severity findings
pgspectre audit --db-url "$DATABASE_URL" --format json | jq '.findings[] | select(.severity == "high")'

# Count by type
pgspectre audit --db-url "$DATABASE_URL" --format json | jq '.findings | group_by(.type) | map({type: .[0].type, count: length})'

# CI gate — fail on high severity
pgspectre audit --db-url "$DATABASE_URL" --format json --fail-on high
```

## Exit Codes

- `0` — success (or no findings matching --fail-on)
- `1` — error
- `2` — findings matched --fail-on criteria

## What pgspectre Does NOT Do

- Does not modify database schema — read-only analysis
- Does not use ML — deterministic query analysis and statistics
- Does not store results remotely — local output only
- Does not require superuser — works with read-only database access
