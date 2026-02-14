package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Inspector reads PostgreSQL catalog metadata and statistics.
type Inspector struct {
	pool *pgxpool.Pool
}

// NewInspector connects to PostgreSQL and verifies the connection.
func NewInspector(ctx context.Context, cfg Config) (*Inspector, error) {
	pool, err := pgxpool.New(ctx, cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	return &Inspector{pool: pool}, nil
}

// Close releases the connection pool.
func (i *Inspector) Close() {
	i.pool.Close()
}

// ServerVersion returns the PostgreSQL server version string.
func (i *Inspector) ServerVersion(ctx context.Context) (string, error) {
	var version string
	err := i.pool.QueryRow(ctx, "SHOW server_version").Scan(&version)
	if err != nil {
		return "", fmt.Errorf("server version: %w", err)
	}
	return version, nil
}

// GetTables fetches all user tables with row estimates.
func (i *Inspector) GetTables(ctx context.Context) ([]TableInfo, error) {
	query := `
		SELECT
			t.table_schema,
			t.table_name,
			t.table_type,
			COALESCE(c.reltuples::bigint, 0) AS estimated_rows
		FROM information_schema.tables t
		LEFT JOIN pg_catalog.pg_class c
			ON c.relname = t.table_name
			AND c.relnamespace = (
				SELECT oid FROM pg_catalog.pg_namespace WHERE nspname = t.table_schema
			)
		WHERE t.table_schema NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
			AND t.table_type = 'BASE TABLE'
		ORDER BY t.table_schema, t.table_name`

	rows, err := i.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("get tables: %w", err)
	}
	defer rows.Close()

	var tables []TableInfo
	for rows.Next() {
		var t TableInfo
		if err := rows.Scan(&t.Schema, &t.Name, &t.Type, &t.EstimatedRows); err != nil {
			return nil, fmt.Errorf("scan table: %w", err)
		}
		tables = append(tables, t)
	}
	return tables, rows.Err()
}

// GetColumns fetches all user table columns.
func (i *Inspector) GetColumns(ctx context.Context) ([]ColumnInfo, error) {
	query := `
		SELECT
			table_schema,
			table_name,
			column_name,
			ordinal_position,
			data_type,
			is_nullable = 'YES' AS is_nullable,
			column_default
		FROM information_schema.columns
		WHERE table_schema NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
		ORDER BY table_schema, table_name, ordinal_position`

	rows, err := i.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("get columns: %w", err)
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var c ColumnInfo
		if err := rows.Scan(&c.Schema, &c.Table, &c.Name, &c.OrdinalPosition, &c.DataType, &c.IsNullable, &c.ColumnDefault); err != nil {
			return nil, fmt.Errorf("scan column: %w", err)
		}
		columns = append(columns, c)
	}
	return columns, rows.Err()
}

// GetIndexes fetches all user indexes with definitions and usage stats.
func (i *Inspector) GetIndexes(ctx context.Context) ([]IndexInfo, error) {
	query := `
		SELECT
			pi.schemaname,
			pi.tablename,
			pi.indexname,
			pi.indexdef,
			COALESCE(pg_catalog.pg_relation_size(si.indexrelid), 0) AS size_bytes,
			COALESCE(si.idx_scan, 0) AS idx_scan,
			COALESCE(si.idx_tup_read, 0) AS idx_tup_read,
			COALESCE(si.idx_tup_fetch, 0) AS idx_tup_fetch
		FROM pg_catalog.pg_indexes pi
		LEFT JOIN pg_catalog.pg_stat_user_indexes si
			ON si.indexrelname = pi.indexname
			AND si.schemaname = pi.schemaname
		WHERE pi.schemaname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
		ORDER BY pi.schemaname, pi.tablename, pi.indexname`

	rows, err := i.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("get indexes: %w", err)
	}
	defer rows.Close()

	var indexes []IndexInfo
	for rows.Next() {
		var idx IndexInfo
		if err := rows.Scan(&idx.Schema, &idx.Table, &idx.Name, &idx.Definition, &idx.SizeBytes, &idx.IndexScans, &idx.TupRead, &idx.TupFetch); err != nil {
			return nil, fmt.Errorf("scan index: %w", err)
		}
		indexes = append(indexes, idx)
	}
	return indexes, rows.Err()
}

