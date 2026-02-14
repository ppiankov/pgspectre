package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScan_MultiLanguage(t *testing.T) {
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
class Order(Base):
    __tablename__ = 'orders'
class Payment(Base):
    __tablename__ = 'payments'
`)

	writeFile(t, dir, "schema.prisma", `model User {
  id Int @id
  @@map("users")
}
model Session {
  id Int @id
  @@map("sessions")
}`)

	writeFile(t, dir, "migrate.sql", `CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY
);
CREATE TABLE orders (
    id SERIAL PRIMARY KEY
);`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	if result.FilesScanned != 4 {
		t.Errorf("filesScanned = %d, want 4", result.FilesScanned)
	}

	// Should find: users, orders, payments, sessions
	tableSet := make(map[string]bool)
	for _, tbl := range result.Tables {
		tableSet[tbl] = true
	}

	for _, want := range []string{"users", "orders", "payments", "sessions"} {
		if !tableSet[want] {
			t.Errorf("expected table %q in results, got %v", want, result.Tables)
		}
	}
}

func TestScan_SkipsDirs(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "app.go", `db.Query("SELECT * FROM users")`)
	writeFile(t, dir, "node_modules/lib.js", `db.query("SELECT * FROM secret_table")`)
	writeFile(t, dir, "vendor/dep.go", `db.Query("SELECT * FROM vendor_table")`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	if result.FilesScanned != 1 {
		t.Errorf("filesScanned = %d, want 1 (should skip node_modules and vendor)", result.FilesScanned)
	}

	for _, r := range result.Refs {
		if r.Table == "secret_table" || r.Table == "vendor_table" {
			t.Errorf("should not find table from skipped directory: %s", r.Table)
		}
	}
}

func TestScan_SkipsNonCode(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "app.go", `db.Query("SELECT * FROM users")`)
	writeFile(t, dir, "README.md", `SELECT * FROM fake_table`)
	writeFile(t, dir, "data.json", `{"query": "SELECT * FROM json_table"}`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	if result.FilesScanned != 1 {
		t.Errorf("filesScanned = %d, want 1", result.FilesScanned)
	}
}

func TestScan_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	if result.FilesScanned != 0 {
		t.Errorf("filesScanned = %d, want 0", result.FilesScanned)
	}
	if len(result.Tables) != 0 {
		t.Errorf("tables = %v, want empty", result.Tables)
	}
}

func TestScan_Deduplication(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "a.go", `db.Query("SELECT * FROM users")`)
	writeFile(t, dir, "b.go", `db.Query("SELECT * FROM users")`)
	writeFile(t, dir, "c.py", `cursor.execute("SELECT * FROM users")`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Tables) != 1 {
		t.Errorf("expected 1 unique table, got %v", result.Tables)
	}
	if result.Tables[0] != "users" {
		t.Errorf("expected 'users', got %q", result.Tables[0])
	}
	// Refs should still have 3 entries (one per file)
	if len(result.Refs) != 3 {
		t.Errorf("expected 3 refs, got %d", len(result.Refs))
	}
}

func TestUniqueTables_Sorted(t *testing.T) {
	refs := []TableRef{
		{Table: "Zebra"},
		{Table: "apple"},
		{Table: "Apple"},
		{Table: "banana"},
	}

	tables := uniqueTables(refs)

	if len(tables) != 3 {
		t.Errorf("expected 3 unique tables, got %v", tables)
	}
	// Should be sorted: apple, banana, zebra
	if tables[0] != "apple" || tables[1] != "banana" || tables[2] != "zebra" {
		t.Errorf("expected [apple banana zebra], got %v", tables)
	}
}
