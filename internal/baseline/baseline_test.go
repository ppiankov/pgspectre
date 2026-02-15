package baseline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ppiankov/pgspectre/internal/analyzer"
)

func TestFingerprint_Stable(t *testing.T) {
	f := analyzer.Finding{Type: analyzer.FindingMissingTable, Schema: "public", Table: "users"}
	fp1 := Fingerprint(&f)
	fp2 := Fingerprint(&f)
	if fp1 != fp2 {
		t.Errorf("fingerprint not stable: %q != %q", fp1, fp2)
	}
}

func TestFingerprint_Distinct(t *testing.T) {
	f1 := analyzer.Finding{Type: analyzer.FindingMissingTable, Schema: "public", Table: "users"}
	f2 := analyzer.Finding{Type: analyzer.FindingMissingTable, Schema: "public", Table: "orders"}
	if Fingerprint(&f1) == Fingerprint(&f2) {
		t.Error("different findings should have different fingerprints")
	}
}

func TestFingerprint_IncludesColumn(t *testing.T) {
	f1 := analyzer.Finding{Type: analyzer.FindingMissingColumn, Schema: "public", Table: "users", Column: "name"}
	f2 := analyzer.Finding{Type: analyzer.FindingMissingColumn, Schema: "public", Table: "users", Column: "email"}
	if Fingerprint(&f1) == Fingerprint(&f2) {
		t.Error("findings with different columns should have different fingerprints")
	}
}

func TestLoad_NoFile(t *testing.T) {
	b, err := Load("/nonexistent/path.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Fingerprints) != 0 {
		t.Errorf("expected empty baseline, got %d fingerprints", len(b.Fingerprints))
	}
}

func TestLoad_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")

	findings := []analyzer.Finding{
		{Type: analyzer.FindingMissingTable, Schema: "public", Table: "users"},
		{Type: analyzer.FindingUnusedIndex, Schema: "public", Table: "orders", Index: "idx_old"},
	}

	if err := Save(path, findings); err != nil {
		t.Fatal(err)
	}

	b, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Fingerprints) != 2 {
		t.Errorf("expected 2 fingerprints, got %d", len(b.Fingerprints))
	}

	// Verify contains works
	if !b.Contains(&findings[0]) {
		t.Error("baseline should contain first finding")
	}
	if !b.Contains(&findings[1]) {
		t.Error("baseline should contain second finding")
	}

	// New finding should not be contained
	newFinding := analyzer.Finding{Type: analyzer.FindingMissingTable, Schema: "public", Table: "payments"}
	if b.Contains(&newFinding) {
		t.Error("baseline should not contain new finding")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSave_Deduplicate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")

	findings := []analyzer.Finding{
		{Type: analyzer.FindingMissingTable, Schema: "public", Table: "users"},
		{Type: analyzer.FindingMissingTable, Schema: "public", Table: "users"}, // duplicate
	}

	if err := Save(path, findings); err != nil {
		t.Fatal(err)
	}

	b, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Fingerprints) != 1 {
		t.Errorf("expected 1 unique fingerprint, got %d", len(b.Fingerprints))
	}
}

func TestFilter(t *testing.T) {
	findings := []analyzer.Finding{
		{Type: analyzer.FindingMissingTable, Schema: "public", Table: "users"},
		{Type: analyzer.FindingMissingTable, Schema: "public", Table: "orders"},
		{Type: analyzer.FindingUnusedIndex, Schema: "public", Table: "orders", Index: "idx_old"},
	}

	// Baseline contains only the first finding
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")
	if err := Save(path, findings[:1]); err != nil {
		t.Fatal(err)
	}
	b, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	filtered, suppressed := b.Filter(findings)
	if suppressed != 1 {
		t.Errorf("expected 1 suppressed, got %d", suppressed)
	}
	if len(filtered) != 2 {
		t.Errorf("expected 2 remaining findings, got %d", len(filtered))
	}
}

func TestFilter_EmptyBaseline(t *testing.T) {
	b := &Baseline{set: make(map[string]bool)}
	findings := []analyzer.Finding{
		{Type: analyzer.FindingMissingTable, Schema: "public", Table: "users"},
	}

	filtered, suppressed := b.Filter(findings)
	if suppressed != 0 {
		t.Errorf("expected 0 suppressed, got %d", suppressed)
	}
	if len(filtered) != 1 {
		t.Errorf("expected 1 finding, got %d", len(filtered))
	}
}
