# ios-promote Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an `ios-promote` composite action and Go binary that submits a TestFlight-processed build for App Store review via the App Store Connect REST API.

**Architecture:** A new `cmd/ios-promote` binary reads inputs from `INPUT_*` env vars and calls `appstore.PromoteBuild()`, which orchestrates 5 ASC REST API calls (resolve app ID → find build → find/create App Store version → attach build → submit for review). JWT auth (RS256) is implemented using Go stdlib with no new external dependencies. Mirrors `android-promote` exactly in structure.

**Tech Stack:** Go stdlib (`crypto/rsa`, `crypto/x509`, `encoding/pem`, `net/http`), `net/http/httptest` for tests. No new external dependencies.

---

## File Map

| File | Action | Purpose |
|---|---|---|
| `internal/appstore/appstore.go` | Create | Client struct, JWT signing, all ASC API functions, PromoteBuild orchestrator |
| `internal/appstore/appstore_test.go` | Create | Unit tests using httptest.Server |
| `cmd/ios-promote/main.go` | Create | Binary entry point — reads INPUT_* env vars, calls PromoteBuild |
| `cmd/ios-promote/main_test.go` | Create | Tests for run(): missing inputs, dry-run |
| `ios-promote/action.yml` | Create | Composite action — binary download + single run step |
| `.github/workflows/build-binaries.yml` | Modify | Add `ios-promote` to PROGRAMS list (line 23) |

---

## Task 1: JWT signing and NewClient

**Files:**
- Create: `internal/appstore/appstore_test.go`
- Create: `internal/appstore/appstore.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/appstore/appstore_test.go`:

```go
package appstore

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testKeyB64 generates a base64-encoded PKCS#8 PEM RSA private key (.p8 format).
func testKeyB64(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return base64.StdEncoding.EncodeToString(pemBlock)
}

// testClientWithServer creates a Client using a generated RSA key and the given test server's HTTP client.
func testClientWithServer(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return &Client{
		keyID:      "test-key",
		issuerID:   "test-issuer",
		privateKey: key,
		httpClient: srv.Client(),
	}
}

func TestNewClient_InvalidBase64(t *testing.T) {
	_, err := NewClient("not-valid-base64!!!", "kid", "iss")
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

func TestNewClient_NoPEMBlock(t *testing.T) {
	_, err := NewClient(base64.StdEncoding.EncodeToString([]byte("not pem")), "kid", "iss")
	if err == nil {
		t.Fatal("expected error for missing PEM block, got nil")
	}
}

func TestNewClient_Valid(t *testing.T) {
	c, err := NewClient(testKeyB64(t), "kid1", "iss1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.keyID != "kid1" {
		t.Errorf("keyID = %q, want kid1", c.keyID)
	}
	if c.issuerID != "iss1" {
		t.Errorf("issuerID = %q, want iss1", c.issuerID)
	}
}

func TestGenerateJWT_Structure(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	c := &Client{keyID: "kid1", issuerID: "iss1", privateKey: key, httpClient: http.DefaultClient}

	token, err := c.generateJWT()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT parts, got %d", len(parts))
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	var header map[string]string
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if header["alg"] != "RS256" {
		t.Errorf("alg = %q, want RS256", header["alg"])
	}
	if header["kid"] != "kid1" {
		t.Errorf("kid = %q, want kid1", header["kid"])
	}
	if header["typ"] != "JWT" {
		t.Errorf("typ = %q, want JWT", header["typ"])
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["iss"] != "iss1" {
		t.Errorf("iss = %v, want iss1", payload["iss"])
	}
	if payload["aud"] != "appstoreconnect-v1" {
		t.Errorf("aud = %v, want appstoreconnect-v1", payload["aud"])
	}
	exp := int64(payload["exp"].(float64))
	iat := int64(payload["iat"].(float64))
	if exp-iat != 1200 {
		t.Errorf("exp-iat = %d, want 1200", exp-iat)
	}
}
```

- [ ] **Step 2: Run tests to confirm compile failure**

```bash
go test ./internal/appstore/... -v -count=1
```

Expected: compile error — `appstore` package and `Client` type do not exist yet.

- [ ] **Step 3: Implement NewClient and JWT signing**

Create `internal/appstore/appstore.go`:

