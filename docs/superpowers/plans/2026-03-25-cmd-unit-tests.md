# cmd/ Unit Tests Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add unit tests for all testable pure/filesystem functions in every `cmd/` binary, establishing the same quality bar as the existing `internal/` tests.

**Architecture:** Each `cmd/<binary>/main_test.go` lives in `package main` (white-box) to access unexported functions directly, exactly like the existing `cmd/android-upload/main_test.go`. Tests avoid calling macOS-only tools (`security`, `xcodebuild`, `jarsigner`) directly — instead they test the pure functions and, where the function calls an external binary, they inject a fake binary via a temp `PATH`. All filesystem side-effects use `t.TempDir()`. All env-var side-effects use `t.Setenv()`.

**Tech Stack:** Go standard library only — `testing`, `os`, `path/filepath`, `crypto/sha256`, `encoding/base64`, `net/http/httptest`, `strings`. No third-party assertion libraries (consistent with existing tests).

---

## File Map

| File | Status | What changes |
|---|---|---|
| `cmd/verify-checksums/main_test.go` | **Create** | Tests for `sha256File` and `verify` |
| `cmd/android-build/main_test.go` | **Create** | Tests for `findArtifact` |
| `cmd/android-sign/main_test.go` | **Create** | Tests for `findApksigner` and `copyFile` |
| `cmd/android-upload/main_test.go` | **Extend** | Add tests for `createEdit`, `updateTrack`, `checkStatus`, dry-run |
| `cmd/ios-setup-signing/main_test.go` | **Create** | Tests for `generateKeychainPassword`, `writeSigningState`, `copyFile` |
| `cmd/ios-teardown-signing/main_test.go` | **Create** | Tests for `run()` with fake `security` binary |
| `cmd/ios-build/main_test.go` | **Create** | Tests for `detectWorkspace`, `writeExportOptions`, `findIPA` |
| `cmd/ios-upload/main_test.go` | **Create** | Tests for `installP8Key` |

---

## Task 1: `cmd/verify-checksums` — sha256File and verify

**Files:**
- Create: `cmd/verify-checksums/main_test.go`

This command is the most self-contained: pure filesystem operations, no external tools.

- [ ] **Step 1: Write the failing tests**

```go
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSha256File_Success(t *testing.T) {
	content := []byte("hello world")
	f, err := os.CreateTemp(t.TempDir(), "*.bin")
	if err != nil {
		t.Fatal(err)
	}
	f.Write(content)
	f.Close()

	h := sha256.Sum256(content)
	want := hex.EncodeToString(h[:])

	got, err := sha256File(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestSha256File_Missing(t *testing.T) {
	_, err := sha256File(filepath.Join(t.TempDir(), "nonexistent"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestVerify_AllMatch(t *testing.T) {
	dir := t.TempDir()
	content1 := []byte("file one content")
	content2 := []byte("file two content")
	os.WriteFile(filepath.Join(dir, "a.bin"), content1, 0644)
	os.WriteFile(filepath.Join(dir, "b.bin"), content2, 0644)
	h1 := sha256.Sum256(content1)
	h2 := sha256.Sum256(content2)
	checksums := fmt.Sprintf("%s  a.bin\n%s  b.bin\n",
		hex.EncodeToString(h1[:]), hex.EncodeToString(h2[:]))
	checkFile := filepath.Join(dir, "sums.txt")
	os.WriteFile(checkFile, []byte(checksums), 0644)

	if err := verify(checkFile, dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerify_HashMismatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.bin"), []byte("real content"), 0644)
	wrong := "0000000000000000000000000000000000000000000000000000000000000000"
	checkFile := filepath.Join(dir, "sums.txt")
	os.WriteFile(checkFile, []byte(wrong+"  a.bin\n"), 0644)

	err := verify(checkFile, dir)
	if err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("expected 'hash mismatch' in error, got: %v", err)
	}
}

func TestVerify_MissingFile(t *testing.T) {
	dir := t.TempDir()
	wrong := "0000000000000000000000000000000000000000000000000000000000000000"
	checkFile := filepath.Join(dir, "sums.txt")
	os.WriteFile(checkFile, []byte(wrong+"  missing.bin\n"), 0644)

	err := verify(checkFile, dir)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestVerify_MalformedLine(t *testing.T) {
	dir := t.TempDir()
	checkFile := filepath.Join(dir, "sums.txt")
	// Single space instead of two — malformed
	os.WriteFile(checkFile, []byte("abc123 single-space.bin\n"), 0644)

	err := verify(checkFile, dir)
	if err == nil {
		t.Fatal("expected error for malformed line, got nil")
	}
	if !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("expected 'malformed' in error, got: %v", err)
	}
}

func TestVerify_EmptyLinesIgnored(t *testing.T) {
	dir := t.TempDir()
	checkFile := filepath.Join(dir, "sums.txt")
	os.WriteFile(checkFile, []byte("\n\n\n"), 0644)

	if err := verify(checkFile, dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerify_MissingChecksumsFile(t *testing.T) {
	err := verify(filepath.Join(t.TempDir(), "no-such-file.txt"), t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing checksums file, got nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./cmd/verify-checksums/... -v -count=1
```

