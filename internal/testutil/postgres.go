package testutil

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// SeedSQL creates tables, indexes, constraints, and data for integration tests.
const SeedSQL = `
CREATE TABLE users (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	email TEXT NOT NULL UNIQUE,
	status TEXT DEFAULT 'active'
);

CREATE TABLE orders (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL REFERENCES users(id),
	amount NUMERIC(10,2) NOT NULL,
	created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE empty_table (
	id SERIAL PRIMARY KEY,
	data TEXT
);

CREATE INDEX idx_users_email ON users (email);
CREATE INDEX idx_orders_user_id ON orders (user_id);
CREATE INDEX idx_orders_created ON orders (created_at);

INSERT INTO users (name, email, status) VALUES
	('Alice', 'alice@example.com', 'active'),
	('Bob', 'bob@example.com', 'inactive'),
	('Charlie', 'charlie@example.com', 'active');

INSERT INTO orders (user_id, amount) VALUES
	(1, 99.99),
	(1, 49.50),
	(2, 150.00);

ANALYZE;
`

// runPostgresContainer starts a PG container, recovering from panics if Docker is unavailable.
func runPostgresContainer(ctx context.Context) (container *postgres.PostgresContainer, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	return postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
	)
}

// Setup starts a PostgreSQL container, seeds it with test data,
// and returns the connection string and a cleanup function.
// Returns an error if Docker is not available.
func Setup() (string, func(), error) {
	ctx := context.Background()

	container, err := runPostgresContainer(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("docker not available: %w", err)
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		return "", nil, fmt.Errorf("connection string: %w", err)
	}

	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		_ = container.Terminate(ctx)
		return "", nil, fmt.Errorf("seed connect: %w", err)
	}
	if _, err := conn.Exec(ctx, SeedSQL); err != nil {
		_ = conn.Close(ctx)
		_ = container.Terminate(ctx)
		return "", nil, fmt.Errorf("seed: %w", err)
	}
	_ = conn.Close(ctx)

	cleanup := func() {
		_ = container.Terminate(ctx)
	}
	return connStr, cleanup, nil
}

// SetupPostgres is a test helper that starts a PostgreSQL container and seeds it.
// Skips the test if Docker is not available.
func SetupPostgres(t *testing.T) (string, func()) {
	t.Helper()
	connStr, cleanup, err := Setup()
	if err != nil {
		t.Skipf("skipping: %v", err)
	}
	return connStr, cleanup
}
