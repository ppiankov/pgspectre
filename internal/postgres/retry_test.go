package postgres

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsRetryable_ConnectionRefused(t *testing.T) {
	err := fmt.Errorf("dial tcp: connection refused")
	if !isRetryable(err) {
		t.Error("connection refused should be retryable")
	}
}

func TestIsRetryable_ConnectionReset(t *testing.T) {
	err := fmt.Errorf("read: connection reset by peer")
	if !isRetryable(err) {
		t.Error("connection reset should be retryable")
	}
}

func TestIsRetryable_IOTimeout(t *testing.T) {
	err := fmt.Errorf("dial tcp: i/o timeout")
	if !isRetryable(err) {
		t.Error("i/o timeout should be retryable")
	}
}

func TestIsRetryable_DeadlineExceeded(t *testing.T) {
	err := context.DeadlineExceeded
	if !isRetryable(err) {
		t.Error("deadline exceeded should be retryable")
	}
}

func TestIsRetryable_AuthFailed(t *testing.T) {
	err := &pgconn.PgError{Code: "28P01", Message: "password authentication failed"}
	if isRetryable(err) {
		t.Error("auth failure should NOT be retryable")
	}
}

func TestIsRetryable_AuthFailedString(t *testing.T) {
	err := fmt.Errorf("password authentication failed for user \"test\"")
	if isRetryable(err) {
		t.Error("auth failure string should NOT be retryable")
	}
}

func TestIsRetryable_HBAError(t *testing.T) {
	err := fmt.Errorf("no pg_hba.conf entry for host")
	if isRetryable(err) {
		t.Error("pg_hba.conf error should NOT be retryable")
	}
}

func TestIsRetryable_ParseConfigError(t *testing.T) {
	err := pgconn.NewParseConfigError("not-a-url", "failed to parse as keyword/value", errors.New("invalid keyword/value"))
	if isRetryable(err) {
		t.Error("parse config errors should NOT be retryable")
	}
}

func TestIsRetryable_NoSuchHost(t *testing.T) {
	err := fmt.Errorf("lookup invalid: no such host")
	if isRetryable(err) {
		t.Error("no such host should NOT be retryable")
	}
}

func TestIsRetryable_TooManyConnections(t *testing.T) {
	err := &pgconn.PgError{Code: "53300", Message: "too many connections"}
	if !isRetryable(err) {
		t.Error("too many connections should be retryable")
	}
}

func TestIsRetryable_InvalidCatalogName(t *testing.T) {
	err := &pgconn.PgError{Code: "3D000", Message: "database does not exist"}
	if isRetryable(err) {
		t.Error("invalid catalog name should NOT be retryable")
	}
}

func TestIsRetryable_NetOpError(t *testing.T) {
	err := &net.OpError{Op: "dial", Err: errors.New("connection refused")}
	if !isRetryable(err) {
		t.Error("net.OpError should be retryable")
	}
}

func TestIsRetryable_UnknownError(t *testing.T) {
	err := fmt.Errorf("something unexpected")
	if !isRetryable(err) {
		t.Error("unknown errors should be retryable by default")
	}
}

func TestBackoffDelay(t *testing.T) {
	d0 := backoffDelay(0)
	d1 := backoffDelay(1)
	d2 := backoffDelay(2)

	// Base delays: 1s, 2s, 4s (plus jitter up to 500ms)
	if d0 < 1*time.Second || d0 > 1500*time.Millisecond {
		t.Errorf("attempt 0: got %v, want ~1s", d0)
	}
	if d1 < 2*time.Second || d1 > 2500*time.Millisecond {
		t.Errorf("attempt 1: got %v, want ~2s", d1)
	}
	if d2 < 4*time.Second || d2 > 4500*time.Millisecond {
		t.Errorf("attempt 2: got %v, want ~4s", d2)
	}
}

func TestConnectWithRetry_InvalidHost_Retries(t *testing.T) {
	// Use an invalid URL that will fail with connection refused
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := connectWithRetry(ctx, Config{URL: "postgres://localhost:1/nonexistent"})

	if err == nil {
		t.Fatal("expected error")
	}
}

func TestConnectWithRetry_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := connectWithRetry(ctx, Config{URL: "postgres://localhost:1/test"})
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestConnectWithRetry_InvalidURL_FailsFast(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	_, err := connectWithRetry(ctx, Config{URL: "not-a-url"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "cannot parse `not-a-url`") {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed >= baseDelay {
		t.Fatalf("expected fail-fast without retry delay, took %v", elapsed)
	}
}
