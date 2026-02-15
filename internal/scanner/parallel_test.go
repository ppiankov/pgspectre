package scanner

import (
	"fmt"
	"sort"
	"testing"
)

func TestScanParallel_SameAsSequential(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "app.go", `package main
import "database/sql"
func main() {
	db.Query("SELECT * FROM users WHERE active = true")
	db.Exec("INSERT INTO orders (user_id) VALUES ($1)", uid)
}`)
	writeFile(t, dir, "models.py", `from sqlalchemy import Column
class User(Base):
    __tablename__ = 'users'
class Payment(Base):
    __tablename__ = 'payments'
`)
	writeFile(t, dir, "schema.sql", `CREATE TABLE sessions (id SERIAL PRIMARY KEY);`)

	seq, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	par, err := ScanParallel(dir, 4)
	if err != nil {
		t.Fatal(err)
	}

	// Same table count
	if len(seq.Tables) != len(par.Tables) {
		t.Fatalf("tables: seq=%d par=%d", len(seq.Tables), len(par.Tables))
	}

	// Same tables (both are sorted)
	for i := range seq.Tables {
		if seq.Tables[i] != par.Tables[i] {
			t.Errorf("tables[%d]: seq=%q par=%q", i, seq.Tables[i], par.Tables[i])
		}
	}

	// Same file counts
	if seq.FilesScanned != par.FilesScanned {
		t.Errorf("filesScanned: seq=%d par=%d", seq.FilesScanned, par.FilesScanned)
	}
	if seq.FilesSkipped != par.FilesSkipped {
		t.Errorf("filesSkipped: seq=%d par=%d", seq.FilesSkipped, par.FilesSkipped)
	}

	// Same ref count (order may differ due to goroutine scheduling)
	if len(seq.Refs) != len(par.Refs) {
		t.Errorf("refs: seq=%d par=%d", len(seq.Refs), len(par.Refs))
	}
}

func TestScanParallel_Workers1_Sequential(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app.go", `db.Query("SELECT * FROM users")`)

	result, err := ScanParallel(dir, 1)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Tables) != 1 || result.Tables[0] != "users" {
		t.Errorf("expected [users], got %v", result.Tables)
	}
}

func TestScanParallel_Workers0_DefaultsCPU(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app.go", `db.Query("SELECT * FROM orders")`)

	result, err := ScanParallel(dir, 0)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Tables) != 1 || result.Tables[0] != "orders" {
		t.Errorf("expected [orders], got %v", result.Tables)
	}
}

func TestScanParallel_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	result, err := ScanParallel(dir, 2)
	if err != nil {
		t.Fatal(err)
	}

	if result.FilesScanned != 0 {
		t.Errorf("expected 0 files, got %d", result.FilesScanned)
	}
	if len(result.Tables) != 0 {
		t.Errorf("expected 0 tables, got %v", result.Tables)
	}
}

func TestScanParallel_SkipsDirs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app.go", `db.Query("SELECT * FROM users")`)
	writeFile(t, dir, "node_modules/lib.js", `db.query("SELECT * FROM secret")`)

	result, err := ScanParallel(dir, 2)
	if err != nil {
		t.Fatal(err)
	}

	if result.FilesScanned != 1 {
		t.Errorf("expected 1 file, got %d", result.FilesScanned)
	}
	for _, r := range result.Refs {
		if r.Table == "secret" {
			t.Error("should not find table from node_modules")
		}
	}
}

func TestScanParallel_ManyFiles(t *testing.T) {
	dir := t.TempDir()

	// Create 20 files to exercise worker pool
	tables := make([]string, 20)
	for i := range 20 {
		name := fmt.Sprintf("table_%02d", i)
		tables[i] = name
		writeFile(t, dir, fmt.Sprintf("file%02d.go", i),
			fmt.Sprintf(`db.Query("SELECT * FROM %s")`, name))
	}

	result, err := ScanParallel(dir, 4)
	if err != nil {
		t.Fatal(err)
	}

	if result.FilesScanned != 20 {
		t.Errorf("expected 20 files, got %d", result.FilesScanned)
	}

	sort.Strings(tables)
	if len(result.Tables) != 20 {
		t.Fatalf("expected 20 tables, got %d", len(result.Tables))
	}
	for i := range tables {
		if result.Tables[i] != tables[i] {
			t.Errorf("tables[%d]: got %q, want %q", i, result.Tables[i], tables[i])
		}
	}
}
