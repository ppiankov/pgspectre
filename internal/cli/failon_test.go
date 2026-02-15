package cli

import (
	"testing"

	"github.com/ppiankov/pgspectre/internal/analyzer"
)

func TestShouldFailOn_ByType(t *testing.T) {
	findings := []analyzer.Finding{
		{Type: analyzer.FindingMissingTable, Severity: analyzer.SeverityHigh},
		{Type: analyzer.FindingUnusedIndex, Severity: analyzer.SeverityLow},
	}

	if !shouldFailOn(findings, "MISSING_TABLE") {
		t.Error("should fail on MISSING_TABLE")
	}
	if shouldFailOn(findings, "MISSING_COLUMN") {
		t.Error("should not fail on MISSING_COLUMN")
	}
}

func TestShouldFailOn_BySeverity(t *testing.T) {
	findings := []analyzer.Finding{
		{Type: analyzer.FindingMissingTable, Severity: analyzer.SeverityHigh},
		{Type: analyzer.FindingUnusedIndex, Severity: analyzer.SeverityLow},
	}

	if !shouldFailOn(findings, "high") {
		t.Error("should fail on high severity")
	}
	if !shouldFailOn(findings, "low") {
		t.Error("should fail on low severity")
	}
	if shouldFailOn(findings, "medium") {
		t.Error("should not fail on medium severity")
	}
}

func TestShouldFailOn_CommaSeparated(t *testing.T) {
	findings := []analyzer.Finding{
		{Type: analyzer.FindingMissingColumn, Severity: analyzer.SeverityMedium},
	}

	if !shouldFailOn(findings, "MISSING_TABLE,MISSING_COLUMN") {
		t.Error("should fail on MISSING_COLUMN in comma list")
	}
}

func TestShouldFailOn_MixedTypesAndSeverity(t *testing.T) {
	findings := []analyzer.Finding{
		{Type: analyzer.FindingUnusedIndex, Severity: analyzer.SeverityLow},
	}

	if !shouldFailOn(findings, "MISSING_TABLE,low") {
		t.Error("should fail on low severity in mixed list")
	}
}

func TestShouldFailOn_Empty(t *testing.T) {
	findings := []analyzer.Finding{
		{Type: analyzer.FindingMissingTable, Severity: analyzer.SeverityHigh},
	}

	if shouldFailOn(findings, "") {
		t.Error("should not fail on empty string")
	}
}

func TestShouldFailOn_CaseInsensitive(t *testing.T) {
	findings := []analyzer.Finding{
		{Type: analyzer.FindingMissingTable, Severity: analyzer.SeverityHigh},
	}

	if !shouldFailOn(findings, "missing_table") {
		t.Error("should match case-insensitively")
	}
}

func TestShouldFailOn_NoFindings(t *testing.T) {
	if shouldFailOn(nil, "MISSING_TABLE") {
		t.Error("should not fail with no findings")
	}
}
