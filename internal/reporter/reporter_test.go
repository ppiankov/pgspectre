package reporter

import (
	"bytes"
	"encoding/json"
	"os"
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
	if !strings.Contains(out, "[MED]") {
		t.Error("expected [MED] label in output")
	}
	if !strings.Contains(out, "UNUSED_TABLE") {
		t.Error("expected UNUSED_TABLE in output")
	}
	if !strings.Contains(out, "public.old_data\n") {
		t.Error("expected table header public.old_data")
	}
	if !strings.Contains(out, "public.users\n") {
		t.Error("expected table header public.users")
	}
	if !strings.Contains(out, "idx_old") {
		t.Error("expected index name in finding line")
	}
	if !strings.Contains(out, "Summary\n") {
		t.Error("expected summary header in output")
	}
	if !strings.Contains(out, "Total findings: 3") {
		t.Error("expected total findings line in output")
	}
	if !strings.Contains(out, "By severity: [HIGH] 1  [MED] 1  [LOW] 1  [INFO] 0") {
		t.Error("expected severity summary in output")
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
	if !strings.Contains(out, "    idx_scan:  0\n") {
		t.Errorf("expected detail line 'idx_scan: 0', got:\n%s", out)
	}
	if !strings.Contains(out, "    size:      2.0 MB\n") {
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
	count := strings.Count(out, "public.users\n")
	if count != 1 {
		t.Errorf("expected public.users header once, got %d times in:\n%s", count, out)
	}
}

func TestWriteText_AlignsColumnsWithinGroup(t *testing.T) {
	findings := []analyzer.Finding{
		{
			Type:     analyzer.FindingUnusedIndex,
			Severity: analyzer.SeverityMedium,
			Schema:   "public",
			Table:    "users",
			Index:    "idx_short",
			Message:  "index never used",
		},
		{
			Type:     analyzer.FindingMissingColumn,
			Severity: analyzer.SeverityHigh,
			Schema:   "public",
			Table:    "users",
			Column:   "email_address",
			Message:  "column missing",
		},
		{
			Type:     analyzer.FindingNoPrimaryKey,
			Severity: analyzer.SeverityLow,
			Schema:   "public",
			Table:    "users",
			Message:  "table has no primary key",
		},
	}

	r := NewReport("audit", findings, "test")
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatText); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var findingLines []string
	for _, line := range lines {
		if strings.HasPrefix(line, "  [") {
			findingLines = append(findingLines, line)
		}
	}
	if len(findingLines) != 3 {
		t.Fatalf("expected 3 finding lines, got %d in:\n%s", len(findingLines), buf.String())
	}

	positions := []int{
		strings.Index(findingLines[0], "index never used"),
		strings.Index(findingLines[1], "column missing"),
		strings.Index(findingLines[2], "table has no primary key"),
	}
	for i, pos := range positions {
		if pos < 0 {
			t.Fatalf("message not found in line %d: %q", i, findingLines[i])
		}
	}
	if positions[0] != positions[1] || positions[1] != positions[2] {
		t.Fatalf("message columns are not aligned: %v\n%s", positions, buf.String())
	}
}

func TestWriteText_TOC(t *testing.T) {
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
	if !strings.Contains(buf.String(), "Table of contents") {
		t.Error("expected TOC for >20 findings")
	}
	if !strings.Contains(buf.String(), "1 finding") {
		t.Error("expected TOC entries to include finding counts")
	}
}

func TestWriteText_NoTOC(t *testing.T) {
	r := NewReport("audit", testFindings, "test")
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatText); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "Table of contents") {
		t.Error("did not expect TOC for <=20 findings")
	}
}

func TestWriteText_AutoDisablesColorForNonTTY(t *testing.T) {
	r := NewReport("audit", testFindings, "test")
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatText); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "\033[") {
		t.Error("expected no ANSI escape codes when writing to buffer")
	}
}

func TestWriteText_UsesColorWhenTTY(t *testing.T) {
	restore := stubTerminal(t, true)
	defer restore()

	file, err := os.CreateTemp(t.TempDir(), "report-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })

	r := NewReport("audit", testFindings, "test")
	if err := Write(file, &r, FormatText); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\033[") {
		t.Fatalf("expected ANSI escape codes in TTY output, got:\n%s", string(data))
	}
}

func TestWriteText_NoColorOptionDisablesANSI(t *testing.T) {
	restore := stubTerminal(t, true)
	defer restore()

	file, err := os.CreateTemp(t.TempDir(), "report-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })

	r := NewReport("audit", testFindings, "test")
	if err := Write(file, &r, FormatText, WriteOptions{NoColor: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "\033[") {
		t.Fatalf("expected no ANSI escape codes with --no-color, got:\n%s", string(data))
	}
}

func TestTopFindingTypes_LimitsAndSorts(t *testing.T) {
	findings := []analyzer.Finding{
		{Type: analyzer.FindingUnusedIndex},
		{Type: analyzer.FindingUnusedIndex},
		{Type: analyzer.FindingUnusedIndex},
		{Type: analyzer.FindingMissingVacuum},
		{Type: analyzer.FindingMissingVacuum},
		{Type: analyzer.FindingNoPrimaryKey},
		{Type: analyzer.FindingNoPrimaryKey},
		{Type: analyzer.FindingUnusedTable},
	}

	got := topFindingTypes(findings)
	if len(got) != 3 {
		t.Fatalf("expected 3 top types, got %d", len(got))
	}

	want := []findingTypeCount{
		{ft: analyzer.FindingUnusedIndex, count: 3},
		{ft: analyzer.FindingMissingVacuum, count: 2},
		{ft: analyzer.FindingNoPrimaryKey, count: 2},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("topFindingTypes[%d] = %+v, want %+v", i, got[i], want[i])
		}
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

func stubTerminal(t *testing.T, value bool) func() {
	t.Helper()

	previous := isTerminal
	isTerminal = func(int) bool {
		return value
	}

	return func() {
		isTerminal = previous
	}
}
