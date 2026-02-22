package postgres

import "time"

// Config holds PostgreSQL connection settings.
type Config struct {
	URL string
}

// TableInfo describes a table from information_schema + pg_class.
type TableInfo struct {
	Schema        string `json:"schema"`
	Name          string `json:"name"`
	Type          string `json:"type"`          // BASE TABLE, VIEW, etc.
	EstimatedRows int64  `json:"estimatedRows"` // from pg_class.reltuples
	SizeBytes     int64  `json:"sizeBytes"`     // from pg_total_relation_size
}

// ColumnInfo describes a table column.
type ColumnInfo struct {
	Schema          string  `json:"schema"`
	Table           string  `json:"table"`
	Name            string  `json:"name"`
	OrdinalPosition int     `json:"ordinalPosition"`
	DataType        string  `json:"dataType"`
	IsNullable      bool    `json:"isNullable"`
	ColumnDefault   *string `json:"columnDefault,omitempty"`
}

// IndexInfo describes an index with definition and usage stats.
type IndexInfo struct {
	Schema     string `json:"schema"`
	Table      string `json:"table"`
	Name       string `json:"name"`
	Definition string `json:"definition"`
	SizeBytes  int64  `json:"sizeBytes"`
	IndexScans int64  `json:"indexScans"`
	TupRead    int64  `json:"tupRead"`
	TupFetch   int64  `json:"tupFetch"`
}

// TableStats holds usage statistics from pg_stat_user_tables.
type TableStats struct {
	Schema           string     `json:"schema"`
	Name             string     `json:"name"`
	SeqScan          int64      `json:"seqScan"`
	SeqTupRead       int64      `json:"seqTupRead"`
	IdxScan          int64      `json:"idxScan"`
	IdxTupFetch      int64      `json:"idxTupFetch"`
	LiveTuples       int64      `json:"liveTuples"`
	DeadTuples       int64      `json:"deadTuples"`
	LastVacuum       *time.Time `json:"lastVacuum,omitempty"`
	LastAutovacuum   *time.Time `json:"lastAutovacuum,omitempty"`
	LastAnalyze      *time.Time `json:"lastAnalyze,omitempty"`
	LastAutoanalyze  *time.Time `json:"lastAutoanalyze,omitempty"`
	VacuumCount      int64      `json:"vacuumCount"`
	AutovacuumCount  int64      `json:"autovacuumCount"`
	AnalyzeCount     int64      `json:"analyzeCount"`
	AutoanalyzeCount int64      `json:"autoanalyzeCount"`
}

// ConstraintInfo describes a table constraint.
type ConstraintInfo struct {
	Schema     string   `json:"schema"`
	Table      string   `json:"table"`
	Name       string   `json:"name"`
	Type       string   `json:"type"` // p=primary key, u=unique, f=foreign key, c=check
	Columns    []string `json:"columns"`
	RefTable   *string  `json:"refTable,omitempty"`
	RefColumns []string `json:"refColumns,omitempty"`
}

// Snapshot holds the complete catalog metadata for a database.
type Snapshot struct {
	Tables      []TableInfo      `json:"tables"`
	Columns     []ColumnInfo     `json:"columns"`
	Indexes     []IndexInfo      `json:"indexes"`
	Stats       []TableStats     `json:"stats"`
	Constraints []ConstraintInfo `json:"constraints"`
}
