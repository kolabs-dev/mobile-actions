// cmd/ios-teardown-signing/main_test.go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeFakeSecurity creates a fake `security` binary in a temp dir that exits 0
// and prepends the dir to PATH. PATH is automatically restored by t.Setenv.
func writeFakeSecurity(t *testing.T) {
	t.Helper()
	fakeDir := t.TempDir()
	fakeBin := filepath.Join(fakeDir, "security")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write fake security: %v", err)
	}
	t.Setenv("PATH", fakeDir+":"+os.Getenv("PATH"))
}

// writeStateFile writes a SigningState JSON to $RUNNER_TEMP/mobile-actions/signing-state.json.
func writeStateFile(t *testing.T, runnerTemp string, state SigningState) {
	t.Helper()
	dir := filepath.Join(runnerTemp, "mobile-actions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "signing-state.json"), data, 0600); err != nil {
		t.Fatalf("write state: %v", err)
	}
}

func TestRun_MissingStateFile(t *testing.T) {
	t.Setenv("RUNNER_TEMP", t.TempDir())

	err := run()
	if err == nil {
		t.Fatal("expected error when state file is missing, got nil")
	}
}

func TestRun_MalformedStateFile(t *testing.T) {
	runnerTemp := t.TempDir()
	t.Setenv("RUNNER_TEMP", runnerTemp)
	dir := filepath.Join(runnerTemp, "mobile-actions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "signing-state.json"), []byte("NOT JSON"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	err := run()
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestRun_RemovesProvisioningProfile(t *testing.T) {
	writeFakeSecurity(t)
	runnerTemp := t.TempDir()
	t.Setenv("RUNNER_TEMP", runnerTemp)

	profileDir := t.TempDir()
	profilePath := filepath.Join(profileDir, "test-profile.mobileprovision")
	if err := os.WriteFile(profilePath, []byte("fake profile"), 0644); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	writeStateFile(t, runnerTemp, SigningState{
		KeychainName:            "test.keychain-db",
		OriginalKeychains:       []string{},
		ProvisioningProfilePath: profilePath,
	})

	if err := run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, statErr := os.Stat(profilePath)
	if statErr == nil {
		t.Fatal("expected provisioning profile to be removed, but it still exists")
	} else if !os.IsNotExist(statErr) {
		t.Fatalf("unexpected error checking profile path: %v", statErr)
	}
}

func TestRun_ProfileAlreadyAbsent(t *testing.T) {
	writeFakeSecurity(t)
	runnerTemp := t.TempDir()
	t.Setenv("RUNNER_TEMP", runnerTemp)

	writeStateFile(t, runnerTemp, SigningState{
		KeychainName:            "test.keychain-db",
		OriginalKeychains:       []string{},
		ProvisioningProfilePath: filepath.Join(t.TempDir(), "already-gone.mobileprovision"),
	})

	if err := run(); err != nil {
		t.Fatalf("unexpected error when profile already absent: %v", err)
	}
}

func TestRun_EmptyProvisioningProfilePath(t *testing.T) {
	writeFakeSecurity(t)
	runnerTemp := t.TempDir()
	t.Setenv("RUNNER_TEMP", runnerTemp)

	writeStateFile(t, runnerTemp, SigningState{
		KeychainName:            "test.keychain-db",
		OriginalKeychains:       []string{},
		ProvisioningProfilePath: "",
	})

	if err := run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