Expected: compilation error (file doesn't exist yet).

- [ ] **Step 3: Write the test file**

Create `cmd/verify-checksums/main_test.go` with the code from Step 1 above.

- [ ] **Step 4: Run tests to verify they pass**

```
go test ./cmd/verify-checksums/... -v -count=1
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/verify-checksums/main_test.go
git commit -m "test: add unit tests for cmd/verify-checksums"
```

---

## Task 2: `cmd/android-build` — findArtifact

**Files:**
- Create: `cmd/android-build/main_test.go`

`findArtifact` walks a standard Gradle output directory and finds `.aab` or `.apk` files. This is pure filesystem.

- [ ] **Step 1: Write the failing tests**

```go
// cmd/android-build/main_test.go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindArtifact_AAB(t *testing.T) {
	appPath := t.TempDir()
	// Gradle standard output path for AAB
	dir := filepath.Join(appPath, "app", "build", "outputs", "bundle", "release")
	os.MkdirAll(dir, 0755)
	expected := filepath.Join(dir, "app-release.aab")
	os.WriteFile(expected, []byte("fake aab"), 0644)

	got, err := findArtifact(appPath, "aab")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Fatalf("got %s, want %s", got, expected)
	}
}

func TestFindArtifact_APK(t *testing.T) {
	appPath := t.TempDir()
	// Gradle standard output path for APK
	dir := filepath.Join(appPath, "app", "build", "outputs", "apk", "release")
	os.MkdirAll(dir, 0755)
	expected := filepath.Join(dir, "app-release.apk")
	os.WriteFile(expected, []byte("fake apk"), 0644)

	got, err := findArtifact(appPath, "apk")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Fatalf("got %s, want %s", got, expected)
	}
}

func TestFindArtifact_NotFound(t *testing.T) {
	appPath := t.TempDir()
	// Don't create any artifact

	_, err := findArtifact(appPath, "aab")
	if err == nil {
		t.Fatal("expected error when no artifact found, got nil")
	}
	if !strings.Contains(err.Error(), "no .aab artifact found") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestFindArtifact_APK_NotFound(t *testing.T) {
	appPath := t.TempDir()

	_, err := findArtifact(appPath, "apk")
	if err == nil {
		t.Fatal("expected error when no artifact found, got nil")
	}
	if !strings.Contains(err.Error(), "no .apk artifact found") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestFindArtifact_ReturnsFirstWhenMultiple(t *testing.T) {
	appPath := t.TempDir()
	dir := filepath.Join(appPath, "app", "build", "outputs", "bundle", "release")
	os.MkdirAll(dir, 0755)
	// Create two files; findArtifact returns the first one found by WalkDir (lexicographic)
	os.WriteFile(filepath.Join(dir, "app-1.aab"), []byte("fake aab 1"), 0644)
	os.WriteFile(filepath.Join(dir, "app-2.aab"), []byte("fake aab 2"), 0644)

	got, err := findArtifact(appPath, "aab")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(got, ".aab") {
		t.Fatalf("expected .aab file, got: %s", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./cmd/android-build/... -v -count=1
```

Expected: compilation error or test failures.

- [ ] **Step 3: Write the test file**

Create `cmd/android-build/main_test.go` with the code from Step 1.

- [ ] **Step 4: Run tests to verify they pass**

```
go test ./cmd/android-build/... -v -count=1
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/android-build/main_test.go
git commit -m "test: add unit tests for cmd/android-build"
```

---

## Task 3: `cmd/android-sign` — findApksigner and copyFile

**Files:**
- Create: `cmd/android-sign/main_test.go`

`findApksigner` walks `$ANDROID_SDK_ROOT/build-tools/` and picks the highest semver version. `copyFile` is simple file I/O. Both are pure filesystem operations.

- [ ] **Step 1: Write the failing tests**

```go
// cmd/android-sign/main_test.go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindApksigner_PicksHighestVersion(t *testing.T) {
	sdkRoot := t.TempDir()
	buildTools := filepath.Join(sdkRoot, "build-tools")
	// Create three versioned directories with a fake apksigner in each
	for _, v := range []string{"33.0.0", "34.0.1", "34.0.0"} {
		dir := filepath.Join(buildTools, v)
		os.MkdirAll(dir, 0755)
		os.WriteFile(filepath.Join(dir, "apksigner"), []byte("#!/bin/sh\n"), 0755)
	}
	t.Setenv("ANDROID_SDK_ROOT", sdkRoot)

	got, err := findApksigner()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Must pick 34.0.1, the highest
	want := filepath.Join(buildTools, "34.0.1", "apksigner")
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestFindApksigner_MissingSDKRoot(t *testing.T) {
	t.Setenv("ANDROID_SDK_ROOT", "")

	_, err := findApksigner()
	if err == nil {
		t.Fatal("expected error when ANDROID_SDK_ROOT is empty, got nil")
	}
	if !strings.Contains(err.Error(), "ANDROID_SDK_ROOT") {
		t.Fatalf("expected ANDROID_SDK_ROOT in error, got: %v", err)
	}
}

func TestFindApksigner_NoValidVersions(t *testing.T) {
	sdkRoot := t.TempDir()
	buildTools := filepath.Join(sdkRoot, "build-tools")
	os.MkdirAll(buildTools, 0755)
	// Create directories with non-semver names
	os.MkdirAll(filepath.Join(buildTools, "not-a-version"), 0755)
	os.MkdirAll(filepath.Join(buildTools, "also-bad"), 0755)
	t.Setenv("ANDROID_SDK_ROOT", sdkRoot)

	_, err := findApksigner()
	if err == nil {
		t.Fatal("expected error when no valid semver versions found, got nil")
	}
	if !strings.Contains(err.Error(), "no valid build-tools versions") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFindApksigner_BuildToolsDirMissing(t *testing.T) {
	sdkRoot := t.TempDir()
	// Don't create build-tools directory
	t.Setenv("ANDROID_SDK_ROOT", sdkRoot)

	_, err := findApksigner()
	if err == nil {
		t.Fatal("expected error when build-tools dir missing, got nil")
	}
	if !strings.Contains(err.Error(), "read build-tools dir") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCopyFile_Success(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	dst := filepath.Join(dir, "dest.txt")
	content := []byte("test content 1234")
	os.WriteFile(src, content, 0644)

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("failed to read dest: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("got %q, want %q", got, content)
	}
}

func TestCopyFile_SourceMissing(t *testing.T) {
	dir := t.TempDir()
	err := copyFile(filepath.Join(dir, "missing.txt"), filepath.Join(dir, "dest.txt"))
	if err == nil {
		t.Fatal("expected error for missing source, got nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./cmd/android-sign/... -v -count=1
```

Expected: compilation error or test failures.

- [ ] **Step 3: Write the test file**

Create `cmd/android-sign/main_test.go` with the code from Step 1.

- [ ] **Step 4: Run tests to verify they pass**

```
go test ./cmd/android-sign/... -v -count=1
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/android-sign/main_test.go
git commit -m "test: add unit tests for cmd/android-sign"
```

---

## Task 4: `cmd/android-upload` — extend existing tests

**Files:**
- Modify: `cmd/android-upload/main_test.go`

The existing tests cover `commitEdit`. Add tests for `createEdit`, `updateTrack`, `checkStatus`, and dry-run mode.

- [ ] **Step 1: Write the failing tests**

Append to the existing `cmd/android-upload/main_test.go`:

```go
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

	// Set dry-run flag
	orig := *dryRun
	*dryRun = true
	defer func() { *dryRun = orig }()

	// Should return nil without making any HTTP calls or touching the filesystem for upload
	if err := run(); err != nil {
		t.Fatalf("unexpected error in dry-run: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify the new ones fail**

```
go test ./cmd/android-upload/... -v -count=1 -run "TestCreateEdit|TestUpdateTrack|TestCheckStatus|TestRun_DryRun"
```

Expected: compilation errors for missing functions or test failures.

- [ ] **Step 3: Append tests to the existing file**

Add the new test functions to `cmd/android-upload/main_test.go`. Add `"net/http"` to the import if not already present (it is already imported).

- [ ] **Step 4: Run all android-upload tests to verify they pass**

```
go test ./cmd/android-upload/... -v -count=1
```

Expected: all 7 tests PASS (3 existing + 4 new... plus checkStatus = 7 total).

- [ ] **Step 5: Commit**

```bash
git add cmd/android-upload/main_test.go
git commit -m "test: extend cmd/android-upload tests with createEdit, updateTrack, checkStatus, dry-run"
```

---

## Task 5: `cmd/ios-setup-signing` — generateKeychainPassword, writeSigningState, copyFile

**Files:**
- Create: `cmd/ios-setup-signing/main_test.go`

These three functions have no external binary dependencies.

- [ ] **Step 1: Write the failing tests**

```go
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
	// Should be 32 hex characters (16 bytes * 2 chars/byte)
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
	// Two calls should almost certainly produce different passwords
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
	// Use a nested path that doesn't exist yet
	t.Setenv("RUNNER_TEMP", dir)

	state := SigningState{KeychainName: "test.keychain-db"}
	if err := writeSigningState(state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Directory should have been created
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
	os.WriteFile(src, content, 0644)

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
```

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./cmd/ios-setup-signing/... -v -count=1
```

Expected: compilation error or test failures.

- [ ] **Step 3: Write the test file**

Create `cmd/ios-setup-signing/main_test.go` with the code from Step 1.

- [ ] **Step 4: Run tests to verify they pass**

```
go test ./cmd/ios-setup-signing/... -v -count=1
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/ios-setup-signing/main_test.go
git commit -m "test: add unit tests for cmd/ios-setup-signing"
```

---

## Task 6: `cmd/ios-teardown-signing` — run() with fake security binary

**Files:**
- Create: `cmd/ios-teardown-signing/main_test.go`

`run()` reads a state file, then calls `security` (external) and removes a file. We inject a fake `security` shell script via `PATH`. The provisioning profile removal is pure filesystem — we can test that independently.

The fake binary technique: create a shell script `security` in a temp dir, make it executable, prepend the dir to `PATH`. On Linux this works because `#!/bin/sh\nexit 0\n` is valid.

- [ ] **Step 1: Write the failing tests**

```go
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
	data, _ := json.Marshal(state)
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
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "signing-state.json"), []byte("NOT JSON"), 0600)

	err := run()
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestRun_RemovesProvisioningProfile(t *testing.T) {
	writeFakeSecurity(t)
	runnerTemp := t.TempDir()
	t.Setenv("RUNNER_TEMP", runnerTemp)

	// Create a real provisioning profile file
	profileDir := t.TempDir()
	profilePath := filepath.Join(profileDir, "test-profile.mobileprovision")
	if err := os.WriteFile(profilePath, []byte("fake profile"), 0644); err != nil {
		t.Fatal(err)
	}

	writeStateFile(t, runnerTemp, SigningState{
		KeychainName:            "test.keychain-db",
		OriginalKeychains:       []string{},
		ProvisioningProfilePath: profilePath,
	})

	if err := run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Profile should be removed
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

	// Profile path that doesn't exist
	writeStateFile(t, runnerTemp, SigningState{
		KeychainName:            "test.keychain-db",
		OriginalKeychains:       []string{},
		ProvisioningProfilePath: filepath.Join(t.TempDir(), "already-gone.mobileprovision"),
	})

	// os.IsNotExist should be swallowed — run() returns nil
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
		ProvisioningProfilePath: "", // empty path — no removal attempted
	})

	if err := run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./cmd/ios-teardown-signing/... -v -count=1
```

Expected: compilation error or test failures.

- [ ] **Step 3: Write the test file**

Create `cmd/ios-teardown-signing/main_test.go` with the code from Step 1.

- [ ] **Step 4: Run tests to verify they pass**

```
go test ./cmd/ios-teardown-signing/... -v -count=1
```

Expected: all 5 tests PASS.

Note: `TestRun_RemovesProvisioningProfile` and `TestRun_ProfileAlreadyAbsent` require the fake `security` binary on the PATH. Verify these pass.

- [ ] **Step 5: Commit**

```bash
git add cmd/ios-teardown-signing/main_test.go
git commit -m "test: add unit tests for cmd/ios-teardown-signing"
```

---

## Task 7: `cmd/ios-build` — detectWorkspace, writeExportOptions, findIPA

**Files:**
- Create: `cmd/ios-build/main_test.go`

All three functions are pure filesystem operations with no external binary dependency.

`writeExportOptions` renders a Go template to produce a plist file. Tests verify that the output contains the expected field values.

- [ ] **Step 1: Write the failing tests**

```go
// cmd/ios-build/main_test.go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectWorkspace_Found(t *testing.T) {
	appPath := t.TempDir()
	wsDir := filepath.Join(appPath, "MyApp.xcworkspace")
	os.Mkdir(wsDir, 0755)

	got, err := detectWorkspace(appPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != wsDir {
		t.Fatalf("got %s, want %s", got, wsDir)
	}
}

func TestDetectWorkspace_NotFound(t *testing.T) {
	appPath := t.TempDir()
	// No .xcworkspace directory

	_, err := detectWorkspace(appPath)
	if err == nil {
		t.Fatal("expected error when no workspace found, got nil")
	}
	if !strings.Contains(err.Error(), "no .xcworkspace found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDetectWorkspace_MultipleFound(t *testing.T) {
	appPath := t.TempDir()
	os.Mkdir(filepath.Join(appPath, "App1.xcworkspace"), 0755)
	os.Mkdir(filepath.Join(appPath, "App2.xcworkspace"), 0755)

	_, err := detectWorkspace(appPath)
	if err == nil {
		t.Fatal("expected error when multiple workspaces found, got nil")
	}
	if !strings.Contains(err.Error(), "multiple .xcworkspace found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDetectWorkspace_NestedWorkspace(t *testing.T) {
	// xcworkspace can be nested inside a subdirectory
	appPath := t.TempDir()
	nested := filepath.Join(appPath, "subdir", "MyApp.xcworkspace")
	os.MkdirAll(nested, 0755)

	got, err := detectWorkspace(appPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nested {
		t.Fatalf("got %s, want %s", got, nested)
	}
}

func TestWriteExportOptions_ContainsExpectedValues(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "ExportOptions.plist")

	err := writeExportOptions(outPath, "TEAM123", "com.example.myapp", "profile-uuid-456", "app-store")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	content := string(data)

	for _, want := range []string{
		"TEAM123",
		"com.example.myapp",
		"profile-uuid-456",
		"app-store",
		"manual",         // signingStyle
		"uploadSymbols",  // key present
	} {
		if !strings.Contains(content, want) {
			t.Errorf("expected %q in ExportOptions.plist, got:\n%s", want, content)
		}
	}
}

func TestWriteExportOptions_InvalidPath(t *testing.T) {
	err := writeExportOptions("/nonexistent-dir/ExportOptions.plist", "T1", "com.x", "uuid", "app-store")
	if err == nil {
		t.Fatal("expected error for invalid path, got nil")
	}
}

func TestFindIPA_Found(t *testing.T) {
	exportPath := t.TempDir()
	ipaPath := filepath.Join(exportPath, "MyApp.ipa")
	os.WriteFile(ipaPath, []byte("fake ipa"), 0644)

	got, err := findIPA(exportPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ipaPath {
		t.Fatalf("got %s, want %s", got, ipaPath)
	}
}

func TestFindIPA_NotFound(t *testing.T) {
	exportPath := t.TempDir()
	// No .ipa file

	_, err := findIPA(exportPath)
	if err == nil {
		t.Fatal("expected error when no IPA found, got nil")
	}
	if !strings.Contains(err.Error(), "no .ipa found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFindIPA_NestedIPA(t *testing.T) {
	exportPath := t.TempDir()
	nested := filepath.Join(exportPath, "Apps", "MyApp.ipa")
	os.MkdirAll(filepath.Dir(nested), 0755)
	os.WriteFile(nested, []byte("fake ipa"), 0644)

	got, err := findIPA(exportPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nested {
		t.Fatalf("got %s, want %s", got, nested)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./cmd/ios-build/... -v -count=1
```

Expected: compilation error or test failures.

- [ ] **Step 3: Write the test file**

Create `cmd/ios-build/main_test.go` with the code from Step 1.

- [ ] **Step 4: Run tests to verify they pass**

```
go test ./cmd/ios-build/... -v -count=1
```

Expected: all 9 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/ios-build/main_test.go
git commit -m "test: add unit tests for cmd/ios-build"
```

---

## Task 8: `cmd/ios-upload` — installP8Key

**Files:**
- Create: `cmd/ios-upload/main_test.go`

`installP8Key` base64-decodes a key and writes it to `~/.appstoreconnect/private_keys/AuthKey_<keyID>.p8`. By setting `HOME` to a temp dir via `t.Setenv`, we redirect the write without touching the real home directory. The cleanup function removes the file.

- [ ] **Step 1: Write the failing tests**

```go
// cmd/ios-upload/main_test.go
package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// skipOnMacOS skips tests that inject HOME to redirect os.UserHomeDir(),
// since os.UserHomeDir() on macOS uses getpwuid_r (ignores $HOME).
func skipOnMacOS(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "darwin" {
		t.Skip("os.UserHomeDir() ignores $HOME on macOS — test only provides isolation on Linux")
	}
}

func TestInstallP8Key_WritesCorrectContent(t *testing.T) {
	skipOnMacOS(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	keyContent := []byte("-----BEGIN PRIVATE KEY-----\nfakekey\n-----END PRIVATE KEY-----\n")
	keyB64 := base64.StdEncoding.EncodeToString(keyContent)
	keyID := "ABC123DEF"

	path, cleanup, err := installP8Key(keyB64, keyID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	// Verify correct path
	wantPath := filepath.Join(home, ".appstoreconnect", "private_keys", "AuthKey_"+keyID+".p8")
	if path != wantPath {
		t.Fatalf("got path %s, want %s", path, wantPath)
	}

	// Verify content matches decoded bytes
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read key file: %v", err)
	}
	if string(data) != string(keyContent) {
		t.Fatalf("content mismatch: got %q, want %q", data, keyContent)
	}
}

func TestInstallP8Key_CleanupRemovesFile(t *testing.T) {
	skipOnMacOS(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	keyContent := []byte("fake key data")
	keyB64 := base64.StdEncoding.EncodeToString(keyContent)

	path, cleanup, err := installP8Key(keyB64, "MYKEY01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// File should exist before cleanup
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist before cleanup: %v", err)
	}

	cleanup()

	// File should be gone after cleanup
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expected file to be removed after cleanup")
	}
}

func TestInstallP8Key_InvalidBase64(t *testing.T) {
	skipOnMacOS(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, _, err := installP8Key("!!!not-valid-base64!!!", "MYKEY01")
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
	if !strings.Contains(err.Error(), "decode p8 key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInstallP8Key_FilePermissions(t *testing.T) {
	skipOnMacOS(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	keyContent := []byte("key data")
	keyB64 := base64.StdEncoding.EncodeToString(keyContent)

	path, cleanup, err := installP8Key(keyB64, "PERMKEY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	// File should be 0600 (owner read/write only)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("expected mode 0600, got %v", info.Mode().Perm())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./cmd/ios-upload/... -v -count=1
```

Expected: compilation error or test failures.

- [ ] **Step 3: Write the test file**

Create `cmd/ios-upload/main_test.go` with the code from Step 1.

- [ ] **Step 4: Run tests to verify they pass**

```
go test ./cmd/ios-upload/... -v -count=1
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/ios-upload/main_test.go
git commit -m "test: add unit tests for cmd/ios-upload"
```

---

## Final Verification

- [ ] **Run the full test suite**

```
go test ./internal/... ./cmd/... -v -count=1
```

Expected: all tests pass. Confirm no compilation errors in any package.

- [ ] **Lint check**

```
go vet ./...
```

Expected: no issues.

- [ ] **Final commit if any cleanup was needed**

```bash
git add -p
git commit -m "chore: cleanup after cmd unit test implementation"
```
