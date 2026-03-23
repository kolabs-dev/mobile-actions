package secrets_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/your-org/mobile-actions/internal/secrets"
)

func setupRunnerTemp(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("RUNNER_TEMP", dir)
}

func TestDecodeToFile_Success(t *testing.T) {
	setupRunnerTemp(t)
	content := []byte("my secret data")
	encoded := base64.StdEncoding.EncodeToString(content)

	path, cleanup, err := secrets.DecodeToFile(encoded, "test-*.bin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	got, _ := os.ReadFile(path)
	if string(got) != string(content) {
		t.Fatalf("content mismatch: got %q, want %q", got, content)
	}
}

func TestDecodeToFile_InvalidBase64(t *testing.T) {
	setupRunnerTemp(t)
	_, _, err := secrets.DecodeToFile("not-valid-base64!!!", "test-*.bin")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestDecodeToFile_CleanupRemovesFile(t *testing.T) {
	setupRunnerTemp(t)
	encoded := base64.StdEncoding.EncodeToString([]byte("data"))
	path, cleanup, err := secrets.DecodeToFile(encoded, "test-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expected file to be removed after cleanup")
	}
}

func TestDecodeToFile_WritesToRunnerTemp(t *testing.T) {
	setupRunnerTemp(t)
	encoded := base64.StdEncoding.EncodeToString([]byte("data"))
	path, cleanup, _ := secrets.DecodeToFile(encoded, "test-*.bin")
	defer cleanup()

	runnerTemp := os.Getenv("RUNNER_TEMP")
	expected := filepath.Join(runnerTemp, "mobile-actions")
	if filepath.Dir(path) != expected {
		t.Fatalf("file not in expected dir: %s", path)
	}
}

func TestDecodeToFile_MissingRunnerTemp(t *testing.T) {
	t.Setenv("RUNNER_TEMP", "")
	encoded := base64.StdEncoding.EncodeToString([]byte("data"))
	_, _, err := secrets.DecodeToFile(encoded, "test-*.bin")
	if err == nil {
		t.Fatal("expected error when RUNNER_TEMP is unset")
	}
}
