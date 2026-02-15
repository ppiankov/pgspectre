package logging

import (
	"bytes"
	"log/slog"
	"testing"
)

func TestInit_Verbose(t *testing.T) {
	var buf bytes.Buffer
	Init(true, &buf)

	slog.Debug("test debug message")
	if buf.Len() == 0 {
		t.Error("expected debug message in verbose mode")
	}
}

func TestInit_Default(t *testing.T) {
	var buf bytes.Buffer
	Init(false, &buf)

	slog.Debug("should not appear")
	slog.Info("should not appear")
	if buf.Len() != 0 {
		t.Errorf("expected no output in default mode, got %q", buf.String())
	}
}

func TestInit_WarnVisible(t *testing.T) {
	var buf bytes.Buffer
	Init(false, &buf)

	slog.Warn("warning message")
	if buf.Len() == 0 {
		t.Error("expected warn message in default mode")
	}
}

func TestInit_NilOutput(t *testing.T) {
	// Should not panic with nil output (defaults to stderr)
	Init(false, nil)
}
