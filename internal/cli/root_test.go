package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestAuditCmd_InvalidDBURL_ErrorIsGraceful(t *testing.T) {
	cmd := newRootCmd(BuildInfo{Version: "test"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"audit", "--db-url", "not-a-url", "--format", "json"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected connection error")
	}
	if strings.Contains(err.Error(), "connect: connect:") {
		t.Fatalf("unexpected duplicated prefix: %v", err)
	}
	if !strings.Contains(err.Error(), "connect: cannot parse `not-a-url`") {
		t.Fatalf("unexpected error: %v", err)
	}
}
