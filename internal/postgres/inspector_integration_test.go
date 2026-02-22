//go:build integration

package postgres

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/ppiankov/pgspectre/internal/testutil"
)

func TestIntegration_Inspector(t *testing.T) {
	connStr, cleanup := testutil.SetupPostgres(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	inspector, err := NewInspector(ctx, Config{URL: connStr})
	if err != nil {
		t.Fatalf("NewInspector: %v", err)
	}
	defer inspector.Close()

	// ServerVersion
	ver, err := inspector.ServerVersion(ctx)
	if err != nil {
		t.Fatalf("ServerVersion: %v", err)
	}
	if ver == "" {
		t.Error("server version is empty")
	}
	t.Logf("PostgreSQL version: %s", ver)

	// GetTables
	tables, err := inspector.GetTables(ctx)
	if err != nil {
		t.Fatalf("GetTables: %v", err)
	}
	tableNames := make(map[string]bool)
	for _, tbl := range tables {
		tableNames[tbl.Name] = true
	}
	for _, want := range []string{"users", "orders", "empty_table"} {
		if !tableNames[want] {
			t.Errorf("GetTables: missing table %q", want)
		}
	}
	// Verify users has estimated rows > 0
	for _, tbl := range tables {
		if tbl.Name == "users" {
			if tbl.EstimatedRows <= 0 {
				t.Errorf("users estimated_rows = %d, want > 0", tbl.EstimatedRows)
			}
			if tbl.SizeBytes <= 0 {
				t.Errorf("users size_bytes = %d, want > 0", tbl.SizeBytes)
			}
			if tbl.Schema != "public" {
				t.Errorf("users schema = %q, want public", tbl.Schema)
			}
		}
	}

	// GetColumns
	columns, err := inspector.GetColumns(ctx)
	if err != nil {
		t.Fatalf("GetColumns: %v", err)
	}
	userCols := make(map[string]bool)
	for _, col := range columns {
		if col.Table == "users" {
			userCols[col.Name] = true
		}
	}
	for _, want := range []string{"id", "name", "email", "status"} {
		if !userCols[want] {
			t.Errorf("GetColumns: users missing column %q", want)
		}
	}

	// GetIndexes
	indexes, err := inspector.GetIndexes(ctx)
	if err != nil {
		t.Fatalf("GetIndexes: %v", err)
	}
	idxNames := make(map[string]bool)
	for _, idx := range indexes {
		idxNames[idx.Name] = true
	}
	for _, want := range []string{"idx_users_email", "idx_orders_user_id", "idx_orders_created"} {
		if !idxNames[want] {
			t.Errorf("GetIndexes: missing index %q", want)
		}
	}

	// GetTableStats
	stats, err := inspector.GetTableStats(ctx)
	if err != nil {
		t.Fatalf("GetTableStats: %v", err)
	}
	statsMap := make(map[string]TableStats)
	for _, s := range stats {
		statsMap[s.Name] = s
	}
	if s, ok := statsMap["users"]; !ok {
		t.Error("GetTableStats: missing users stats")
	} else if s.LiveTuples <= 0 {
		t.Errorf("users live_tuples = %d, want > 0", s.LiveTuples)
	}
	if _, ok := statsMap["empty_table"]; !ok {
		t.Error("GetTableStats: missing empty_table stats")
	}

	// GetConstraints
	constraints, err := inspector.GetConstraints(ctx)
	if err != nil {
		t.Fatalf("GetConstraints: %v", err)
	}

	var (
		hasPK bool
		hasFK bool
		hasUQ bool
	)
	for _, c := range constraints {
		switch {
		case c.Table == "users" && c.Type == "p":
			hasPK = true
			if len(c.Columns) != 1 || c.Columns[0] != "id" {
				t.Errorf("users PK columns = %v, want [id]", c.Columns)
			}
		case c.Table == "orders" && c.Type == "f":
			hasFK = true
			if c.RefTable == nil || *c.RefTable != "users" {
				t.Errorf("orders FK ref_table = %v, want users", c.RefTable)
			}
		case c.Table == "users" && c.Type == "u":
			hasUQ = true
		}
	}
	if !hasPK {
		t.Error("GetConstraints: no primary key found for users")
	}
	if !hasFK {
		t.Error("GetConstraints: no foreign key found for orders")
	}
	if !hasUQ {
		t.Error("GetConstraints: no unique constraint found for users.email")
	}

	// Inspect (full snapshot)
	snap, err := inspector.Inspect(ctx)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(snap.Tables) < 3 {
		t.Errorf("Inspect tables = %d, want >= 3", len(snap.Tables))
	}
	if len(snap.Columns) == 0 {
		t.Error("Inspect returned no columns")
	}
	if len(snap.Indexes) == 0 {
		t.Error("Inspect returned no indexes")
	}
	if len(snap.Stats) == 0 {
		t.Error("Inspect returned no stats")
	}
	if len(snap.Constraints) == 0 {
		t.Error("Inspect returned no constraints")
	}
	t.Logf("Inspect: %d tables, %d columns, %d indexes, %d stats, %d constraints",
		len(snap.Tables), len(snap.Columns), len(snap.Indexes), len(snap.Stats), len(snap.Constraints))
}

func TestIntegration_NewInspector_BadURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := NewInspector(ctx, Config{URL: "postgres://invalid:5432/nodb"})
	if err == nil {
		t.Error("expected error for bad URL")
	}
	fmt.Println("Expected error:", err)
}

