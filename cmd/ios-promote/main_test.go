package main

import (
	"strings"
	"testing"
)

func TestRun_MissingInputs(t *testing.T) {
	for _, key := range []string{
		"INPUT_APP_STORE_CONNECT_KEY",
		"INPUT_APP_STORE_CONNECT_KEY_ID",
		"INPUT_APP_STORE_CONNECT_ISSUER_ID",
		"INPUT_BUNDLE_ID",
		"INPUT_VERSION",
		"INPUT_BUILD_NUMBER",
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

func TestRun_DryRun(t *testing.T) {
	t.Setenv("INPUT_APP_STORE_CONNECT_KEY", "ZmFrZQ==")
	t.Setenv("INPUT_APP_STORE_CONNECT_KEY_ID", "ABCD123456")
	t.Setenv("INPUT_APP_STORE_CONNECT_ISSUER_ID", "issuer-id")
	t.Setenv("INPUT_BUNDLE_ID", "com.example.app")
	t.Setenv("INPUT_VERSION", "1.2.3")
	t.Setenv("INPUT_BUILD_NUMBER", "42")

	orig := *dryRun
	*dryRun = true
	defer func() { *dryRun = orig }()

	if err := run(); err != nil {
		t.Fatalf("unexpected error in dry-run: %v", err)
	}
}
