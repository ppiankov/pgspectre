package scanner

import (
	"regexp"
	"strings"
)

type tableMatch struct {
	Table   string
	Schema  string
	Pattern PatternType
	Context Context
}

type pattern struct {
	re         *regexp.Regexp
	tableGroup int
	patType    PatternType
	context    Context
	// schemaGroup is set when the pattern captures schema.table separately
	schemaGroup int
}

// Compiled patterns — all case-insensitive.
var patterns = []pattern{
	// SQL: SELECT ... FROM table / FROM schema.table
	{re: regexp.MustCompile(`(?i)\bFROM\s+(\w+)\.(\w+)`),
		schemaGroup: 1, tableGroup: 2, patType: PatternSQL, context: ContextSelect},
	{re: regexp.MustCompile(`(?i)\bFROM\s+(\w+)`),
		tableGroup: 1, patType: PatternSQL, context: ContextSelect},

	// SQL: JOIN variants (LEFT/RIGHT/INNER/OUTER/CROSS/FULL)
	{re: regexp.MustCompile(`(?i)\bJOIN\s+(\w+)\.(\w+)`),
		schemaGroup: 1, tableGroup: 2, patType: PatternSQL, context: ContextSelect},
	{re: regexp.MustCompile(`(?i)\bJOIN\s+(\w+)`),
		tableGroup: 1, patType: PatternSQL, context: ContextSelect},

	// SQL: INSERT INTO table
	{re: regexp.MustCompile(`(?i)\bINSERT\s+INTO\s+(\w+)\.(\w+)`),
		schemaGroup: 1, tableGroup: 2, patType: PatternSQL, context: ContextInsert},
	{re: regexp.MustCompile(`(?i)\bINSERT\s+INTO\s+(\w+)`),
		tableGroup: 1, patType: PatternSQL, context: ContextInsert},

	// SQL: UPDATE table SET
	{re: regexp.MustCompile(`(?i)\bUPDATE\s+(\w+)\.(\w+)\s+SET\b`),
		schemaGroup: 1, tableGroup: 2, patType: PatternSQL, context: ContextUpdate},
	{re: regexp.MustCompile(`(?i)\bUPDATE\s+(\w+)\s+SET\b`),
		tableGroup: 1, patType: PatternSQL, context: ContextUpdate},

	// SQL: DELETE FROM table
	{re: regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+(\w+)\.(\w+)`),
		schemaGroup: 1, tableGroup: 2, patType: PatternSQL, context: ContextDelete},
	{re: regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+(\w+)`),
		tableGroup: 1, patType: PatternSQL, context: ContextDelete},

	// ORM: SQLAlchemy __tablename__
	{re: regexp.MustCompile(`__tablename__\s*=\s*['"](\w+)['"]`),
		tableGroup: 1, patType: PatternORM, context: ContextUnknown},

	// ORM: Django db_table
	{re: regexp.MustCompile(`db_table\s*=\s*['"](\w+)['"]`),
		tableGroup: 1, patType: PatternORM, context: ContextUnknown},

	// ORM: GORM TableName() method
	{re: regexp.MustCompile(`TableName\(\)\s*.*return\s*["'](\w+)["']`),
		tableGroup: 1, patType: PatternORM, context: ContextUnknown},

	// ORM: GORM .Table("name")
	{re: regexp.MustCompile(`\.Table\(["'](\w+)["']\)`),
		tableGroup: 1, patType: PatternORM, context: ContextUnknown},

	// ORM: Prisma @@map("name")
	{re: regexp.MustCompile(`@@map\(["'](\w+)["']\)`),
		tableGroup: 1, patType: PatternORM, context: ContextUnknown},

	// Migration: CREATE TABLE [IF NOT EXISTS] table
	{re: regexp.MustCompile(`(?i)\bCREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(\w+)\.(\w+)`),
		schemaGroup: 1, tableGroup: 2, patType: PatternMigration, context: ContextDDL},
	{re: regexp.MustCompile(`(?i)\bCREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(\w+)`),
		tableGroup: 1, patType: PatternMigration, context: ContextDDL},

	// Migration: ALTER TABLE table
	{re: regexp.MustCompile(`(?i)\bALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?(\w+)\.(\w+)`),
		schemaGroup: 1, tableGroup: 2, patType: PatternMigration, context: ContextDDL},
	{re: regexp.MustCompile(`(?i)\bALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?(\w+)`),
		tableGroup: 1, patType: PatternMigration, context: ContextDDL},

	// Migration: DROP TABLE table
	{re: regexp.MustCompile(`(?i)\bDROP\s+TABLE\s+(?:IF\s+EXISTS\s+)?(\w+)`),
		tableGroup: 1, patType: PatternMigration, context: ContextDDL},

	// Migration: CREATE [UNIQUE] INDEX name ON table
	{re: regexp.MustCompile(`(?i)\bCREATE\s+(?:UNIQUE\s+)?INDEX\s+\w+\s+ON\s+(\w+)`),
		tableGroup: 1, patType: PatternMigration, context: ContextDDL},
}

