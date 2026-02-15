package postgres

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"net"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	maxRetries    = 3
	baseDelay     = 1 * time.Second
	maxJitter     = 500 * time.Millisecond
	authErrorCode = "28P01" // invalid_password
)

// connectWithRetry wraps NewInspector logic with exponential backoff.
// Retries on transient errors (connection refused, timeout).
// Fails fast on auth errors.
func connectWithRetry(ctx context.Context, cfg Config) (*Inspector, error) {
	var lastErr error

	for attempt := range maxRetries {
		inspector, err := newInspectorOnce(ctx, cfg)
		if err == nil {
			if attempt > 0 {
				slog.Info("connected after retry", "attempt", attempt+1)
			}
			return inspector, nil
		}

		if !isRetryable(err) {
			return nil, err
		}

		lastErr = err
		delay := backoffDelay(attempt)

		slog.Warn("connection failed, retrying",
			"attempt", attempt+1,
			"error", err,
			"retry_in", delay)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}

	return nil, lastErr
}

// isRetryable classifies errors as retryable or fail-fast.
func isRetryable(err error) bool {
	// Auth failures — fail fast
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == authErrorCode {
			return false
		}
	}

	// Check for common auth error strings (some drivers wrap differently)
	msg := err.Error()
	if strings.Contains(msg, "password authentication failed") {
		return false
	}
	if strings.Contains(msg, "no pg_hba.conf entry") {
		return false
	}

	// Network errors — retry
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return true
	}

	// Connection refused, reset, timeout — retry
	if strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "i/o timeout") ||
		errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// DNS resolution — retry (transient DNS failure)
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr.Temporary()
	}

	// Default: retry (unknown errors may be transient)
	return true
}

// backoffDelay returns exponential backoff with jitter.
func backoffDelay(attempt int) time.Duration {
	delay := baseDelay << uint(attempt) // 1s, 2s, 4s
	jitter := time.Duration(rand.Int64N(int64(maxJitter)))
	return delay + jitter
}
