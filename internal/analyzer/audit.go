package analyzer

import (
	"fmt"
	"strings"
	"time"

	"github.com/ppiankov/pgspectre/internal/postgres"
)

const missingVacuumThreshold = 30 * 24 * time.Hour

// tableKey builds a lookup key from schema and table name.
func tableKey(schema, table string) string {
	return schema + "." + table
}

// Audit analyzes a catalog snapshot and returns findings.
func Audit(snap *postgres.Snapshot) []Finding {
	statsMap := make(map[string]postgres.TableStats, len(snap.Stats))
	for i := range snap.Stats {
		s := &snap.Stats[i]
		statsMap[tableKey(s.Schema, s.Name)] = *s
	}

	pkSet := make(map[string]bool)
	for _, c := range snap.Constraints {
		if c.Type == "p" {
			pkSet[tableKey(c.Schema, c.Table)] = true
		}
	}

	tableSizeMap := make(map[string]int64, len(snap.Tables))
	for _, t := range snap.Tables {
		if t.EstimatedRows > 0 {
			tableSizeMap[tableKey(t.Schema, t.Name)] = t.EstimatedRows
		}
	}

	var findings []Finding

	findings = append(findings, detectUnusedTables(snap.Stats)...)
	findings = append(findings, detectUnusedIndexes(snap.Indexes)...)
	findings = append(findings, detectBloatedIndexes(snap.Indexes, tableSizeMap)...)
	findings = append(findings, detectMissingVacuum(snap.Stats, time.Now())...)
	findings = append(findings, detectNoPrimaryKey(snap.Tables, pkSet)...)
	findings = append(findings, detectDuplicateIndexes(snap.Indexes)...)

	return findings
}

func detectUnusedTables(stats []postgres.TableStats) []Finding {
	var findings []Finding
	for i := range stats {
		s := &stats[i]
		if s.SeqScan == 0 && s.IdxScan == 0 {
			findings = append(findings, Finding{
				Type:     FindingUnusedTable,
				Severity: SeverityHigh,
				Schema:   s.Schema,
				Table:    s.Name,
				Message:  "table has no sequential or index scans",
			})
		}
	}
	return findings
}

func detectUnusedIndexes(indexes []postgres.IndexInfo) []Finding {
	var findings []Finding
	for _, idx := range indexes {
		if idx.IndexScans == 0 && idx.SizeBytes > 0 {
			findings = append(findings, Finding{
				Type:     FindingUnusedIndex,
				Severity: SeverityMedium,
				Schema:   idx.Schema,
				Table:    idx.Table,
				Index:    idx.Name,
				Message:  fmt.Sprintf("index %q has never been used (%d bytes)", idx.Name, idx.SizeBytes),
			})
		}
	}
	return findings
}

func detectBloatedIndexes(indexes []postgres.IndexInfo, tableSizeMap map[string]int64) []Finding {
	// Group total index size by table
	tableIndexSize := make(map[string]int64)
	for _, idx := range indexes {
		key := tableKey(idx.Schema, idx.Table)
		tableIndexSize[key] += idx.SizeBytes
	}

	var findings []Finding
	for _, idx := range indexes {
		key := tableKey(idx.Schema, idx.Table)
		estRows := tableSizeMap[key]
		if estRows == 0 {
			continue
		}
		// Flag individual indexes larger than the table's estimated row count
		// (using estimated rows as a rough proxy — an index on a small table shouldn't be huge)
		if idx.SizeBytes > 0 && tableIndexSize[key] > 0 {
			// Simple heuristic: flag if this single index is larger than total index size / 2
			// and the index has zero scans (already caught by unused, but bloat is about size)
			// More useful: flag if index size exceeds a reasonable multiple of estimated rows
			// For now: flag if index has 0 scans and is > 1MB
			if idx.IndexScans == 0 && idx.SizeBytes > 1024*1024 {
				findings = append(findings, Finding{
					Type:     FindingBloatedIndex,
					Severity: SeverityLow,
					Schema:   idx.Schema,
					Table:    idx.Table,
					Index:    idx.Name,
					Message:  fmt.Sprintf("unused index %q is %s", idx.Name, formatBytes(idx.SizeBytes)),
				})
			}
		}
	}
	return findings
}

