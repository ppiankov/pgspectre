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
	FormatText       Format = "text"
	FormatJSON       Format = "json"
	FormatSARIF      Format = "sarif"
	FormatSpectreHub Format = "spectrehub"
)

// Metadata holds report context.
type Metadata struct {
	Tool      string `json:"tool"`
	Version   string `json:"version"`
	Command   string `json:"command"`
	Timestamp string `json:"timestamp"`
	URIHash   string `json:"uri_hash,omitempty"`
	Database  string `json:"database,omitempty"`
}

// Summary counts findings by severity.
type Summary struct {
	Total  int `json:"total"`
	High   int `json:"high"`
	Medium int `json:"medium"`
	Low    int `json:"low"`
	Info   int `json:"info"`
}

// ScanContext holds context about what was scanned.
type ScanContext struct {
	Tables  int `json:"tables"`
	Indexes int `json:"indexes"`
	Schemas int `json:"schemas"`
}

// Report is the top-level audit/check output.
type Report struct {
	Metadata    Metadata           `json:"metadata"`
	Findings    []analyzer.Finding `json:"findings"`
	MaxSeverity analyzer.Severity  `json:"maxSeverity"`
	Summary     Summary            `json:"summary"`
	Scanned     ScanContext        `json:"scanned,omitempty"`
}

// NewReport builds a report from findings.
func NewReport(command string, findings []analyzer.Finding, version string) Report {
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
			Version:   version,
			Command:   command,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		Findings:    findings,
		MaxSeverity: analyzer.MaxSeverity(findings),
		Summary:     summary,
	}
}

// WriteOptions controls text output behavior.
type WriteOptions struct {
	NoColor bool
}

// Write outputs the report in the given format.
func Write(w io.Writer, report *Report, format Format, opts ...WriteOptions) error {
	switch format {
	case FormatJSON:
		return writeJSON(w, report)
	case FormatSARIF:
		return writeSARIF(w, report)
	case FormatSpectreHub:
		return writeSpectreHub(w, report)
	default:
		var opt WriteOptions
		if len(opts) > 0 {
			opt = opts[0]
		}
		useColor := !opt.NoColor && isTTY(w)
		return writeText(w, report, useColor)
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

// tableGroup holds findings grouped by schema.table.
type tableGroup struct {
	key      string
	findings []analyzer.Finding
}

func writeText(w io.Writer, report *Report, useColor bool) error {
	if report.Summary.Total == 0 {
		if report.Scanned.Tables > 0 {
			_, err := fmt.Fprintf(w, "No issues detected. %d tables, %d indexes scanned.\n",
				report.Scanned.Tables, report.Scanned.Indexes)
			return err
		}
		_, err := fmt.Fprintln(w, "No findings.")
		return err
	}

	groups := groupByTable(report.Findings)

	// Table of contents for large reports
	if report.Summary.Total > 20 {
		if _, err := fmt.Fprintln(w, "Tables with findings:"); err != nil {
			return err
		}
		for _, g := range groups {
			if _, err := fmt.Fprintf(w, "  %s (%d)\n", g.key, len(g.findings)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}

	// Grouped findings
	for i, g := range groups {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		header := g.key
		if useColor {
			header = colorBold + header + colorReset
		}
		if _, err := fmt.Fprintln(w, header); err != nil {
			return err
		}

		for _, f := range g.findings {
			label := severityLabel[f.Severity]
			if useColor {
				c := severityColor[f.Severity]
				label = c + label + colorReset
			}

			target := string(f.Type)
			if f.Index != "" {
				target += " (" + f.Index + ")"
			}

			if _, err := fmt.Fprintf(w, "  [%s] %s: %s\n", label, target, f.Message); err != nil {
				return err
			}

			if len(f.Detail) > 0 {
				keys := make([]string, 0, len(f.Detail))
				for k := range f.Detail {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					if _, err := fmt.Fprintf(w, "    %s: %s\n", k, f.Detail[k]); err != nil {
						return err
					}
				}
			}
		}
	}

	// Summary
	if _, err := fmt.Fprintf(w, "\nSummary: %d findings (high=%d medium=%d low=%d info=%d)\n",
		report.Summary.Total, report.Summary.High, report.Summary.Medium, report.Summary.Low, report.Summary.Info); err != nil {
		return err
	}

	// Top finding types
	typeCounts := make(map[analyzer.FindingType]int)
	for _, f := range report.Findings {
		typeCounts[f.Type]++
	}
	type typeCount struct {
		ft    analyzer.FindingType
		count int
	}
	var sorted []typeCount
	for ft, n := range typeCounts {
		sorted = append(sorted, typeCount{ft, n})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })
	limit := 3
	if len(sorted) < limit {
		limit = len(sorted)
	}
	if _, err := fmt.Fprint(w, "Top types: "); err != nil {
		return err
	}
	for i := 0; i < limit; i++ {
		sep := ", "
		if i == limit-1 {
			sep = ""
		}
		if _, err := fmt.Fprintf(w, "%s (%d)%s", sorted[i].ft, sorted[i].count, sep); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

// groupByTable groups findings by schema.table, preserving encounter order.
func groupByTable(findings []analyzer.Finding) []tableGroup {
	order := make(map[string]int)
	var groups []tableGroup

	for _, f := range findings {
		key := f.Schema + "." + f.Table
		idx, exists := order[key]
		if !exists {
			idx = len(groups)
			order[key] = idx
			groups = append(groups, tableGroup{key: key})
		}
		groups[idx].findings = append(groups[idx].findings, f)
	}

	return groups
}