// SQL keywords that should not be treated as table names.
var sqlKeywords = map[string]bool{
	"select": true, "from": true, "where": true, "and": true, "or": true,
	"not": true, "in": true, "is": true, "null": true, "as": true,
	"on": true, "set": true, "values": true, "into": true, "join": true,
	"left": true, "right": true, "inner": true, "outer": true, "cross": true,
	"full": true, "group": true, "by": true, "order": true, "having": true,
	"limit": true, "offset": true, "union": true, "all": true, "distinct": true,
	"case": true, "when": true, "then": true, "else": true, "end": true,
	"exists": true, "between": true, "like": true, "true": true, "false": true,
	"table": true, "index": true, "create": true, "alter": true, "drop": true,
	"insert": true, "update": true, "delete": true, "begin": true, "commit": true,
	"rollback": true, "if": true, "with": true, "returning": true,
	// Common false positives from import statements
	"sqlalchemy": true, "django": true, "gorm": true, "prisma": true,
	"import": true, "package": true, "require": true, "include": true,
}

// ScanLine extracts table references from a single line of code.
func ScanLine(line string) []tableMatch {
	var matches []tableMatch
	seen := make(map[string]bool)

	for _, p := range patterns {
		for _, m := range p.re.FindAllStringSubmatch(line, -1) {
			table := m[p.tableGroup]
			if !isValidTableName(table) {
				continue
			}

			var schema string
			if p.schemaGroup > 0 && p.schemaGroup < len(m) {
				schema = m[p.schemaGroup]
			}

			key := schema + "." + table + string(p.context)
			if seen[key] {
				continue
			}
			seen[key] = true

			matches = append(matches, tableMatch{
				Table:   table,
				Schema:  schema,
				Pattern: p.patType,
				Context: p.context,
			})
		}
	}

	return matches
}

type columnMatch struct {
	Table   string
	Column  string
	Schema  string
	Context Context
}

// Column extraction patterns.
var columnPatterns = []struct {
	re      *regexp.Regexp
	extract func([]string) []columnMatch
}{
	// table.column dotted reference (e.g., users.email, u.name)
	{re: regexp.MustCompile(`(?i)\b(\w+)\.(\w+)\b`), extract: extractDottedColumn},

	// SELECT col1, col2, ... FROM table
	{re: regexp.MustCompile(`(?i)\bSELECT\s+(.+?)\s+FROM\s+`), extract: extractSelectColumns},

	// WHERE/AND/OR col = / col IN / col IS / col LIKE / col >
	{re: regexp.MustCompile(`(?i)\b(?:WHERE|AND|OR)\s+(\w+)\s*(?:=|<|>|!=|<>|IS\b|IN\b|LIKE\b|BETWEEN\b|NOT\b)`),
		extract: extractConditionColumn},

	// ORDER BY col / GROUP BY col
	{re: regexp.MustCompile(`(?i)\b(?:ORDER|GROUP)\s+BY\s+(\w+)`),
		extract: extractByColumn},

	// INSERT INTO table (col1, col2, ...)
	{re: regexp.MustCompile(`(?i)\bINSERT\s+INTO\s+\w+\s*\(([^)]+)\)`),
		extract: extractInsertColumns},
}

