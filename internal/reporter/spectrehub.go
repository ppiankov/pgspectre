package reporter

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"time"
)

// SpectreHubEnvelope is the spectre/v1 cross-tool ingestion format.
type SpectreHubEnvelope struct {
	Schema    string              `json:"schema"`
	Tool      string              `json:"tool"`
	Version   string              `json:"version"`
	Timestamp string              `json:"timestamp"`
	Target    SpectreHubTarget    `json:"target"`
	Findings  []SpectreHubFinding `json:"findings"`
	Summary   SpectreHubSummary   `json:"summary"`
}

// SpectreHubTarget describes the audited system.
type SpectreHubTarget struct {
	Type     string `json:"type"`
	URIHash  string `json:"uri_hash"`
	Database string `json:"database,omitempty"`
}

// SpectreHubFinding is a single finding in the spectre/v1 format.
type SpectreHubFinding struct {
	ID       string         `json:"id"`
	Severity string         `json:"severity"`
	Location string         `json:"location"`
	Message  string         `json:"message"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// SpectreHubSummary counts findings by severity.
type SpectreHubSummary struct {
	Total  int `json:"total"`
	High   int `json:"high"`
	Medium int `json:"medium"`
	Low    int `json:"low"`
	Info   int `json:"info"`
}

// HashURI produces a sha256 hash of the URI with credentials stripped.
func HashURI(rawURI string) string {
	u, err := url.Parse(rawURI)
	if err != nil {
		h := sha256.Sum256([]byte(rawURI))
		return fmt.Sprintf("sha256:%x", h)
	}
	u.User = nil
	safe := u.String()
	h := sha256.Sum256([]byte(safe))
	return fmt.Sprintf("sha256:%x", h)
}

func writeSpectreHub(w io.Writer, report *Report) error {
	envelope := SpectreHubEnvelope{
		Schema:    "spectre/v1",
		Tool:      "pgspectre",
		Version:   report.Metadata.Version,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Target: SpectreHubTarget{
			Type:     "postgresql",
			URIHash:  report.Metadata.URIHash,
			Database: report.Metadata.Database,
		},
		Summary: SpectreHubSummary{
			Total:  report.Summary.Total,
			High:   report.Summary.High,
			Medium: report.Summary.Medium,
			Low:    report.Summary.Low,
			Info:   report.Summary.Info,
		},
	}

	for _, f := range report.Findings {
		loc := f.Schema + "." + f.Table
		if f.Index != "" {
			loc += "." + f.Index
		} else if f.Column != "" {
			loc += "." + f.Column
		}
		envelope.Findings = append(envelope.Findings, SpectreHubFinding{
			ID:       string(f.Type),
			Severity: string(f.Severity),
			Location: loc,
			Message:  f.Message,
		})
	}

	if envelope.Findings == nil {
		envelope.Findings = []SpectreHubFinding{}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(envelope)
}
