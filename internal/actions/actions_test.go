package actions_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kolabs-dev/mobile-actions/internal/actions"
)

func TestSetOutput(t *testing.T) {
	f, _ := os.CreateTemp(t.TempDir(), "output-*")
	t.Setenv("GITHUB_OUTPUT", f.Name())

	actions.SetOutput("artifact-path", "/tmp/app.aab")

	content, _ := os.ReadFile(f.Name())
	if !strings.Contains(string(content), "artifact-path=/tmp/app.aab\n") {
		t.Fatalf("expected output file to contain key=value, got: %s", content)
	}
}

func TestAddMask(t *testing.T) {
	var out strings.Builder
	actions.SetWriter(&out)
	defer actions.ResetWriter()

	actions.AddMask("supersecret")
	if !strings.Contains(out.String(), "::add-mask::supersecret\n") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestGroup(t *testing.T) {
	var out strings.Builder
	actions.SetWriter(&out)
	defer actions.ResetWriter()

	actions.Group("my group")
	actions.EndGroup()

	if !strings.Contains(out.String(), "::group::my group\n") {
		t.Fatal("missing group command")
	}
	if !strings.Contains(out.String(), "::endgroup::\n") {
		t.Fatal("missing endgroup command")
	}
}

func TestError(t *testing.T) {
	var out strings.Builder
	actions.SetWriter(&out)
	defer actions.ResetWriter()

	actions.Error("something went wrong")
	if !strings.Contains(out.String(), "::error::something went wrong\n") {
		t.Fatalf("unexpected: %s", out.String())
	}
}

func TestSetOutputMissingEnv(t *testing.T) {
	t.Setenv("GITHUB_OUTPUT", "")
	err := actions.SetOutput("key", "value")
	if err == nil {
		t.Fatal("expected error when GITHUB_OUTPUT is unset")
	}
}

func TestSetOutputCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "output")
	f := filepath.Join(dir, "out.txt")
	t.Setenv("GITHUB_OUTPUT", f)
	if err := actions.SetOutput("k", "v"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
