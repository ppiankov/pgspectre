package analyzer

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ppiankov/pgspectre/internal/postgres"
)

// tableKey builds a lookup key from schema and table name.
func tableKey(schema, table string) string {
	return schema + "." + table
}

// Audit analyzes a catalog snapshot and returns findings.
func Audit(snap *postgres.Snapshot, opts AuditOptions) []Finding {
	defaults := DefaultAuditOptions()
	if opts.VacuumDays <= 0 {
		opts.VacuumDays = defaults.VacuumDays
	}
	if opts.UnusedIndexMinBytes <= 0 {
		opts.UnusedIndexMinBytes = defaults.UnusedIndexMinBytes
	}
	if opts.BloatMinBytes <= 0 {
		opts.BloatMinBytes = defaults.BloatMinBytes
	}

	excludeTable := make(map[string]bool, len(opts.ExcludeTables))
	for _, t := range opts.ExcludeTables {
		excludeTable[strings.ToLower(t)] = true
	}
	excludeSchema := make(map[string]bool, len(opts.ExcludeSchemas))
	for _, s := range opts.ExcludeSchemas {
		excludeSchema[strings.ToLower(s)] = true
	}

	vacuumThreshold := time.Duration(opts.VacuumDays) * 24 * time.Hour
	unusedIndexMin := opts.UnusedIndexMinBytes
	bloatMin := opts.BloatMinBytes

	pkSet := make(map[string]bool)
	for _, c := range snap.Constraints {
		if c.Type == "p" {
			pkSet[tableKey(c.Schema, c.Table)] = true
		}
	}

	tableSizeMap := make(map[string]int64, len(snap.Tables))
	for _, t := range snap.Tables {
		if t.SizeBytes > 0 {
			tableSizeMap[tableKey(t.Schema, t.Name)] = t.SizeBytes
		}
	}

	// Filter stats and tables by exclusions
	var filteredStats []postgres.TableStats
	for i := range snap.Stats {
		s := &snap.Stats[i]
		if excludeTable[strings.ToLower(s.Name)] || excludeSchema[strings.ToLower(s.Schema)] {
			continue
		}
		filteredStats = append(filteredStats, *s)
	}

	var filteredTables []postgres.TableInfo
	for _, t := range snap.Tables {
		if excludeTable[strings.ToLower(t.Name)] || excludeSchema[strings.ToLower(t.Schema)] {
			continue
		}
		filteredTables = append(filteredTables, t)
	}

	var filteredIndexes []postgres.IndexInfo
	for _, idx := range snap.Indexes {
		if excludeTable[strings.ToLower(idx.Table)] || excludeSchema[strings.ToLower(idx.Schema)] {
			continue
		}
		filteredIndexes = append(filteredIndexes, idx)
	}

	var findings []Finding

	findings = append(findings, detectUnusedTables(filteredStats)...)
	findings = append(findings, detectUnusedIndexes(filteredIndexes, unusedIndexMin)...)
	findings = append(findings, detectBloatedIndexes(filteredIndexes, tableSizeMap, bloatMin)...)
	findings = append(findings, detectMissingVacuum(filteredStats, time.Now(), vacuumThreshold)...)
	findings = append(findings, detectNoPrimaryKey(filteredTables, pkSet)...)
	findings = append(findings, detectDuplicateIndexes(filteredIndexes)...)

	return findings
}

func detectUnusedTables(stats []postgres.TableStats) []Finding {
	var findings []Finding
	for i := range stats {
		s := &stats[i]
		if s.SeqScan == 0 && s.IdxScan == 0 {
			detail := map[string]string{
				"live_tuples": strconv.FormatInt(s.LiveTuples, 10),
				"dead_tuples": strconv.FormatInt(s.DeadTuples, 10),
			}
			if s.LastVacuum != nil {
				detail["last_vacuum"] = s.LastVacuum.Format(time.RFC3339)
			}
			if s.LastAutovacuum != nil {
				detail["last_autovacuum"] = s.LastAutovacuum.Format(time.RFC3339)
			}
			findings = append(findings, Finding{
				Type:     FindingUnusedTable,
				Severity: SeverityHigh,
				Schema:   s.Schema,
				Table:    s.Name,
				Message:  "table has no sequential or index scans",
				Detail:   detail,
			})
		}
	}
	return findings
}

