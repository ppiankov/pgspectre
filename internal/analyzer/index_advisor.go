package analyzer

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ppiankov/pgspectre/internal/postgres"
	"github.com/ppiankov/pgspectre/internal/scanner"
)

// indexColumnRe extracts column names from an index definition.
// Matches the parenthesized column list in CREATE INDEX ... (col1, col2, ...).
var indexColumnRe = regexp.MustCompile(`\(([^)]+)\)`)

// DetectUnindexedQueries finds columns used in WHERE/ORDER BY/JOIN that lack indexes.
func DetectUnindexedQueries(columnRefs []scanner.ColumnRef, indexes []postgres.IndexInfo, tables []postgres.TableInfo) []Finding {
	// Build set of indexed columns: "schema.table.column" → true
	indexedCols := buildIndexedColumns(indexes)

	// Build table lookup
	tableSet := make(map[string]postgres.TableInfo)
	for _, t := range tables {
		key := strings.ToLower(t.Schema + "." + t.Name)
		tableSet[key] = t
	}

	// Count references by table.column for indexable contexts
	type colKey struct {
		schema string
		table  string
		column string
	}
	refCounts := make(map[colKey]int)
	for _, cr := range columnRefs {
		if !isIndexableContext(cr.Context) {
			continue
		}
		if cr.Table == "" || strings.EqualFold(cr.Table, "unknown") {
			continue
		}
		k := colKey{
			schema: strings.ToLower(cr.Schema),
			table:  strings.ToLower(cr.Table),
			column: strings.ToLower(cr.Column),
		}
		refCounts[k]++
	}

	var findings []Finding
	for k, count := range refCounts {
		// Resolve schema — try to find the table in DB
		schema := k.schema
		if schema == "" {
			// Try public schema
			if _, ok := tableSet["public."+k.table]; ok {
				schema = "public"
			} else {
				continue // Unknown table, skip
			}
		}

		// Check if any index covers this column
		fqCol := schema + "." + k.table + "." + k.column
		if indexedCols[fqCol] {
			continue
		}

		findings = append(findings, Finding{
			Type:     FindingUnindexedQuery,
			Severity: SeverityMedium,
			Schema:   schema,
			Table:    k.table,
			Column:   k.column,
			Message:  fmt.Sprintf("column %q used in WHERE/ORDER BY (%d references) but has no index", k.column, count),
		})
	}

	return findings
}

// buildIndexedColumns parses index definitions and returns indexed column keys.
func buildIndexedColumns(indexes []postgres.IndexInfo) map[string]bool {
	result := make(map[string]bool)

	for _, idx := range indexes {
		cols := parseIndexColumns(idx.Definition)
		schema := strings.ToLower(idx.Schema)
		table := strings.ToLower(idx.Table)

		// All columns in the index are considered covered.
		// Composite indexes cover all their columns individually.
		for _, col := range cols {
			key := schema + "." + table + "." + strings.ToLower(col)
			result[key] = true
		}
	}

	return result
}

// parseIndexColumns extracts column names from an index definition.
func parseIndexColumns(def string) []string {
	m := indexColumnRe.FindStringSubmatch(def)
	if len(m) < 2 {
		return nil
	}

	var cols []string
	for _, part := range strings.Split(m[1], ",") {
		col := strings.TrimSpace(part)
		// Remove ASC/DESC/NULLS FIRST/NULLS LAST
		col = strings.SplitN(col, " ", 2)[0]
		// Remove any LOWER() or other function wrapping
		if strings.Contains(col, "(") {
			continue
		}
		if col != "" {
			cols = append(cols, col)
		}
	}
	return cols
}

func isIndexableContext(ctx scanner.Context) bool {
	return ctx == scanner.ContextWhere || ctx == scanner.ContextOrderBy
}