// GetTableStats fetches usage statistics for all user tables.
func (i *Inspector) GetTableStats(ctx context.Context) ([]TableStats, error) {
	query := `
		SELECT
			schemaname,
			relname,
			COALESCE(seq_scan, 0),
			COALESCE(seq_tup_read, 0),
			COALESCE(idx_scan, 0),
			COALESCE(idx_tup_fetch, 0),
			COALESCE(n_live_tup, 0),
			COALESCE(n_dead_tup, 0),
			last_vacuum,
			last_autovacuum,
			last_analyze,
			last_autoanalyze,
			COALESCE(vacuum_count, 0),
			COALESCE(autovacuum_count, 0),
			COALESCE(analyze_count, 0),
			COALESCE(autoanalyze_count, 0)
		FROM pg_catalog.pg_stat_user_tables
		ORDER BY schemaname, relname`

	rows, err := i.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("get table stats: %w", err)
	}
	defer rows.Close()

	var stats []TableStats
	for rows.Next() {
		var s TableStats
		if err := rows.Scan(
			&s.Schema, &s.Name,
			&s.SeqScan, &s.SeqTupRead, &s.IdxScan, &s.IdxTupFetch,
			&s.LiveTuples, &s.DeadTuples,
			&s.LastVacuum, &s.LastAutovacuum, &s.LastAnalyze, &s.LastAutoanalyze,
			&s.VacuumCount, &s.AutovacuumCount, &s.AnalyzeCount, &s.AutoanalyzeCount,
		); err != nil {
			return nil, fmt.Errorf("scan table stats: %w", err)
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// GetConstraints fetches all user table constraints with column names.
func (i *Inspector) GetConstraints(ctx context.Context) ([]ConstraintInfo, error) {
	query := `
		SELECT
			n.nspname AS schema,
			rel.relname AS table_name,
			c.conname AS name,
			c.contype::text AS type,
			COALESCE(
				ARRAY(
					SELECT a.attname
					FROM unnest(c.conkey) WITH ORDINALITY AS u(attnum, ord)
					JOIN pg_catalog.pg_attribute a
						ON a.attrelid = c.conrelid AND a.attnum = u.attnum
					ORDER BY u.ord
				),
				'{}'
			) AS columns,
			frel.relname AS ref_table,
			COALESCE(
				ARRAY(
					SELECT a.attname
					FROM unnest(c.confkey) WITH ORDINALITY AS u(attnum, ord)
					JOIN pg_catalog.pg_attribute a
						ON a.attrelid = c.confrelid AND a.attnum = u.attnum
					ORDER BY u.ord
				),
				'{}'
			) AS ref_columns
		FROM pg_catalog.pg_constraint c
		JOIN pg_catalog.pg_namespace n ON n.oid = c.connamespace
		JOIN pg_catalog.pg_class rel ON rel.oid = c.conrelid
		LEFT JOIN pg_catalog.pg_class frel ON frel.oid = c.confrelid
		WHERE n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
			AND c.conrelid > 0
		ORDER BY n.nspname, rel.relname, c.conname`

	rows, err := i.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("get constraints: %w", err)
	}
	defer rows.Close()

	var constraints []ConstraintInfo
	for rows.Next() {
		var ci ConstraintInfo
		if err := rows.Scan(&ci.Schema, &ci.Table, &ci.Name, &ci.Type, &ci.Columns, &ci.RefTable, &ci.RefColumns); err != nil {
			return nil, fmt.Errorf("scan constraint: %w", err)
		}
		constraints = append(constraints, ci)
	}
	return constraints, rows.Err()
}

// Inspect gathers the full catalog snapshot for the connected database.
func (i *Inspector) Inspect(ctx context.Context) (*Snapshot, error) {
	tables, err := i.GetTables(ctx)
	if err != nil {
		return nil, err
	}

	columns, err := i.GetColumns(ctx)
	if err != nil {
		return nil, err
	}

	indexes, err := i.GetIndexes(ctx)
	if err != nil {
		return nil, err
	}

	stats, err := i.GetTableStats(ctx)
	if err != nil {
		return nil, err
	}

	constraints, err := i.GetConstraints(ctx)
	if err != nil {
		return nil, err
	}

	return &Snapshot{
		Tables:      tables,
		Columns:     columns,
		Indexes:     indexes,
		Stats:       stats,
		Constraints: constraints,
	}, nil
}