// SQL functions that should not be treated as column names.
var sqlFunctions = map[string]bool{
	"count": true, "sum": true, "avg": true, "min": true, "max": true,
	"coalesce": true, "nullif": true, "cast": true, "extract": true,
	"lower": true, "upper": true, "trim": true, "length": true,
	"now": true, "current_timestamp": true, "current_date": true,
	"row_number": true, "rank": true, "dense_rank": true,
	"array_agg": true, "string_agg": true, "json_agg": true,
	"exists": true, "not": true, "case": true,
}

func isValidColumnName(name string) bool {
	if len(name) < 2 || len(name) > 120 {
		return false
	}
	lower := strings.ToLower(name)
	if sqlKeywords[lower] || sqlFunctions[lower] {
		return false
	}
	// Reject numeric literals
	if name[0] >= '0' && name[0] <= '9' {
		return false
	}
	return true
}

func extractDottedColumn(m []string) []columnMatch {
	table, col := m[1], m[2]
	if !isValidTableName(table) || !isValidColumnName(col) {
		return nil
	}
	// Reject if column starts with uppercase — likely a method call (e.g., fmt.Println)
	if col[0] >= 'A' && col[0] <= 'Z' {
		return nil
	}
	return []columnMatch{{Table: table, Column: col, Context: ContextUnknown}}
}

func extractSelectColumns(m []string) []columnMatch {
	colList := m[1]
	if strings.Contains(strings.ToUpper(colList), "*") {
		return nil // SELECT *
	}
	var matches []columnMatch
	for _, part := range strings.Split(colList, ",") {
		col := strings.TrimSpace(part)
		// Handle "col AS alias" — take the column, not alias
		if idx := strings.Index(strings.ToUpper(col), " AS "); idx > 0 {
			col = strings.TrimSpace(col[:idx])
		}
		// Handle table.col — extract as dotted ref (allow single-char aliases like u.name)
		if dotIdx := strings.Index(col, "."); dotIdx > 0 {
			table := col[:dotIdx]
			colName := col[dotIdx+1:]
			if !sqlKeywords[strings.ToLower(table)] && isValidColumnName(colName) {
				matches = append(matches, columnMatch{Table: table, Column: colName, Context: ContextSelect})
			}
			continue
		}
		if isValidColumnName(col) {
			matches = append(matches, columnMatch{Column: col, Context: ContextSelect})
		}
	}
	return matches
}

func extractConditionColumn(m []string) []columnMatch {
	col := m[1]
	if !isValidColumnName(col) {
		return nil
	}
	return []columnMatch{{Column: col, Context: ContextSelect}}
}

func extractByColumn(m []string) []columnMatch {
	col := m[1]
	if !isValidColumnName(col) {
		return nil
	}
	return []columnMatch{{Column: col, Context: ContextSelect}}
}

func extractInsertColumns(m []string) []columnMatch {
	colList := m[1]
	var matches []columnMatch
	for _, part := range strings.Split(colList, ",") {
		col := strings.TrimSpace(part)
		if isValidColumnName(col) {
			matches = append(matches, columnMatch{Column: col, Context: ContextInsert})
		}
	}
	return matches
}

// ScanLineColumns extracts column references from a single line of code.
func ScanLineColumns(line string) []columnMatch {
	var matches []columnMatch
	seen := make(map[string]bool)

	for _, p := range columnPatterns {
		for _, m := range p.re.FindAllStringSubmatch(line, -1) {
			for _, cm := range p.extract(m) {
				key := cm.Table + "." + cm.Column
				if seen[key] {
					continue
				}
				seen[key] = true
				matches = append(matches, cm)
			}
		}
	}

	return matches
}

func isValidTableName(name string) bool {
	if len(name) < 2 || len(name) > 120 {
		return false
	}
	if sqlKeywords[strings.ToLower(name)] {
		return false
	}
	return true
}