```go
package appstore

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"time"
)

var ascBaseURL = "https://api.appstoreconnect.apple.com"

// Client holds App Store Connect API credentials and an HTTP client.
type Client struct {
	keyID      string
	issuerID   string
	privateKey *rsa.PrivateKey
	httpClient *http.Client
}

// NewClient creates a Client from a base64-encoded .p8 API key.
func NewClient(keyB64, keyID, issuerID string) (*Client, error) {
	keyPEM, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("decode p8 key: %w", err)
	}
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, fmt.Errorf("p8 key: no PEM block found")
	}
	raw, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse p8 key: %w", err)
	}
	rsaKey, ok := raw.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("p8 key: not an RSA key")
	}
	return &Client{
		keyID:      keyID,
		issuerID:   issuerID,
		privateKey: rsaKey,
		httpClient: http.DefaultClient,
	}, nil
}

// generateJWT creates a signed RS256 JWT for the ASC API.
func (c *Client) generateJWT() (string, error) {
	now := time.Now().Unix()
	headerJSON, _ := json.Marshal(map[string]string{
		"alg": "RS256",
		"kid": c.keyID,
		"typ": "JWT",
	})
	payloadJSON, _ := json.Marshal(map[string]interface{}{
		"iss": c.issuerID,
		"iat": now,
		"exp": now + 1200,
		"aud": "appstoreconnect-v1",
	})
	h64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	p64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	si := h64 + "." + p64

	digest := sha256.Sum256([]byte(si))
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign JWT: %w", err)
	}
	return si + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// do attaches a fresh JWT Authorization header and executes the request.
func (c *Client) do(req *http.Request) (*http.Response, error) {
	token, err := c.generateJWT()
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return c.httpClient.Do(req)
}

func checkStatus(resp *http.Response, expected int) error {
	if resp.StatusCode == expected {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
}

// PromoteBuild is a placeholder — implemented in Task 8.
func PromoteBuild(c *Client, bundleID, version, buildNumber string) error {
	return fmt.Errorf("not implemented")
}

// Silence unused import warnings until later tasks add uses.
var _ = bytes.NewReader
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./internal/appstore/... -v -count=1 -run "TestNewClient|TestGenerateJWT"
```

Expected: PASS for all four tests.

- [ ] **Step 5: Commit**

```bash
git add internal/appstore/appstore.go internal/appstore/appstore_test.go
git commit -m "feat: add internal/appstore package with JWT signing and NewClient"
```

---

## Task 2: resolveAppID

**Files:**
- Modify: `internal/appstore/appstore_test.go`
- Modify: `internal/appstore/appstore.go`

- [ ] **Step 1: Add tests for resolveAppID**

Append to `internal/appstore/appstore_test.go`:

```go
func TestResolveAppID_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Query().Get("filter[bundleId]") != "com.example.app" {
			t.Errorf("unexpected bundleId query param: %s", r.URL.Query().Get("filter[bundleId]"))
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"data":[{"id":"app-123","type":"apps"}]}`))
	}))
	defer srv.Close()

	orig := ascBaseURL
	ascBaseURL = srv.URL
	defer func() { ascBaseURL = orig }()

	id, err := resolveAppID(testClientWithServer(t, srv), "com.example.app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "app-123" {
		t.Errorf("id = %q, want app-123", id)
	}
}

