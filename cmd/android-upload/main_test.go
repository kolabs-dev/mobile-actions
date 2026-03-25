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
