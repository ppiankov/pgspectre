package reporter

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/ppiankov/pgspectre/internal/analyzer"
)

// SARIF 2.1.0 types — minimal subset for valid output.

type sarifLog struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string            `json:"id"`
	ShortDescription sarifMessage      `json:"shortDescription"`
	DefaultConfig    sarifRuleDefaults `json:"defaultConfiguration"`
}

type sarifRuleDefaults struct {
	Level string `json:"level"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations,omitempty"`
}

type sarifLocation struct {
	LogicalLocations []sarifLogicalLocation `json:"logicalLocations,omitempty"`
}

type sarifLogicalLocation struct {
	Name               string `json:"name"`
	FullyQualifiedName string `json:"fullyQualifiedName"`
	Kind               string `json:"kind"`
}

var ruleDescriptions = map[analyzer.FindingType]string{
	analyzer.FindingMissingTable:      "Table referenced in code does not exist in database",
	analyzer.FindingMissingColumn:     "Column referenced in code does not exist in table",
	analyzer.FindingUnusedTable:       "Table has no read activity (seq_scan=0, idx_scan=0)",
	analyzer.FindingUnreferencedTable: "Table exists in database but not referenced in code",
	analyzer.FindingUnusedIndex:       "Index has never been used for scans",
	analyzer.FindingBloatedIndex:      "Index size exceeds table size",
	analyzer.FindingMissingVacuum:     "Table has not been vacuumed recently",
	analyzer.FindingNoPrimaryKey:      "Table has no primary key constraint",
	analyzer.FindingDuplicateIndex:    "Multiple indexes with same definition on same table",
	analyzer.FindingCodeMatch:         "Table reference in code matches database table",
	analyzer.FindingOK:                "No issues detected",
}

var severityToLevel = map[analyzer.Severity]string{
	analyzer.SeverityHigh:   "error",
	analyzer.SeverityMedium: "warning",
	analyzer.SeverityLow:    "note",
	analyzer.SeverityInfo:   "note",
}

func writeSARIF(w io.Writer, report *Report) error {
	// Collect unique rule IDs
	ruleSet := make(map[analyzer.FindingType]bool)
	for _, f := range report.Findings {
		ruleSet[f.Type] = true
	}

	rules := make([]sarifRule, 0)
	for ft := range ruleSet {
		desc := ruleDescriptions[ft]
		if desc == "" {
			desc = string(ft)
		}
		rules = append(rules, sarifRule{
			ID:               "pgspectre/" + string(ft),
			ShortDescription: sarifMessage{Text: desc},
			DefaultConfig:    sarifRuleDefaults{Level: "warning"},
		})
	}

	var results []sarifResult
	for _, f := range report.Findings {
		level := severityToLevel[f.Severity]
		if level == "" {
			level = "note"
		}

		fqn := f.Schema + "." + f.Table
		if f.Column != "" {
			fqn += "." + f.Column
		} else if f.Index != "" {
			fqn += "." + f.Index
		}

		msgText := f.Message
		if len(f.Detail) > 0 {
			keys := make([]string, 0, len(f.Detail))
			for k := range f.Detail {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				msgText += fmt.Sprintf(" [%s=%s]", k, f.Detail[k])
			}
		}

		r := sarifResult{
			RuleID:  "pgspectre/" + string(f.Type),
			Level:   level,
			Message: sarifMessage{Text: msgText},
			Locations: []sarifLocation{
				{
					LogicalLocations: []sarifLogicalLocation{
						{
							Name:               f.Table,
							FullyQualifiedName: fqn,
							Kind:               "database/table",
						},
					},
				},
			},
		}
		results = append(results, r)
	}

	log := sarifLog{
		Version: "2.1.0",
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json",
		Runs: []sarifRun{
			{
				Tool: sarifTool{
					Driver: sarifDriver{
						Name:           "pgspectre",
						Version:        report.Metadata.Version,
						InformationURI: "https://github.com/ppiankov/pgspectre",
						Rules:          rules,
					},
				},
				Results: results,
			},
		},
	}

	if log.Runs[0].Results == nil {
		log.Runs[0].Results = []sarifResult{}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(log); err != nil {
		return fmt.Errorf("encode SARIF: %w", err)
	}
	return nil
}
