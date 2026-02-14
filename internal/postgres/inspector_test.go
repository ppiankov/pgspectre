package postgres

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestNewInspector_InvalidURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := NewInspector(ctx, Config{URL: "postgres://localhost:1/nonexistent"})
	if err == nil {
		t.Fatal("expected error for invalid connection, got nil")
	}
}

func TestNewInspector_EmptyURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := NewInspector(ctx, Config{URL: ""})
	if err == nil {
		t.Fatal("expected error for empty URL, got nil")
	}
}

func TestSnapshotJSON(t *testing.T) {
	now := time.Now()
	defVal := "0"
	refTable := "orders"

	snap := Snapshot{
		Tables: []TableInfo{
			{Schema: "public", Name: "users", Type: "BASE TABLE", EstimatedRows: 100},
		},
		Columns: []ColumnInfo{
			{Schema: "public", Table: "users", Name: "id", OrdinalPosition: 1, DataType: "integer", IsNullable: false, ColumnDefault: &defVal},
		},
		Indexes: []IndexInfo{
			{Schema: "public", Table: "users", Name: "users_pkey", Definition: "CREATE UNIQUE INDEX users_pkey ON public.users USING btree (id)", SizeBytes: 8192, IndexScans: 42, TupRead: 100, TupFetch: 50},
		},
		Stats: []TableStats{
			{Schema: "public", Name: "users", SeqScan: 10, SeqTupRead: 1000, IdxScan: 50, IdxTupFetch: 500, LiveTuples: 100, DeadTuples: 5, LastVacuum: &now, VacuumCount: 3},
		},
		Constraints: []ConstraintInfo{
			{Schema: "public", Table: "users", Name: "users_pkey", Type: "p", Columns: []string{"id"}},
			{Schema: "public", Table: "users", Name: "users_order_fk", Type: "f", Columns: []string{"order_id"}, RefTable: &refTable, RefColumns: []string{"id"}},
		},
	}

	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}

	var decoded Snapshot
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}

	if len(decoded.Tables) != 1 {
		t.Errorf("expected 1 table, got %d", len(decoded.Tables))
	}
	if decoded.Tables[0].Name != "users" {
		t.Errorf("expected table name 'users', got %q", decoded.Tables[0].Name)
	}
	if len(decoded.Constraints) != 2 {
		t.Errorf("expected 2 constraints, got %d", len(decoded.Constraints))
	}
}

func TestTypeFieldsHaveJSONTags(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeOf(TableInfo{}),
		reflect.TypeOf(ColumnInfo{}),
		reflect.TypeOf(IndexInfo{}),
		reflect.TypeOf(TableStats{}),
		reflect.TypeOf(ConstraintInfo{}),
		reflect.TypeOf(Snapshot{}),
	}

	for _, typ := range types {
		t.Run(typ.Name(), func(t *testing.T) {
			for i := range typ.NumField() {
				field := typ.Field(i)
				tag := field.Tag.Get("json")
				if tag == "" {
					t.Errorf("field %s.%s has no json tag", typ.Name(), field.Name)
				}
			}
		})
	}
}