func detectUnusedIndexes(indexes []postgres.IndexInfo, minSizeBytes int64) []Finding {
	var findings []Finding
	for _, idx := range indexes {
		if idx.IndexScans == 0 && idx.SizeBytes > minSizeBytes {
			findings = append(findings, Finding{
				Type:     FindingUnusedIndex,
				Severity: SeverityMedium,
				Schema:   idx.Schema,
				Table:    idx.Table,
				Index:    idx.Name,
				Message:  fmt.Sprintf("index %q has never been used (%s)", idx.Name, formatBytes(idx.SizeBytes)),
				Detail: map[string]string{
					"size_bytes": strconv.FormatInt(idx.SizeBytes, 10),
					"size":       formatBytes(idx.SizeBytes),
					"idx_scan":   strconv.FormatInt(idx.IndexScans, 10),
				},
			})
		}
	}
	return findings
}

func detectBloatedIndexes(indexes []postgres.IndexInfo, tableSizeMap map[string]int64, bloatMin int64) []Finding {
	var findings []Finding
	for _, idx := range indexes {
		key := tableKey(idx.Schema, idx.Table)
		tableSize := tableSizeMap[key]
		if tableSize <= 0 {
			continue
		}
		if idx.SizeBytes <= bloatMin {
			continue
		}
		if idx.SizeBytes > tableSize {
			findings = append(findings, Finding{
				Type:     FindingBloatedIndex,
				Severity: SeverityLow,
				Schema:   idx.Schema,
				Table:    idx.Table,
				Index:    idx.Name,
				Message:  fmt.Sprintf("index %q (%s) is larger than table (%s)", idx.Name, formatBytes(idx.SizeBytes), formatBytes(tableSize)),
				Detail: map[string]string{
					"index_size_bytes": strconv.FormatInt(idx.SizeBytes, 10),
					"index_size":       formatBytes(idx.SizeBytes),
					"table_size_bytes": strconv.FormatInt(tableSize, 10),
					"table_size":       formatBytes(tableSize),
				},
			})
		}
	}
	return findings
}

func detectMissingVacuum(stats []postgres.TableStats, now time.Time, threshold time.Duration) []Finding {
	var findings []Finding
	for i := range stats {
		s := &stats[i]
		// Only flag active tables (those with some scan activity)
		if s.SeqScan == 0 && s.IdxScan == 0 {
			continue
		}

		detail := map[string]string{
			"dead_tuples": strconv.FormatInt(s.DeadTuples, 10),
			"live_tuples": strconv.FormatInt(s.LiveTuples, 10),
		}
		if s.LastAutovacuum != nil {
			detail["last_autovacuum"] = s.LastAutovacuum.Format(time.RFC3339)
		}

		if s.LastAutovacuum == nil {
			findings = append(findings, Finding{
				Type:     FindingMissingVacuum,
				Severity: SeverityLow,
				Schema:   s.Schema,
				Table:    s.Name,
				Message:  "active table has no autovacuum history",
				Detail:   detail,
			})
			continue
		}

		if now.Sub(*s.LastAutovacuum) > threshold {
			findings = append(findings, Finding{
				Type:     FindingMissingVacuum,
				Severity: SeverityLow,
				Schema:   s.Schema,
				Table:    s.Name,
				Message:  fmt.Sprintf("last autovacuum was %d days ago", int(now.Sub(*s.LastAutovacuum).Hours()/24)),
				Detail:   detail,
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