func TestResolveAppID_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	orig := ascBaseURL
	ascBaseURL = srv.URL
	defer func() { ascBaseURL = orig }()

	_, err := resolveAppID(testClientWithServer(t, srv), "com.missing.app")
	if err == nil {
		t.Fatal("expected error for missing app, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestResolveAppID_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`unauthorized`))
	}))
	defer srv.Close()

	orig := ascBaseURL
	ascBaseURL = srv.URL
	defer func() { ascBaseURL = orig }()

	_, err := resolveAppID(testClientWithServer(t, srv), "com.example.app")
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
}
```

- [ ] **Step 2: Run tests to confirm failure**

```bash
go test ./internal/appstore/... -v -count=1 -run "TestResolveAppID"
```

Expected: compile error — `resolveAppID` undefined.

- [ ] **Step 3: Implement resolveAppID**

Add to `internal/appstore/appstore.go` (remove the `var _ = bytes.NewReader` placeholder line, add the real function):

```go
// resolveAppID returns the ASC app ID for the given bundle ID.
func resolveAppID(c *Client, bundleID string) (string, error) {
	url := fmt.Sprintf("%s/v1/apps?filter[bundleId]=%s", ascBaseURL, bundleID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("resolveAppID: %w", err)
	}
	resp, err := c.do(req)
	if err != nil {
		return "", fmt.Errorf("resolveAppID: %w", err)
	}
	defer resp.Body.Close()
	if err := checkStatus(resp, 200); err != nil {
		return "", fmt.Errorf("resolveAppID: %w", err)
	}
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("resolveAppID: decode: %w", err)
	}
	if len(result.Data) == 0 {
		return "", fmt.Errorf("app with bundle ID %q not found", bundleID)
	}
	return result.Data[0].ID, nil
}
```

Also remove the `var _ = bytes.NewReader` placeholder (it's no longer needed once `bytes` is used in later tasks; keep `bytes` import for now by leaving the `_ =` line until Task 5 adds real uses).

- [ ] **Step 4: Run tests**

```bash
go test ./internal/appstore/... -v -count=1 -run "TestResolveAppID"
```

Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/appstore/appstore.go internal/appstore/appstore_test.go
git commit -m "feat: add resolveAppID to appstore package"
```

---

## Task 3: findBuild

**Files:**
- Modify: `internal/appstore/appstore_test.go`
- Modify: `internal/appstore/appstore.go`

- [ ] **Step 1: Add tests for findBuild**

Append to `internal/appstore/appstore_test.go`:

```go
func TestFindBuild_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		q := r.URL.Query()
		if q.Get("filter[app]") != "app-1" {
			t.Errorf("unexpected filter[app]: %s", q.Get("filter[app]"))
		}
		if q.Get("filter[version]") != "42" {
			t.Errorf("unexpected filter[version]: %s", q.Get("filter[version]"))
		}
		if q.Get("filter[preReleaseVersion.version]") != "1.2.3" {
			t.Errorf("unexpected filter[preReleaseVersion.version]: %s", q.Get("filter[preReleaseVersion.version]"))
		}
		if q.Get("filter[processingState]") != "VALID" {
			t.Errorf("unexpected processingState filter: %s", q.Get("filter[processingState]"))
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"data":[{"id":"build-abc","type":"builds"}]}`))
	}))
	defer srv.Close()

	orig := ascBaseURL
	ascBaseURL = srv.URL
	defer func() { ascBaseURL = orig }()

	id, err := findBuild(testClientWithServer(t, srv), "app-1", "1.2.3", "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "build-abc" {
		t.Errorf("id = %q, want build-abc", id)
	}
}

func TestFindBuild_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	orig := ascBaseURL
	ascBaseURL = srv.URL
	defer func() { ascBaseURL = orig }()

	_, err := findBuild(testClientWithServer(t, srv), "app-1", "1.2.3", "42")
	if err == nil {
		t.Fatal("expected error for missing build, got nil")
	}
	if !strings.Contains(err.Error(), "not found or still processing") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestFindBuild_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`forbidden`))
	}))
	defer srv.Close()

	orig := ascBaseURL
	ascBaseURL = srv.URL
	defer func() { ascBaseURL = orig }()

	_, err := findBuild(testClientWithServer(t, srv), "app-1", "1.2.3", "42")
	if err == nil {
		t.Fatal("expected error for 403, got nil")
	}
}
```

- [ ] **Step 2: Run tests to confirm failure**

```bash
go test ./internal/appstore/... -v -count=1 -run "TestFindBuild"
```

Expected: compile error — `findBuild` undefined.

- [ ] **Step 3: Implement findBuild**

Add to `internal/appstore/appstore.go`:

```go
// findBuild returns the build ID for a specific version + build number.
// The build must be fully processed (processingState=VALID).
func findBuild(c *Client, appID, version, buildNumber string) (string, error) {
	url := fmt.Sprintf("%s/v1/builds?filter[app]=%s&filter[version]=%s&filter[preReleaseVersion.version]=%s&filter[processingState]=VALID",
		ascBaseURL, appID, buildNumber, version)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("findBuild: %w", err)
	}
	resp, err := c.do(req)
	if err != nil {
		return "", fmt.Errorf("findBuild: %w", err)
	}
	defer resp.Body.Close()
	if err := checkStatus(resp, 200); err != nil {
		return "", fmt.Errorf("findBuild: %w", err)
	}
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("findBuild: decode: %w", err)
	}
	if len(result.Data) == 0 {
		return "", fmt.Errorf("build %s(%s) not found or still processing", version, buildNumber)
	}
	return result.Data[0].ID, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/appstore/... -v -count=1 -run "TestFindBuild"
