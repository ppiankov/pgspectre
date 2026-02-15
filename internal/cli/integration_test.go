//go:build integration

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ppiankov/pgspectre/internal/analyzer"
	"github.com/ppiankov/pgspectre/internal/reporter"
	"github.com/ppiankov/pgspectre/internal/testutil"
)

// connStr is set by TestMain and shared across all integration tests.
var connStr string

func TestMain(m *testing.M) {
	cs, cleanup, err := testutil.Setup()
	if err != nil {
		fmt.Println("skipping integration tests:", err)
		os.Exit(0)
	}
	connStr = cs
	code := m.Run()
	cleanup()
	os.Exit(code)
}

// runCmd executes a CLI command and returns stdout and error.
func runCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newRootCmd(BuildInfo{Version: "test"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// writeFile creates a file in the given directory.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIntegration_Audit_JSON(t *testing.T) {
	stdout, err := runCmd(t, "audit", "--db-url", connStr, "--format", "json")

	// Expect ExitError because findings have high severity
	var ee *ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected ExitError, got: %v", err)
	}

	var report reporter.Report
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}

	if report.Summary.Total == 0 {
		t.Error("expected findings, got 0")
	}
	if report.Metadata.Command != "audit" {
		t.Errorf("command = %q, want audit", report.Metadata.Command)
	}
	if report.Scanned.Tables < 3 {
		t.Errorf("scanned tables = %d, want >= 3", report.Scanned.Tables)
	}

	// Verify expected finding types exist
	types := make(map[analyzer.FindingType]bool)
	for _, f := range report.Findings {
		types[f.Type] = true
	}
	if !types[analyzer.FindingUnusedTable] {
		t.Error("expected UNUSED_TABLE finding")
	}
}

func TestIntegration_Audit_Text(t *testing.T) {
	stdout, _ := runCmd(t, "audit", "--db-url", connStr, "--format", "text", "--no-color")

	for _, want := range []string{"Summary:", "UNUSED_TABLE"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("expected %q in output, got:\n%s", want, stdout)
		}
	}
}

func TestIntegration_Audit_SARIF(t *testing.T) {
	stdout, _ := runCmd(t, "audit", "--db-url", connStr, "--format", "sarif")

	var sarif map[string]any
	if err := json.Unmarshal([]byte(stdout), &sarif); err != nil {
		t.Fatalf("invalid SARIF JSON: %v\n%s", err, stdout)
	}

	schema, ok := sarif["$schema"].(string)
	if !ok || !strings.Contains(schema, "sarif") {
		t.Errorf("missing or invalid $schema: %v", sarif["$schema"])
	}

	runs, ok := sarif["runs"].([]any)
	if !ok || len(runs) == 0 {
		t.Error("expected at least one run in SARIF output")
	}
}

func TestIntegration_Audit_TypeFilter(t *testing.T) {
	stdout, err := runCmd(t, "audit", "--db-url", connStr, "--format", "json", "--type", "UNUSED_TABLE")

	// May or may not have ExitError depending on filtered severity
	var ee *ExitError
	if err != nil && !errors.As(err, &ee) {
		t.Fatalf("unexpected error: %v", err)
	}

	var report reporter.Report
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}

	for _, f := range report.Findings {
		if f.Type != analyzer.FindingUnusedTable {
			t.Errorf("expected only UNUSED_TABLE, got %s", f.Type)
		}
	}
	if report.Summary.Total == 0 {
		t.Error("expected at least one UNUSED_TABLE finding")
	}
}

