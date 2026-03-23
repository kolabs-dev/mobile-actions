package exec_test

import (
	"strings"
	"testing"

	"github.com/your-org/mobile-actions/internal/exec"
)

func TestRun_Success(t *testing.T) {
	if err := exec.Run("echo", "hello"); err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
}

func TestRun_Failure(t *testing.T) {
	err := exec.Run("false")
	if err == nil {
		t.Fatal("expected error for failing command")
	}
	if !strings.Contains(err.Error(), "false") {
		t.Fatalf("error should mention command name, got: %v", err)
	}
}

func TestRunOutput_Success(t *testing.T) {
	out, err := exec.RunOutput("echo", "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(out) != "hello world" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestRunOutput_Failure(t *testing.T) {
	_, err := exec.RunOutput("false")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_NonexistentCommand(t *testing.T) {
	err := exec.Run("this-command-does-not-exist-xyz")
	if err == nil {
		t.Fatal("expected error for missing command")
	}
}
