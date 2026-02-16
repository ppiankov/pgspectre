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
	r := NewReport("audit", testFindings, "test")

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
	r := NewReport("audit", nil, "test")

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
	r := NewReport("audit", testFindings, "test")
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
	// Table headers (grouped by table)
	if !strings.Contains(out, "public.old_data\n") {
		t.Error("expected table header public.old_data")
	}
	if !strings.Contains(out, "public.users\n") {
		t.Error("expected table header public.users")
	}
	// Index shown in finding line
	if !strings.Contains(out, "(idx_old)") {
		t.Error("expected index name in finding line")
	}
	if !strings.Contains(out, "Summary: 3 findings") {
		t.Error("expected summary line in output")
	}
	if !strings.Contains(out, "Top types:") {
		t.Error("expected top types line in output")
	}
}

func TestWriteText_Empty(t *testing.T) {
	r := NewReport("audit", nil, "test")
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatText); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No findings.") {
		t.Errorf("expected 'No findings.' in output, got %q", buf.String())
	}
}

func TestWriteText_EmptyWithScanContext(t *testing.T) {
	r := NewReport("audit", nil, "test")
	r.Scanned = ScanContext{Tables: 42, Indexes: 15, Schemas: 2}
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatText); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "No issues detected") {
		t.Errorf("expected 'No issues detected' in output, got %q", out)
	}
	if !strings.Contains(out, "42 tables") {
		t.Errorf("expected '42 tables' in output, got %q", out)
	}
	if !strings.Contains(out, "15 indexes") {
		t.Errorf("expected '15 indexes' in output, got %q", out)
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
	r := NewReport("audit", findings, "test")
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatText); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// Details are indented 4 spaces under grouped output
	if !strings.Contains(out, "    idx_scan: 0\n") {
		t.Errorf("expected detail line 'idx_scan: 0', got:\n%s", out)
	}
	if !strings.Contains(out, "    size: 2.0 MB\n") {
		t.Errorf("expected detail line 'size: 2.0 MB', got:\n%s", out)
	}
}

func TestWriteText_GroupsByTable(t *testing.T) {
	findings := []analyzer.Finding{
		{Type: analyzer.FindingUnusedIndex, Severity: analyzer.SeverityMedium, Schema: "public", Table: "users", Index: "idx_a", Message: "unused"},
		{Type: analyzer.FindingNoPrimaryKey, Severity: analyzer.SeverityMedium, Schema: "public", Table: "logs", Message: "no PK"},
		{Type: analyzer.FindingUnusedIndex, Severity: analyzer.SeverityMedium, Schema: "public", Table: "users", Index: "idx_b", Message: "unused"},
	}
	r := NewReport("audit", findings, "test")
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatText); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// "public.users" should appear exactly once as a header
	count := strings.Count(out, "public.users\n")
	if count != 1 {
		t.Errorf("expected public.users header once, got %d times in:\n%s", count, out)
	}
}

func TestWriteText_TOC(t *testing.T) {
	// Create >20 findings to trigger TOC
	var findings []analyzer.Finding
	for i := 0; i < 25; i++ {
		findings = append(findings, analyzer.Finding{
			Type: analyzer.FindingUnusedTable, Severity: analyzer.SeverityLow,
			Schema: "public", Table: "t" + strings.Repeat("x", i),
			Message: "unused",
		})
	}
	r := NewReport("audit", findings, "test")
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatText); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Tables with findings:") {
		t.Error("expected TOC for >20 findings")
	}
}

func TestWriteText_NoTOC(t *testing.T) {
	r := NewReport("audit", testFindings, "test") // 3 findings
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatText); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "Tables with findings:") {
		t.Error("did not expect TOC for <=20 findings")
	}
}

func TestWriteText_NoColor(t *testing.T) {
	r := NewReport("audit", testFindings, "test")
	var buf bytes.Buffer
	// bytes.Buffer is not a TTY, so color is auto-disabled
	if err := Write(&buf, &r, FormatText); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "\033[") {
		t.Error("expected no ANSI escape codes when writing to buffer")
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
	r := NewReport("audit", findings, "test")
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
	r := NewReport("audit", testFindings, "test")
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

func TestGroupByTable(t *testing.T) {
	findings := []analyzer.Finding{
		{Schema: "public", Table: "users"},
		{Schema: "public", Table: "logs"},
		{Schema: "public", Table: "users"},
		{Schema: "app", Table: "orders"},
	}
	groups := groupByTable(findings)
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}
	if groups[0].key != "public.users" || len(groups[0].findings) != 2 {
		t.Errorf("group[0] = %v", groups[0])
	}
	if groups[1].key != "public.logs" || len(groups[1].findings) != 1 {
		t.Errorf("group[1] = %v", groups[1])
	}
}
