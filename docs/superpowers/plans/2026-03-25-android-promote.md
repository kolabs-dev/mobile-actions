# Android Promote Action Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a new `android-promote` composite action that promotes an already-uploaded Android app version between Play Store tracks, and rename `android/` → `android-upload/` and `ios/` → `ios-upload/` for consistency.

**Architecture:** Extract shared Play Store API logic (client, edit lifecycle, track update) into `internal/playstore`, refactor `android-upload` to use it, then build `android-promote` on top of the same shared package. The composite action wires the binary download pattern already established in `android-upload`.

**Tech Stack:** Go 1.26, `golang.org/x/oauth2/google` (JWT auth), `net/http/httptest` (unit tests), GitHub Actions composite actions

---

## File Structure

| File | Change | Responsibility |
|---|---|---|
| `internal/playstore/playstore.go` | Create | OAuth2 client, edit lifecycle, track promotion |
| `internal/playstore/playstore_test.go` | Create | Tests for all package functions (migrated from android-upload) |
| `cmd/android-upload/main.go` | Modify | Remove duplicated functions; use `playstore` package |
| `cmd/android-upload/main_test.go` | Modify | Remove migrated tests; keep dry-run and checkStatus tests |
| `cmd/android-promote/main.go` | Create | Input validation, dry-run, calls `playstore` functions |
| `cmd/android-promote/main_test.go` | Create | Unit tests for run(), validation, dry-run |
| `android-promote/action.yml` | Create | Composite action: binary download + run promote binary |
| `android-upload/action.yml` | Create (rename) | Moved from `android/action.yml` |
| `ios-upload/action.yml` | Create (rename) | Moved from `ios/action.yml` |
| `android/` | Delete | Replaced by `android-upload/` |
| `ios/` | Delete | Replaced by `ios-upload/` |
| `.github/workflows/build-binaries.yml` | Modify | Add `android-promote` to PROGRAMS list |
| `.github/workflows/test.yml` | Modify | Add android-promote to build step + dry-run integration step |
| `CLAUDE.md` | Modify | Update binary list to include `android-promote` |

---

## Task 1: Create `internal/playstore` package

**Files:**
- Create: `internal/playstore/playstore.go`
- Create: `internal/playstore/playstore_test.go`

- [ ] **Step 1.1: Write failing tests**

Create `internal/playstore/playstore_test.go`:

```go
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
```

- [ ] **Step 1.2: Run tests — verify they fail (package does not exist yet)**

```bash
go test ./internal/playstore/... -v -count=1
```
Expected: `cannot find package` or similar compilation error.

- [ ] **Step 1.3: Implement `internal/playstore/playstore.go`**

