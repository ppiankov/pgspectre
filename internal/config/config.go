package config

import (
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v3"
)

// Config holds all pgspectre configuration.
type Config struct {
	DBURL      string     `yaml:"db_url"`
	Thresholds Thresholds `yaml:"thresholds"`
	Exclude    Exclude    `yaml:"exclude"`
	Defaults   Defaults   `yaml:"defaults"`
}

// Thresholds control detection sensitivity.
type Thresholds struct {
	VacuumDays    int   `yaml:"vacuum_days"`     // days since last vacuum to flag
	BloatMinBytes int64 `yaml:"bloat_min_bytes"` // minimum index size to flag as bloated
}

// Exclude lists tables, schemas, and finding types to skip during analysis.
type Exclude struct {
	Tables   []string `yaml:"tables"`
	Schemas  []string `yaml:"schemas"`
	Findings []string `yaml:"findings"`
}

// Defaults holds default CLI flag values.
type Defaults struct {
	Format  string `yaml:"format"`
	Timeout string `yaml:"timeout"` // parsed as time.Duration
}

// DefaultConfig returns the built-in defaults.
func DefaultConfig() Config {
	return Config{
		Thresholds: Thresholds{
			VacuumDays:    30,
			BloatMinBytes: 1024 * 1024, // 1 MB
		},
		Defaults: Defaults{
			Format:  "text",
			Timeout: "30s",
		},
	}
}

// Load reads configuration from .pgspectre.yml in the given directory,
// falling back to ~/.pgspectre.yml. Returns DefaultConfig if no file found.
func Load(dir string) (Config, error) {
	cfg := DefaultConfig()

	paths := []string{filepath.Join(dir, ".pgspectre.yml")}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".pgspectre.yml"))
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, err
		}
		return cfg, nil
	}

	return cfg, nil
}

// TimeoutDuration parses the Defaults.Timeout string as a time.Duration.
// Returns 30s if parsing fails.
func (c *Config) TimeoutDuration() time.Duration {
	if c.Defaults.Timeout == "" {
		return 30 * time.Second
	}
	d, err := time.ParseDuration(c.Defaults.Timeout)
	if err != nil {
		return 30 * time.Second
	}
	return d
}
