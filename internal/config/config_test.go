package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Thresholds.VacuumDays != 30 {
		t.Errorf("VacuumDays = %d, want 30", cfg.Thresholds.VacuumDays)
	}
	if cfg.Thresholds.BloatMinBytes != 1024*1024 {
		t.Errorf("BloatMinBytes = %d, want %d", cfg.Thresholds.BloatMinBytes, 1024*1024)
	}
	if cfg.Defaults.Format != "text" {
		t.Errorf("Format = %q, want text", cfg.Defaults.Format)
	}
	if cfg.Defaults.Timeout != "30s" {
		t.Errorf("Timeout = %q, want 30s", cfg.Defaults.Timeout)
	}
}

func TestLoad_NoFile(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Should return defaults
	if cfg.Thresholds.VacuumDays != 30 {
		t.Errorf("expected default VacuumDays=30, got %d", cfg.Thresholds.VacuumDays)
	}
}

func TestLoad_FromDir(t *testing.T) {
	dir := t.TempDir()
	content := []byte(`
db_url: "postgres://localhost:5432/test"
thresholds:
  vacuum_days: 14
  bloat_min_bytes: 2097152
exclude:
  tables:
    - migrations
    - schema_versions
  schemas:
    - pg_catalog
defaults:
  format: json
  timeout: "60s"
`)
	if err := os.WriteFile(filepath.Join(dir, ".pgspectre.yml"), content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.DBURL != "postgres://localhost:5432/test" {
		t.Errorf("DBURL = %q", cfg.DBURL)
	}
	if cfg.Thresholds.VacuumDays != 14 {
		t.Errorf("VacuumDays = %d, want 14", cfg.Thresholds.VacuumDays)
	}
	if cfg.Thresholds.BloatMinBytes != 2097152 {
		t.Errorf("BloatMinBytes = %d, want 2097152", cfg.Thresholds.BloatMinBytes)
	}
	if len(cfg.Exclude.Tables) != 2 {
		t.Errorf("Exclude.Tables = %v, want 2 entries", cfg.Exclude.Tables)
	}
	if len(cfg.Exclude.Schemas) != 1 {
		t.Errorf("Exclude.Schemas = %v, want 1 entry", cfg.Exclude.Schemas)
	}
	if cfg.Defaults.Format != "json" {
		t.Errorf("Format = %q, want json", cfg.Defaults.Format)
	}
	if cfg.Defaults.Timeout != "60s" {
		t.Errorf("Timeout = %q, want 60s", cfg.Defaults.Timeout)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".pgspectre.yml"), []byte("{{invalid"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestTimeoutDuration(t *testing.T) {
	tests := []struct {
		name    string
		timeout string
		want    time.Duration
	}{
		{"valid 60s", "60s", 60 * time.Second},
		{"valid 2m", "2m", 2 * time.Minute},
		{"empty", "", 30 * time.Second},
		{"invalid", "notaduration", 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Defaults: Defaults{Timeout: tt.timeout}}
			got := cfg.TimeoutDuration()
			if got != tt.want {
				t.Errorf("TimeoutDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExists_Found(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".pgspectre.yml"), []byte("db_url: test"), 0644); err != nil {
		t.Fatal(err)
	}
	if !Exists(dir) {
		t.Error("Exists() = false, want true")
	}
}

func TestExists_NotFound(t *testing.T) {
	if Exists(t.TempDir()) {
		t.Error("Exists() = true, want false")
	}
}

func TestLoad_PartialOverride(t *testing.T) {
	dir := t.TempDir()
	// Only override vacuum_days — other fields should keep defaults
	content := []byte(`
thresholds:
  vacuum_days: 7
`)
	if err := os.WriteFile(filepath.Join(dir, ".pgspectre.yml"), content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Thresholds.VacuumDays != 7 {
		t.Errorf("VacuumDays = %d, want 7", cfg.Thresholds.VacuumDays)
	}
	// BloatMinBytes should remain default
	if cfg.Thresholds.BloatMinBytes != 1024*1024 {
		t.Errorf("BloatMinBytes = %d, want default %d", cfg.Thresholds.BloatMinBytes, 1024*1024)
	}
}
