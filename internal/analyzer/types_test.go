package analyzer

import "testing"

func TestMaxSeverity(t *testing.T) {
	tests := []struct {
		name     string
		findings []Finding
		want     Severity
	}{
		{"empty", nil, SeverityInfo},
		{"single info", []Finding{{Severity: SeverityInfo}}, SeverityInfo},
		{"single high", []Finding{{Severity: SeverityHigh}}, SeverityHigh},
		{"mixed", []Finding{
			{Severity: SeverityLow},
			{Severity: SeverityHigh},
			{Severity: SeverityMedium},
		}, SeverityHigh},
		{"low and medium", []Finding{
			{Severity: SeverityLow},
			{Severity: SeverityMedium},
		}, SeverityMedium},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaxSeverity(tt.findings)
			if got != tt.want {
				t.Errorf("MaxSeverity() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		severity Severity
		want     int
	}{
		{SeverityHigh, 2},
		{SeverityMedium, 1},
		{SeverityLow, 0},
		{SeverityInfo, 0},
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			got := ExitCode(tt.severity)
			if got != tt.want {
				t.Errorf("ExitCode(%q) = %d, want %d", tt.severity, got, tt.want)
			}
		})
	}
}
