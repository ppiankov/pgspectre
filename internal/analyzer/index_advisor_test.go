package analyzer

import (
	"testing"

	"github.com/ppiankov/pgspectre/internal/postgres"
	"github.com/ppiankov/pgspectre/internal/scanner"
)

func TestParseIndexColumns(t *testing.T) {
	tests := []struct {
		name string
		def  string
		want []string
	}{
		{
			"simple single column",
			"CREATE INDEX idx_email ON users (email)",
			[]string{"email"},
		},
		{
			"composite index",
			"CREATE INDEX idx_user_created ON orders (user_id, created_at)",
			[]string{"user_id", "created_at"},
		},
		{
			"with DESC",
			"CREATE INDEX idx_sort ON orders (created_at DESC)",
			[]string{"created_at"},
		},
		{
			"with NULLS LAST",
			"CREATE INDEX idx_sort ON orders (created_at DESC NULLS LAST)",
			[]string{"created_at"},
		},
		{
			"function-based (skipped)",
			"CREATE INDEX idx_lower ON users (lower(email))",
			nil,
		},
		{
			"unique index",
			"CREATE UNIQUE INDEX idx_uniq ON users (username)",
			[]string{"username"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseIndexColumns(tt.def)
			if len(got) != len(tt.want) {
				t.Errorf("parseIndexColumns(%q) = %v, want %v", tt.def, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("col[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestDetectUnindexedQueries_Basic(t *testing.T) {
	columnRefs := []scanner.ColumnRef{
		{Table: "users", Column: "email", Context: scanner.ContextWhere},
		{Table: "users", Column: "email", Context: scanner.ContextWhere}, // duplicate
		{Table: "users", Column: "name", Context: scanner.ContextSelect}, // SELECT only, should be skipped
	}
	indexes := []postgres.IndexInfo{} // No indexes
	tables := []postgres.TableInfo{
		{Schema: "public", Name: "users"},
	}

	findings := DetectUnindexedQueries(columnRefs, indexes, tables)

	// Should find one unindexed query (email in WHERE, name is SELECT-only)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d: %v", len(findings), findings)
	}
	if findings[0].Column != "email" {
		t.Errorf("expected column email, got %q", findings[0].Column)
	}
	if findings[0].Type != FindingUnindexedQuery {
		t.Errorf("expected UNINDEXED_QUERY, got %s", findings[0].Type)
	}
}

func TestDetectUnindexedQueries_IndexExists(t *testing.T) {
	columnRefs := []scanner.ColumnRef{
		{Table: "users", Column: "email", Context: scanner.ContextWhere},
	}
	indexes := []postgres.IndexInfo{
		{Schema: "public", Table: "users", Name: "idx_email", Definition: "CREATE INDEX idx_email ON users (email)"},
	}
	tables := []postgres.TableInfo{
		{Schema: "public", Name: "users"},
	}

	findings := DetectUnindexedQueries(columnRefs, indexes, tables)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings when index exists, got %d: %v", len(findings), findings)
	}
}

func TestDetectUnindexedQueries_CompositeIndex(t *testing.T) {
	columnRefs := []scanner.ColumnRef{
		{Table: "orders", Column: "user_id", Context: scanner.ContextWhere},
	}
	indexes := []postgres.IndexInfo{
		{Schema: "public", Table: "orders", Name: "idx_composite", Definition: "CREATE INDEX idx_composite ON orders (user_id, created_at)"},
	}
	tables := []postgres.TableInfo{
		{Schema: "public", Name: "orders"},
	}

	findings := DetectUnindexedQueries(columnRefs, indexes, tables)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings — composite index covers user_id, got %d", len(findings))
	}
}

func TestDetectUnindexedQueries_OrderByContext(t *testing.T) {
	columnRefs := []scanner.ColumnRef{
		{Table: "orders", Column: "created_at", Context: scanner.ContextOrderBy},
	}
	indexes := []postgres.IndexInfo{} // No indexes
	tables := []postgres.TableInfo{
		{Schema: "public", Name: "orders"},
	}

	findings := DetectUnindexedQueries(columnRefs, indexes, tables)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding for ORDER BY without index, got %d", len(findings))
	}
}

func TestDetectUnindexedQueries_UnknownTable(t *testing.T) {
	columnRefs := []scanner.ColumnRef{
		{Table: "nonexistent", Column: "id", Context: scanner.ContextWhere},
	}
	indexes := []postgres.IndexInfo{}
	tables := []postgres.TableInfo{} // No tables in DB

	findings := DetectUnindexedQueries(columnRefs, indexes, tables)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for unknown table, got %d", len(findings))
	}
}

func TestBuildIndexedColumns(t *testing.T) {
	indexes := []postgres.IndexInfo{
		{Schema: "public", Table: "users", Definition: "CREATE INDEX idx_email ON users (email)"},
		{Schema: "public", Table: "orders", Definition: "CREATE INDEX idx_user ON orders (user_id, created_at)"},
	}

	cols := buildIndexedColumns(indexes)

	want := []string{
		"public.users.email",
		"public.orders.user_id",
		"public.orders.created_at",
	}
	for _, w := range want {
		if !cols[w] {
			t.Errorf("expected %q in indexed columns, got %v", w, cols)
		}
	}

	if cols["public.users.name"] {
		t.Error("should not contain public.users.name")
	}
}
