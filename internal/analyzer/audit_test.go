package analyzer

import (
	"testing"
	"time"

	"github.com/ppiankov/pgspectre/internal/postgres"
)

func makeStats(schema, table string, seqScan, idxScan int64) postgres.TableStats {
	return postgres.TableStats{
		Schema:  schema,
		Name:    table,
		SeqScan: seqScan,
		IdxScan: idxScan,
	}
}

func makeIndex(schema, table, name, def string, size, scans int64) postgres.IndexInfo {
	return postgres.IndexInfo{
		Schema:     schema,
		Table:      table,
		Name:       name,
		Definition: def,
		SizeBytes:  size,
		IndexScans: scans,
	}
}

func makeConstraint(schema, table, name, ctype string) postgres.ConstraintInfo {
	return postgres.ConstraintInfo{
		Schema: schema,
		Table:  table,
		Name:   name,
		Type:   ctype,
	}
}

func TestDetectUnusedTables(t *testing.T) {
	tests := []struct {
		name  string
		stats []postgres.TableStats
		want  int
	}{
		{"no stats", nil, 0},
		{"active table", []postgres.TableStats{makeStats("public", "users", 100, 50)}, 0},
		{"seq only", []postgres.TableStats{makeStats("public", "users", 10, 0)}, 0},
		{"idx only", []postgres.TableStats{makeStats("public", "users", 0, 5)}, 0},
		{"unused", []postgres.TableStats{makeStats("public", "users", 0, 0)}, 1},
		{"mixed", []postgres.TableStats{
			makeStats("public", "users", 100, 50),
			makeStats("public", "old_data", 0, 0),
		}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := detectUnusedTables(tt.stats)
			if len(findings) != tt.want {
				t.Errorf("got %d findings, want %d", len(findings), tt.want)
			}
			for _, f := range findings {
				if f.Type != FindingUnusedTable {
					t.Errorf("expected type UNUSED_TABLE, got %s", f.Type)
				}
				if f.Severity != SeverityHigh {
					t.Errorf("expected severity high, got %s", f.Severity)
				}
			}
		})
	}
}

func TestDetectUnusedTables_Detail(t *testing.T) {
	vac := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	stats := []postgres.TableStats{
		{Schema: "public", Name: "old", SeqScan: 0, IdxScan: 0, LiveTuples: 100, DeadTuples: 5, LastVacuum: &vac},
	}
	findings := detectUnusedTables(stats)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	d := findings[0].Detail
	if d["live_tuples"] != "100" {
		t.Errorf("live_tuples = %q, want 100", d["live_tuples"])
	}
	if d["dead_tuples"] != "5" {
		t.Errorf("dead_tuples = %q, want 5", d["dead_tuples"])
	}
	if d["last_vacuum"] != "2025-06-01T00:00:00Z" {
		t.Errorf("last_vacuum = %q", d["last_vacuum"])
	}
}

func TestDetectUnusedIndexes(t *testing.T) {
	tests := []struct {
		name       string
		indexes    []postgres.IndexInfo
		minSize    int64
		want       int
		wantMedium bool
	}{
		{"no indexes", nil, 1024, 0, false},
		{"used index", []postgres.IndexInfo{makeIndex("public", "users", "users_pkey", "CREATE ...", 8192, 100)}, 1024, 0, false},
		{"unused below threshold", []postgres.IndexInfo{makeIndex("public", "users", "idx_small", "CREATE ...", 512, 0)}, 1024, 0, false},
		{"unused above threshold", []postgres.IndexInfo{makeIndex("public", "users", "idx_old", "CREATE ...", 8192, 0)}, 1024, 1, true},
		{"unused equal threshold", []postgres.IndexInfo{makeIndex("public", "users", "idx_equal", "CREATE ...", 1024, 0)}, 1024, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := detectUnusedIndexes(tt.indexes, tt.minSize)
			if len(findings) != tt.want {
				t.Errorf("got %d findings, want %d", len(findings), tt.want)
			}
			for _, f := range findings {
				if f.Type != FindingUnusedIndex {
					t.Errorf("expected type UNUSED_INDEX, got %s", f.Type)
				}
				if tt.wantMedium && f.Severity != SeverityMedium {
					t.Errorf("expected severity medium, got %s", f.Severity)
				}
			}
		})
	}
}