```

Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/appstore/appstore.go internal/appstore/appstore_test.go
git commit -m "feat: add findBuild to appstore package"
```

---

## Task 4: findOrCreateVersion

**Files:**
- Modify: `internal/appstore/appstore_test.go`
- Modify: `internal/appstore/appstore.go`

- [ ] **Step 1: Add tests for findOrCreateVersion**

Append to `internal/appstore/appstore_test.go`:

```go
func TestFindOrCreateVersion_Create(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/appStoreVersions" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
			return
		}
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		data := body["data"].(map[string]interface{})
		attrs := data["attributes"].(map[string]interface{})
		if attrs["platform"] != "IOS" {
			t.Errorf("platform = %v, want IOS", attrs["platform"])
		}
		if attrs["versionString"] != "1.2.3" {
			t.Errorf("versionString = %v, want 1.2.3", attrs["versionString"])
		}
		w.WriteHeader(201)
		w.Write([]byte(`{"data":{"id":"ver-1","type":"appStoreVersions"}}`))
	}))
	defer srv.Close()

	orig := ascBaseURL
	ascBaseURL = srv.URL
	defer func() { ascBaseURL = orig }()

	id, err := findOrCreateVersion(testClientWithServer(t, srv), "app-1", "1.2.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "ver-1" {
		t.Errorf("id = %q, want ver-1", id)
	}
}

func TestFindOrCreateVersion_ReuseExisting(t *testing.T) {
	// First request: POST → 409 (already exists)
	// Second request: GET existing version → PREPARE_FOR_SUBMISSION
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			// POST create → conflict
			w.WriteHeader(409)
			w.Write([]byte(`{"errors":[{"code":"VERSION_ALREADY_CREATED_WITH_SAME_VERSION_STRING"}]}`))
			return
		}
		// GET existing version
		if r.Method != http.MethodGet {
			t.Errorf("call 2: expected GET, got %s", r.Method)
		}
		if r.URL.Query().Get("filter[versionString]") != "1.2.3" {
			t.Errorf("call 2: unexpected versionString filter: %s", r.URL.Query().Get("filter[versionString]"))
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"data":[{"id":"ver-existing","type":"appStoreVersions","attributes":{"appStoreState":"PREPARE_FOR_SUBMISSION"}}]}`))
	}))
	defer srv.Close()

	orig := ascBaseURL
	ascBaseURL = srv.URL
	defer func() { ascBaseURL = orig }()

	id, err := findOrCreateVersion(testClientWithServer(t, srv), "app-1", "1.2.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "ver-existing" {
		t.Errorf("id = %q, want ver-existing", id)
	}
}

func TestFindOrCreateVersion_ExistingNonEditable(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(409)
			w.Write([]byte(`{}`))
			return
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"data":[{"id":"ver-2","type":"appStoreVersions","attributes":{"appStoreState":"WAITING_FOR_REVIEW"}}]}`))
	}))
	defer srv.Close()

	orig := ascBaseURL
	ascBaseURL = srv.URL
	defer func() { ascBaseURL = orig }()

	_, err := findOrCreateVersion(testClientWithServer(t, srv), "app-1", "1.2.3")
	if err == nil {
		t.Fatal("expected error for non-editable state, got nil")
	}
	if !strings.Contains(err.Error(), "WAITING_FOR_REVIEW") {
		t.Errorf("expected state name in error, got: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to confirm failure**

```bash
go test ./internal/appstore/... -v -count=1 -run "TestFindOrCreateVersion"
```

Expected: compile error — `findOrCreateVersion` undefined.

- [ ] **Step 3: Implement findOrCreateVersion and findExistingVersion**

Add to `internal/appstore/appstore.go`:

```go
// findOrCreateVersion returns the App Store version ID for the given version string.
// Creates it if it does not exist. If a version in PREPARE_FOR_SUBMISSION state already
// exists, it is reused. Any other existing state is an error.
func findOrCreateVersion(c *Client, appID, version string) (string, error) {
	createURL := fmt.Sprintf("%s/v1/appStoreVersions", ascBaseURL)
	body := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "appStoreVersions",
			"attributes": map[string]interface{}{
				"platform":      "IOS",
				"versionString": version,
			},
			"relationships": map[string]interface{}{
				"app": map[string]interface{}{
					"data": map[string]interface{}{
						"type": "apps",
						"id":   appID,
					},
				},
			},
		},
	}
	data, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, createURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("findOrCreateVersion: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(req)
	if err != nil {
		return "", fmt.Errorf("findOrCreateVersion: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 201 {
		var result struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return "", fmt.Errorf("findOrCreateVersion: decode: %w", err)
		}
		return result.Data.ID, nil
	}

	if resp.StatusCode == 409 {
		io.ReadAll(resp.Body) // drain
		return findExistingVersion(c, appID, version)
	}

	respBody, _ := io.ReadAll(resp.Body)
	return "", fmt.Errorf("findOrCreateVersion: unexpected status %d: %s", resp.StatusCode, string(respBody))
}