```go
package playstore

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"golang.org/x/oauth2/google"
)

var playBaseURL = "https://androidpublisher.googleapis.com/androidpublisher/v3/applications"

// NewClient creates an authenticated Google Play API HTTP client from a
// base64-encoded service account JSON. The JSON is decoded in memory — no
// temp file is written.
func NewClient(serviceAccountB64 string) (*http.Client, error) {
	data, err := base64.StdEncoding.DecodeString(serviceAccountB64)
	if err != nil {
		return nil, fmt.Errorf("decode service account: %w", err)
	}
	conf, err := google.JWTConfigFromJSON(data, "https://www.googleapis.com/auth/androidpublisher")
	if err != nil {
		return nil, fmt.Errorf("parse service account: %w", err)
	}
	return conf.Client(context.Background()), nil
}

// CreateEdit opens a new Play Store edit and returns its ID.
func CreateEdit(client *http.Client, packageName string) (string, error) {
	url := fmt.Sprintf("%s/%s/edits", playBaseURL, packageName)
	resp, err := client.Post(url, "application/json", bytes.NewBufferString("{}"))
	if err != nil {
		return "", fmt.Errorf("edits.insert: %w", err)
	}
	defer resp.Body.Close()
	if err := checkStatus(resp, 200); err != nil {
		return "", fmt.Errorf("edits.insert: %w", err)
	}
	var result struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.ID, nil
}

// PromoteTrack updates the given track to contain versionCode at 100% rollout.
// For alpha and beta tracks, userFraction is ignored by the Play Store API.
func PromoteTrack(client *http.Client, packageName, editID, track, versionCode string) error {
	url := fmt.Sprintf("%s/%s/edits/%s/tracks/%s", playBaseURL, packageName, editID, track)
	body := map[string]interface{}{
		"track": track,
		"releases": []map[string]interface{}{
			{
				"versionCodes": []string{versionCode},
				"status":       "completed",
				"userFraction": 1.0,
			},
		},
	}
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("edits.tracks.update: %w", err)
	}
	defer resp.Body.Close()
	return checkStatus(resp, 200)
}

// CommitEdit commits the edit. Retries with changesNotSentForReview=true if
// the API explicitly requests it (managed publishing accounts).
func CommitEdit(client *http.Client, packageName, editID string) error {
	url := fmt.Sprintf("%s/%s/edits/%s:commit", playBaseURL, packageName, editID)
	resp, err := client.Post(url, "application/json", nil)
	if err != nil {
		return fmt.Errorf("edits.commit: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode == 200 {
		return nil
	}

	if resp.StatusCode == 400 && strings.Contains(string(body), "changesNotSentForReview") {
		fmt.Println("retrying commit with changesNotSentForReview=true")
		retryURL := fmt.Sprintf("%s/%s/edits/%s:commit?changesNotSentForReview=true", playBaseURL, packageName, editID)
		resp2, err := client.Post(retryURL, "application/json", nil)
		if err != nil {
			return fmt.Errorf("edits.commit: %w", err)
		}
		defer resp2.Body.Close()
		return checkStatus(resp2, 200)
	}

	return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
}

func checkStatus(resp *http.Response, expected int) error {
	if resp.StatusCode == expected {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
}
```

- [ ] **Step 1.4: Run tests — verify they pass**

```bash
go test ./internal/playstore/... -v -count=1
```
Expected: all tests PASS.

- [ ] **Step 1.5: Commit**

```bash
git add internal/playstore/
git commit -m "feat: add internal/playstore shared package"
```

---

## Task 2: Refactor `cmd/android-upload` to use `internal/playstore`

**Files:**
- Modify: `cmd/android-upload/main.go`
- Modify: `cmd/android-upload/main_test.go`

- [ ] **Step 2.1: Rewrite `cmd/android-upload/main.go`**

Replace the file contents. The key changes:
- Remove: `googleHTTPClient`, `createEdit`, `updateTrack`, `commitEdit`, `playBaseURL`
- Import and use: `playstore.NewClient`, `playstore.CreateEdit`, `playstore.PromoteTrack`, `playstore.CommitEdit`
- Keep: `uploadArtifact`, `checkStatus`, `envOrDefault`, `dryRun`, `main`, `run`
- The service account is no longer written to a temp file — `playstore.NewClient` decodes in memory