func TestDetectUnusedIndexes_Detail(t *testing.T) {
	indexes := []postgres.IndexInfo{
		makeIndex("public", "users", "idx_old", "CREATE ...", 8192, 0),
	}
	findings := detectUnusedIndexes(indexes, 4096)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	d := findings[0].Detail
	if d["size_bytes"] != "8192" {
		t.Errorf("size_bytes = %q, want 8192", d["size_bytes"])
	}
	if d["size"] != "8.0 KB" {
		t.Errorf("size = %q, want 8.0 KB", d["size"])
	}
	if d["idx_scan"] != "0" {
		t.Errorf("idx_scan = %q, want 0", d["idx_scan"])
	}
}

func TestDetectBloatedIndexes(t *testing.T) {
	tableSizeMap := map[string]int64{"public.users": 4 * 1024 * 1024}

	tests := []struct {
		name    string
		indexes []postgres.IndexInfo
		want    int
	}{
		{"no indexes", nil, 0},
		{"index smaller than table", []postgres.IndexInfo{makeIndex("public", "users", "idx_a", "CREATE ...", 2*1024*1024, 0)}, 0},
		{"index larger than table", []postgres.IndexInfo{makeIndex("public", "users", "idx_big", "CREATE ...", 6*1024*1024, 10)}, 1},
		{"index below bloat floor", []postgres.IndexInfo{makeIndex("public", "users", "idx_tiny", "CREATE ...", 512, 0)}, 0},
		{"missing table size", []postgres.IndexInfo{makeIndex("public", "orders", "idx_orders", "CREATE ...", 6*1024*1024, 0)}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := detectBloatedIndexes(tt.indexes, tableSizeMap, 1024*1024)
			if len(findings) != tt.want {
				t.Errorf("got %d findings, want %d", len(findings), tt.want)
			}
		})
	}
}

func TestDetectMissingVacuum(t *testing.T) {
	now := time.Now()
	recent := now.Add(-24 * time.Hour)
	old := now.Add(-60 * 24 * time.Hour)

	tests := []struct {
		name  string
		stats []postgres.TableStats
		want  int
	}{
		{"inactive table", []postgres.TableStats{makeStats("public", "old", 0, 0)}, 0},
		{"active, recent vacuum", []postgres.TableStats{{
			Schema: "public", Name: "users", SeqScan: 10,
			LastAutovacuum: &recent,
		}}, 0},
		{"active, old vacuum", []postgres.TableStats{{
			Schema: "public", Name: "users", SeqScan: 10,
			LastAutovacuum: &old,
		}}, 1},
		{"active, never vacuumed", []postgres.TableStats{{
			Schema: "public", Name: "users", SeqScan: 10,
		}}, 1},
		{"manual vacuum only still missing auto", []postgres.TableStats{{
			Schema: "public", Name: "users", SeqScan: 10,
			LastVacuum: &recent,
		}}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := detectMissingVacuum(tt.stats, now, 30*24*time.Hour)
			if len(findings) != tt.want {
				t.Errorf("got %d findings, want %d", len(findings), tt.want)
			}
			for _, f := range findings {
				if f.Type != FindingMissingVacuum {
					t.Errorf("expected type MISSING_VACUUM, got %s", f.Type)
				}
			}
		})
	}
}

func TestDetectNoPrimaryKey(t *testing.T) {
	tables := []postgres.TableInfo{
		{Schema: "public", Name: "users"},
		{Schema: "public", Name: "logs"},
	}

	tests := []struct {
		name  string
		pkSet map[string]bool
		want  int
	}{
		{"all have PK", map[string]bool{"public.users": true, "public.logs": true}, 0},
		{"one missing PK", map[string]bool{"public.users": true}, 1},
		{"none have PK", map[string]bool{}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := detectNoPrimaryKey(tables, tt.pkSet)
			if len(findings) != tt.want {
				t.Errorf("got %d findings, want %d", len(findings), tt.want)
			}
			for _, f := range findings {
				if f.Type != FindingNoPrimaryKey {
					t.Errorf("expected type NO_PRIMARY_KEY, got %s", f.Type)
				}
			}
		})
	}
}