// findExistingVersion looks up an existing App Store version by version string.
// Only versions in PREPARE_FOR_SUBMISSION state may be reused.
func findExistingVersion(c *Client, appID, version string) (string, error) {
	url := fmt.Sprintf("%s/v1/apps/%s/appStoreVersions?filter[versionString]=%s&filter[platform]=IOS",
		ascBaseURL, appID, version)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("findExistingVersion: %w", err)
	}
	resp, err := c.do(req)
	if err != nil {
		return "", fmt.Errorf("findExistingVersion: %w", err)
	}
	defer resp.Body.Close()
	if err := checkStatus(resp, 200); err != nil {
		return "", fmt.Errorf("findExistingVersion: %w", err)
	}
	var result struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				AppStoreState string `json:"appStoreState"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("findExistingVersion: decode: %w", err)
	}
	if len(result.Data) == 0 {
		return "", fmt.Errorf("App Store version %s not found after conflict", version)
	}
	v := result.Data[0]
	if v.Attributes.AppStoreState != "PREPARE_FOR_SUBMISSION" {
		return "", fmt.Errorf("App Store version %s already exists in state %s and cannot be edited",
			version, v.Attributes.AppStoreState)
	}
	return v.ID, nil
}
```

Also remove the `var _ = bytes.NewReader` placeholder line at this point — `bytes` is now used in `findOrCreateVersion`.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/appstore/... -v -count=1 -run "TestFindOrCreateVersion"
```

Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/appstore/appstore.go internal/appstore/appstore_test.go
git commit -m "feat: add findOrCreateVersion to appstore package"
```

---

## Task 5: attachBuild

**Files:**
- Modify: `internal/appstore/appstore_test.go`
- Modify: `internal/appstore/appstore.go`

- [ ] **Step 1: Add tests for attachBuild**

Append to `internal/appstore/appstore_test.go`:

```go
func TestAttachBuild_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/v1/appStoreVersions/ver-1/relationships/build") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		data := body["data"].(map[string]interface{})
		if data["type"] != "builds" {
			t.Errorf("type = %v, want builds", data["type"])
		}
		if data["id"] != "build-1" {
			t.Errorf("id = %v, want build-1", data["id"])
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()

	orig := ascBaseURL
	ascBaseURL = srv.URL
	defer func() { ascBaseURL = orig }()

	if err := attachBuild(testClientWithServer(t, srv), "ver-1", "build-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAttachBuild_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		w.Write([]byte(`unprocessable entity`))
	}))
	defer srv.Close()

	orig := ascBaseURL
	ascBaseURL = srv.URL
	defer func() { ascBaseURL = orig }()

	if err := attachBuild(testClientWithServer(t, srv), "ver-1", "build-1"); err == nil {
		t.Fatal("expected error for 422, got nil")
	}
}
```

- [ ] **Step 2: Run tests to confirm failure**

```bash
go test ./internal/appstore/... -v -count=1 -run "TestAttachBuild"
```

Expected: compile error — `attachBuild` undefined.

- [ ] **Step 3: Implement attachBuild**

Add to `internal/appstore/appstore.go`:

