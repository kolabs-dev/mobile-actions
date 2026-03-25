// cmd/ios-upload/main_test.go
package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// skipOnMacOS skips tests that inject HOME to redirect os.UserHomeDir(),
// since os.UserHomeDir() on macOS uses getpwuid_r (ignores $HOME).
func skipOnMacOS(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "darwin" {
		t.Skip("os.UserHomeDir() ignores $HOME on macOS — test only provides isolation on Linux")
	}
}

func TestInstallP8Key_WritesCorrectContent(t *testing.T) {
	skipOnMacOS(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	keyContent := []byte("-----BEGIN PRIVATE KEY-----\nfakekey\n-----END PRIVATE KEY-----\n")
	keyB64 := base64.StdEncoding.EncodeToString(keyContent)
	keyID := "ABC123DEF"

	path, cleanup, err := installP8Key(keyB64, keyID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	wantPath := filepath.Join(home, ".appstoreconnect", "private_keys", "AuthKey_"+keyID+".p8")
	if path != wantPath {
		t.Fatalf("got path %s, want %s", path, wantPath)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read key file: %v", err)
	}
	if string(data) != string(keyContent) {
		t.Fatalf("content mismatch: got %q, want %q", data, keyContent)
	}
}

func TestInstallP8Key_CleanupRemovesFile(t *testing.T) {
	skipOnMacOS(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	keyContent := []byte("fake key data")
	keyB64 := base64.StdEncoding.EncodeToString(keyContent)

	path, cleanup, err := installP8Key(keyB64, "MYKEY01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist before cleanup: %v", err)
	}

	cleanup()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expected file to be removed after cleanup")
	}
}

func TestInstallP8Key_InvalidBase64(t *testing.T) {
	skipOnMacOS(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, _, err := installP8Key("!!!not-valid-base64!!!", "MYKEY01")
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
	if !strings.Contains(err.Error(), "decode p8 key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInstallP8Key_FilePermissions(t *testing.T) {
	skipOnMacOS(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	keyContent := []byte("key data")
	keyB64 := base64.StdEncoding.EncodeToString(keyContent)

	path, cleanup, err := installP8Key(keyB64, "PERMKEY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("expected mode 0600, got %v", info.Mode().Perm())
	}
}
