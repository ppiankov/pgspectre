package reporter

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ppiankov/pgspectre/internal/analyzer"
)

var testFindings = []analyzer.Finding{
	{Type: analyzer.FindingUnusedTable, Severity: analyzer.SeverityHigh, Schema: "public", Table: "old_data", Message: "table has no scans"},
	{Type: analyzer.FindingUnusedIndex, Severity: analyzer.SeverityMedium, Schema: "public", Table: "users", Index: "idx_old", Message: "index never used"},
	{Type: analyzer.FindingMissingVacuum, Severity: analyzer.SeverityLow, Schema: "public", Table: "logs", Message: "never vacuumed"},
}

func TestNewReport_Summary(t *testing.T) {
	r := NewReport("audit", testFindings)

	if r.Summary.Total != 3 {
		t.Errorf("total = %d, want 3", r.Summary.Total)
	}
	if r.Summary.High != 1 {
		t.Errorf("high = %d, want 1", r.Summary.High)
	}
	if r.Summary.Medium != 1 {
		t.Errorf("medium = %d, want 1", r.Summary.Medium)
	}
	if r.Summary.Low != 1 {
		t.Errorf("low = %d, want 1", r.Summary.Low)
	}
	if r.MaxSeverity != analyzer.SeverityHigh {
		t.Errorf("maxSeverity = %q, want %q", r.MaxSeverity, analyzer.SeverityHigh)
	}
	if r.Metadata.Tool != "pgspectre" {
		t.Errorf("tool = %q, want %q", r.Metadata.Tool, "pgspectre")
	}
	if r.Metadata.Command != "audit" {
		t.Errorf("command = %q, want %q", r.Metadata.Command, "audit")
	}
}

func TestNewReport_Empty(t *testing.T) {
	r := NewReport("audit", nil)

	if r.Summary.Total != 0 {
		t.Errorf("total = %d, want 0", r.Summary.Total)
	}
	if r.MaxSeverity != analyzer.SeverityInfo {
		t.Errorf("maxSeverity = %q, want %q", r.MaxSeverity, analyzer.SeverityInfo)
	}
	if r.Findings == nil {
		t.Error("findings should be empty slice, not nil")
	}
}

func TestWriteText(t *testing.T) {
	r := NewReport("audit", testFindings)
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatText); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "[HIGH]") {
		t.Error("expected [HIGH] label in output")
	}
	if !strings.Contains(out, "UNUSED_TABLE") {
		t.Error("expected UNUSED_TABLE in output")
	}
	if !strings.Contains(out, "public.old_data") {
		t.Error("expected location public.old_data in output")
	}
	if !strings.Contains(out, "public.users.idx_old") {
		t.Error("expected location public.users.idx_old in output")
	}
	if !strings.Contains(out, "Summary: 3 findings") {
		t.Error("expected summary line in output")
	}
}

func TestWriteText_Empty(t *testing.T) {
	r := NewReport("audit", nil)
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatText); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No findings.") {
		t.Errorf("expected 'No findings.' in output, got %q", buf.String())
	}
}

func TestWriteText_WithDetails(t *testing.T) {
	findings := []analyzer.Finding{
		{
			Type: analyzer.FindingUnusedIndex, Severity: analyzer.SeverityMedium,
			Schema: "public", Table: "users", Index: "idx_old",
			Message: "index never used",
			Detail:  map[string]string{"size": "2.0 MB", "idx_scan": "0"},
		},
	}
	r := NewReport("audit", findings)
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatText); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "  idx_scan: 0\n") {
		t.Error("expected detail line 'idx_scan: 0'")
	}
	if !strings.Contains(out, "  size: 2.0 MB\n") {
		t.Error("expected detail line 'size: 2.0 MB'")
	}
}

func TestWriteText_NoDetails(t *testing.T) {
	findings := []analyzer.Finding{
		{Type: analyzer.FindingNoPrimaryKey, Severity: analyzer.SeverityMedium, Schema: "public", Table: "t", Message: "no PK"},
	}
	r := NewReport("audit", findings)
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatText); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	// Should be just the finding line + summary line (no detail lines)
	if len(lines) != 3 { // finding, empty line from \n before Summary, summary
		t.Errorf("expected 3 lines, got %d: %q", len(lines), out)
	}
}

func TestWriteJSON_WithDetails(t *testing.T) {
	findings := []analyzer.Finding{
		{
			Type: analyzer.FindingUnusedTable, Severity: analyzer.SeverityHigh,
			Schema: "public", Table: "old", Message: "unused",
			Detail: map[string]string{"live_tuples": "100", "dead_tuples": "50"},
		},
	}
	r := NewReport("audit", findings)
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatJSON); err != nil {
		t.Fatal(err)
	}
	var decoded Report
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if decoded.Findings[0].Detail["live_tuples"] != "100" {
		t.Errorf("expected live_tuples=100, got %q", decoded.Findings[0].Detail["live_tuples"])
	}
}

func TestWriteJSON(t *testing.T) {
	r := NewReport("audit", testFindings)
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatJSON); err != nil {
		t.Fatal(err)
	}

	var decoded Report
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if decoded.Summary.Total != 3 {
		t.Errorf("total = %d, want 3", decoded.Summary.Total)
	}
	if len(decoded.Findings) != 3 {
		t.Errorf("findings = %d, want 3", len(decoded.Findings))
	}
}