func TestIntegration_Audit_Baseline(t *testing.T) {
	baselinePath := filepath.Join(t.TempDir(), "baseline.json")

	// First run: save baseline
	_, err := runCmd(t, "audit", "--db-url", connStr, "--format", "json", "--update-baseline", baselinePath)
	var ee *ExitError
	if err != nil && !errors.As(err, &ee) {
		t.Fatalf("save baseline: %v", err)
	}

	// Verify baseline file exists
	if _, err := os.Stat(baselinePath); err != nil {
		t.Fatalf("baseline file not created: %v", err)
	}

	// Second run: load baseline, all findings should be suppressed
	stdout, err := runCmd(t, "audit", "--db-url", connStr, "--format", "json", "--baseline", baselinePath)
	if err != nil {
		var ee2 *ExitError
		if !errors.As(err, &ee2) {
			t.Fatalf("load baseline: %v", err)
		}
	}

	var report reporter.Report
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}

	if report.Summary.Total != 0 {
		t.Errorf("expected 0 findings after baseline, got %d", report.Summary.Total)
	}
}

func TestIntegration_Check_JSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app.go", `package main
func main() { db.Query("SELECT * FROM users") }`)

	stdout, err := runCmd(t, "check", "--db-url", connStr, "--repo", dir, "--format", "json")

	// Expect ExitError (audit findings have high severity)
	var ee *ExitError
	if err != nil && !errors.As(err, &ee) {
		t.Fatalf("unexpected error: %v", err)
	}

	var report reporter.Report
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}

	if report.Metadata.Command != "check" {
		t.Errorf("command = %q, want check", report.Metadata.Command)
	}

	// "users" exists in DB — should NOT have MISSING_TABLE
	for _, f := range report.Findings {
		if f.Type == analyzer.FindingMissingTable && strings.EqualFold(f.Table, "users") {
			t.Error("users should not be MISSING_TABLE — it exists in DB")
		}
	}

	// Should have CODE_MATCH for users
	types := make(map[analyzer.FindingType]bool)
	for _, f := range report.Findings {
		types[f.Type] = true
	}
	if !types[analyzer.FindingCodeMatch] {
		t.Error("expected CODE_MATCH finding for users")
	}
}

func TestIntegration_Check_MissingTable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app.go", `package main
func main() { db.Query("SELECT * FROM nonexistent_table") }`)

	stdout, err := runCmd(t, "check", "--db-url", connStr, "--repo", dir, "--format", "json")

	var ee *ExitError
	if err != nil && !errors.As(err, &ee) {
		t.Fatalf("unexpected error: %v", err)
	}

	var report reporter.Report
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}

	found := false
	for _, f := range report.Findings {
		if f.Type == analyzer.FindingMissingTable && strings.EqualFold(f.Table, "nonexistent_table") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected MISSING_TABLE finding for nonexistent_table")
	}
}

func TestIntegration_Check_Parallel(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app.go", `package main
func main() { db.Query("SELECT * FROM users") }`)

	stdout, err := runCmd(t, "check", "--db-url", connStr, "--repo", dir, "--format", "json", "--parallel", "2")

	var ee *ExitError
	if err != nil && !errors.As(err, &ee) {
		t.Fatalf("unexpected error: %v", err)
	}

	var report reporter.Report
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}

	// Should produce same results as non-parallel
	types := make(map[analyzer.FindingType]bool)
	for _, f := range report.Findings {
		types[f.Type] = true
	}
	if !types[analyzer.FindingCodeMatch] {
		t.Error("expected CODE_MATCH finding for users")
	}
}

func TestIntegration_Audit_BadURL(t *testing.T) {
	_, err := runCmd(t, "audit", "--db-url", "postgres://invalid:5432/nodb", "--format", "json")

	if err == nil {
		t.Error("expected error for invalid URL")
	}
	var ee *ExitError
	if errors.As(err, &ee) {
		t.Error("expected connection error, not ExitError")
	}
}

func TestIntegration_Audit_ExitCode(t *testing.T) {
	_, err := runCmd(t, "audit", "--db-url", connStr, "--format", "json")

	var ee *ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected ExitError, got: %v", err)
	}
	// Seed data has UNUSED_TABLE (high severity) → ExitCode should be 2
	if ee.Code != 2 {
		t.Errorf("exit code = %d, want 2", ee.Code)
	}
}