func TestDetectDuplicateIndexes(t *testing.T) {
	tests := []struct {
		name    string
		indexes []postgres.IndexInfo
		want    int
	}{
		{"no indexes", nil, 0},
		{"unique definitions", []postgres.IndexInfo{
			makeIndex("public", "users", "idx_a", "CREATE INDEX idx_a ON users (name)", 8192, 10),
			makeIndex("public", "users", "idx_b", "CREATE INDEX idx_b ON users (email)", 8192, 5),
		}, 0},
		{"duplicate definitions", []postgres.IndexInfo{
			makeIndex("public", "users", "idx_a", "CREATE INDEX idx_a ON users (name)", 8192, 10),
			makeIndex("public", "users", "idx_b", "CREATE INDEX idx_b ON users (name)", 8192, 5),
		}, 1},
		{"whitespace difference", []postgres.IndexInfo{
			makeIndex("public", "users", "idx_a", "CREATE INDEX idx_a ON users  (name)", 8192, 10),
			makeIndex("public", "users", "idx_b", "CREATE INDEX idx_b ON users (name)", 8192, 5),
		}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := detectDuplicateIndexes(tt.indexes)
			if len(findings) != tt.want {
				t.Errorf("got %d findings, want %d", len(findings), tt.want)
			}
		})
	}
}

func TestAudit_Integration(t *testing.T) {
	snap := &postgres.Snapshot{
		Tables: []postgres.TableInfo{
			{Schema: "public", Name: "users", EstimatedRows: 1000, SizeBytes: 300 * 1024 * 1024},
			{Schema: "public", Name: "logs", EstimatedRows: 0, SizeBytes: 1024},
		},
		Stats: []postgres.TableStats{
			makeStats("public", "users", 100, 50),
			makeStats("public", "logs", 0, 0),
		},
		Indexes: []postgres.IndexInfo{
			makeIndex("public", "users", "users_pkey", "CREATE UNIQUE INDEX users_pkey ON users USING btree (id)", 8192, 50),
			makeIndex("public", "users", "idx_unused", "CREATE INDEX idx_unused ON users (old_col)", 200*1024*1024, 0),
		},
		Constraints: []postgres.ConstraintInfo{
			makeConstraint("public", "users", "users_pkey", "p"),
		},
	}

	findings := Audit(snap, DefaultAuditOptions())

	// Expected: UNUSED_TABLE(logs), UNUSED_INDEX(idx_unused), NO_PRIMARY_KEY(logs)
	typeCounts := make(map[FindingType]int)
	for _, f := range findings {
		typeCounts[f.Type]++
	}

	if typeCounts[FindingUnusedTable] != 1 {
		t.Errorf("expected 1 UNUSED_TABLE, got %d", typeCounts[FindingUnusedTable])
	}
	if typeCounts[FindingUnusedIndex] != 1 {
		t.Errorf("expected 1 UNUSED_INDEX, got %d", typeCounts[FindingUnusedIndex])
	}
	if typeCounts[FindingNoPrimaryKey] != 1 {
		t.Errorf("expected 1 NO_PRIMARY_KEY, got %d", typeCounts[FindingNoPrimaryKey])
	}
}

func TestLatestVacuum(t *testing.T) {
	t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		s    postgres.TableStats
		want *time.Time
	}{
		{"none", postgres.TableStats{}, nil},
		{"manual only", postgres.TableStats{LastVacuum: &t1}, &t1},
		{"auto only", postgres.TableStats{LastAutovacuum: &t2}, &t2},
		{"both, auto newer", postgres.TableStats{LastVacuum: &t1, LastAutovacuum: &t2}, &t2},
		{"both, manual newer", postgres.TableStats{LastVacuum: &t2, LastAutovacuum: &t1}, &t2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := latestVacuum(&tt.s)
			if tt.want == nil && got != nil {
				t.Errorf("expected nil, got %v", got)
			}
			if tt.want != nil && (got == nil || !got.Equal(*tt.want)) {
				t.Errorf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestNormalizeDef(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"CREATE INDEX foo ON bar (baz)", " ON bar (baz)"},
		{"CREATE  INDEX  foo  ON  bar  (baz)", " ON bar (baz)"},
		{"CREATE UNIQUE INDEX foo ON bar (baz)", " ON bar (baz)"},
		{"plain text without keyword", "plain text without keyword"},
	}

	for _, tt := range tests {
		got := normalizeDef(tt.in)
		if got != tt.want {
			t.Errorf("normalizeDef(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{500, "500 bytes"},
		{1536, "1.5 KB"},
		{2 * 1024 * 1024, "2.0 MB"},
		{3 * 1024 * 1024 * 1024, "3.0 GB"},
	}

	for _, tt := range tests {
		got := formatBytes(tt.in)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
