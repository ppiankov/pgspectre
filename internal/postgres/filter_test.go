package postgres

import "testing"

func TestResolveSchemas_Empty(t *testing.T) {
	got := ResolveSchemas(nil)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestResolveSchemas_All(t *testing.T) {
	for _, input := range []string{"all", "ALL", "*", " all "} {
		got := ResolveSchemas([]string{input})
		if got != nil {
			t.Errorf("ResolveSchemas(%q) = %v, want nil", input, got)
		}
	}
}

func TestResolveSchemas_Specific(t *testing.T) {
	got := ResolveSchemas([]string{"public", "reporting"})
	if len(got) != 2 {
		t.Fatalf("expected 2 schemas, got %d", len(got))
	}
	if got[0] != "public" || got[1] != "reporting" {
		t.Errorf("got %v, want [public reporting]", got)
	}
}

func TestResolveSchemas_TrimsWhitespace(t *testing.T) {
	got := ResolveSchemas([]string{" public ", " app "})
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0] != "public" || got[1] != "app" {
		t.Errorf("got %v", got)
	}
}

func TestResolveSchemas_SkipsEmpty(t *testing.T) {
	got := ResolveSchemas([]string{"public", "", " ", "app"})
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d: %v", len(got), got)
	}
}

func TestFilterSnapshot_Nil(t *testing.T) {
	snap := &Snapshot{
		Tables: []TableInfo{{Schema: "public", Name: "users"}, {Schema: "app", Name: "orders"}},
	}
	got := FilterSnapshot(snap, nil)
	if got != snap {
		t.Error("expected same pointer when schemas is nil")
	}
}

func TestFilterSnapshot_Empty(t *testing.T) {
	snap := &Snapshot{
		Tables: []TableInfo{{Schema: "public", Name: "users"}},
	}
	got := FilterSnapshot(snap, []string{})
	if got != snap {
		t.Error("expected same pointer when schemas is empty")
	}
}

func TestFilterSnapshot_SingleSchema(t *testing.T) {
	snap := &Snapshot{
		Tables:      []TableInfo{{Schema: "public", Name: "users"}, {Schema: "app", Name: "orders"}},
		Columns:     []ColumnInfo{{Schema: "public", Table: "users", Name: "id"}, {Schema: "app", Table: "orders", Name: "id"}},
		Indexes:     []IndexInfo{{Schema: "public", Table: "users", Name: "users_pkey"}, {Schema: "app", Table: "orders", Name: "orders_pkey"}},
		Stats:       []TableStats{{Schema: "public", Name: "users"}, {Schema: "app", Name: "orders"}},
		Constraints: []ConstraintInfo{{Schema: "public", Table: "users", Name: "pk"}, {Schema: "app", Table: "orders", Name: "pk"}},
	}

	got := FilterSnapshot(snap, []string{"public"})

	if len(got.Tables) != 1 || got.Tables[0].Schema != "public" {
		t.Errorf("tables: got %v", got.Tables)
	}
	if len(got.Columns) != 1 || got.Columns[0].Schema != "public" {
		t.Errorf("columns: got %v", got.Columns)
	}
	if len(got.Indexes) != 1 || got.Indexes[0].Schema != "public" {
		t.Errorf("indexes: got %v", got.Indexes)
	}
	if len(got.Stats) != 1 || got.Stats[0].Schema != "public" {
		t.Errorf("stats: got %v", got.Stats)
	}
	if len(got.Constraints) != 1 || got.Constraints[0].Schema != "public" {
		t.Errorf("constraints: got %v", got.Constraints)
	}
}

func TestFilterSnapshot_MultipleSchemas(t *testing.T) {
	snap := &Snapshot{
		Tables: []TableInfo{
			{Schema: "public", Name: "users"},
			{Schema: "app", Name: "orders"},
			{Schema: "staging", Name: "temp"},
		},
	}

	got := FilterSnapshot(snap, []string{"public", "app"})
	if len(got.Tables) != 2 {
		t.Errorf("expected 2 tables, got %d", len(got.Tables))
	}
}

func TestFilterSnapshot_CaseInsensitive(t *testing.T) {
	snap := &Snapshot{
		Tables: []TableInfo{{Schema: "Public", Name: "users"}},
	}

	got := FilterSnapshot(snap, []string{"public"})
	if len(got.Tables) != 1 {
		t.Errorf("expected 1 table (case-insensitive match), got %d", len(got.Tables))
	}
}

func TestFilterSnapshot_NoMatch(t *testing.T) {
	snap := &Snapshot{
		Tables: []TableInfo{{Schema: "public", Name: "users"}},
	}

	got := FilterSnapshot(snap, []string{"nonexistent"})
	if len(got.Tables) != 0 {
		t.Errorf("expected 0 tables, got %d", len(got.Tables))
	}
}