```go
// cmd/android-upload/main.go
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/kolabs-dev/mobile-actions/internal/actions"
	"github.com/kolabs-dev/mobile-actions/internal/playstore"
)

var dryRun = flag.Bool("dry-run", false, "log what would be uploaded without actually uploading")

func main() {
	flag.Parse()
	if err := run(); err != nil {
		actions.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	serviceAccountB64 := os.Getenv("INPUT_SERVICE_ACCOUNT_JSON")
	packageName := os.Getenv("INPUT_PACKAGE_NAME")
	versionCode := os.Getenv("INPUT_VERSION_CODE")
	track := envOrDefault("INPUT_TRACK", "internal")
	buildType := strings.ToLower(envOrDefault("INPUT_BUILD_TYPE", "aab"))
	artifactPath := os.Getenv("INPUT_ARTIFACT_PATH")

	if packageName == "" || versionCode == "" || artifactPath == "" || serviceAccountB64 == "" {
		return fmt.Errorf("INPUT_PACKAGE_NAME, INPUT_VERSION_CODE, INPUT_ARTIFACT_PATH, and INPUT_SERVICE_ACCOUNT_JSON are required")
	}

	if *dryRun {
		fmt.Printf("[dry-run] would upload %s to Play Store package=%s track=%s version-code=%s\n",
			artifactPath, packageName, track, versionCode)
		return nil
	}

	client, err := playstore.NewClient(serviceAccountB64)
	if err != nil {
		return err
	}

	actions.Group("Uploading to Google Play")
	defer actions.EndGroup()

	editID, err := playstore.CreateEdit(client, packageName)
	if err != nil {
		return err
	}
	fmt.Printf("created edit: %s\n", editID)

	if err := uploadArtifact(client, packageName, editID, buildType, artifactPath); err != nil {
		return err
	}

	if err := playstore.PromoteTrack(client, packageName, editID, track, versionCode); err != nil {
		return err
	}

	if err := playstore.CommitEdit(client, packageName, editID); err != nil {
		return err
	}

	fmt.Println("upload complete")
	return nil
}

func uploadArtifact(client *http.Client, packageName, editID, buildType, artifactPath string) error {
	f, err := os.Open(artifactPath)
	if err != nil {
		return fmt.Errorf("open artifact: %w", err)
	}
	defer f.Close()

	var resource string
	var mimeType string
	switch buildType {
	case "aab":
		resource = "bundles"
		mimeType = "application/octet-stream"
	default:
		resource = "apks"
		mimeType = "application/vnd.android.package-archive"
	}

	url := fmt.Sprintf("https://androidpublisher.googleapis.com/upload/androidpublisher/v3/applications/%s/edits/%s/%s?uploadType=media",
		packageName, editID, resource)

	req, _ := http.NewRequest("POST", url, f)
	req.Header.Set("Content-Type", mimeType)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("edits.%s.upload: %w", resource, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 409 {
		body, _ := io.ReadAll(resp.Body)
		actions.Error(fmt.Sprintf("version code conflict: %s", string(body)))
		return fmt.Errorf("edits.%s.upload: version code already exists (HTTP 409)", resource)
	}
	return checkStatus(resp, 200)
}

func checkStatus(resp *http.Response, expected int) error {
	if resp.StatusCode == expected {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	actions.Error(fmt.Sprintf("API error %d: %s", resp.StatusCode, string(body)))
	return fmt.Errorf("unexpected status %d", resp.StatusCode)
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
```

- [ ] **Step 2.2: Rewrite `cmd/android-upload/main_test.go`**

Remove the tests that are now covered by `internal/playstore` (`TestCommitEdit_*`, `TestCreateEdit_*`, `TestUpdateTrack_Success`). Keep only tests for code still in this package.

```go
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
```

- [ ] **Step 2.3: Run all tests — verify they pass**

```bash
go test ./internal/... ./cmd/android-upload/... -v -count=1
```
Expected: all tests PASS.

- [ ] **Step 2.4: Commit**

```bash
git add cmd/android-upload/
git commit -m "refactor: android-upload uses internal/playstore"
```

---

## Task 3: Create `cmd/android-promote` binary

**Files:**
- Create: `cmd/android-promote/main.go`
- Create: `cmd/android-promote/main_test.go`

- [ ] **Step 3.1: Write failing tests**

`run()` calls `playstore.NewClient` which returns an OAuth2 client that can't be redirected to a test server. Test `run()` for: input validation, invalid track, and dry-run. API call shape is fully covered by `internal/playstore/playstore_test.go`.

Create `cmd/android-promote/main_test.go`:

```go
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
```

- [ ] **Step 3.2: Run tests — verify they fail (package does not exist yet)**

```bash
go test ./cmd/android-promote/... -v -count=1
```
Expected: `cannot find package` error.

- [ ] **Step 3.3: Implement `cmd/android-promote/main.go`**

