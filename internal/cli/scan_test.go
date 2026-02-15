package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ppiankov/pgspectre/internal/scanner"
)

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanCmd_Text(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "app.go", `package main
func main() { db.Query("SELECT * FROM users") }`)

	cmd := newRootCmd("test")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"scan", "--repo", dir, "--format", "text"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	output := out.String()
	if !strings.Contains(output, "users") {
		t.Errorf("expected 'users' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Tables (1)") {
		t.Errorf("expected 'Tables (1)' in output, got:\n%s", output)
	}
}

func TestScanCmd_JSON(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "query.sql", "SELECT name FROM accounts;")

	cmd := newRootCmd("test")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"scan", "--repo", dir, "--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result scanner.ScanResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if len(result.Tables) != 1 || result.Tables[0] != "accounts" {
		t.Errorf("expected [accounts], got %v", result.Tables)
	}
}

func TestScanCmd_NoRepo(t *testing.T) {
	cmd := newRootCmd("test")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"scan"})

	if err := cmd.Execute(); err == nil {
		t.Error("expected error when --repo not provided")
	}
}

func TestScanCmd_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	cmd := newRootCmd("test")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"scan", "--repo", dir})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "No table references found") {
		t.Errorf("expected 'No table references found', got:\n%s", out.String())
	}
}

func TestWriteScanResult_Text(t *testing.T) {
	result := &scanner.ScanResult{
		Tables:       []string{"orders", "users"},
		Columns:      []string{"orders.id", "users.name"},
		Refs:         []scanner.TableRef{{Table: "users", File: "app.go", Line: 5, Context: "SELECT", Pattern: "sql"}},
		FilesScanned: 3,
	}

	var buf bytes.Buffer
	if err := writeScanResult(&buf, result, "text"); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	for _, want := range []string{"Tables (2)", "orders", "users", "Columns (2)", "References (1)", "app.go:5"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output, got:\n%s", want, output)
		}
	}
}

func TestWriteScanResult_JSON(t *testing.T) {
	result := &scanner.ScanResult{
		Tables: []string{"users"},
		Refs:   []scanner.TableRef{{Table: "users"}},
	}

	var buf bytes.Buffer
	if err := writeScanResult(&buf, result, "json"); err != nil {
		t.Fatal(err)
	}

	var parsed scanner.ScanResult
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(parsed.Tables) != 1 {
		t.Errorf("expected 1 table, got %d", len(parsed.Tables))
	}
}
