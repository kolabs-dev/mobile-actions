package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCommitEdit_SuccessWithoutFlag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "changesNotSentForReview") {
			t.Error("changesNotSentForReview should not be set on first attempt")
		}
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	// Temporarily override playBaseURL to point to test server.
	orig := playBaseURL
	playBaseURL = srv.URL
	defer func() { playBaseURL = orig }()

	if err := commitEdit(srv.Client(), "com.example", "edit123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCommitEdit_RetryWithFlagWhenRequired(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			// First attempt: return 400 mentioning changesNotSentForReview
			w.WriteHeader(400)
			w.Write([]byte(`{"error":{"code":400,"message":"The edit must be committed with changesNotSentForReview set to true.","status":"INVALID_ARGUMENT"}}`))
			return
		}
		// Second attempt: expect flag to be set
		if !strings.Contains(r.URL.RawQuery, "changesNotSentForReview=true") {
			t.Error("expected changesNotSentForReview=true on retry")
		}
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	orig := playBaseURL
	playBaseURL = srv.URL
	defer func() { playBaseURL = orig }()

	if err := commitEdit(srv.Client(), "com.example", "edit123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestCommitEdit_NoRetryOnUnrelatedError(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(400)
		w.Write([]byte(`{"error":{"code":400,"message":"Package not found.","status":"NOT_FOUND"}}`))
	}))
	defer srv.Close()

	orig := playBaseURL
	playBaseURL = srv.URL
	defer func() { playBaseURL = orig }()

	if err := commitEdit(srv.Client(), "com.example", "edit123"); err == nil {
		t.Fatal("expected error but got nil")
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}

func TestCreateEdit_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/edits") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"edit-abc"}`))
	}))
	defer srv.Close()

	orig := playBaseURL
	playBaseURL = srv.URL
	defer func() { playBaseURL = orig }()

	id, err := createEdit(srv.Client(), "com.example.app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "edit-abc" {
		t.Fatalf("got id %q, want %q", id, "edit-abc")
	}
}

func TestCreateEdit_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`internal server error`))
	}))
	defer srv.Close()

	orig := playBaseURL
	playBaseURL = srv.URL
	defer func() { playBaseURL = orig }()

	_, err := createEdit(srv.Client(), "com.example.app")
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestUpdateTrack_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/tracks/internal") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	orig := playBaseURL
	playBaseURL = srv.URL
	defer func() { playBaseURL = orig }()

	err := updateTrack(srv.Client(), "com.example.app", "edit-123", "internal", "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

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
	t.Setenv("INPUT_ARTIFACT_PATH", "/tmp/fake.aab")
	t.Setenv("INPUT_SERVICE_ACCOUNT_JSON", "ZmFrZQ==") // base64("fake")
	t.Setenv("RUNNER_TEMP", t.TempDir())

	orig := *dryRun
	*dryRun = true
	defer func() { *dryRun = orig }()

	if err := run(); err != nil {
		t.Fatalf("unexpected error in dry-run: %v", err)
	}
}
