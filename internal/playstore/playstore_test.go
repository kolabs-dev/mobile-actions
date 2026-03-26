package playstore

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testServiceAccountB64 generates a valid base64-encoded service account JSON
// with a real RSA private key, suitable for NewClient tests.
func testServiceAccountB64(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	privDER := x509.MarshalPKCS1PrivateKey(key)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privDER})
	sa := map[string]string{
		"type":                        "service_account",
		"project_id":                  "test-project",
		"private_key_id":              "key1",
		"private_key":                 string(privPEM),
		"client_email":                "test@test-project.iam.gserviceaccount.com",
		"client_id":                   "123456789",
		"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
		"token_uri":                   "https://oauth2.googleapis.com/token",
	}
	data, _ := json.Marshal(sa)
	return base64.StdEncoding.EncodeToString(data)
}

func TestNewClient_InvalidBase64(t *testing.T) {
	_, err := NewClient("not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

func TestNewClient_InvalidJSON(t *testing.T) {
	_, err := NewClient(base64.StdEncoding.EncodeToString([]byte("not json")))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestNewClient_ValidServiceAccount(t *testing.T) {
	_, err := NewClient(testServiceAccountB64(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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

	id, err := CreateEdit(srv.Client(), "com.example.app")
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

	_, err := CreateEdit(srv.Client(), "com.example.app")
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestPromoteTrack_RequestBody(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/tracks/production") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	orig := playBaseURL
	playBaseURL = srv.URL
	defer func() { playBaseURL = orig }()

	err := PromoteTrack(srv.Client(), "com.example.app", "edit-123", "production", "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(gotBody, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["track"] != "production" {
		t.Errorf("track = %v, want production", body["track"])
	}
	releases := body["releases"].([]interface{})
	release := releases[0].(map[string]interface{})
	if release["status"] != "completed" {
		t.Errorf("status = %v, want completed", release["status"])
	}
	if release["userFraction"] != 1.0 {
		t.Errorf("userFraction = %v, want 1.0", release["userFraction"])
	}
	vcs := release["versionCodes"].([]interface{})
	if vcs[0] != "42" {
		t.Errorf("versionCodes[0] = %v, want 42", vcs[0])
	}
}

func TestPromoteTrack_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`forbidden`))
	}))
	defer srv.Close()

	orig := playBaseURL
	playBaseURL = srv.URL
	defer func() { playBaseURL = orig }()

	err := PromoteTrack(srv.Client(), "com.example.app", "edit-123", "production", "42")
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
}

func TestCommitEdit_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "changesNotSentForReview") {
			t.Error("changesNotSentForReview should not be set on first attempt")
		}
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	orig := playBaseURL
	playBaseURL = srv.URL
	defer func() { playBaseURL = orig }()

	if err := CommitEdit(srv.Client(), "com.example", "edit123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCommitEdit_RetryWithFlagWhenRequired(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":{"code":400,"message":"The edit must be committed with changesNotSentForReview set to true.","status":"INVALID_ARGUMENT"}}`))
			return
		}
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

	if err := CommitEdit(srv.Client(), "com.example", "edit123"); err != nil {
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

	if err := CommitEdit(srv.Client(), "com.example", "edit123"); err == nil {
		t.Fatal("expected error but got nil")
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}
