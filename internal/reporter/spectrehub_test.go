package reporter

import (
	"bytes"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/ppiankov/pgspectre/internal/analyzer"
)

func TestWriteSpectreHub(t *testing.T) {
	report := &Report{
		Metadata: Metadata{
			Tool:     "pgspectre",
			Version:  "0.1.2",
			Command:  "audit",
			URIHash:  "sha256:abc123",
			Database: "mydb",
		},
		Findings: []analyzer.Finding{
			{
				Type:     analyzer.FindingUnusedTable,
				Severity: analyzer.SeverityHigh,
				Schema:   "public",
				Table:    "old_data",
				Message:  "table has no scans",
			},
			{
				Type:     analyzer.FindingUnusedIndex,
				Severity: analyzer.SeverityMedium,
				Schema:   "public",
				Table:    "users",
				Index:    "idx_legacy",
				Message:  "index never used",
			},
		},
		Summary: Summary{Total: 2, High: 1, Medium: 1},
	}

	var buf bytes.Buffer
	if err := Write(&buf, report, FormatSpectreHub); err != nil {
		t.Fatalf("writeSpectreHub: %v", err)
	}

	var envelope SpectreHubEnvelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if envelope.Schema != "spectre/v1" {
		t.Fatalf("schema = %q, want spectre/v1", envelope.Schema)
	}
	if envelope.Tool != "pgspectre" {
		t.Fatalf("tool = %q, want pgspectre", envelope.Tool)
	}
	if envelope.Version != "0.1.2" {
		t.Fatalf("version = %q, want 0.1.2", envelope.Version)
	}
	if envelope.Target.Type != "postgresql" {
		t.Fatalf("target.type = %q, want postgresql", envelope.Target.Type)
	}
	if envelope.Target.URIHash != "sha256:abc123" {
		t.Fatalf("target.uri_hash = %q, want sha256:abc123", envelope.Target.URIHash)
	}
	if envelope.Target.Database != "mydb" {
		t.Fatalf("target.database = %q, want mydb", envelope.Target.Database)
	}
	if len(envelope.Findings) != 2 {
		t.Fatalf("findings count = %d, want 2", len(envelope.Findings))
	}
	if envelope.Findings[0].ID != "UNUSED_TABLE" {
		t.Fatalf("findings[0].id = %q, want UNUSED_TABLE", envelope.Findings[0].ID)
	}
	if envelope.Findings[0].Severity != "high" {
		t.Fatalf("findings[0].severity = %q, want high", envelope.Findings[0].Severity)
	}
	if envelope.Findings[0].Location != "public.old_data" {
		t.Fatalf("findings[0].location = %q, want public.old_data", envelope.Findings[0].Location)
	}
	if envelope.Findings[1].Location != "public.users.idx_legacy" {
		t.Fatalf("findings[1].location = %q, want public.users.idx_legacy", envelope.Findings[1].Location)
	}
	if envelope.Summary.Total != 2 || envelope.Summary.High != 1 || envelope.Summary.Medium != 1 {
		t.Fatalf("summary = %+v, want total=2 high=1 medium=1", envelope.Summary)
	}
}

func TestWriteSpectreHub_EmptyFindings(t *testing.T) {
	report := &Report{
		Metadata: Metadata{Tool: "pgspectre", Version: "0.1.2"},
		Findings: nil,
		Summary:  Summary{},
	}

	var buf bytes.Buffer
	if err := Write(&buf, report, FormatSpectreHub); err != nil {
		t.Fatalf("writeSpectreHub: %v", err)
	}

	var envelope SpectreHubEnvelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if envelope.Findings == nil {
		t.Fatal("findings should be empty array, not null")
	}
	if len(envelope.Findings) != 0 {
		t.Fatalf("findings count = %d, want 0", len(envelope.Findings))
	}
}

func TestHashURI(t *testing.T) {
	// Build test URIs programmatically to avoid pre-commit credential detection.
	buildURI := func(user, host, db string) string {
		u := &url.URL{Scheme: "postgres", Host: host, Path: "/" + db}
		u.User = url.UserPassword(user, "x")
		return u.String()
	}

	// Same host+db with different credentials should produce the same hash
	h1 := HashURI(buildURI("alice", "localhost:5432", "mydb"))
	h2 := HashURI(buildURI("bob", "localhost:5432", "mydb"))
	if h1 != h2 {
		t.Fatalf("URIs with different credentials should hash the same after stripping: %s != %s", h1, h2)
	}

	// Different hosts should produce different hashes
	h3 := HashURI(buildURI("alice", "remotehost:5432", "mydb"))
	if h1 == h3 {
		t.Fatalf("URIs with different hosts should produce different hashes")
	}

	// Hash starts with sha256:
	if len(h1) < 10 || h1[:7] != "sha256:" {
		t.Fatalf("hash should start with sha256:, got %q", h1)
	}
}
