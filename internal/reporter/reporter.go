package reporter

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
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
	analyzer.SeverityMedium: "MED",
	analyzer.SeverityLow:    "LOW",
	analyzer.SeverityInfo:   "INFO",
}

const (
	largeReportThreshold = 20
	topTypesLimit        = 3
	severityPrefixWidth  = len("[HIGH]")
	unknownGroupLabel    = "<unknown>"
)

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

	if report.Summary.Total > largeReportThreshold {
		if err := writeTableOfContents(w, groups); err != nil {
			return err
		}
	}

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

		if err := writeGroupFindings(w, g, useColor); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(w, "\nSummary"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Total findings: %d\n", report.Summary.Total); err != nil {
		return err
	}
	if err := writeSeveritySummary(w, report.Summary, useColor); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "  Top types:"); err != nil {
		return err
	}
	for _, entry := range topFindingTypes(report.Findings) {
		if _, err := fmt.Fprintf(w, "    %-18s %d\n", entry.ft, entry.count); err != nil {
			return err
		}
	}
	return nil
}

func groupByTable(findings []analyzer.Finding) []tableGroup {
	order := make(map[string]int)
	var groups []tableGroup

	for _, f := range findings {
		key := tableGroupKey(&f)
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

func writeTableOfContents(w io.Writer, groups []tableGroup) error {
	if _, err := fmt.Fprintln(w, "Table of contents"); err != nil {
		return err
	}

	width := 0
	for _, g := range groups {
		if len(g.key) > width {
			width = len(g.key)
		}
	}

	for _, g := range groups {
		label := "findings"
		if len(g.findings) == 1 {
			label = "finding"
		}
		if _, err := fmt.Fprintf(w, "  %-*s  %2d %s\n", width, g.key, len(g.findings), label); err != nil {
			return err
		}
	}

	_, err := fmt.Fprintln(w)
	return err
}

func writeGroupFindings(w io.Writer, group tableGroup, useColor bool) error {
	typeWidth := 0
	targetWidth := 0
	for _, f := range group.findings {
		if n := len(string(f.Type)); n > typeWidth {
			typeWidth = n
		}
		if n := len(findingTarget(&f)); n > targetWidth {
			targetWidth = n
		}
	}

	for _, f := range group.findings {
		if _, err := fmt.Fprintf(
			w,
			"  %s  %-*s",
			severityPrefix(f.Severity, useColor),
			typeWidth,
			f.Type,
		); err != nil {
			return err
		}

		if targetWidth > 0 {
			if _, err := fmt.Fprintf(w, "  %-*s", targetWidth, findingTarget(&f)); err != nil {
				return err
			}
		}

		if _, err := fmt.Fprintf(w, "  %s\n", f.Message); err != nil {
			return err
		}

		if err := writeDetailLines(w, f.Detail); err != nil {
			return err
		}
	}

	return nil
}

func writeDetailLines(w io.Writer, detail map[string]string) error {
	if len(detail) == 0 {
		return nil
	}

	keys := make([]string, 0, len(detail))
	width := 0
	for k := range detail {
		keys = append(keys, k)
		if len(k) > width {
			width = len(k)
		}
	}
	sort.Strings(keys)

	for _, k := range keys {
		if _, err := fmt.Fprintf(w, "    %-*s  %s\n", width+1, k+":", detail[k]); err != nil {
			return err
		}
	}

	return nil
}

func writeSeveritySummary(w io.Writer, summary Summary, useColor bool) error {
	if _, err := fmt.Fprintf(
		w,
		"  By severity: %s %d  %s %d  %s %d  %s %d\n",
		summarySeverityPrefix(analyzer.SeverityHigh, useColor),
		summary.High,
		summarySeverityPrefix(analyzer.SeverityMedium, useColor),
		summary.Medium,
		summarySeverityPrefix(analyzer.SeverityLow, useColor),
		summary.Low,
		summarySeverityPrefix(analyzer.SeverityInfo, useColor),
		summary.Info,
	); err != nil {
		return err
	}
	return nil
}

type findingTypeCount struct {
	ft    analyzer.FindingType
	count int
}

func topFindingTypes(findings []analyzer.Finding) []findingTypeCount {
	typeCounts := make(map[analyzer.FindingType]int)
	for _, f := range findings {
		typeCounts[f.Type]++
	}

	sorted := make([]findingTypeCount, 0, len(typeCounts))
	for ft, count := range typeCounts {
		sorted = append(sorted, findingTypeCount{ft: ft, count: count})
	}

	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].count == sorted[j].count {
			return sorted[i].ft < sorted[j].ft
		}
		return sorted[i].count > sorted[j].count
	})

	if len(sorted) > topTypesLimit {
		sorted = sorted[:topTypesLimit]
	}

	return sorted
}

func tableGroupKey(f *analyzer.Finding) string {
	if f.Schema == "" && f.Table == "" {
		return unknownGroupLabel
	}
	if f.Schema == "" {
		return unknownGroupLabel + "." + f.Table
	}
	if f.Table == "" {
		return f.Schema
	}
	return f.Schema + "." + f.Table
}

func findingTarget(f *analyzer.Finding) string {
	switch {
	case f.Index != "":
		return f.Index
	case f.Column != "":
		return f.Column
	default:
		return ""
	}
}

func severityPrefix(severity analyzer.Severity, useColor bool) string {
	raw := "[" + severityLabel[severity] + "]"
	padding := strings.Repeat(" ", severityPrefixWidth-len(raw))
	if !useColor {
		return raw + padding
	}
	return severityColor[severity] + raw + colorReset + padding
}

func summarySeverityPrefix(severity analyzer.Severity, useColor bool) string {
	raw := "[" + severityLabel[severity] + "]"
	if !useColor {
		return raw
	}
	return severityColor[severity] + raw + colorReset
}
