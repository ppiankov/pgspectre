package suppress

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ppiankov/pgspectre/internal/analyzer"
)

func TestLoadRules_NoFile(t *testing.T) {
	rules, err := LoadRules(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(rules.ignoreFile.Suppressions) != 0 {
		t.Error("expected empty rules")
	}
}

func TestLoadRules_ValidFile(t *testing.T) {
	dir := t.TempDir()
	content := `suppressions:
  - table: legacy_audit_log
    reason: "Intentionally unused"
  - table: temp_migration_*
    type: UNUSED_TABLE
    reason: "Cleaned up monthly"
`
	if err := os.WriteFile(filepath.Join(dir, ".pgspectre-ignore.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	rules, err := LoadRules(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules.ignoreFile.Suppressions) != 2 {
		t.Fatalf("expected 2 suppressions, got %d", len(rules.ignoreFile.Suppressions))
	}
}

func TestLoadRules_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".pgspectre-ignore.yml"), []byte("{{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadRules(dir)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestIsSuppressed_ExactMatch(t *testing.T) {
	rules := &Rules{
		ignoreFile: IgnoreFile{
			Suppressions: []Suppression{
				{Table: "legacy_audit_log"},
			},
		},
	}

	f := analyzer.Finding{Type: analyzer.FindingUnusedTable, Table: "legacy_audit_log"}
	if !rules.IsSuppressed(&f) {
		t.Error("should be suppressed")
	}

	f2 := analyzer.Finding{Type: analyzer.FindingUnusedTable, Table: "users"}
	if rules.IsSuppressed(&f2) {
		t.Error("should not be suppressed")
	}
}

func TestIsSuppressed_GlobPattern(t *testing.T) {
	rules := &Rules{
		ignoreFile: IgnoreFile{
			Suppressions: []Suppression{
				{Table: "temp_migration_*"},
			},
		},
	}

	f := analyzer.Finding{Type: analyzer.FindingUnusedTable, Table: "temp_migration_001"}
	if !rules.IsSuppressed(&f) {
		t.Error("glob pattern should match")
	}

	f2 := analyzer.Finding{Type: analyzer.FindingUnusedTable, Table: "real_table"}
	if rules.IsSuppressed(&f2) {
		t.Error("glob pattern should not match")
	}
}

func TestIsSuppressed_TypeFilter(t *testing.T) {
	rules := &Rules{
		ignoreFile: IgnoreFile{
			Suppressions: []Suppression{
				{Table: "orders", Type: "UNUSED_TABLE"},
			},
		},
	}

	f1 := analyzer.Finding{Type: analyzer.FindingUnusedTable, Table: "orders"}
	if !rules.IsSuppressed(&f1) {
		t.Error("matching type should be suppressed")
	}

	f2 := analyzer.Finding{Type: analyzer.FindingMissingVacuum, Table: "orders"}
	if rules.IsSuppressed(&f2) {
		t.Error("non-matching type should not be suppressed")
	}
}

func TestIsSuppressed_CaseInsensitive(t *testing.T) {
	rules := &Rules{
		ignoreFile: IgnoreFile{
			Suppressions: []Suppression{
				{Table: "Users"},
			},
		},
	}

	f := analyzer.Finding{Type: analyzer.FindingUnusedTable, Table: "users"}
	if !rules.IsSuppressed(&f) {
		t.Error("case-insensitive match should suppress")
	}
}

func TestIsSuppressed_ConfigFindings(t *testing.T) {
	rules := &Rules{
		configFindings: []string{"UNUSED_TABLE"},
	}

	f1 := analyzer.Finding{Type: analyzer.FindingUnusedTable, Table: "anything"}
	if !rules.IsSuppressed(&f1) {
		t.Error("config finding type should be suppressed")
	}

	f2 := analyzer.Finding{Type: analyzer.FindingMissingTable, Table: "anything"}
	if rules.IsSuppressed(&f2) {
		t.Error("non-matching finding type should not be suppressed")
	}
}

func TestFilter(t *testing.T) {
	rules := &Rules{
		ignoreFile: IgnoreFile{
			Suppressions: []Suppression{
				{Table: "legacy_log"},
			},
		},
	}

	findings := []analyzer.Finding{
		{Type: analyzer.FindingUnusedTable, Table: "legacy_log"},
		{Type: analyzer.FindingMissingTable, Table: "users"},
		{Type: analyzer.FindingUnusedIndex, Table: "orders", Index: "idx_old"},
	}

	filtered, suppressed := rules.Filter(findings)
	if suppressed != 1 {
		t.Errorf("expected 1 suppressed, got %d", suppressed)
	}
	if len(filtered) != 2 {
		t.Errorf("expected 2 remaining, got %d", len(filtered))
	}
}

func TestFilter_NoRules(t *testing.T) {
	rules := &Rules{}
	findings := []analyzer.Finding{
		{Type: analyzer.FindingUnusedTable, Table: "users"},
	}

	filtered, suppressed := rules.Filter(findings)
	if suppressed != 0 {
		t.Errorf("expected 0 suppressed, got %d", suppressed)
	}
	if len(filtered) != 1 {
		t.Errorf("expected 1 remaining, got %d", len(filtered))
	}
}

func TestHasInlineIgnore(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{`db.Query("SELECT * FROM users") // pgspectre:ignore`, true},
		{`# pgspectre:ignore`, true},
		{`-- pgspectre:ignore`, true},
		{`db.Query("SELECT * FROM users")`, false},
		{`// some other comment`, false},
	}
	for _, tt := range tests {
		got := HasInlineIgnore(tt.line)
		if got != tt.want {
			t.Errorf("HasInlineIgnore(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestMatchTable(t *testing.T) {
	tests := []struct {
		pattern, table string
		want           bool
	}{
		{"users", "users", true},
		{"users", "orders", false},
		{"temp_*", "temp_migration_001", true},
		{"temp_*", "permanent_table", false},
		{"Users", "users", true}, // case-insensitive
	}
	for _, tt := range tests {
		got := matchTable(tt.pattern, tt.table)
		if got != tt.want {
			t.Errorf("matchTable(%q, %q) = %v, want %v", tt.pattern, tt.table, got, tt.want)
		}
	}
}