```go
// attachBuild sets the build on an App Store version.
func attachBuild(c *Client, versionID, buildID string) error {
	url := fmt.Sprintf("%s/v1/appStoreVersions/%s/relationships/build", ascBaseURL, versionID)
	body := map[string]interface{}{
		"data": map[string]string{
			"type": "builds",
			"id":   buildID,
		},
	}
	data, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("attachBuild: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(req)
	if err != nil {
		return fmt.Errorf("attachBuild: %w", err)
	}
	defer resp.Body.Close()
	return checkStatus(resp, 204)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/appstore/... -v -count=1 -run "TestAttachBuild"
```

Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/appstore/appstore.go internal/appstore/appstore_test.go
git commit -m "feat: add attachBuild to appstore package"
```

---

## Task 6: submitForReview

**Files:**
- Modify: `internal/appstore/appstore_test.go`
- Modify: `internal/appstore/appstore.go`

- [ ] **Step 1: Add tests for submitForReview**

Append to `internal/appstore/appstore_test.go`:

```go
func TestSubmitForReview_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/appStoreVersionSubmissions" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
			return
		}
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		data := body["data"].(map[string]interface{})
		if data["type"] != "appStoreVersionSubmissions" {
			t.Errorf("type = %v, want appStoreVersionSubmissions", data["type"])
		}
		rels := data["relationships"].(map[string]interface{})
		ver := rels["appStoreVersion"].(map[string]interface{})
		verData := ver["data"].(map[string]interface{})
		if verData["id"] != "ver-1" {
			t.Errorf("version id = %v, want ver-1", verData["id"])
		}
		w.WriteHeader(201)
		w.Write([]byte(`{"data":{"id":"sub-1","type":"appStoreVersionSubmissions"}}`))
	}))
	defer srv.Close()

	orig := ascBaseURL
	ascBaseURL = srv.URL
	defer func() { ascBaseURL = orig }()

	if err := submitForReview(testClientWithServer(t, srv), "ver-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSubmitForReview_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(409)
		w.Write([]byte(`already submitted`))
	}))
	defer srv.Close()

	orig := ascBaseURL
	ascBaseURL = srv.URL
	defer func() { ascBaseURL = orig }()

	if err := submitForReview(testClientWithServer(t, srv), "ver-1"); err == nil {
		t.Fatal("expected error for 409, got nil")
	}
}
```

- [ ] **Step 2: Run tests to confirm failure**

```bash
go test ./internal/appstore/... -v -count=1 -run "TestSubmitForReview"
```

Expected: compile error — `submitForReview` undefined.

- [ ] **Step 3: Implement submitForReview**

Add to `internal/appstore/appstore.go`:

```go
// submitForReview submits an App Store version for review. Fire-and-forget.
func submitForReview(c *Client, versionID string) error {
	url := fmt.Sprintf("%s/v1/appStoreVersionSubmissions", ascBaseURL)
	body := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "appStoreVersionSubmissions",
			"relationships": map[string]interface{}{
				"appStoreVersion": map[string]interface{}{
					"data": map[string]interface{}{
						"type": "appStoreVersions",
						"id":   versionID,
					},
				},
			},
		},
	}
	data, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("submitForReview: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(req)
	if err != nil {
		return fmt.Errorf("submitForReview: %w", err)
	}
	defer resp.Body.Close()
	return checkStatus(resp, 201)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/appstore/... -v -count=1 -run "TestSubmitForReview"
```

Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/appstore/appstore.go internal/appstore/appstore_test.go
git commit -m "feat: add submitForReview to appstore package"
```

---

## Task 7: PromoteBuild orchestrator

**Files:**
- Modify: `internal/appstore/appstore_test.go`
- Modify: `internal/appstore/appstore.go`

- [ ] **Step 1: Add end-to-end test for PromoteBuild**

Append to `internal/appstore/appstore_test.go`:

```go
func TestPromoteBuild_HappyPath(t *testing.T) {
	calls := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/apps":
			w.Write([]byte(`{"data":[{"id":"app-1"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/builds":
			w.Write([]byte(`{"data":[{"id":"build-1"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/appStoreVersions":
			w.WriteHeader(201)
			w.Write([]byte(`{"data":{"id":"ver-1"}}`))
		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/relationships/build"):
			w.WriteHeader(204)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/appStoreVersionSubmissions":
			w.WriteHeader(201)
			w.Write([]byte(`{"data":{"id":"sub-1"}}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()

	orig := ascBaseURL
	ascBaseURL = srv.URL
	defer func() { ascBaseURL = orig }()

	if err := PromoteBuild(testClientWithServer(t, srv), "com.example.app", "1.2.3", "42"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 5 {
		t.Errorf("expected 5 API calls, got %d: %v", len(calls), calls)
	}
}

func TestPromoteBuild_BuildNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/apps":
			w.Write([]byte(`{"data":[{"id":"app-1"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/builds":
			w.Write([]byte(`{"data":[]}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()

	orig := ascBaseURL
	ascBaseURL = srv.URL
	defer func() { ascBaseURL = orig }()

	err := PromoteBuild(testClientWithServer(t, srv), "com.example.app", "1.2.3", "42")
	if err == nil {
		t.Fatal("expected error for missing build, got nil")
	}
	if !strings.Contains(err.Error(), "not found or still processing") {
		t.Errorf("unexpected error: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to confirm the placeholder fails**

```bash
go test ./internal/appstore/... -v -count=1 -run "TestPromoteBuild"
```

Expected: FAIL — `PromoteBuild` returns "not implemented".

- [ ] **Step 3: Replace PromoteBuild placeholder with real implementation**

Replace the placeholder `PromoteBuild` function in `internal/appstore/appstore.go`:

```go
// PromoteBuild submits a TestFlight-processed build for App Store review.
// It resolves app and build IDs, creates or reuses an App Store version,
// attaches the build, and submits for review (fire-and-forget).
func PromoteBuild(c *Client, bundleID, version, buildNumber string) error {
	appID, err := resolveAppID(c, bundleID)
	if err != nil {
		return err
	}
	buildID, err := findBuild(c, appID, version, buildNumber)
	if err != nil {
		return err
	}
	versionID, err := findOrCreateVersion(c, appID, version)
	if err != nil {
		return err
	}
	if err := attachBuild(c, versionID, buildID); err != nil {
		return err
	}
	return submitForReview(c, versionID)
}
```

- [ ] **Step 4: Run all appstore tests**

```bash
go test ./internal/appstore/... -v -count=1
```

Expected: PASS — all tests in the package.

- [ ] **Step 5: Commit**

```bash
git add internal/appstore/appstore.go internal/appstore/appstore_test.go
git commit -m "feat: implement PromoteBuild orchestrator in appstore package"
```

---

## Task 8: cmd/ios-promote binary

**Files:**
- Create: `cmd/ios-promote/main_test.go`
- Create: `cmd/ios-promote/main.go`

- [ ] **Step 1: Write tests**

Create `cmd/ios-promote/main_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to confirm compile failure**

```bash
go test ./cmd/ios-promote/... -v -count=1
```

Expected: compile error — package does not exist.

- [ ] **Step 3: Implement the binary**

Create `cmd/ios-promote/main.go`:

```go
// cmd/ios-promote/main.go
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kolabs-dev/mobile-actions/internal/actions"
	"github.com/kolabs-dev/mobile-actions/internal/appstore"
)

var dryRun = flag.Bool("dry-run", false, "log what would be promoted without calling the API")

func main() {
	flag.Parse()
	if err := run(); err != nil {
		actions.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	keyB64 := os.Getenv("INPUT_APP_STORE_CONNECT_KEY")
	keyID := os.Getenv("INPUT_APP_STORE_CONNECT_KEY_ID")
	issuerID := os.Getenv("INPUT_APP_STORE_CONNECT_ISSUER_ID")
	bundleID := os.Getenv("INPUT_BUNDLE_ID")
	version := os.Getenv("INPUT_VERSION")
	buildNumber := os.Getenv("INPUT_BUILD_NUMBER")

	if keyB64 == "" || keyID == "" || issuerID == "" || bundleID == "" || version == "" || buildNumber == "" {
		return fmt.Errorf("INPUT_APP_STORE_CONNECT_KEY, INPUT_APP_STORE_CONNECT_KEY_ID, INPUT_APP_STORE_CONNECT_ISSUER_ID, INPUT_BUNDLE_ID, INPUT_VERSION, and INPUT_BUILD_NUMBER are required")
	}

	if *dryRun {
		fmt.Printf("[dry-run] would promote build %s(%s) of %s for App Store review\n",
			version, buildNumber, bundleID)
		return nil
	}

	client, err := appstore.NewClient(keyB64, keyID, issuerID)
	if err != nil {
		return err
	}

	actions.Group("Submitting for App Store review")
	defer actions.EndGroup()

	if err := appstore.PromoteBuild(client, bundleID, version, buildNumber); err != nil {
		return err
	}

	fmt.Printf("submitted %s(%s) of %s for App Store review\n", version, buildNumber, bundleID)
	return nil
}
```

- [ ] **Step 4: Run all tests**

```bash
go test ./cmd/ios-promote/... ./internal/appstore/... -v -count=1
```

Expected: PASS — all tests.

- [ ] **Step 5: Confirm it builds**

```bash
CGO_ENABLED=0 go build -o /tmp/ios-promote ./cmd/ios-promote
```

Expected: binary created with no errors.

- [ ] **Step 6: Commit**

```bash
git add cmd/ios-promote/main.go cmd/ios-promote/main_test.go
git commit -m "feat: add cmd/ios-promote binary"
```

---

## Task 9: ios-promote/action.yml

**Files:**
- Create: `ios-promote/action.yml`

- [ ] **Step 1: Create the composite action**

Create `ios-promote/action.yml`:

```yaml
name: 'mobile-actions/ios-promote'
description: 'Submit an iOS build to App Store review'

inputs:
  app-store-connect-key:
    description: 'Base64-encoded .p8 API key'
    required: true
  app-store-connect-key-id:
    description: 'App Store Connect key ID'
    required: true
  app-store-connect-issuer-id:
    description: 'App Store Connect issuer ID'
    required: true
  bundle-id:
    description: 'App bundle identifier (e.g. com.myapp)'
    required: true
  version:
    description: 'Version string (e.g. 1.2.3)'
    required: true
  build-number:
    description: 'Build number (e.g. 42)'
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

    - name: Submit for App Store review
      shell: bash
      env:
        INPUT_APP_STORE_CONNECT_KEY: ${{ inputs.app-store-connect-key }}
        INPUT_APP_STORE_CONNECT_KEY_ID: ${{ inputs.app-store-connect-key-id }}
        INPUT_APP_STORE_CONNECT_ISSUER_ID: ${{ inputs.app-store-connect-issuer-id }}
        INPUT_BUNDLE_ID: ${{ inputs.bundle-id }}
        INPUT_VERSION: ${{ inputs.version }}
        INPUT_BUILD_NUMBER: ${{ inputs.build-number }}
      run: $MOBILE_ACTIONS_BIN/ios-promote-$MOBILE_ACTIONS_OS_ARCH
```

- [ ] **Step 2: Commit**

```bash
git add ios-promote/action.yml
git commit -m "feat: add ios-promote composite action"
```

---

## Task 10: Update build-binaries workflow

**Files:**
- Modify: `.github/workflows/build-binaries.yml` (line 23)

- [ ] **Step 1: Add ios-promote to the PROGRAMS list**

In `.github/workflows/build-binaries.yml`, change line 23 from:

```yaml
          PROGRAMS="android-build android-sign android-upload android-promote ios-setup-signing ios-teardown-signing ios-build ios-upload verify-checksums"
```

to:

```yaml
          PROGRAMS="android-build android-sign android-upload android-promote ios-setup-signing ios-teardown-signing ios-build ios-upload ios-promote verify-checksums"
```

- [ ] **Step 2: Verify all tests still pass**

```bash
go test ./internal/... ./cmd/... -v -count=1
```

Expected: PASS — all tests.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/build-binaries.yml
git commit -m "chore: add ios-promote to build-binaries workflow"
```

---

## Self-Review

**Spec coverage check:**
- ✅ 5 ASC API calls (resolveAppID, findBuild, findOrCreateVersion, attachBuild, submitForReview)
- ✅ JWT RS256 auth with stdlib only, no new dependencies
- ✅ `--dry-run` flag
- ✅ `INPUT_*` env vars
- ✅ Build not found / still processing → clear error
- ✅ Version in PREPARE_FOR_SUBMISSION → reuse
- ✅ Version in non-editable state → error with state name
- ✅ `ios-promote/action.yml` with same binary download pattern
- ✅ `build-binaries.yml` updated
- ✅ Unit tests with httptest.Server for all API functions
- ✅ End-to-end test for PromoteBuild happy path + build not found
- ✅ Dry-run test in main_test.go
- ✅ Missing inputs test in main_test.go
