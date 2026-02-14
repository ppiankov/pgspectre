package analyzer

import (
	"testing"

	"github.com/ppiankov/pgspectre/internal/postgres"
	"github.com/ppiankov/pgspectre/internal/scanner"
)

func scanResult(tables ...string) scanner.ScanResult {
	var refs []scanner.TableRef
	for _, t := range tables {
		refs = append(refs, scanner.TableRef{Table: t, File: "app.go", Line: 1})
	}
	return scanner.ScanResult{
		Refs:   refs,
		Tables: tables,
	}
}

func tableInfo(schema, name string, rows int64) postgres.TableInfo {
	return postgres.TableInfo{Schema: schema, Name: name, Type: "BASE TABLE", EstimatedRows: rows}
}

func TestDiff_MissingTable(t *testing.T) {
	scan := scanResult("users", "nonexistent")
	snap := &postgres.Snapshot{
		Tables: []postgres.TableInfo{tableInfo("public", "users", 100)},
		Stats:  []postgres.TableStats{makeStats("public", "users", 10, 5)},
	}

	findings := Diff(&scan, snap)

	var missing int
	for _, f := range findings {
		if f.Type == FindingMissingTable && f.Table == "nonexistent" {
			missing++
		}
	}
	if missing != 1 {
		t.Errorf("expected 1 MISSING_TABLE for nonexistent, got %d", missing)
	}
}

func TestDiff_CodeMatch(t *testing.T) {
	scan := scanResult("users")
	snap := &postgres.Snapshot{
		Tables: []postgres.TableInfo{tableInfo("public", "users", 100)},
		Stats:  []postgres.TableStats{makeStats("public", "users", 10, 5)},
	}

	findings := Diff(&scan, snap)

	var matched int
	for _, f := range findings {
		if f.Type == FindingCodeMatch && f.Table == "users" {
			matched++
		}
	}
	if matched != 1 {
		t.Errorf("expected 1 CODE_MATCH for users, got %d", matched)
	}
}

func TestDiff_UnreferencedTable(t *testing.T) {
	scan := scanResult("users")
	snap := &postgres.Snapshot{
		Tables: []postgres.TableInfo{
			tableInfo("public", "users", 100),
			tableInfo("public", "old_data", 0),
		},
		Stats: []postgres.TableStats{
			makeStats("public", "users", 10, 5),
			makeStats("public", "old_data", 0, 0),
		},
	}

	findings := Diff(&scan, snap)

	var unreferenced int
	for _, f := range findings {
		if f.Type == FindingUnreferencedTable && f.Table == "old_data" {
			unreferenced++
		}
	}
	if unreferenced != 1 {
		t.Errorf("expected 1 UNREFERENCED_TABLE for old_data, got %d", unreferenced)
	}
}

func TestDiff_ActiveUnreferencedTable_NotFlagged(t *testing.T) {
	scan := scanResult("users")
	snap := &postgres.Snapshot{
		Tables: []postgres.TableInfo{
			tableInfo("public", "users", 100),
			tableInfo("public", "active_table", 500),
		},
		Stats: []postgres.TableStats{
			makeStats("public", "users", 10, 5),
			makeStats("public", "active_table", 100, 50),
		},
	}

	findings := Diff(&scan, snap)

	for _, f := range findings {
		if f.Type == FindingUnreferencedTable && f.Table == "active_table" {
			t.Error("active_table has scans and should not be flagged as UNREFERENCED_TABLE")
		}
	}
}

func TestDiff_CaseInsensitive(t *testing.T) {
	scan := scanResult("Users")
	snap := &postgres.Snapshot{
		Tables: []postgres.TableInfo{tableInfo("public", "users", 100)},
		Stats:  []postgres.TableStats{makeStats("public", "users", 10, 5)},
	}

	findings := Diff(&scan, snap)

	var matched int
	for _, f := range findings {
		if f.Type == FindingCodeMatch {
			matched++
		}
	}
	if matched != 1 {
		t.Errorf("expected case-insensitive match, got %d CODE_MATCH findings", matched)
	}
}

func TestDiff_Empty(t *testing.T) {
	scan := scanResult()
	snap := &postgres.Snapshot{}

	findings := Diff(&scan, snap)

	// Should only have audit findings (none, since snapshot is empty)
	for _, f := range findings {
		if f.Type == FindingMissingTable || f.Type == FindingUnreferencedTable || f.Type == FindingCodeMatch {
			t.Errorf("unexpected finding type %s with empty inputs", f.Type)
		}
	}
}

func TestDiff_IncludesAuditFindings(t *testing.T) {
	scan := scanResult("users")
	snap := &postgres.Snapshot{
		Tables:      []postgres.TableInfo{tableInfo("public", "users", 100)},
		Stats:       []postgres.TableStats{makeStats("public", "users", 10, 5)},
		Constraints: []postgres.ConstraintInfo{
			// No PK for users → should produce NO_PRIMARY_KEY from audit
		},
	}

	findings := Diff(&scan, snap)

	var noPK int
	for _, f := range findings {
		if f.Type == FindingNoPrimaryKey {
			noPK++
		}
	}
	if noPK != 1 {
		t.Errorf("expected 1 NO_PRIMARY_KEY from audit, got %d", noPK)
	}
}
