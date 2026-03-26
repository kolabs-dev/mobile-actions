package main

import (
	"strings"
	"testing"
)

func TestRun_MissingInputs(t *testing.T) {
	for _, key := range []string{
		"INPUT_SERVICE_ACCOUNT_JSON",
		"INPUT_PACKAGE_NAME",
		"INPUT_VERSION_CODE",
		"INPUT_TARGET_TRACK",
	} {
		t.Setenv(key, "")
	}

	err := run()
	if err == nil {
		t.Fatal("expected error for missing inputs, got nil")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("expected 'required' in error, got: %v", err)
	}
}

func TestRun_InvalidTargetTrack(t *testing.T) {
	t.Setenv("INPUT_SERVICE_ACCOUNT_JSON", "ZmFrZQ==")
	t.Setenv("INPUT_PACKAGE_NAME", "com.example.app")
	t.Setenv("INPUT_VERSION_CODE", "1")
	t.Setenv("INPUT_TARGET_TRACK", "internal")

	err := run()
	if err == nil {
		t.Fatal("expected error for invalid track, got nil")
	}
	if !strings.Contains(err.Error(), "target-track must be one of: alpha, beta, production") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_DryRun(t *testing.T) {
	t.Setenv("INPUT_SERVICE_ACCOUNT_JSON", "ZmFrZQ==")
	t.Setenv("INPUT_PACKAGE_NAME", "com.example.app")
	t.Setenv("INPUT_VERSION_CODE", "42")
	t.Setenv("INPUT_TARGET_TRACK", "production")

	orig := *dryRun
	*dryRun = true
	defer func() { *dryRun = orig }()

	if err := run(); err != nil {
		t.Fatalf("unexpected error in dry-run: %v", err)
	}
}