```go
// cmd/android-promote/main.go
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kolabs-dev/mobile-actions/internal/actions"
	"github.com/kolabs-dev/mobile-actions/internal/playstore"
)

var dryRun = flag.Bool("dry-run", false, "log what would be promoted without actually calling the API")

func main() {
	flag.Parse()
	if err := run(); err != nil {
		actions.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	serviceAccountB64 := os.Getenv("INPUT_SERVICE_ACCOUNT_JSON")
	packageName := os.Getenv("INPUT_PACKAGE_NAME")
	versionCode := os.Getenv("INPUT_VERSION_CODE")
	targetTrack := os.Getenv("INPUT_TARGET_TRACK")

	if serviceAccountB64 == "" || packageName == "" || versionCode == "" || targetTrack == "" {
		return fmt.Errorf("INPUT_SERVICE_ACCOUNT_JSON, INPUT_PACKAGE_NAME, INPUT_VERSION_CODE, and INPUT_TARGET_TRACK are required")
	}

	switch targetTrack {
	case "alpha", "beta", "production":
		// valid
	default:
		return fmt.Errorf("target-track must be one of: alpha, beta, production")
	}

	if *dryRun {
		fmt.Printf("[dry-run] would promote version-code=%s to track=%s for package=%s\n",
			versionCode, targetTrack, packageName)
		return nil
	}

	client, err := playstore.NewClient(serviceAccountB64)
	if err != nil {
		return err
	}

	actions.Group("Promoting on Google Play")
	defer actions.EndGroup()

	editID, err := playstore.CreateEdit(client, packageName)
	if err != nil {
		return err
	}
	fmt.Printf("created edit: %s\n", editID)

	if err := playstore.PromoteTrack(client, packageName, editID, targetTrack, versionCode); err != nil {
		return err
	}

	if err := playstore.CommitEdit(client, packageName, editID); err != nil {
		return err
	}

	fmt.Printf("promoted version-code=%s to track=%s\n", versionCode, targetTrack)
	return nil
}
```

- [ ] **Step 3.4: Run tests — verify they pass**

```bash
go test ./cmd/android-promote/... -v -count=1
```
Expected: all tests PASS.

- [ ] **Step 3.5: Verify binary builds**

```bash
CGO_ENABLED=0 go build -o /tmp/android-promote-test ./cmd/android-promote
```
Expected: no errors.

- [ ] **Step 3.6: Run full test suite**

```bash
go test ./internal/... ./cmd/... -v -count=1
```
Expected: all tests PASS.

- [ ] **Step 3.7: Commit**

```bash
git add cmd/android-promote/
git commit -m "feat: add android-promote binary"
```

---

## Task 4: Create `android-promote/action.yml`

**Files:**
- Create: `android-promote/action.yml`

- [ ] **Step 4.1: Create the composite action**