func TestIntegration_Inspector_NonSuperuser(t *testing.T) {
	connStr, cleanup := testutil.SetupPostgres(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adminConn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer func() { _ = adminConn.Close(ctx) }()

	parsed, err := url.Parse(connStr)
	if err != nil {
		t.Fatalf("parse conn string: %v", err)
	}
	dbName := strings.TrimPrefix(parsed.Path, "/")
	if dbName == "" {
		t.Fatalf("empty db name in conn string: %q", connStr)
	}

	roleName := fmt.Sprintf("pgspectre_reader_%d", time.Now().UnixNano())
	rolePassword := "pgspectre_reader"

	setupSQL := fmt.Sprintf(`
		CREATE ROLE %s LOGIN PASSWORD '%s' NOSUPERUSER NOCREATEDB NOCREATEROLE;
		GRANT CONNECT ON DATABASE %s TO %s;
		GRANT USAGE ON SCHEMA public TO %s;
		GRANT SELECT ON ALL TABLES IN SCHEMA public TO %s;
		GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO %s;
	`,
		quoteIdentifier(roleName), rolePassword, quoteIdentifier(dbName), quoteIdentifier(roleName),
		quoteIdentifier(roleName), quoteIdentifier(roleName), quoteIdentifier(roleName),
	)
	if _, err := adminConn.Exec(ctx, setupSQL); err != nil {
		t.Skipf("skipping non-superuser role setup: %v", err)
	}

	parsed.User = url.UserPassword(roleName, rolePassword)
	readerConnStr := parsed.String()

	inspector, err := NewInspector(ctx, Config{URL: readerConnStr})
	if err != nil {
		t.Fatalf("NewInspector (non-superuser): %v", err)
	}
	defer inspector.Close()

	snap, err := inspector.Inspect(ctx)
	if err != nil {
		t.Fatalf("Inspect (non-superuser): %v", err)
	}

	if len(snap.Tables) == 0 {
		t.Fatal("expected tables for non-superuser")
	}
	if len(snap.Columns) == 0 {
		t.Fatal("expected columns for non-superuser")
	}
	if len(snap.Indexes) == 0 {
		t.Fatal("expected indexes for non-superuser")
	}
	if len(snap.Stats) == 0 {
		t.Fatal("expected table stats for non-superuser")
	}
}

func quoteIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
