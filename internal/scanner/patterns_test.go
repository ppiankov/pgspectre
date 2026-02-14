package scanner

import "testing"

func TestScanLine_SQLFrom(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		table   string
		context Context
	}{
		{"simple", `SELECT * FROM users WHERE id = 1`, "users", ContextSelect},
		{"lowercase", `select name from orders`, "orders", ContextSelect},
		{"schema qualified", `SELECT * FROM public.users`, "users", ContextSelect},
		{"with alias", `SELECT u.name FROM users u`, "users", ContextSelect},
		{"subquery", `SELECT * FROM accounts WHERE id IN (SELECT id FROM users)`, "accounts", ContextSelect},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := ScanLine(tt.line)
			found := false
			for _, m := range matches {
				if m.Table == tt.table && m.Context == tt.context {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected table %q with context %s in %q, got %v", tt.table, tt.context, tt.line, matches)
			}
		})
	}
}

func TestScanLine_SQLJoin(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		table string
	}{
		{"inner join", `SELECT * FROM users INNER JOIN orders ON users.id = orders.user_id`, "orders"},
		{"left join", `LEFT JOIN payments ON orders.id = payments.order_id`, "payments"},
		{"schema qualified", `JOIN public.accounts ON a.id = b.account_id`, "accounts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := ScanLine(tt.line)
			found := false
			for _, m := range matches {
				if m.Table == tt.table {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected table %q in %q, got %v", tt.table, tt.line, matches)
			}
		})
	}
}

func TestScanLine_SQLInsert(t *testing.T) {
	matches := ScanLine(`INSERT INTO users (name, email) VALUES ('alice', 'a@b.com')`)
	found := false
	for _, m := range matches {
		if m.Table == "users" && m.Context == ContextInsert {
			found = true
		}
	}
	if !found {
		t.Errorf("expected INSERT context for users, got %v", matches)
	}
}

func TestScanLine_SQLUpdate(t *testing.T) {
	matches := ScanLine(`UPDATE orders SET status = 'shipped' WHERE id = 1`)
	found := false
	for _, m := range matches {
		if m.Table == "orders" && m.Context == ContextUpdate {
			found = true
		}
	}
	if !found {
		t.Errorf("expected UPDATE context for orders, got %v", matches)
	}
}

func TestScanLine_SQLDelete(t *testing.T) {
	matches := ScanLine(`DELETE FROM sessions WHERE expired = true`)
	found := false
	for _, m := range matches {
		if m.Table == "sessions" && m.Context == ContextDelete {
			found = true
		}
	}
	if !found {
		t.Errorf("expected DELETE context for sessions, got %v", matches)
	}
}

func TestScanLine_SchemaQualified(t *testing.T) {
	matches := ScanLine(`SELECT * FROM public.users`)
	found := false
	for _, m := range matches {
		if m.Table == "users" && m.Schema == "public" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected schema=public table=users, got %v", matches)
	}
}

func TestScanLine_ORM(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		table string
	}{
		{"sqlalchemy", `    __tablename__ = 'users'`, "users"},
		{"django", `        db_table = "orders"`, "orders"},
		{"gorm tablename", `func (User) TableName() string { return "users" }`, "users"},
		{"gorm table", `db.Table("orders").Find(&results)`, "orders"},
		{"prisma", `  @@map("user_accounts")`, "user_accounts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := ScanLine(tt.line)
			found := false
			for _, m := range matches {
				if m.Table == tt.table && m.Pattern == PatternORM {
					found = true
				}
			}
			if !found {
				t.Errorf("expected ORM table %q in %q, got %v", tt.table, tt.line, matches)
			}
		})
	}
}

func TestScanLine_Migration(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		table string
	}{
		{"create table", `CREATE TABLE users (`, "users"},
		{"create if not exists", `CREATE TABLE IF NOT EXISTS orders (`, "orders"},
		{"alter table", `ALTER TABLE users ADD COLUMN email TEXT`, "users"},
		{"drop table", `DROP TABLE IF EXISTS sessions`, "sessions"},
		{"create index", `CREATE INDEX idx_users_email ON users (email)`, "users"},
		{"create unique index", `CREATE UNIQUE INDEX idx_orders_id ON orders (id)`, "orders"},
		{"schema qualified", `CREATE TABLE public.users (`, "users"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := ScanLine(tt.line)
			found := false
			for _, m := range matches {
				if m.Table == tt.table && m.Pattern == PatternMigration {
					found = true
				}
			}
			if !found {
				t.Errorf("expected migration table %q in %q, got %v", tt.table, tt.line, matches)
			}
		})
	}
}

func TestScanLine_NoMatch(t *testing.T) {
	lines := []string{
		"fmt.Println(\"hello world\")",
		"var x = 42",
		"",
		"import os",
	}

	for _, line := range lines {
		matches := ScanLine(line)
		if len(matches) > 0 {
			t.Errorf("unexpected match in %q: %v", line, matches)
		}
	}
}

func TestScanLine_RejectsKeywords(t *testing.T) {
	// "FROM select" should not match "select" as a table name
	matches := ScanLine(`SELECT * FROM (SELECT 1)`)
	for _, m := range matches {
		if m.Table == "SELECT" || m.Table == "select" {
			t.Errorf("should not match SQL keyword as table: %v", m)
		}
	}
}

func TestIsValidTableName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"normal", "users", true},
		{"too short", "u", false},
		{"keyword select", "select", false},
		{"keyword from", "FROM", false},
		{"common word", "public", true},
		{"underscore", "user_accounts", true},
		{"long", "a" + string(make([]byte, 120)), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidTableName(tt.input)
			if got != tt.valid {
				t.Errorf("isValidTableName(%q) = %v, want %v", tt.input, got, tt.valid)
			}
		})
	}
}
