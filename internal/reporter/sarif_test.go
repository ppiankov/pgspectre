package reporter

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/ppiankov/pgspectre/internal/analyzer"
)

func TestWriteSARIF_ValidStructure(t *testing.T) {
	findings := []analyzer.Finding{
		{
			Type:     analyzer.FindingMissingTable,
			Severity: analyzer.SeverityHigh,
			Schema:   "public",
			Table:    "users",
			Message:  "table \"users\" referenced in code but does not exist",
		},
		{
			Type:     analyzer.FindingUnusedIndex,
			Severity: analyzer.SeverityLow,
			Schema:   "public",
			Table:    "orders",
			Index:    "idx_old",
			Message:  "index idx_old has 0 scans",
		},
	}

	report := NewReport("check", findings)
	var buf bytes.Buffer
	if err := Write(&buf, &report, FormatSARIF); err != nil {
		t.Fatal(err)
	}

	// Parse the output
	var log sarifLog
	if err := json.Unmarshal(buf.Bytes(), &log); err != nil {
		t.Fatalf("invalid SARIF JSON: %v\n%s", err, buf.String())
	}

	// Verify structure
	if log.Version != "2.1.0" {
		t.Errorf("version = %q, want 2.1.0", log.Version)
	}
	if len(log.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(log.Runs))
	}

	run := log.Runs[0]
	if run.Tool.Driver.Name != "pgspectre" {
		t.Errorf("tool name = %q", run.Tool.Driver.Name)
	}
	if len(run.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(run.Results))
	}

	// Verify first result
	r0 := run.Results[0]
	if r0.RuleID != "pgspectre/MISSING_TABLE" {
		t.Errorf("ruleId = %q", r0.RuleID)
	}
	if r0.Level != "error" {
		t.Errorf("level = %q, want error", r0.Level)
	}
	if len(r0.Locations) != 1 {
		t.Fatalf("expected 1 location, got %d", len(r0.Locations))
	}
	loc := r0.Locations[0].LogicalLocations[0]
	if loc.FullyQualifiedName != "public.users" {
		t.Errorf("fqn = %q", loc.FullyQualifiedName)
	}

	// Verify second result
	r1 := run.Results[1]
	if r1.Level != "note" {
		t.Errorf("level = %q, want note for low severity", r1.Level)
	}
}

func TestWriteSARIF_Empty(t *testing.T) {
	report := NewReport("audit", nil)
	var buf bytes.Buffer
	if err := Write(&buf, &report, FormatSARIF); err != nil {
		t.Fatal(err)
	}

	var log sarifLog
	if err := json.Unmarshal(buf.Bytes(), &log); err != nil {
		t.Fatalf("invalid SARIF JSON: %v", err)
	}
	if len(log.Runs[0].Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(log.Runs[0].Results))
	}
}

func TestWriteSARIF_ColumnInFQN(t *testing.T) {
	findings := []analyzer.Finding{
		{
			Type:     analyzer.FindingMissingColumn,
			Severity: analyzer.SeverityMedium,
			Schema:   "public",
			Table:    "users",
			Column:   "email",
			Message:  "column \"email\" not found in table \"users\"",
		},
	}

	report := NewReport("check", findings)
	var buf bytes.Buffer
	if err := Write(&buf, &report, FormatSARIF); err != nil {
		t.Fatal(err)
	}

	var log sarifLog
	if err := json.Unmarshal(buf.Bytes(), &log); err != nil {
		t.Fatalf("invalid SARIF JSON: %v", err)
	}

	fqn := log.Runs[0].Results[0].Locations[0].LogicalLocations[0].FullyQualifiedName
	if fqn != "public.users.email" {
		t.Errorf("fqn = %q, want public.users.email", fqn)
	}
}

func TestWriteSARIF_WithDetails(t *testing.T) {
	findings := []analyzer.Finding{
		{
			Type: analyzer.FindingUnusedIndex, Severity: analyzer.SeverityMedium,
			Schema: "public", Table: "users", Index: "idx_old",
			Message: "index never used",
			Detail:  map[string]string{"size": "2.0 MB", "idx_scan": "0"},
		},
	}
	report := NewReport("audit", findings)
	var buf bytes.Buffer
	if err := Write(&buf, &report, FormatSARIF); err != nil {
		t.Fatal(err)
	}

	var log sarifLog
	if err := json.Unmarshal(buf.Bytes(), &log); err != nil {
		t.Fatalf("invalid SARIF JSON: %v", err)
	}

	msg := log.Runs[0].Results[0].Message.Text
	// Details should be appended to message in sorted key order
	if msg != "index never used [idx_scan=0] [size=2.0 MB]" {
		t.Errorf("message = %q, want details appended", msg)
	}
}

func TestWriteSARIF_SeverityMapping(t *testing.T) {
	tests := []struct {
		severity analyzer.Severity
		want     string
	}{
		{analyzer.SeverityHigh, "error"},
		{analyzer.SeverityMedium, "warning"},
		{analyzer.SeverityLow, "note"},
		{analyzer.SeverityInfo, "note"},
	}

	for _, tt := range tests {
		findings := []analyzer.Finding{
			{Type: analyzer.FindingMissingTable, Severity: tt.severity, Schema: "public", Table: "t"},
		}
		report := NewReport("test", findings)
		var buf bytes.Buffer
		if err := Write(&buf, &report, FormatSARIF); err != nil {
			t.Fatal(err)
		}

		var log sarifLog
		if err := json.Unmarshal(buf.Bytes(), &log); err != nil {
			t.Fatal(err)
		}
		got := log.Runs[0].Results[0].Level
		if got != tt.want {
			t.Errorf("severity %s → level %q, want %q", tt.severity, got, tt.want)
		}
	}
}
