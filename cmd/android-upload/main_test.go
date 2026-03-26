package main

import (
	"net/http"
	"path/filepath"
	"testing"
)

func TestCheckStatus_Match(t *testing.T) {
	resp := &http.Response{
		StatusCode: 200,
		Body:       http.NoBody,
	}
	if err := checkStatus(resp, 200); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckStatus_Mismatch(t *testing.T) {
	resp := &http.Response{
		StatusCode: 404,
		Body:       http.NoBody,
	}
	if err := checkStatus(resp, 200); err == nil {
		t.Fatal("expected error for status mismatch, got nil")
	}
}

func TestRun_DryRun(t *testing.T) {
	t.Setenv("INPUT_PACKAGE_NAME", "com.example.app")
	t.Setenv("INPUT_VERSION_CODE", "42")
	t.Setenv("INPUT_ARTIFACT_PATH", filepath.Join(t.TempDir(), "fake.aab"))
	t.Setenv("INPUT_SERVICE_ACCOUNT_JSON", "ZmFrZQ==") // base64("fake")

	orig := *dryRun
	*dryRun = true
	defer func() { *dryRun = orig }()

	if err := run(); err != nil {
		t.Fatalf("unexpected error in dry-run: %v", err)
	}
}
