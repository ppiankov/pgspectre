package analyzer

import (
	"fmt"
	"strings"

	"github.com/ppiankov/pgspectre/internal/postgres"
	"github.com/ppiankov/pgspectre/internal/scanner"
)

// Diff compares code repo references against the live database snapshot.
// It also includes audit findings for cluster-only issues.
func Diff(scan *scanner.ScanResult, snap *postgres.Snapshot, opts AuditOptions) []Finding {
	// Build lookup of DB tables by lowercase name
	dbTables := make(map[string]postgres.TableInfo, len(snap.Tables))
	for _, t := range snap.Tables {
		dbTables[strings.ToLower(t.Name)] = t
	}

	// Build lookup of DB table stats by lowercase name
	statsMap := make(map[string]postgres.TableStats, len(snap.Stats))
	for i := range snap.Stats {
		s := &snap.Stats[i]
		statsMap[strings.ToLower(s.Name)] = *s
	}

	// Build set of code-referenced table names (lowercased)
	codeRefs := make(map[string]bool, len(scan.Tables))
	for _, t := range scan.Tables {
		codeRefs[strings.ToLower(t)] = true
	}

	var findings []Finding

	// Check code refs against DB
	for _, tableName := range scan.Tables {
		lower := strings.ToLower(tableName)
		if _, ok := dbTables[lower]; !ok {
			findings = append(findings, Finding{
				Type:     FindingMissingTable,
				Severity: SeverityHigh,
				Table:    tableName,
				Message:  fmt.Sprintf("table %q referenced in code but does not exist in database", tableName),
			})
		} else {
			findings = append(findings, Finding{
				Type:     FindingCodeMatch,
				Severity: SeverityInfo,
				Schema:   dbTables[lower].Schema,
				Table:    tableName,
				Message:  fmt.Sprintf("table %q exists in database and is referenced in code", tableName),
			})
		}
	}

	// Check column refs against DB columns
	dbColumns := make(map[string]bool, len(snap.Columns))
	for _, c := range snap.Columns {
		key := strings.ToLower(c.Table) + "." + strings.ToLower(c.Name)
		dbColumns[key] = true
	}
	seenCols := make(map[string]bool)
	for _, cr := range scan.ColumnRefs {
		tableLower := strings.ToLower(cr.Table)
		colLower := strings.ToLower(cr.Column)
		if tableLower == "" {
			continue // no table association, skip
		}
		// Only check columns for tables that exist in the DB
		if _, ok := dbTables[tableLower]; !ok {
			continue
		}
		key := tableLower + "." + colLower
		if seenCols[key] {
			continue
		}
		seenCols[key] = true
		if !dbColumns[key] {
			findings = append(findings, Finding{
				Type:     FindingMissingColumn,
				Severity: SeverityMedium,
				Schema:   dbTables[tableLower].Schema,
				Table:    cr.Table,
				Column:   cr.Column,
				Message:  fmt.Sprintf("column %q referenced in code but does not exist in table %q", cr.Column, cr.Table),
			})
		}
	}

	// Check DB tables not referenced in code
	for _, t := range snap.Tables {
		lower := strings.ToLower(t.Name)
		if codeRefs[lower] {
			continue
		}
		stats := statsMap[lower]
		if stats.SeqScan == 0 && stats.IdxScan == 0 {
			findings = append(findings, Finding{
				Type:     FindingUnreferencedTable,
				Severity: SeverityLow,
				Schema:   t.Schema,
				Table:    t.Name,
				Message:  fmt.Sprintf("table %q exists in database with no activity and is not referenced in code", t.Name),
			})
		}
	}

	// Include audit findings for cluster-only issues
	findings = append(findings, Audit(snap, opts)...)

	return findings
}
