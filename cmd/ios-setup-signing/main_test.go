// cmd/ios-setup-signing/main_test.go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateKeychainPassword_Format(t *testing.T) {
	pass := generateKeychainPassword()
	if len(pass) != 32 {
		t.Fatalf("expected 32 chars, got %d: %q", len(pass), pass)
	}
	for _, c := range pass {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Fatalf("non-hex character %q in password %q", c, pass)
		}
	}
}

func TestGenerateKeychainPassword_Unique(t *testing.T) {
	p1 := generateKeychainPassword()
	p2 := generateKeychainPassword()
	if p1 == p2 {
		t.Fatalf("two consecutive passwords are identical: %q", p1)
	}
}

func TestWriteSigningState_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNNER_TEMP", dir)

	state := SigningState{
		KeychainName:            "mobile-actions-123.keychain-db",
		OriginalKeychains:       []string{"/Users/runner/Library/Keychains/login.keychain-db"},
		ProvisioningProfilePath: "/Users/runner/Library/MobileDevice/Provisioning Profiles/abc.mobileprovision",
	}

	if err := writeSigningState(state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stateFile := filepath.Join(dir, "mobile-actions", "signing-state.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("signing-state.json not created: %v", err)
	}

	var got SigningState
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("failed to parse signing-state.json: %v", err)
	}
	if got.KeychainName != state.KeychainName {
		t.Fatalf("KeychainName: got %q, want %q", got.KeychainName, state.KeychainName)
	}
	if got.ProvisioningProfilePath != state.ProvisioningProfilePath {
		t.Fatalf("ProvisioningProfilePath: got %q, want %q", got.ProvisioningProfilePath, state.ProvisioningProfilePath)
	}
	if len(got.OriginalKeychains) != 1 || got.OriginalKeychains[0] != state.OriginalKeychains[0] {
		t.Fatalf("OriginalKeychains: got %v, want %v", got.OriginalKeychains, state.OriginalKeychains)
	}
}

func TestWriteSigningState_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNNER_TEMP", dir)

	state := SigningState{KeychainName: "test.keychain-db"}
	if err := writeSigningState(state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stateDir := filepath.Join(dir, "mobile-actions")
	info, err := os.Stat(stateDir)
	if err != nil {
		t.Fatalf("expected directory to be created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected a directory")
	}
}

func TestCopyFile_Success(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.dat")
	dst := filepath.Join(dir, "dest.dat")
	content := []byte("binary content \x00\x01\x02")
	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("failed to read dest: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("content mismatch: got %v, want %v", got, content)
	}
}

func TestCopyFile_SourceMissing(t *testing.T) {
	dir := t.TempDir()
	err := copyFile(filepath.Join(dir, "no-such-file"), filepath.Join(dir, "dest"))
	if err == nil {
		t.Fatal("expected error for missing source, got nil")
	}
}
