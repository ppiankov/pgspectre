package cli

import (
	"testing"

	"github.com/ppiankov/pgspectre/internal/analyzer"
)

var testFindings = []analyzer.Finding{
	{Type: analyzer.FindingUnusedTable, Severity: analyzer.SeverityLow, Table: "t1"},
	{Type: analyzer.FindingUnusedIndex, Severity: analyzer.SeverityMedium, Table: "t2"},
	{Type: analyzer.FindingMissingTable, Severity: analyzer.SeverityHigh, Table: "t3"},
	{Type: analyzer.FindingCodeMatch, Severity: analyzer.SeverityInfo, Table: "t4"},
	{Type: analyzer.FindingMissingVacuum, Severity: analyzer.SeverityMedium, Table: "t5"},
}

func TestFilterBySeverity_High(t *testing.T) {
	result := filterBySeverity(testFindings, "high")
	if len(result) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result))
	}
	if result[0].Type != analyzer.FindingMissingTable {
		t.Errorf("expected MISSING_TABLE, got %s", result[0].Type)
	}
}

func TestFilterBySeverity_Medium(t *testing.T) {
	result := filterBySeverity(testFindings, "medium")
	if len(result) != 3 {
		t.Fatalf("expected 3 findings (2 medium + 1 high), got %d", len(result))
	}
}

func TestFilterBySeverity_Low(t *testing.T) {
	result := filterBySeverity(testFindings, "low")
	if len(result) != 4 {
		t.Fatalf("expected 4 findings (low+medium+high), got %d", len(result))
	}
}

func TestFilterBySeverity_Info(t *testing.T) {
	result := filterBySeverity(testFindings, "info")
	if len(result) != 5 {
		t.Fatalf("expected 5 findings (all), got %d", len(result))
	}
}

func TestFilterBySeverity_Unknown(t *testing.T) {
	result := filterBySeverity(testFindings, "unknown")
	if len(result) != 5 {
		t.Fatalf("unknown severity should return all, got %d", len(result))
	}
}

func TestFilterBySeverity_CaseInsensitive(t *testing.T) {
	result := filterBySeverity(testFindings, "HIGH")
	if len(result) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result))
	}
}

func TestFilterByType_Single(t *testing.T) {
	result := filterByType(testFindings, "UNUSED_TABLE")
	if len(result) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result))
	}
	if result[0].Type != analyzer.FindingUnusedTable {
		t.Errorf("expected UNUSED_TABLE, got %s", result[0].Type)
	}
}

func TestFilterByType_Multiple(t *testing.T) {
	result := filterByType(testFindings, "UNUSED_TABLE,MISSING_TABLE")
	if len(result) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(result))
	}
}

func TestFilterByType_CaseInsensitive(t *testing.T) {
	result := filterByType(testFindings, "unused_index")
	if len(result) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result))
	}
}

func TestFilterByType_Empty(t *testing.T) {
	result := filterByType(testFindings, "")
	if len(result) != 5 {
		t.Fatalf("empty filter should return all, got %d", len(result))
	}
}

func TestFilterByType_NoMatch(t *testing.T) {
	result := filterByType(testFindings, "NONEXISTENT_TYPE")
	if len(result) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(result))
	}
}

func TestApplyReportFilters_Both(t *testing.T) {
	// Filter by medium+ severity AND UNUSED_INDEX type
	result := applyReportFilters(testFindings, "medium", "UNUSED_INDEX")
	if len(result) != 1 {
		t.Fatalf("expected 1 finding (medium UNUSED_INDEX), got %d", len(result))
	}
	if result[0].Type != analyzer.FindingUnusedIndex {
		t.Errorf("expected UNUSED_INDEX, got %s", result[0].Type)
	}
}

func TestApplyReportFilters_None(t *testing.T) {
	result := applyReportFilters(testFindings, "", "")
	if len(result) != 5 {
		t.Fatalf("no filters should return all, got %d", len(result))
	}
}