func detectMissingVacuum(stats []postgres.TableStats, now time.Time) []Finding {
	var findings []Finding
	for i := range stats {
		s := &stats[i]
		// Only flag active tables (those with some scan activity)
		if s.SeqScan == 0 && s.IdxScan == 0 {
			continue
		}

		lastVac := latestVacuum(s)
		if lastVac == nil {
			findings = append(findings, Finding{
				Type:     FindingMissingVacuum,
				Severity: SeverityLow,
				Schema:   s.Schema,
				Table:    s.Name,
				Message:  "active table has never been vacuumed",
			})
			continue
		}

		if now.Sub(*lastVac) > missingVacuumThreshold {
			findings = append(findings, Finding{
				Type:     FindingMissingVacuum,
				Severity: SeverityLow,
				Schema:   s.Schema,
				Table:    s.Name,
				Message:  fmt.Sprintf("last vacuum was %d days ago", int(now.Sub(*lastVac).Hours()/24)),
			})
		}
	}
	return findings
}

func detectNoPrimaryKey(tables []postgres.TableInfo, pkSet map[string]bool) []Finding {
	var findings []Finding
	for _, t := range tables {
		if !pkSet[tableKey(t.Schema, t.Name)] {
			findings = append(findings, Finding{
				Type:     FindingNoPrimaryKey,
				Severity: SeverityMedium,
				Schema:   t.Schema,
				Table:    t.Name,
				Message:  "table has no primary key",
			})
		}
	}
	return findings
}

func detectDuplicateIndexes(indexes []postgres.IndexInfo) []Finding {
	// Group indexes by table
	byTable := make(map[string][]postgres.IndexInfo)
	for _, idx := range indexes {
		key := tableKey(idx.Schema, idx.Table)
		byTable[key] = append(byTable[key], idx)
	}

	var findings []Finding
	for _, group := range byTable {
		for i := 0; i < len(group); i++ {
			for j := i + 1; j < len(group); j++ {
				if normalizeDef(group[i].Definition) == normalizeDef(group[j].Definition) {
					findings = append(findings, Finding{
						Type:     FindingDuplicateIndex,
						Severity: SeverityLow,
						Schema:   group[i].Schema,
						Table:    group[i].Table,
						Index:    group[j].Name,
						Message:  fmt.Sprintf("index %q has the same definition as %q", group[j].Name, group[i].Name),
					})
				}
			}
		}
	}
	return findings
}

// latestVacuum returns the most recent vacuum timestamp (manual or auto).
func latestVacuum(s *postgres.TableStats) *time.Time {
	var latest *time.Time
	for _, t := range []*time.Time{s.LastVacuum, s.LastAutovacuum} {
		if t != nil && (latest == nil || t.After(*latest)) {
			latest = t
		}
	}
	return latest
}

// normalizeDef strips the index name and whitespace from a definition
// so that "CREATE INDEX idx_a ON t (col)" and "CREATE INDEX idx_b ON t (col)"
// compare as equal.
func normalizeDef(def string) string {
	normalized := strings.Join(strings.Fields(def), " ")
	// Strip everything before " ON " to remove "CREATE [UNIQUE] INDEX <name>"
	if idx := strings.Index(strings.ToUpper(normalized), " ON "); idx >= 0 {
		return normalized[idx:]
	}
	return normalized
}

func formatBytes(b int64) string {
	switch {
	case b >= 1024*1024*1024:
		return fmt.Sprintf("%.1f GB", float64(b)/(1024*1024*1024))
	case b >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	default:
		return fmt.Sprintf("%d bytes", b)
	}
}
