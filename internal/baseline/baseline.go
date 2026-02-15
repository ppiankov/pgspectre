package baseline

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/ppiankov/pgspectre/internal/analyzer"
)

// Baseline holds fingerprints of previously seen findings.
type Baseline struct {
	Fingerprints []string `json:"fingerprints"`
	set          map[string]bool
}

// Load reads a baseline file. Returns an empty baseline if the file does not exist.
func Load(path string) (*Baseline, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Baseline{set: make(map[string]bool)}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read baseline: %w", err)
	}

	var b Baseline
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parse baseline: %w", err)
	}
	b.set = make(map[string]bool, len(b.Fingerprints))
	for _, fp := range b.Fingerprints {
		b.set[fp] = true
	}
	return &b, nil
}

// Save writes the baseline to a file.
func Save(path string, findings []analyzer.Finding) error {
	fps := make([]string, 0, len(findings))
	seen := make(map[string]bool)
	for i := range findings {
		fp := Fingerprint(&findings[i])
		if !seen[fp] {
			fps = append(fps, fp)
			seen[fp] = true
		}
	}
	sort.Strings(fps)

	b := Baseline{Fingerprints: fps}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal baseline: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// Contains returns true if the finding's fingerprint is in the baseline.
func (b *Baseline) Contains(f *analyzer.Finding) bool {
	return b.set[Fingerprint(f)]
}

// Filter removes baselined findings and returns the remaining ones.
// Returns the filtered list and the number of suppressed findings.
func (b *Baseline) Filter(findings []analyzer.Finding) ([]analyzer.Finding, int) {
	if len(b.set) == 0 {
		return findings, 0
	}

	var filtered []analyzer.Finding
	suppressed := 0
	for i := range findings {
		if b.Contains(&findings[i]) {
			suppressed++
		} else {
			filtered = append(filtered, findings[i])
		}
	}
	return filtered, suppressed
}

// Fingerprint computes a stable identifier for a finding.
func Fingerprint(f *analyzer.Finding) string {
	key := fmt.Sprintf("%s|%s|%s|%s|%s", f.Type, f.Schema, f.Table, f.Column, f.Index)
	h := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", h[:16])
}
