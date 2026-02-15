package suppress

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ppiankov/pgspectre/internal/analyzer"
	"go.yaml.in/yaml/v3"
)

// Suppression is a single rule in the ignore file.
type Suppression struct {
	Table  string `yaml:"table"`
	Type   string `yaml:"type,omitempty"`
	Reason string `yaml:"reason,omitempty"`
}

// IgnoreFile is the structure of .pgspectre-ignore.yml.
type IgnoreFile struct {
	Suppressions []Suppression `yaml:"suppressions"`
}

// Rules holds loaded suppression rules from all sources.
type Rules struct {
	ignoreFile IgnoreFile
	// Tables from config exclude.findings
	configFindings []string
}

// LoadRules loads suppression rules from .pgspectre-ignore.yml in the given directory.
func LoadRules(dir string) (*Rules, error) {
	r := &Rules{}

	path := filepath.Join(dir, ".pgspectre-ignore.yml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return r, nil
	}
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(data, &r.ignoreFile); err != nil {
		return nil, err
	}
	return r, nil
}

// WithConfigFindings adds finding-type suppressions from config.
func (r *Rules) WithConfigFindings(findings []string) {
	r.configFindings = findings
}

// IsSuppressed returns true if the finding should be suppressed.
func (r *Rules) IsSuppressed(f *analyzer.Finding) bool {
	// Check config-level finding type suppressions
	for _, ft := range r.configFindings {
		if strings.EqualFold(string(f.Type), ft) {
			return true
		}
	}

	// Check ignore file suppressions
	for _, s := range r.ignoreFile.Suppressions {
		if matchTable(s.Table, f.Table) {
			if s.Type == "" || strings.EqualFold(s.Type, string(f.Type)) {
				return true
			}
		}
	}

	return false
}

// Filter removes suppressed findings and returns the remaining ones.
// Returns the filtered list and the number of suppressed findings.
func (r *Rules) Filter(findings []analyzer.Finding) ([]analyzer.Finding, int) {
	if len(r.ignoreFile.Suppressions) == 0 && len(r.configFindings) == 0 {
		return findings, 0
	}

	var filtered []analyzer.Finding
	suppressed := 0
	for i := range findings {
		if r.IsSuppressed(&findings[i]) {
			suppressed++
		} else {
			filtered = append(filtered, findings[i])
		}
	}
	return filtered, suppressed
}

// matchTable matches a table name against a pattern that supports trailing wildcards.
func matchTable(pattern, table string) bool {
	pattern = strings.ToLower(pattern)
	table = strings.ToLower(table)

	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(table, prefix)
	}
	return pattern == table
}

// HasInlineIgnore returns true if the line contains a pgspectre:ignore comment.
func HasInlineIgnore(line string) bool {
	return strings.Contains(line, "pgspectre:ignore")
}
