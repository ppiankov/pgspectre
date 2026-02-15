package reporter

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/ppiankov/pgspectre/internal/analyzer"
)

// Format controls report output format.
type Format string

const (
	FormatText  Format = "text"
	FormatJSON  Format = "json"
	FormatSARIF Format = "sarif"
)

// Metadata holds report context.
type Metadata struct {
	Tool      string `json:"tool"`
	Command   string `json:"command"`
	Timestamp string `json:"timestamp"`
}

// Summary counts findings by severity.
type Summary struct {
	Total  int `json:"total"`
	High   int `json:"high"`
	Medium int `json:"medium"`
	Low    int `json:"low"`
	Info   int `json:"info"`
}

// Report is the top-level audit/check output.
type Report struct {
	Metadata    Metadata           `json:"metadata"`
	Findings    []analyzer.Finding `json:"findings"`
	MaxSeverity analyzer.Severity  `json:"maxSeverity"`
	Summary     Summary            `json:"summary"`
}

// NewReport builds a report from findings.
func NewReport(command string, findings []analyzer.Finding) Report {
	var summary Summary
	for _, f := range findings {
		summary.Total++
		switch f.Severity {
		case analyzer.SeverityHigh:
			summary.High++
		case analyzer.SeverityMedium:
			summary.Medium++
		case analyzer.SeverityLow:
			summary.Low++
		case analyzer.SeverityInfo:
			summary.Info++
		}
	}

	if findings == nil {
		findings = []analyzer.Finding{}
	}

	return Report{
		Metadata: Metadata{
			Tool:      "pgspectre",
			Command:   command,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		Findings:    findings,
		MaxSeverity: analyzer.MaxSeverity(findings),
		Summary:     summary,
	}
}

// Write outputs the report in the given format.
func Write(w io.Writer, report *Report, format Format) error {
	switch format {
	case FormatJSON:
		return writeJSON(w, report)
	case FormatSARIF:
		return writeSARIF(w, report)
	default:
		return writeText(w, report)
	}
}

func writeJSON(w io.Writer, report *Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

var severityLabel = map[analyzer.Severity]string{
	analyzer.SeverityHigh:   "HIGH",
	analyzer.SeverityMedium: "MEDIUM",
	analyzer.SeverityLow:    "LOW",
	analyzer.SeverityInfo:   "INFO",
}

func writeText(w io.Writer, report *Report) error {
	if report.Summary.Total == 0 {
		_, err := fmt.Fprintln(w, "No findings.")
		return err
	}

	for _, f := range report.Findings {
		location := f.Schema + "." + f.Table
		if f.Index != "" {
			location += "." + f.Index
		}
		_, err := fmt.Fprintf(w, "[%s] %s: %s (%s)\n",
			severityLabel[f.Severity], f.Type, f.Message, location)
		if err != nil {
			return err
		}
		if len(f.Detail) > 0 {
			keys := make([]string, 0, len(f.Detail))
			for k := range f.Detail {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if _, err := fmt.Fprintf(w, "  %s: %s\n", k, f.Detail[k]); err != nil {
					return err
				}
			}
		}
	}

	_, err := fmt.Fprintf(w, "\nSummary: %d findings (high=%d medium=%d low=%d info=%d)\n",
		report.Summary.Total, report.Summary.High, report.Summary.Medium, report.Summary.Low, report.Summary.Info)
	return err
}
