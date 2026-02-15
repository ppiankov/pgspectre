package analyzer

// Severity indicates the risk level of a finding.
type Severity string

const (
	SeverityHigh   Severity = "high"
	SeverityMedium Severity = "medium"
	SeverityLow    Severity = "low"
	SeverityInfo   Severity = "info"
)

// FindingType identifies what kind of issue was detected.
type FindingType string

const (
	FindingUnusedTable       FindingType = "UNUSED_TABLE"
	FindingUnusedIndex       FindingType = "UNUSED_INDEX"
	FindingBloatedIndex      FindingType = "BLOATED_INDEX"
	FindingMissingVacuum     FindingType = "MISSING_VACUUM"
	FindingNoPrimaryKey      FindingType = "NO_PRIMARY_KEY"
	FindingDuplicateIndex    FindingType = "DUPLICATE_INDEX"
	FindingMissingTable      FindingType = "MISSING_TABLE"
	FindingMissingColumn     FindingType = "MISSING_COLUMN"
	FindingUnreferencedTable FindingType = "UNREFERENCED_TABLE"
	FindingCodeMatch         FindingType = "CODE_MATCH"
	FindingUnindexedQuery    FindingType = "UNINDEXED_QUERY"
	FindingOK                FindingType = "OK"
)

// Finding represents a single audit or check result.
type Finding struct {
	Type     FindingType       `json:"type"`
	Severity Severity          `json:"severity"`
	Schema   string            `json:"schema"`
	Table    string            `json:"table"`
	Column   string            `json:"column,omitempty"`
	Index    string            `json:"index,omitempty"`
	Message  string            `json:"message"`
	Detail   map[string]string `json:"detail,omitempty"`
}

// AuditOptions controls thresholds and exclusions for analysis.
type AuditOptions struct {
	VacuumDays     int
	BloatMinBytes  int64
	ExcludeTables  []string
	ExcludeSchemas []string
}

// DefaultAuditOptions returns sensible defaults matching the config defaults.
func DefaultAuditOptions() AuditOptions {
	return AuditOptions{
		VacuumDays:    30,
		BloatMinBytes: 1024 * 1024, // 1 MB
	}
}

var severityOrder = map[Severity]int{
	SeverityInfo:   0,
	SeverityLow:    1,
	SeverityMedium: 2,
	SeverityHigh:   3,
}

// MaxSeverity returns the highest severity among findings.
func MaxSeverity(findings []Finding) Severity {
	max := SeverityInfo
	for _, f := range findings {
		if severityOrder[f.Severity] > severityOrder[max] {
			max = f.Severity
		}
	}
	return max
}

// ExitCode maps severity to a CLI exit code.
func ExitCode(s Severity) int {
	switch s {
	case SeverityHigh:
		return 2
	case SeverityMedium:
		return 1
	default:
		return 0
	}
}
