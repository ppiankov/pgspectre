package scanner

// PatternType identifies how a table reference was detected.
type PatternType string

const (
	PatternSQL       PatternType = "sql"
	PatternORM       PatternType = "orm"
	PatternMigration PatternType = "migration"
)

// Context describes the SQL operation context.
type Context string

const (
	ContextSelect  Context = "SELECT"
	ContextInsert  Context = "INSERT"
	ContextUpdate  Context = "UPDATE"
	ContextDelete  Context = "DELETE"
	ContextDDL     Context = "DDL"
	ContextUnknown Context = "UNKNOWN"
)

// TableRef is a single reference to a database table found in code.
type TableRef struct {
	Table      string      `json:"table"`
	Schema     string      `json:"schema,omitempty"`
	File       string      `json:"file"`
	Line       int         `json:"line"`
	Pattern    PatternType `json:"pattern"`
	Context    Context     `json:"context"`
	Suppressed bool        `json:"suppressed,omitempty"`
}

// ColumnRef is a single reference to a database column found in code.
type ColumnRef struct {
	Table      string  `json:"table"`
	Column     string  `json:"column"`
	Schema     string  `json:"schema,omitempty"`
	File       string  `json:"file"`
	Line       int     `json:"line"`
	Context    Context `json:"context"`
	Suppressed bool    `json:"suppressed,omitempty"`
}

// ScanResult holds all table and column references found in a code repository.
type ScanResult struct {
	RepoPath     string      `json:"repoPath"`
	Refs         []TableRef  `json:"refs"`
	ColumnRefs   []ColumnRef `json:"columnRefs,omitempty"`
	Tables       []string    `json:"tables"`
	Columns      []string    `json:"columns,omitempty"`
	FilesScanned int         `json:"filesScanned"`
	FilesSkipped int         `json:"filesSkipped,omitempty"`
}