```yaml
name: 'mobile-actions/android-promote'
description: 'Promote an Android app version to a Play Store track'

inputs:
  service-account-json:
    description: 'Base64-encoded Google service account JSON'
    required: true
  package-name:
    description: 'App package name (e.g. com.myapp)'
    required: true
  version-code:
    description: 'Integer version code to promote'
    required: true
  target-track:
    description: "Play Store track: 'alpha', 'beta', or 'production'"
    required: true

runs:
  using: 'composite'
  steps:
    - name: Download mobile-actions binaries
      shell: bash
      env:
        GH_TOKEN: ${{ github.token }}
      run: |
        RAW_ARCH=$(uname -m)
        ARCH=$( [ "$RAW_ARCH" = "x86_64" ] && echo "amd64" || echo "arm64" )
        OS=$(uname -s | tr '[:upper:]' '[:lower:]')
        VERSION=$GITHUB_ACTION_REF
        DEST=$RUNNER_TEMP/mobile-actions-bin
        mkdir -p $DEST

        gh release download "$VERSION" \
          --repo kolabs-dev/mobile-actions \
          --pattern "verify-checksums-${OS}-${ARCH}" \
          --pattern "checksums.txt" \
          --dir $DEST

        HASH_FILE=${{ github.action_path }}/../verify-checksums.sha256
        cd $DEST
        if [ "$OS" = "linux" ]; then
          grep "verify-checksums-${OS}-${ARCH}" "$HASH_FILE" | sha256sum --check
        else
          grep "verify-checksums-${OS}-${ARCH}" "$HASH_FILE" | shasum -a 256 --check
        fi
        chmod +x verify-checksums-${OS}-${ARCH}

        gh release download "$VERSION" \
          --repo kolabs-dev/mobile-actions \
          --pattern "*-${OS}-${ARCH}" \
          --dir $DEST \
          --clobber

        grep "${OS}-${ARCH}" checksums.txt > checksums-filtered.txt
        ./verify-checksums-${OS}-${ARCH} --file checksums-filtered.txt --dir $DEST
        chmod +x $DEST/*-${OS}-${ARCH}
        echo "MOBILE_ACTIONS_BIN=$DEST" >> $GITHUB_ENV
        echo "MOBILE_ACTIONS_OS_ARCH=${OS}-${ARCH}" >> $GITHUB_ENV

    - name: Promote on Play Store
      shell: bash
      env:
        INPUT_SERVICE_ACCOUNT_JSON: ${{ inputs.service-account-json }}
        INPUT_PACKAGE_NAME: ${{ inputs.package-name }}
        INPUT_VERSION_CODE: ${{ inputs.version-code }}
        INPUT_TARGET_TRACK: ${{ inputs.target-track }}
      run: $MOBILE_ACTIONS_BIN/android-promote-$MOBILE_ACTIONS_OS_ARCH
```

- [ ] **Step 4.2: Commit**

```bash
git add android-promote/
git commit -m "feat: add android-promote composite action"
```

---

## Task 5: Rename `android/` → `android-upload/` and `ios/` → `ios-upload/`

**Files:**
- Create: `android-upload/action.yml` (content of `android/action.yml`)
- Create: `ios-upload/action.yml` (content of `ios/action.yml`)
- Delete: `android/action.yml` and the `android/` directory
- Delete: `ios/action.yml` and the `ios/` directory

- [ ] **Step 5.1: Copy and rename action files**

```bash
mkdir -p android-upload ios-upload
cp android/action.yml android-upload/action.yml
cp ios/action.yml ios-upload/action.yml
```

- [ ] **Step 5.2: Update the `name:` field in both copied files**

In `android-upload/action.yml`, change line 1:
```yaml
name: 'mobile-actions/android-upload'
```

In `ios-upload/action.yml`, change line 1:
```yaml
name: 'mobile-actions/ios-upload'
```

- [ ] **Step 5.3: Delete old directories**

```bash
git rm -r android/ ios/
```

- [ ] **Step 5.4: Stage and commit**

```bash
git add android-upload/ ios-upload/
git commit -m "feat: rename android/ → android-upload/, ios/ → ios-upload/"
```

---

## Task 6: Update CI workflows and CLAUDE.md

**Files:**
- Modify: `.github/workflows/build-binaries.yml`
- Modify: `.github/workflows/test.yml`
- Modify: `CLAUDE.md`

- [ ] **Step 6.1: Add `android-promote` to `build-binaries.yml` PROGRAMS list**

In `.github/workflows/build-binaries.yml`, find the line:
```
PROGRAMS="android-build android-sign android-upload ios-setup-signing ios-teardown-signing ios-build ios-upload verify-checksums"
```

Change it to:
```
PROGRAMS="android-build android-sign android-upload android-promote ios-setup-signing ios-teardown-signing ios-build ios-upload verify-checksums"
```

- [ ] **Step 6.2: Update `test.yml` — add `android-promote` to build step and add dry-run integration step**

In `.github/workflows/test.yml`, find the `Build Go binaries` step in `integration-android`:
```yaml
      - name: Build Go binaries
        run: |
          for program in android-build android-sign android-upload; do
            CGO_ENABLED=0 go build -o /tmp/ma-bin/${program}-linux-amd64 ./cmd/$program
          done
```

Change it to:
```yaml
      - name: Build Go binaries
        run: |
          for program in android-build android-sign android-upload android-promote; do
            CGO_ENABLED=0 go build -o /tmp/ma-bin/${program}-linux-amd64 ./cmd/$program
          done
```

Then add the dry-run step after `Test android-upload (dry-run)`:
```yaml
      - name: Test android-promote (dry-run)
        env:
          INPUT_SERVICE_ACCOUNT_JSON: e30K  # base64("{}")
          INPUT_PACKAGE_NAME: com.example.testapp
          INPUT_VERSION_CODE: '1'
          INPUT_TARGET_TRACK: production
        run: /tmp/ma-bin/android-promote-linux-amd64 --dry-run
```

- [ ] **Step 6.3: Update `CLAUDE.md`**

Make the following changes to `CLAUDE.md`:

**1. Update the Project Overview action references** (near top of file):

Find:
```
- `kolabs-dev/mobile-actions/android-upload@v1` — Build, sign, and upload Android apps (AAB/APK) to Google Play Store
- `kolabs-dev/mobile-actions/ios-upload@v1` — Build, sign, and upload iOS apps (IPA) to App Store Connect/TestFlight
```

Replace with:
```
- `kolabs-dev/mobile-actions/android-upload@v1` — Build, sign, and upload Android apps (AAB/APK) to Google Play Store
- `kolabs-dev/mobile-actions/android-promote@v1` — Promote an uploaded Android app to a Play Store track (alpha/beta/production)
- `kolabs-dev/mobile-actions/ios-upload@v1` — Build, sign, and upload iOS apps (IPA) to App Store Connect/TestFlight
```

**2. Update the build all binaries command** in the Commands section:

Find:
```bash
for program in android-build android-sign android-upload ios-setup-signing ios-teardown-signing ios-build ios-upload verify-checksums; do
```

Change to:
```bash
for program in android-build android-sign android-upload android-promote ios-setup-signing ios-teardown-signing ios-build ios-upload verify-checksums; do
```

**3. Update the Architecture → Structure section** to add `android-promote` to the composite actions list and `cmd/android-promote/` to the Android pipeline description.

Find in the Architecture section:
```
Each step in the mobile CI pipeline is a **separate Go binary** in `cmd/`. They communicate via `$GITHUB_OUTPUT` and the filesystem. The composite actions in `android/action.yml` and `ios/action.yml` orchestrate the binaries.
```

Replace with:
```
Each step in the mobile CI pipeline is a **separate Go binary** in `cmd/`. They communicate via `$GITHUB_OUTPUT` and the filesystem. The composite actions in `android-upload/action.yml`, `android-promote/action.yml`, and `ios-upload/action.yml` orchestrate the binaries.
```

**4. Update the Shared packages section** to mention `internal/playstore/`:

Find:
```
- **`internal/actions/`** — GitHub Actions helpers
- **`internal/exec/`** — Subprocess wrapper
- **`internal/secrets/`** — Decodes base64 secrets
```

Add after `internal/secrets/`:
```
- **`internal/playstore/`** — Google Play API client and edit lifecycle helpers: `NewClient`, `CreateEdit`, `PromoteTrack`, `CommitEdit`.
```

**5. Add `android-promote` to the Android pipeline list** in the Architecture section:

Find:
```
1. **`android-build`** — ...
2. **`android-sign`** — ...
3. **`android-upload`** — ...
```

Add after item 3:
```
4. **`android-promote`** — OAuth2 JWT with service account JSON, calls Google Play API (create edit → update track → commit). Promotes an already-uploaded version to alpha, beta, or production. Supports `--dry-run`.
```

- [ ] **Step 6.4: Run full test suite one final time**

```bash
go test ./internal/... ./cmd/... -v -count=1
```
Expected: all tests PASS.

- [ ] **Step 6.5: Run vet**

```bash
go vet ./...
```
Expected: no output (no errors).

- [ ] **Step 6.6: Commit**

```bash
git add .github/workflows/build-binaries.yml .github/workflows/test.yml CLAUDE.md
git commit -m "chore: update CI and docs for android-promote"
```
