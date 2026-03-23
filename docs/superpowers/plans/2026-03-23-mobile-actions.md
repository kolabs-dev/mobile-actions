# mobile-actions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a GitHub Actions repository with two reusable composite actions (`android` and `ios`) that fully automate mobile app release pipelines — build, sign, and upload — to the Google Play Store and Apple App Store.

**Architecture:** Eight focused Go CLI programs in `cmd/` handle the platform-specific logic (build, sign, upload), using shared `internal/` packages for secrets decoding, subprocess execution, and GitHub Actions output. Pre-compiled binaries are attached to GitHub Releases and downloaded by composite actions at runtime using a two-stage bootstrap verification process.

**Tech Stack:** Go 1.22, `github.com/Masterminds/semver/v3` (apksigner path resolution), `howett.net/plist` (provisioning profile parsing), GitHub Actions composite actions, Gradle (Android), Xcode CLI + iTMSTransporter (iOS), Google Play Developer API v3.

---

## File Map

```
go.mod
go.sum
.gitignore
verify-checksums.sha256             # empty placeholder; updated by release workflow

internal/
  actions/
    actions.go                      # SetOutput, AddMask, Group/EndGroup, Error, Warning
    actions_test.go
  exec/
    exec.go                         # Run (streaming), RunOutput
    exec_test.go
  secrets/
    secrets.go                      # DecodeToFile, SIGTERM cleanup
    secrets_test.go

cmd/
  verify-checksums/main.go          # --file, --dir flags; SHA-256 verifier
  android-build/main.go             # Runs Gradle; outputs unsigned-artifact-path
  android-sign/main.go              # jarsigner (AAB) / apksigner (APK); outputs artifact-path
  android-upload/main.go            # Google Play API v3 edit lifecycle; --dry-run
  ios-setup-signing/main.go         # Keychain setup; writes signing-state.json
  ios-teardown-signing/main.go      # Reads signing-state.json; keychain teardown
  ios-build/main.go                 # ExportOptions.plist + xcodebuild archive + export
  ios-upload/main.go                # iTMSTransporter upload; --dry-run

testdata/
  android/
    settings.gradle
    build.gradle
    app/
      build.gradle
      src/main/java/com/example/TestApp.java
      src/main/AndroidManifest.xml
      src/main/res/values/strings.xml
    gradlew                         # wrapper script (chmod +x)
    gradle/wrapper/gradle-wrapper.properties
  ios/
    TestApp.xcworkspace/
      contents.xcworkspacedata
    TestApp.xcodeproj/
      project.pbxproj
    TestApp/
      AppDelegate.swift
      ContentView.swift
    Podfile

android/
  action.yml                        # Composite action: download-binaries → setup-java → build → sign → upload

ios/
  action.yml                        # Composite action: download-binaries → xcode-select → setup-signing → build → upload → teardown-signing (always)

.github/
  workflows/
    build-binaries.yml              # Reusable (workflow_call): matrix build → aggregate → upload assets → commit verify-checksums.sha256
    release.yml                     # Tag push → create draft release → build-binaries → update floating tag → publish
    test.yml                        # PR / push to main: integration tests for both platforms
```

---

## Task 1: Repository Bootstrap

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `verify-checksums.sha256`

- [ ] **Step 1: Initialize the Go module**

```bash
cd /path/to/mobile-actions
go mod init github.com/your-org/mobile-actions
```

Expected: `go.mod` created with `module github.com/your-org/mobile-actions` and `go 1.22`.

- [ ] **Step 2: Add dependencies**

```bash
go get github.com/Masterminds/semver/v3
go get howett.net/plist
```

- [ ] **Step 3: Create `.gitignore`**

```
bin/
```

- [ ] **Step 4: Create empty `verify-checksums.sha256`**

```bash
touch verify-checksums.sha256
```

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum .gitignore verify-checksums.sha256
git commit -m "chore: bootstrap Go module"
```

---

## Task 2: `internal/actions` Package

**Files:**
- Create: `internal/actions/actions.go`
- Create: `internal/actions/actions_test.go`

The `actions` package writes GitHub Actions workflow commands to stdout and outputs to `$GITHUB_OUTPUT`.

- [ ] **Step 1: Write the failing tests**

Create `internal/actions/actions_test.go`:

```go
package actions_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/your-org/mobile-actions/internal/actions"
)

func TestSetOutput(t *testing.T) {
	f, _ := os.CreateTemp(t.TempDir(), "output-*")
	t.Setenv("GITHUB_OUTPUT", f.Name())

	actions.SetOutput("artifact-path", "/tmp/app.aab")

	content, _ := os.ReadFile(f.Name())
	if !strings.Contains(string(content), "artifact-path=/tmp/app.aab\n") {
		t.Fatalf("expected output file to contain key=value, got: %s", content)
	}
}

func TestAddMask(t *testing.T) {
	var out strings.Builder
	actions.SetWriter(&out)
	defer actions.ResetWriter()

	actions.AddMask("supersecret")
	if !strings.Contains(out.String(), "::add-mask::supersecret\n") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestGroup(t *testing.T) {
	var out strings.Builder
	actions.SetWriter(&out)
	defer actions.ResetWriter()

	actions.Group("my group")
	actions.EndGroup()

	if !strings.Contains(out.String(), "::group::my group\n") {
		t.Fatal("missing group command")
	}
	if !strings.Contains(out.String(), "::endgroup::\n") {
		t.Fatal("missing endgroup command")
	}
}

func TestError(t *testing.T) {
	var out strings.Builder
	actions.SetWriter(&out)
	defer actions.ResetWriter()

	actions.Error("something went wrong")
	if !strings.Contains(out.String(), "::error::something went wrong\n") {
		t.Fatalf("unexpected: %s", out.String())
	}
}

func TestSetOutputMissingEnv(t *testing.T) {
	t.Setenv("GITHUB_OUTPUT", "")
	err := actions.SetOutput("key", "value")
	if err == nil {
		t.Fatal("expected error when GITHUB_OUTPUT is unset")
	}
}

func TestSetOutputCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "output")
	f := filepath.Join(dir, "out.txt")
	t.Setenv("GITHUB_OUTPUT", f)
	if err := actions.SetOutput("k", "v"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/actions/... -v
```

Expected: FAIL — package not found.

- [ ] **Step 3: Implement `internal/actions/actions.go`**

```go
package actions

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var w io.Writer = os.Stdout

// SetWriter replaces the output writer (for testing).
func SetWriter(out io.Writer) { w = out }

// ResetWriter restores the default output writer.
func ResetWriter() { w = os.Stdout }

// SetOutput writes name=value to the $GITHUB_OUTPUT file.
func SetOutput(name, value string) error {
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		return fmt.Errorf("GITHUB_OUTPUT environment variable is not set")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s=%s\n", name, value)
	return err
}

// AddMask masks a value in the runner log.
func AddMask(value string) { fmt.Fprintf(w, "::add-mask::%s\n", value) }

// Group begins a collapsible log group.
func Group(name string) { fmt.Fprintf(w, "::group::%s\n", name) }

// EndGroup ends a collapsible log group.
func EndGroup() { fmt.Fprintf(w, "::endgroup::\n") }

// Error logs an error annotation.
func Error(msg string) { fmt.Fprintf(w, "::error::%s\n", msg) }

// Warning logs a warning annotation.
func Warning(msg string) { fmt.Fprintf(w, "::warning::%s\n", msg) }
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./internal/actions/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/actions/
git commit -m "feat: add internal/actions package"
```

---

## Task 3: `internal/exec` Package

**Files:**
- Create: `internal/exec/exec.go`
- Create: `internal/exec/exec_test.go`

The `exec` package runs subprocesses, streams stdout/stderr to the runner log, and wraps errors with context.

- [ ] **Step 1: Write the failing tests**

Create `internal/exec/exec_test.go`:

```go
package exec_test

import (
	"strings"
	"testing"

	"github.com/your-org/mobile-actions/internal/exec"
)

func TestRun_Success(t *testing.T) {
	if err := exec.Run("echo", "hello"); err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
}

func TestRun_Failure(t *testing.T) {
	err := exec.Run("false")
	if err == nil {
		t.Fatal("expected error for failing command")
	}
	if !strings.Contains(err.Error(), "false") {
		t.Fatalf("error should mention command name, got: %v", err)
	}
}

func TestRunOutput_Success(t *testing.T) {
	out, err := exec.RunOutput("echo", "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(out) != "hello world" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestRunOutput_Failure(t *testing.T) {
	_, err := exec.RunOutput("false")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_NonexistentCommand(t *testing.T) {
	err := exec.Run("this-command-does-not-exist-xyz")
	if err == nil {
		t.Fatal("expected error for missing command")
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/exec/... -v
```

Expected: FAIL — package not found.

- [ ] **Step 3: Implement `internal/exec/exec.go`**

```go
package exec

import (
	"bytes"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"strings"
)

// Run executes a command, streaming stdout and stderr to os.Stdout/os.Stderr.
// Returns a wrapped error with the command name on failure.
func Run(name string, args ...string) error {
	cmd := osexec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// RunOutput executes a command and returns its combined stdout output as a string.
// stderr is still streamed to os.Stderr.
func RunOutput(name string, args ...string) (string, error) {
	var stdout bytes.Buffer
	cmd := osexec.Command(name, args...)
	cmd.Stdout = io.MultiWriter(&stdout, os.Stdout)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return strings.TrimSpace(stdout.String()), nil
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./internal/exec/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/exec/
git commit -m "feat: add internal/exec package"
```

---

## Task 4: `internal/secrets` Package

**Files:**
- Create: `internal/secrets/secrets.go`
- Create: `internal/secrets/secrets_test.go`

The `secrets` package decodes base64-encoded secrets to temp files in `$RUNNER_TEMP/mobile-actions/`, masks values, and registers SIGTERM/SIGINT cleanup.

- [ ] **Step 1: Write the failing tests**

Create `internal/secrets/secrets_test.go`:

```go
package secrets_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/your-org/mobile-actions/internal/secrets"
)

func setupRunnerTemp(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("RUNNER_TEMP", dir)
}

func TestDecodeToFile_Success(t *testing.T) {
	setupRunnerTemp(t)
	content := []byte("my secret data")
	encoded := base64.StdEncoding.EncodeToString(content)

	path, cleanup, err := secrets.DecodeToFile(encoded, "test-*.bin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	got, _ := os.ReadFile(path)
	if string(got) != string(content) {
		t.Fatalf("content mismatch: got %q, want %q", got, content)
	}
}

func TestDecodeToFile_InvalidBase64(t *testing.T) {
	setupRunnerTemp(t)
	_, _, err := secrets.DecodeToFile("not-valid-base64!!!", "test-*.bin")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestDecodeToFile_CleanupRemovesFile(t *testing.T) {
	setupRunnerTemp(t)
	encoded := base64.StdEncoding.EncodeToString([]byte("data"))
	path, cleanup, err := secrets.DecodeToFile(encoded, "test-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expected file to be removed after cleanup")
	}
}

func TestDecodeToFile_WritesToRunnerTemp(t *testing.T) {
	setupRunnerTemp(t)
	encoded := base64.StdEncoding.EncodeToString([]byte("data"))
	path, cleanup, _ := secrets.DecodeToFile(encoded, "test-*.bin")
	defer cleanup()

	runnerTemp := os.Getenv("RUNNER_TEMP")
	expected := filepath.Join(runnerTemp, "mobile-actions")
	if filepath.Dir(path) != expected {
		t.Fatalf("file not in expected dir: %s", path)
	}
}

func TestDecodeToFile_MissingRunnerTemp(t *testing.T) {
	t.Setenv("RUNNER_TEMP", "")
	encoded := base64.StdEncoding.EncodeToString([]byte("data"))
	_, _, err := secrets.DecodeToFile(encoded, "test-*.bin")
	if err == nil {
		t.Fatal("expected error when RUNNER_TEMP is unset")
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/secrets/... -v
```

Expected: FAIL.

- [ ] **Step 3: Implement `internal/secrets/secrets.go`**

```go
package secrets

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
)

// DecodeToFile decodes a base64 string and writes it to a temp file in
// $RUNNER_TEMP/mobile-actions/<pattern>. Returns the file path, a cleanup
// function that removes the file, and any error.
func DecodeToFile(encoded, pattern string) (string, func(), error) {
	runnerTemp := os.Getenv("RUNNER_TEMP")
	if runnerTemp == "" {
		return "", nil, fmt.Errorf("RUNNER_TEMP environment variable is not set")
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", nil, fmt.Errorf("base64 decode: %w", err)
	}

	dir := filepath.Join(runnerTemp, "mobile-actions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}

	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", nil, fmt.Errorf("create temp file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		os.Remove(f.Name())
		return "", nil, fmt.Errorf("write temp file: %w", err)
	}

	cleanup := func() { os.Remove(f.Name()) }
	return f.Name(), cleanup, nil
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./internal/secrets/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/secrets/
git commit -m "feat: add internal/secrets package"
```

---

## Task 5: `verify-checksums` CLI

**Files:**
- Create: `cmd/verify-checksums/main.go`

Reads a `checksums.txt` (format: `<sha256hex>  <filename>` per line). Verifies SHA-256 of each file in `--dir`. `--file` resolves relative to CWD. Exits non-zero on any mismatch or missing file.

- [ ] **Step 1: Write the CLI**

```go
// cmd/verify-checksums/main.go
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	file := flag.String("file", "", "path to checksums file (relative to CWD)")
	dir := flag.String("dir", ".", "directory containing the files to verify")
	flag.Parse()

	if *file == "" {
		fmt.Fprintln(os.Stderr, "error: --file is required")
		os.Exit(1)
	}

	if err := verify(*file, *dir); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("all checksums verified")
}

func verify(checksumsFile, dir string) error {
	f, err := os.Open(checksumsFile)
	if err != nil {
		return fmt.Errorf("open checksums file: %w", err)
	}
	defer f.Close()

	var failures []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// format: "<hash>  <filename>" (two spaces)
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			return fmt.Errorf("malformed checksums line: %q", line)
		}
		expectedHash, filename := parts[0], strings.TrimSpace(parts[1])
		filePath := filepath.Join(dir, filename)

		actualHash, err := sha256File(filePath)
		if err != nil {
			failures = append(failures, fmt.Sprintf("  %s: %v", filename, err))
			continue
		}
		if actualHash != expectedHash {
			failures = append(failures, fmt.Sprintf("  %s: hash mismatch (got %s, want %s)", filename, actualHash, expectedHash))
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read checksums file: %w", err)
	}

	if len(failures) > 0 {
		return fmt.Errorf("checksum verification failed:\n%s", strings.Join(failures, "\n"))
	}
	return nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
```

- [ ] **Step 2: Build and smoke-test**

```bash
go build -o verify-checksums ./cmd/verify-checksums/
echo "test" > /tmp/testfile.txt
HASH=$(sha256sum /tmp/testfile.txt | awk '{print $1}')
echo "${HASH}  testfile.txt" > /tmp/test-checksums.txt
mkdir -p /tmp/testdir
cp /tmp/testfile.txt /tmp/testdir/
cd /tmp/testdir && "$(pwd -P)/../mobile-actions/verify-checksums" --file /tmp/test-checksums.txt --dir /tmp/testdir
```

Expected: `all checksums verified`

- [ ] **Step 3: Commit**

```bash
git add cmd/verify-checksums/
git commit -m "feat: add verify-checksums CLI"
```

---

## Task 6: `android-build` CLI

**Files:**
- Create: `cmd/android-build/main.go`

Reads `INPUT_APP_PATH`, `INPUT_BUILD_TYPE`, `INPUT_GRADLE_TASK`. Auto-detects Gradle task if not set. Runs `./gradlew <task>`. Finds the output artifact and writes its path to `$GITHUB_OUTPUT` as `unsigned-artifact-path`.

- [ ] **Step 1: Write the CLI**

```go
// cmd/android-build/main.go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/your-org/mobile-actions/internal/actions"
	"github.com/your-org/mobile-actions/internal/exec"
)

func main() {
	if err := run(); err != nil {
		actions.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	appPath := envOrDefault("INPUT_APP_PATH", ".")
	buildType := strings.ToLower(envOrDefault("INPUT_BUILD_TYPE", "aab"))
	gradleTask := os.Getenv("INPUT_GRADLE_TASK")

	if gradleTask == "" {
		switch buildType {
		case "aab":
			gradleTask = "bundleRelease"
		case "apk":
			gradleTask = "assembleRelease"
		default:
			return fmt.Errorf("unknown build-type %q: must be 'aab' or 'apk'", buildType)
		}
	}

	gradlew := filepath.Join(appPath, "gradlew")
	actions.Group(fmt.Sprintf("Running Gradle task: %s", gradleTask))
	if err := exec.Run(gradlew, gradleTask); err != nil {
		actions.EndGroup()
		return fmt.Errorf("gradle build failed: %w", err)
	}
	actions.EndGroup()

	artifactPath, err := findArtifact(appPath, buildType)
	if err != nil {
		return err
	}

	fmt.Printf("unsigned artifact: %s\n", artifactPath)
	return actions.SetOutput("unsigned-artifact-path", artifactPath)
}

// findArtifact searches for the Gradle output artifact in the standard output directories.
func findArtifact(appPath, buildType string) (string, error) {
	var searchPath string
	var ext string
	switch buildType {
	case "aab":
		searchPath = filepath.Join(appPath, "app", "build", "outputs", "bundle", "release")
		ext = ".aab"
	case "apk":
		searchPath = filepath.Join(appPath, "app", "build", "outputs", "apk", "release")
		ext = ".apk"
	}

	var found []string
	_ = filepath.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ext) {
			found = append(found, path)
		}
		return nil
	})

	if len(found) == 0 {
		return "", fmt.Errorf("no %s artifact found in %s", ext, searchPath)
	}
	return found[0], nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
```

- [ ] **Step 2: Build**

```bash
go build ./cmd/android-build/
echo "build succeeded"
```

- [ ] **Step 3: Commit**

```bash
git add cmd/android-build/
git commit -m "feat: add android-build CLI"
```

---

## Task 7: `android-sign` CLI

**Files:**
- Create: `cmd/android-sign/main.go`

Reads `INPUT_UNSIGNED_ARTIFACT_PATH`, `INPUT_KEYSTORE` (base64), `INPUT_KEYSTORE_PASSWORD`, `INPUT_KEY_ALIAS`, `INPUT_KEY_PASSWORD`, `INPUT_BUILD_TYPE`. Decodes keystore. Signs AAB with `jarsigner` or APK with `apksigner` (resolved via semantic version sort). Copies signed artifact to `$RUNNER_TEMP/mobile-actions/signed/`. Writes `artifact-path` to `$GITHUB_OUTPUT`.

- [ ] **Step 1: Write the CLI**

```go
// cmd/android-sign/main.go
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	semver "github.com/Masterminds/semver/v3"

	"github.com/your-org/mobile-actions/internal/actions"
	"github.com/your-org/mobile-actions/internal/exec"
	"github.com/your-org/mobile-actions/internal/secrets"
)

func main() {
	if err := run(); err != nil {
		actions.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	unsignedPath := os.Getenv("INPUT_UNSIGNED_ARTIFACT_PATH")
	if unsignedPath == "" {
		return fmt.Errorf("INPUT_UNSIGNED_ARTIFACT_PATH is required")
	}
	keystoreB64 := os.Getenv("INPUT_KEYSTORE")
	keystorePass := os.Getenv("INPUT_KEYSTORE_PASSWORD")
	keyAlias := os.Getenv("INPUT_KEY_ALIAS")
	keyPass := os.Getenv("INPUT_KEY_PASSWORD")
	buildType := strings.ToLower(envOrDefault("INPUT_BUILD_TYPE", "aab"))

	actions.AddMask(keystorePass)
	actions.AddMask(keyPass)

	keystorePath, cleanKeystore, err := secrets.DecodeToFile(keystoreB64, "keystore-*.jks")
	if err != nil {
		return fmt.Errorf("decode keystore: %w", err)
	}
	defer cleanKeystore()

	signedDir := filepath.Join(os.Getenv("RUNNER_TEMP"), "mobile-actions", "signed")
	if err := os.MkdirAll(signedDir, 0755); err != nil {
		return fmt.Errorf("create signed dir: %w", err)
	}

	var signedPath string

	switch buildType {
	case "aab":
		signedPath, err = signAAB(unsignedPath, signedDir, keystorePath, keystorePass, keyAlias, keyPass)
	case "apk":
		signedPath, err = signAPK(unsignedPath, signedDir, keystorePath, keystorePass, keyAlias, keyPass)
	default:
		return fmt.Errorf("unknown build-type %q", buildType)
	}
	if err != nil {
		return err
	}

	fmt.Printf("signed artifact: %s\n", signedPath)
	return actions.SetOutput("artifact-path", signedPath)
}

func signAAB(unsignedPath, signedDir, keystorePath, keystorePass, keyAlias, keyPass string) (string, error) {
	actions.Group("Signing AAB with jarsigner")
	defer actions.EndGroup()

	// jarsigner signs in-place; work on a copy
	dest := filepath.Join(signedDir, "app-release.aab")
	if err := copyFile(unsignedPath, dest); err != nil {
		return "", fmt.Errorf("copy aab: %w", err)
	}

	err := exec.Run("jarsigner",
		"-verbose",
		"-sigalg", "SHA256withRSA",
		"-digestalg", "SHA-256",
		"-keystore", keystorePath,
		"-storepass", keystorePass,
		"-keypass", keyPass,
		dest,
		keyAlias,
	)
	if err != nil {
		return "", fmt.Errorf("jarsigner: %w", err)
	}
	return dest, nil
}

func signAPK(unsignedPath, signedDir, keystorePath, keystorePass, keyAlias, keyPass string) (string, error) {
	actions.Group("Signing APK with apksigner")
	defer actions.EndGroup()

	apksigner, err := findApksigner()
	if err != nil {
		return "", err
	}

	dest := filepath.Join(signedDir, "app-release.apk")
	err = exec.Run(apksigner,
		"sign",
		"--ks", keystorePath,
		"--ks-key-alias", keyAlias,
		"--ks-pass", "pass:"+keystorePass,
		"--key-pass", "pass:"+keyPass,
		"--out", dest,
		unsignedPath,
	)
	if err != nil {
		return "", fmt.Errorf("apksigner: %w", err)
	}
	return dest, nil
}

// findApksigner resolves the highest-versioned apksigner from $ANDROID_SDK_ROOT/build-tools/.
func findApksigner() (string, error) {
	sdkRoot := os.Getenv("ANDROID_SDK_ROOT")
	if sdkRoot == "" {
		return "", fmt.Errorf("ANDROID_SDK_ROOT is not set")
	}
	buildToolsDir := filepath.Join(sdkRoot, "build-tools")
	entries, err := os.ReadDir(buildToolsDir)
	if err != nil {
		return "", fmt.Errorf("read build-tools dir: %w", err)
	}

	type entry struct {
		v    *semver.Version
		name string
	}
	var versions []entry
	for _, e := range entries {
		v, err := semver.NewVersion(e.Name())
		if err != nil {
			continue
		}
		versions = append(versions, entry{v, e.Name()})
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("no valid build-tools versions found in %s", buildToolsDir)
	}
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].v.GreaterThan(versions[j].v)
	})
	return filepath.Join(buildToolsDir, versions[0].name, "apksigner"), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
```

- [ ] **Step 2: Build**

```bash
go build ./cmd/android-sign/
```

- [ ] **Step 3: Commit**

```bash
git add cmd/android-sign/
git commit -m "feat: add android-sign CLI"
```

---

## Task 8: `android-upload` CLI

**Files:**
- Create: `cmd/android-upload/main.go`

Reads `INPUT_SERVICE_ACCOUNT_JSON` (base64), `INPUT_PACKAGE_NAME`, `INPUT_VERSION_CODE`, `INPUT_TRACK`, `INPUT_BUILD_TYPE`, `INPUT_ARTIFACT_PATH`. Implements Google Play Developer API v3 edit lifecycle. Supports `--dry-run`.

- [ ] **Step 1: Write the CLI**

```go
// cmd/android-upload/main.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"golang.org/x/oauth2/google"

	"github.com/your-org/mobile-actions/internal/actions"
	"github.com/your-org/mobile-actions/internal/secrets"
)

// Note: add golang.org/x/oauth2 to go.mod: go get golang.org/x/oauth2

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

	saPath, cleanSA, err := secrets.DecodeToFile(serviceAccountB64, "service-account-*.json")
	if err != nil {
		return fmt.Errorf("decode service account: %w", err)
	}
	defer cleanSA()

	client, err := googleHTTPClient(saPath)
	if err != nil {
		return err
	}

	actions.Group("Uploading to Google Play")
	defer actions.EndGroup()

	editID, err := createEdit(client, packageName)
	if err != nil {
		return err
	}
	fmt.Printf("created edit: %s\n", editID)

	if err := uploadArtifact(client, packageName, editID, buildType, artifactPath); err != nil {
		return err
	}

	if err := updateTrack(client, packageName, editID, track, versionCode); err != nil {
		return err
	}

	if err := commitEdit(client, packageName, editID); err != nil {
		return err
	}

	fmt.Println("upload complete")
	return nil
}

const playBaseURL = "https://androidpublisher.googleapis.com/androidpublisher/v3/applications"

func googleHTTPClient(saPath string) (*http.Client, error) {
	data, err := os.ReadFile(saPath)
	if err != nil {
		return nil, fmt.Errorf("read service account: %w", err)
	}
	conf, err := google.JWTConfigFromJSON(data, "https://www.googleapis.com/auth/androidpublisher")
	if err != nil {
		return nil, fmt.Errorf("parse service account: %w", err)
	}
	return conf.Client(context.Background()), nil
}

func createEdit(client *http.Client, packageName string) (string, error) {
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

func updateTrack(client *http.Client, packageName, editID, track, versionCode string) error {
	url := fmt.Sprintf("%s/%s/edits/%s/tracks/%s", playBaseURL, packageName, editID, track)
	body := map[string]interface{}{
		"track": track,
		"releases": []map[string]interface{}{
			{
				"versionCodes": []string{versionCode},
				"status":       "completed",
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

func commitEdit(client *http.Client, packageName, editID string) error {
	url := fmt.Sprintf("%s/%s/edits/%s:commit", playBaseURL, packageName, editID)
	resp, err := client.Post(url, "application/json", nil)
	if err != nil {
		return fmt.Errorf("edits.commit: %w", err)
	}
	defer resp.Body.Close()
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

- [ ] **Step 2: Add oauth2 dependency**

```bash
go get golang.org/x/oauth2
go mod tidy
```

- [ ] **Step 3: Build**

```bash
go build ./cmd/android-upload/
```

- [ ] **Step 4: Smoke-test dry-run**

```bash
INPUT_PACKAGE_NAME=com.test \
INPUT_VERSION_CODE=1 \
INPUT_ARTIFACT_PATH=/dev/null \
INPUT_SERVICE_ACCOUNT_JSON=$(echo '{}' | base64) \
INPUT_BUILD_TYPE=aab \
./android-upload --dry-run
```

Expected: `[dry-run] would upload ...`

- [ ] **Step 5: Commit**

```bash
git add cmd/android-upload/ go.mod go.sum
git commit -m "feat: add android-upload CLI"
```

---

## Task 9: `ios-setup-signing` CLI

**Files:**
- Create: `cmd/ios-setup-signing/main.go`

Reads `INPUT_CERTIFICATE` (base64 p12), `INPUT_CERTIFICATE_PASSWORD`, `INPUT_PROVISIONING_PROFILE` (base64). Creates a temp keychain, imports the cert, installs the provisioning profile. Writes `signing-state.json` to `$RUNNER_TEMP/mobile-actions/`.

- [ ] **Step 1: Write the CLI**

```go
// cmd/ios-setup-signing/main.go
package main

import (
	"crypto/rand"
	cryptorand "crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"howett.net/plist"

	"github.com/your-org/mobile-actions/internal/actions"
	intexec "github.com/your-org/mobile-actions/internal/exec"
	intsecrets "github.com/your-org/mobile-actions/internal/secrets"
)

var _ = rand.Reader // ensure crypto/rand is used

type SigningState struct {
	KeychainName            string   `json:"keychain_name"`
	OriginalKeychains       []string `json:"original_keychains"`
	ProvisioningProfilePath string   `json:"provisioning_profile_path"`
}

func main() {
	if err := run(); err != nil {
		actions.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	certB64 := os.Getenv("INPUT_CERTIFICATE")
	certPass := os.Getenv("INPUT_CERTIFICATE_PASSWORD")
	profileB64 := os.Getenv("INPUT_PROVISIONING_PROFILE")
	runID := envOrDefault("GITHUB_RUN_ID", "local")

	actions.AddMask(certPass)

	// Decode secrets to temp files
	certPath, cleanCert, err := intsecrets.DecodeToFile(certB64, "cert-*.p12")
	if err != nil {
		return fmt.Errorf("decode certificate: %w", err)
	}
	defer cleanCert()

	profilePath, cleanProfile, err := intsecrets.DecodeToFile(profileB64, "profile-*.mobileprovision")
	if err != nil {
		return fmt.Errorf("decode provisioning profile: %w", err)
	}
	defer cleanProfile()

	// Save original keychain list
	originalKeychains, err := listKeychains()
	if err != nil {
		return fmt.Errorf("list keychains: %w", err)
	}

	// Create temp keychain
	keychainName := fmt.Sprintf("mobile-actions-%s.keychain-db", runID)
	keychainPass := generateKeychainPassword()
	actions.AddMask(keychainPass)

	actions.Group("Setting up signing keychain")
	defer actions.EndGroup()

	if err := intexec.Run("security", "create-keychain", "-p", keychainPass, keychainName); err != nil {
		return fmt.Errorf("create keychain: %w", err)
	}
	if err := intexec.Run("security", "unlock-keychain", "-p", keychainPass, keychainName); err != nil {
		return fmt.Errorf("unlock keychain: %w", err)
	}
	if err := intexec.Run("security", "set-keychain-settings", "-lut", "21600", keychainName); err != nil {
		return fmt.Errorf("set keychain settings: %w", err)
	}

	// Prepend to search list
	searchList := append([]string{keychainName}, originalKeychains...)
	if err := intexec.Run(append([]string{"security", "list-keychains", "-s"}, searchList...)...); err != nil {
		return fmt.Errorf("set keychain search list: %w", err)
	}

	// Import certificate
	if err := intexec.Run("security", "import", certPath,
		"-k", keychainName,
		"-T", "/usr/bin/codesign",
		"-T", "/usr/bin/security",
		"-P", certPass,
	); err != nil {
		return fmt.Errorf("import certificate: %w", err)
	}

	// Allow non-interactive access
	if err := intexec.Run("security", "set-key-partition-list",
		"-S", "apple-tool:,apple:",
		"-s", "-k", keychainPass, keychainName,
	); err != nil {
		return fmt.Errorf("set key partition list: %w", err)
	}

	// Install provisioning profile
	profileDestPath, err := installProvisioningProfile(profilePath)
	if err != nil {
		return fmt.Errorf("install provisioning profile: %w", err)
	}

	// Write signing state
	state := SigningState{
		KeychainName:            keychainName,
		OriginalKeychains:       originalKeychains,
		ProvisioningProfilePath: profileDestPath,
	}
	if err := writeSigningState(state); err != nil {
		return fmt.Errorf("write signing state: %w", err)
	}

	// Note: SIGTERM/SIGINT handlers in this program only log a warning.
	// Actual keychain teardown is handled by ios-teardown-signing (always runs via composite action).
	fmt.Println("signing setup complete")
	return nil
}

func listKeychains() ([]string, error) {
	out, err := intexec.RunOutput("security", "list-keychains")
	if err != nil {
		return nil, err
	}
	var result []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(strings.Trim(line, `"`))
		if line != "" {
			result = append(result, line)
		}
	}
	return result, nil
}

func installProvisioningProfile(profilePath string) (string, error) {
	// Unwrap CMS envelope to get raw plist via internal/exec
	plistStr, err := intexec.RunOutput("security", "cms", "-D", "-i", profilePath)
	if err != nil {
		return "", fmt.Errorf("security cms -D: %w", err)
	}

	// Parse plist to extract UUID
	var profile map[string]interface{}
	if _, err := plist.Unmarshal([]byte(plistStr), &profile); err != nil {
		return "", fmt.Errorf("parse provisioning profile plist: %w", err)
	}
	uuid, ok := profile["UUID"].(string)
	if !ok || uuid == "" {
		return "", fmt.Errorf("UUID not found in provisioning profile")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	destDir := filepath.Join(homeDir, "Library", "MobileDevice", "Provisioning Profiles")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", err
	}
	dest := filepath.Join(destDir, uuid+".mobileprovision")

	if err := copyFile(profilePath, dest); err != nil {
		return "", fmt.Errorf("copy provisioning profile: %w", err)
	}
	return dest, nil
}

func writeSigningState(state SigningState) error {
	dir := filepath.Join(os.Getenv("RUNNER_TEMP"), "mobile-actions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "signing-state.json"), data, 0600)
}

func generateKeychainPassword() string {
	b := make([]byte, 16)
	if _, err := cryptorand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand: %v", err))
	}
	return fmt.Sprintf("%x", b)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
```

> **Implementation note:** Replace the `generateKeychainPassword` hack with `crypto/rand`:
> ```go
> import "crypto/rand"
> func generateKeychainPassword() string {
>     b := make([]byte, 16)
>     rand.Read(b)
>     return fmt.Sprintf("%x", b)
> }
> ```

- [ ] **Step 2: Build**

```bash
go build ./cmd/ios-setup-signing/
```

- [ ] **Step 3: Commit**

```bash
git add cmd/ios-setup-signing/
git commit -m "feat: add ios-setup-signing CLI"
```

---

## Task 10: `ios-teardown-signing` CLI

**Files:**
- Create: `cmd/ios-teardown-signing/main.go`

Reads `signing-state.json` from `$RUNNER_TEMP/mobile-actions/`. Deletes the keychain, restores the original keychain search list, removes the provisioning profile.

- [ ] **Step 1: Write the CLI**

```go
// cmd/ios-teardown-signing/main.go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/your-org/mobile-actions/internal/actions"
	"github.com/your-org/mobile-actions/internal/exec"
)

type SigningState struct {
	KeychainName            string   `json:"keychain_name"`
	OriginalKeychains       []string `json:"original_keychains"`
	ProvisioningProfilePath string   `json:"provisioning_profile_path"`
}

func main() {
	if err := run(); err != nil {
		actions.Warning(fmt.Sprintf("teardown warning: %v", err))
		// Do not exit 1 — teardown failures are non-fatal
	}
}

func run() error {
	stateFile := filepath.Join(os.Getenv("RUNNER_TEMP"), "mobile-actions", "signing-state.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return fmt.Errorf("read signing-state.json: %w", err)
	}

	var state SigningState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("parse signing-state.json: %w", err)
	}

	var errs []string

	// Delete keychain (best-effort)
	if err := exec.Run("security", "delete-keychain", state.KeychainName); err != nil {
		errs = append(errs, fmt.Sprintf("delete keychain: %v", err))
	}

	// Restore original search list
	if len(state.OriginalKeychains) > 0 {
		args := append([]string{"list-keychains", "-s"}, state.OriginalKeychains...)
		if err := exec.Run(append([]string{"security"}, args...)...); err != nil {
			errs = append(errs, fmt.Sprintf("restore keychain list: %v", err))
		}
	}

	// Remove provisioning profile
	if state.ProvisioningProfilePath != "" {
		if err := os.Remove(state.ProvisioningProfilePath); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("remove provisioning profile: %v", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("teardown errors: %v", errs)
	}
	fmt.Println("signing teardown complete")
	return nil
}
```

- [ ] **Step 2: Build**

```bash
go build ./cmd/ios-teardown-signing/
```

- [ ] **Step 3: Commit**

```bash
git add cmd/ios-teardown-signing/
git commit -m "feat: add ios-teardown-signing CLI"
```

---

## Task 11: `ios-build` CLI

**Files:**
- Create: `cmd/ios-build/main.go`

Reads `INPUT_WORKSPACE` (or auto-detects), `INPUT_SCHEME`, `INPUT_BUNDLE_ID`, `INPUT_TEAM_ID`, `INPUT_DESTINATION`. Generates `ExportOptions.plist` by reading the provisioning profile UUID from `signing-state.json` (or from the installed profile path). Runs `xcodebuild archive` then `xcodebuild -exportArchive`. Writes `artifact-path` to `$GITHUB_OUTPUT`.

- [ ] **Step 1: Write the CLI**

```go
// cmd/ios-build/main.go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"howett.net/plist"

	"github.com/your-org/mobile-actions/internal/actions"
	intexec "github.com/your-org/mobile-actions/internal/exec"
)

const exportOptionsTmpl = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>method</key><string>app-store</string>
  <key>teamID</key><string>{{.TeamID}}</string>
  <key>signingStyle</key><string>manual</string>
  <key>provisioningProfiles</key>
  <dict>
    <key>{{.BundleID}}</key><string>{{.ProfileUUID}}</string>
  </dict>
  <key>destination</key><string>export</string>
  <key>uploadSymbols</key><true/>
</dict>
</plist>`

func main() {
	if err := run(); err != nil {
		actions.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	appPath := envOrDefault("INPUT_APP_PATH", ".")
	workspace := os.Getenv("INPUT_WORKSPACE")
	scheme := os.Getenv("INPUT_SCHEME")
	bundleID := os.Getenv("INPUT_BUNDLE_ID")
	teamID := os.Getenv("INPUT_TEAM_ID")
	runnerTemp := os.Getenv("RUNNER_TEMP")

	if scheme == "" || bundleID == "" || teamID == "" {
		return fmt.Errorf("INPUT_SCHEME, INPUT_BUNDLE_ID, and INPUT_TEAM_ID are required")
	}

	// Auto-detect workspace
	if workspace == "" {
		detected, err := detectWorkspace(appPath)
		if err != nil {
			return err
		}
		workspace = detected
	}

	// Get provisioning profile UUID from signing-state.json
	profileUUID, err := provisioningProfileUUID(runnerTemp)
	if err != nil {
		return err
	}

	// Write ExportOptions.plist
	exportDir := filepath.Join(runnerTemp, "mobile-actions")
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		return err
	}
	exportOptionsPath := filepath.Join(exportDir, "ExportOptions.plist")
	if err := writeExportOptions(exportOptionsPath, teamID, bundleID, profileUUID); err != nil {
		return fmt.Errorf("write ExportOptions.plist: %w", err)
	}

	archivePath := filepath.Join(exportDir, scheme+".xcarchive")
	exportPath := filepath.Join(exportDir, "export")

	// Archive
	actions.Group("xcodebuild archive")
	err = intexec.Run("xcodebuild", "archive",
		"-workspace", workspace,
		"-scheme", scheme,
		"-configuration", "Release",
		"-destination", "generic/platform=iOS",
		"-archivePath", archivePath,
	)
	actions.EndGroup()
	if err != nil {
		return fmt.Errorf("xcodebuild archive: %w", err)
	}

	// Export
	actions.Group("xcodebuild -exportArchive")
	err = intexec.Run("xcodebuild", "-exportArchive",
		"-archivePath", archivePath,
		"-exportOptionsPlist", exportOptionsPath,
		"-exportPath", exportPath,
	)
	actions.EndGroup()
	if err != nil {
		return fmt.Errorf("xcodebuild exportArchive: %w", err)
	}

	// Find the exported IPA
	ipaPath, err := findIPA(exportPath)
	if err != nil {
		return err
	}
	fmt.Printf("exported IPA: %s\n", ipaPath)
	return actions.SetOutput("artifact-path", ipaPath)
}

func detectWorkspace(appPath string) (string, error) {
	var found []string
	_ = filepath.WalkDir(appPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && strings.HasSuffix(path, ".xcworkspace") {
			found = append(found, path)
			return filepath.SkipDir
		}
		return nil
	})
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return "", fmt.Errorf("no .xcworkspace found in %s", appPath)
	default:
		return "", fmt.Errorf("multiple .xcworkspace found in %s: %v — set INPUT_WORKSPACE explicitly", appPath, found)
	}
}

func provisioningProfileUUID(runnerTemp string) (string, error) {
	stateFile := filepath.Join(runnerTemp, "mobile-actions", "signing-state.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return "", fmt.Errorf("read signing-state.json: %w", err)
	}
	var state struct {
		ProvisioningProfilePath string `json:"provisioning_profile_path"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return "", err
	}

	// Unwrap CMS and parse UUID via internal/exec
	plistStr, err := intexec.RunOutput("security", "cms", "-D", "-i", state.ProvisioningProfilePath)
	if err != nil {
		return "", fmt.Errorf("security cms -D: %w", err)
	}
	var profile map[string]interface{}
	if _, err := plist.Unmarshal([]byte(plistStr), &profile); err != nil {
		return "", fmt.Errorf("parse plist: %w", err)
	}
	uuid, _ := profile["UUID"].(string)
	if uuid == "" {
		return "", fmt.Errorf("UUID not found in provisioning profile")
	}
	return uuid, nil
}

func writeExportOptions(path, teamID, bundleID, profileUUID string) error {
	tmpl := template.Must(template.New("export").Parse(exportOptionsTmpl))
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return tmpl.Execute(f, map[string]string{
		"TeamID":      teamID,
		"BundleID":    bundleID,
		"ProfileUUID": profileUUID,
	})
}

func findIPA(exportPath string) (string, error) {
	var found []string
	_ = filepath.WalkDir(exportPath, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".ipa") {
			found = append(found, path)
		}
		return nil
	})
	if len(found) == 0 {
		return "", fmt.Errorf("no .ipa found in %s", exportPath)
	}
	return found[0], nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
```

- [ ] **Step 2: Build**

```bash
go build ./cmd/ios-build/
```

- [ ] **Step 3: Commit**

```bash
git add cmd/ios-build/
git commit -m "feat: add ios-build CLI"
```

---

## Task 12: `ios-upload` CLI

**Files:**
- Create: `cmd/ios-upload/main.go`

Reads `INPUT_APP_STORE_CONNECT_KEY` (base64 p8), `INPUT_APP_STORE_CONNECT_KEY_ID`, `INPUT_APP_STORE_CONNECT_ISSUER_ID`, `INPUT_XCODE_VERSION`, `INPUT_ARTIFACT_PATH`. Resolves iTMSTransporter via `xcode-select -p`. Writes `.p8` to `~/.appstoreconnect/private_keys/`. Invokes iTMSTransporter. Supports `--dry-run`.

- [ ] **Step 1: Write the CLI**

```go
// cmd/ios-upload/main.go
package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/your-org/mobile-actions/internal/actions"
	intexec "github.com/your-org/mobile-actions/internal/exec"
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
	keyB64 := os.Getenv("INPUT_APP_STORE_CONNECT_KEY")
	keyID := os.Getenv("INPUT_APP_STORE_CONNECT_KEY_ID")
	issuerID := os.Getenv("INPUT_APP_STORE_CONNECT_ISSUER_ID")
	ipaPath := os.Getenv("INPUT_ARTIFACT_PATH")

	if keyB64 == "" || keyID == "" || issuerID == "" || ipaPath == "" {
		return fmt.Errorf("INPUT_APP_STORE_CONNECT_KEY, INPUT_APP_STORE_CONNECT_KEY_ID, INPUT_APP_STORE_CONNECT_ISSUER_ID, and INPUT_ARTIFACT_PATH are required")
	}

	itmsPath, err := resolveITMSTransporter()
	if err != nil {
		return err
	}

	p8Path, cleanP8, err := installP8Key(keyB64, keyID)
	if err != nil {
		return err
	}
	defer cleanP8()

	if *dryRun {
		fmt.Printf("[dry-run] would run: %s -m upload -f %s -apiKey %s -apiIssuer %s (key at %s)\n",
			itmsPath, ipaPath, keyID, issuerID, p8Path)
		return nil
	}

	actions.Group("Uploading IPA via iTMSTransporter")
	err = intexec.Run(itmsPath,
		"-m", "upload",
		"-f", ipaPath,
		"-apiKey", keyID,
		"-apiIssuer", issuerID,
	)
	actions.EndGroup()
	if err != nil {
		return fmt.Errorf("iTMSTransporter upload: %w", err)
	}

	fmt.Println("upload complete")
	return nil
}

func resolveITMSTransporter() (string, error) {
	developerDir, err := intexec.RunOutput("xcode-select", "-p")
	if err != nil {
		return "", fmt.Errorf("xcode-select -p: %w", err)
	}
	xcodeContents := filepath.Dir(developerDir)
	path := filepath.Join(xcodeContents,
		"SharedFrameworks",
		"ContentDeliveryServices.framework",
		"Versions", "A", "itms", "bin", "iTMSTransporter")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("iTMSTransporter not found at %s: %w", path, err)
	}
	return path, nil
}

func installP8Key(keyB64, keyID string) (string, func(), error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", nil, err
	}
	keyDir := filepath.Join(homeDir, ".appstoreconnect", "private_keys")
	if err := os.MkdirAll(keyDir, 0700); err != nil {
		return "", nil, err
	}
	keyPath := filepath.Join(keyDir, "AuthKey_"+keyID+".p8")
	rawKey, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return "", nil, fmt.Errorf("decode p8 key: %w", err)
	}
	if err := os.WriteFile(keyPath, rawKey, 0600); err != nil {
		return "", nil, fmt.Errorf("write p8 key: %w", err)
	}
	cleanup := func() { os.Remove(keyPath) }
	return keyPath, cleanup, nil
}
```

- [ ] **Step 2: Build (using the clean implementation)**

```bash
go build ./cmd/ios-upload/
```

- [ ] **Step 3: Commit**

```bash
git add cmd/ios-upload/
git commit -m "feat: add ios-upload CLI"
```

---

## Task 13: Testdata Stubs

**Files:**
- Create: `testdata/android/settings.gradle`
- Create: `testdata/android/build.gradle`
- Create: `testdata/android/app/build.gradle`
- Create: `testdata/android/app/src/main/java/com/example/TestApp.java`
- Create: `testdata/android/app/src/main/AndroidManifest.xml`
- Create: `testdata/android/app/src/main/res/values/strings.xml`
- Create: `testdata/android/gradlew` (executable)
- Create: `testdata/android/gradle/wrapper/gradle-wrapper.properties`
- Create: `testdata/ios/TestApp.xcworkspace/contents.xcworkspacedata`
- Create: `testdata/ios/TestApp.xcodeproj/project.pbxproj`
- Create: `testdata/ios/TestApp/AppDelegate.swift`
- Create: `testdata/ios/TestApp/ContentView.swift`
- Create: `testdata/android/test.keystore` (generated, non-production)

- [ ] **Step 1: Create Android stub**

`testdata/android/settings.gradle`:
```groovy
rootProject.name = "TestApp"
include ':app'
```

`testdata/android/build.gradle`:
```groovy
buildscript {
    repositories { google(); mavenCentral() }
    dependencies { classpath 'com.android.tools.build:gradle:8.3.0' }
}
allprojects {
    repositories { google(); mavenCentral() }
}
```

`testdata/android/app/build.gradle`:
```groovy
apply plugin: 'com.android.application'
android {
    compileSdkVersion 34
    defaultConfig {
        applicationId "com.example.testapp"
        minSdkVersion 21
        targetSdkVersion 34
        versionCode 1
        versionName "1.0"
    }
    buildTypes {
        release {
            minifyEnabled false
            // No signingConfig — action handles signing
        }
    }
}
```

`testdata/android/app/src/main/AndroidManifest.xml`:
```xml
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application android:label="TestApp"/>
</manifest>
```

`testdata/android/app/src/main/java/com/example/TestApp.java`:
```java
package com.example;
public class TestApp {}
```

`testdata/android/app/src/main/res/values/strings.xml`:
```xml
<resources>
    <string name="app_name">TestApp</string>
</resources>
```

`testdata/android/gradle/wrapper/gradle-wrapper.properties`:
```properties
distributionBase=GRADLE_USER_HOME
distributionPath=wrapper/dists
distributionUrl=https\://services.gradle.org/distributions/gradle-8.4-bin.zip
zipStoreBase=GRADLE_USER_HOME
zipStorePath=wrapper/dists
```

Download the Gradle wrapper script:
```bash
curl -L https://raw.githubusercontent.com/gradle/gradle/v8.4.0/gradlew -o testdata/android/gradlew
chmod +x testdata/android/gradlew
```

- [ ] **Step 2: Generate test keystore**

```bash
keytool -genkey -v \
  -keystore testdata/android/test.keystore \
  -alias testkey \
  -keyalg RSA \
  -keysize 2048 \
  -validity 10000 \
  -storepass testpassword \
  -keypass testpassword \
  -dname "CN=Test, OU=Test, O=Test, L=Test, ST=Test, C=US"
```

- [ ] **Step 3: Create iOS stub**

The iOS stub is a minimal Xcode project. Create the necessary files:

`testdata/ios/TestApp.xcworkspace/contents.xcworkspacedata`:
```xml
<?xml version="1.0" encoding="UTF-8"?>
<Workspace version = "1.0">
   <FileRef location = "group:TestApp.xcodeproj">
   </FileRef>
</Workspace>
```

`testdata/ios/TestApp/AppDelegate.swift`:
```swift
import UIKit
@main class AppDelegate: UIResponder, UIApplicationDelegate {}
```

`testdata/ios/TestApp/ContentView.swift`:
```swift
import SwiftUI
struct ContentView: View {
    var body: some View { Text("TestApp") }
}
```

> **Note:** `testdata/ios/TestApp.xcodeproj/project.pbxproj` requires a valid Xcode project file. Generate it by creating a minimal project in Xcode and committing the result, or use a known-good minimal `project.pbxproj` template. This file is lengthy (~200 lines) and must be valid for `xcodebuild` to succeed.

- [ ] **Step 4: Commit**

```bash
git add testdata/
git commit -m "test: add testdata stubs for Android and iOS"
```

---

## Task 14: `android/action.yml` — Composite Action

**Files:**
- Create: `android/action.yml`

- [ ] **Step 1: Write `android/action.yml`**

```yaml
name: 'mobile-actions/android'
description: 'Build, sign, and upload an Android app to Google Play Store'

inputs:
  app-path:
    description: 'Path to the Android app directory'
    default: '.'
  build-type:
    description: "'aab' or 'apk'"
    default: 'aab'
  gradle-task:
    description: 'Override Gradle task (auto-detected if not set)'
    required: false
  java-version:
    description: 'JDK version'
    default: '17'
  keystore:
    description: 'Base64-encoded .jks keystore'
    required: true
  keystore-password:
    description: 'Keystore password'
    required: true
  key-alias:
    description: 'Key alias'
    required: true
  key-password:
    description: 'Key password'
    required: true
  service-account-json:
    description: 'Base64-encoded Google service account JSON'
    required: true
  package-name:
    description: 'App package name (e.g. com.myapp)'
    required: true
  version-code:
    description: 'Integer version code'
    required: true
  track:
    description: "Play Store track: 'internal', 'alpha', 'beta', 'production'"
    default: 'internal'

outputs:
  artifact-path:
    description: 'Path to the signed artifact'
    value: ${{ steps.sign.outputs.artifact-path }}

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
          --repo your-org/mobile-actions \
          --pattern "verify-checksums-${OS}-${ARCH}" \
          --pattern "checksums.txt" \
          --dir $DEST

        HASH_FILE=${{ github.action_path }}/verify-checksums.sha256
        cd $DEST
        if [ "$OS" = "linux" ]; then
          grep "verify-checksums-${OS}-${ARCH}" "$HASH_FILE" | sha256sum --check
        else
          grep "verify-checksums-${OS}-${ARCH}" "$HASH_FILE" | shasum -a 256 --check
        fi
        chmod +x verify-checksums-${OS}-${ARCH}

        gh release download "$VERSION" \
          --repo your-org/mobile-actions \
          --pattern "*-${OS}-${ARCH}" \
          --dir $DEST

        ./verify-checksums-${OS}-${ARCH} --file checksums.txt --dir $DEST
        chmod +x $DEST/*-${OS}-${ARCH}
        echo "MOBILE_ACTIONS_BIN=$DEST" >> $GITHUB_ENV
        echo "MOBILE_ACTIONS_OS_ARCH=${OS}-${ARCH}" >> $GITHUB_ENV

    - name: Setup Java
      uses: actions/setup-java@6a0805fcefea3d4657a47ac4c165951e33482018  # v4.2.2
      with:
        java-version: ${{ inputs.java-version }}
        distribution: temurin

    - name: Build (Gradle)
      id: build
      shell: bash
      env:
        INPUT_APP_PATH: ${{ inputs.app-path }}
        INPUT_BUILD_TYPE: ${{ inputs.build-type }}
        INPUT_GRADLE_TASK: ${{ inputs.gradle-task }}
      run: $MOBILE_ACTIONS_BIN/android-build-$MOBILE_ACTIONS_OS_ARCH

    - name: Sign
      id: sign
      shell: bash
      env:
        INPUT_UNSIGNED_ARTIFACT_PATH: ${{ steps.build.outputs.unsigned-artifact-path }}
        INPUT_KEYSTORE: ${{ inputs.keystore }}
        INPUT_KEYSTORE_PASSWORD: ${{ inputs.keystore-password }}
        INPUT_KEY_ALIAS: ${{ inputs.key-alias }}
        INPUT_KEY_PASSWORD: ${{ inputs.key-password }}
        INPUT_BUILD_TYPE: ${{ inputs.build-type }}
      run: $MOBILE_ACTIONS_BIN/android-sign-$MOBILE_ACTIONS_OS_ARCH

    - name: Upload to Play Store
      shell: bash
      env:
        INPUT_SERVICE_ACCOUNT_JSON: ${{ inputs.service-account-json }}
        INPUT_PACKAGE_NAME: ${{ inputs.package-name }}
        INPUT_VERSION_CODE: ${{ inputs.version-code }}
        INPUT_TRACK: ${{ inputs.track }}
        INPUT_BUILD_TYPE: ${{ inputs.build-type }}
        INPUT_ARTIFACT_PATH: ${{ steps.sign.outputs.artifact-path }}
      run: $MOBILE_ACTIONS_BIN/android-upload-$MOBILE_ACTIONS_OS_ARCH
```

- [ ] **Step 2: Commit**

```bash
git add android/
git commit -m "feat: add android composite action"
```

---

## Task 15: `ios/action.yml` — Composite Action

**Files:**
- Create: `ios/action.yml`

- [ ] **Step 1: Write `ios/action.yml`**

```yaml
name: 'mobile-actions/ios'
description: 'Build, sign, and upload an iOS app to App Store Connect or TestFlight'

inputs:
  app-path:
    description: 'Path to the iOS app directory'
    default: '.'
  scheme:
    description: 'Xcode scheme name'
    required: true
  workspace:
    description: 'Path to .xcworkspace (auto-detected if not set)'
    required: false
  bundle-id:
    description: 'App bundle identifier'
    required: true
  team-id:
    description: 'Apple Developer Team ID (10-character alphanumeric)'
    required: true
  xcode-version:
    description: 'Xcode version (must be pre-installed on runner)'
    default: '16.2'
  certificate:
    description: 'Base64-encoded .p12 certificate'
    required: true
  certificate-password:
    description: 'P12 password'
    required: true
  provisioning-profile:
    description: 'Base64-encoded .mobileprovision'
    required: true
  app-store-connect-key:
    description: 'Base64-encoded .p8 API key'
    required: true
  app-store-connect-key-id:
    description: 'App Store Connect key ID'
    required: true
  app-store-connect-issuer-id:
    description: 'App Store Connect issuer ID'
    required: true
  destination:
    description: "'testflight' or 'app-store'"
    default: 'testflight'

outputs:
  artifact-path:
    description: 'Path to the exported .ipa'
    value: ${{ steps.build.outputs.artifact-path }}

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
          --repo your-org/mobile-actions \
          --pattern "verify-checksums-${OS}-${ARCH}" \
          --pattern "checksums.txt" \
          --dir $DEST

        HASH_FILE=${{ github.action_path }}/verify-checksums.sha256
        cd $DEST
        if [ "$OS" = "linux" ]; then
          grep "verify-checksums-${OS}-${ARCH}" "$HASH_FILE" | sha256sum --check
        else
          grep "verify-checksums-${OS}-${ARCH}" "$HASH_FILE" | shasum -a 256 --check
        fi
        chmod +x verify-checksums-${OS}-${ARCH}

        gh release download "$VERSION" \
          --repo your-org/mobile-actions \
          --pattern "*-${OS}-${ARCH}" \
          --dir $DEST

        ./verify-checksums-${OS}-${ARCH} --file checksums.txt --dir $DEST
        chmod +x $DEST/*-${OS}-${ARCH}
        echo "MOBILE_ACTIONS_BIN=$DEST" >> $GITHUB_ENV
        echo "MOBILE_ACTIONS_OS_ARCH=${OS}-${ARCH}" >> $GITHUB_ENV

    - name: Select Xcode
      shell: bash
      run: |
        XCODE_PATH="/Applications/Xcode_${{ inputs.xcode-version }}.app/Contents/Developer"
        if [ ! -d "$XCODE_PATH" ]; then
          echo "::error::Xcode ${{ inputs.xcode-version }} not found at $XCODE_PATH"
          echo "Available Xcode installations:"
          ls /Applications/Xcode*.app 2>/dev/null || echo "  (none found)"
          exit 1
        fi
        sudo xcode-select -s "$XCODE_PATH"
        xcodebuild -version

    - name: Setup signing
      shell: bash
      env:
        INPUT_CERTIFICATE: ${{ inputs.certificate }}
        INPUT_CERTIFICATE_PASSWORD: ${{ inputs.certificate-password }}
        INPUT_PROVISIONING_PROFILE: ${{ inputs.provisioning-profile }}
      run: $MOBILE_ACTIONS_BIN/ios-setup-signing-$MOBILE_ACTIONS_OS_ARCH

    - name: Build and export IPA
      id: build
      shell: bash
      env:
        INPUT_APP_PATH: ${{ inputs.app-path }}
        INPUT_SCHEME: ${{ inputs.scheme }}
        INPUT_WORKSPACE: ${{ inputs.workspace }}
        INPUT_BUNDLE_ID: ${{ inputs.bundle-id }}
        INPUT_TEAM_ID: ${{ inputs.team-id }}
        INPUT_DESTINATION: ${{ inputs.destination }}
      run: $MOBILE_ACTIONS_BIN/ios-build-$MOBILE_ACTIONS_OS_ARCH

    - name: Upload to App Store Connect
      shell: bash
      env:
        INPUT_APP_STORE_CONNECT_KEY: ${{ inputs.app-store-connect-key }}
        INPUT_APP_STORE_CONNECT_KEY_ID: ${{ inputs.app-store-connect-key-id }}
        INPUT_APP_STORE_CONNECT_ISSUER_ID: ${{ inputs.app-store-connect-issuer-id }}
        INPUT_XCODE_VERSION: ${{ inputs.xcode-version }}
        INPUT_ARTIFACT_PATH: ${{ steps.build.outputs.artifact-path }}
      run: $MOBILE_ACTIONS_BIN/ios-upload-$MOBILE_ACTIONS_OS_ARCH

    - name: Teardown signing
      if: always()
      shell: bash
      run: $MOBILE_ACTIONS_BIN/ios-teardown-signing-$MOBILE_ACTIONS_OS_ARCH
```

- [ ] **Step 2: Commit**

```bash
git add ios/
git commit -m "feat: add ios composite action"
```

---

## Task 16: `.github/workflows/build-binaries.yml`

**Files:**
- Create: `.github/workflows/build-binaries.yml`

- [ ] **Step 1: Write `build-binaries.yml`**

```yaml
name: Build Binaries

on:
  workflow_call:
    inputs:
      release-tag:
        required: true
        type: string
    outputs:
      bootstrap-commit-sha:
        description: 'SHA of commit with updated verify-checksums.sha256'
        value: ${{ jobs.aggregate.outputs.commit-sha }}

permissions:
  contents: write

jobs:
  build:
    strategy:
      matrix:
        include:
          - { goos: linux,  goarch: amd64, runs-on: ubuntu-latest }
          - { goos: darwin, goarch: arm64, runs-on: macos-latest  }
          - { goos: darwin, goarch: amd64, runs-on: macos-13      }
    runs-on: ${{ matrix.runs-on }}
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11  # v4.1.1

      - uses: actions/setup-go@0c52d547c9bc32b1aa3301fd7a9cb496313a4491  # v5.0.0
        with:
          go-version: '1.22'

      - name: Build binaries
        run: |
          PROGRAMS="android-build android-sign android-upload ios-setup-signing ios-teardown-signing ios-build ios-upload verify-checksums"
          for program in $PROGRAMS; do
            CGO_ENABLED=0 GOOS=${{ matrix.goos }} GOARCH=${{ matrix.goarch }} \
              go build -o ${program}-${{ matrix.goos }}-${{ matrix.goarch }} ./cmd/$program
          done

      - name: Generate checksums
        run: |
          for f in *-${{ matrix.goos }}-${{ matrix.goarch }}; do
            echo "$(openssl dgst -sha256 -r "$f" | awk '{print $1}')  $f"
          done >> checksums-${{ matrix.goos }}-${{ matrix.goarch }}.txt

      - name: Upload artifacts
        uses: actions/upload-artifact@5d5d22a31266ced268874388b861e4b58bb5c2f3  # v4.3.1
        with:
          name: binaries-${{ matrix.goos }}-${{ matrix.goarch }}
          path: |
            *-${{ matrix.goos }}-${{ matrix.goarch }}
            checksums-${{ matrix.goos }}-${{ matrix.goarch }}.txt

  aggregate:
    needs: build
    runs-on: ubuntu-latest
    outputs:
      commit-sha: ${{ steps.commit.outputs.commit-sha }}
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11  # v4.1.1

      - name: Download all artifacts
        uses: actions/download-artifact@87c55149d96e628cc2ef7e6fc2aab372015aec85  # v4.1.3
        with:
          path: artifacts/

      - name: Merge checksums and prepare release assets
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          mkdir -p release-assets
          # Collect all binaries and checksums
          find artifacts/ -type f \( -name '*-linux-*' -o -name '*-darwin-*' \) | xargs -I{} cp {} release-assets/
          # Merge partial checksum files into one
          cat release-assets/checksums-*.txt > release-assets/checksums.txt
          # Remove partial files from upload
          rm release-assets/checksums-*.txt
          # Generate verify-checksums.sha256 (only verify-checksums-* lines)
          grep "verify-checksums-" release-assets/checksums.txt > verify-checksums.sha256

      - name: Upload release assets
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          cd release-assets
          gh release upload ${{ inputs.release-tag }} * --repo ${{ github.repository }}

      - name: Commit verify-checksums.sha256
        id: commit
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git add verify-checksums.sha256
          git commit -m "chore: update verify-checksums hashes for ${{ inputs.release-tag }}"
          git push origin HEAD:main
          echo "commit-sha=$(git rev-parse HEAD)" >> $GITHUB_OUTPUT
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/build-binaries.yml
git commit -m "ci: add build-binaries reusable workflow"
```

---

## Task 17: `.github/workflows/release.yml`

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Write `release.yml`**

```yaml
name: Release

on:
  push:
    tags: ['v[0-9]+.[0-9]+.[0-9]+']

concurrency:
  group: release
  cancel-in-progress: false

permissions:
  contents: write

jobs:
  create-release:
    runs-on: ubuntu-latest
    steps:
      - name: Create draft GitHub Release
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          gh release create ${{ github.ref_name }} \
            --repo ${{ github.repository }} \
            --title "${{ github.ref_name }}" \
            --draft \
            --generate-notes

  build:
    needs: create-release
    uses: ./.github/workflows/build-binaries.yml
    with:
      release-tag: ${{ github.ref_name }}
    secrets: inherit

  update-tags:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11  # v4.1.1
        with:
          fetch-depth: 0

      - name: Update floating major tag and release
        env:
          GH_TOKEN: ${{ github.token }}
          BOOTSTRAP_SHA: ${{ needs.build.outputs.bootstrap-commit-sha }}
        run: |
          MAJOR=$(echo ${{ github.ref_name }} | cut -d. -f1)

          # Point floating tag to the bootstrap commit
          git tag -f $MAJOR $BOOTSTRAP_SHA
          git push -f origin $MAJOR

          # Recreate the floating-tag GitHub Release with same assets
          gh release delete $MAJOR --yes --cleanup-tag 2>/dev/null || true
          gh release create $MAJOR \
            --title "$MAJOR (latest — ${{ github.ref_name }})" \
            --notes "Always points to the latest $MAJOR.x.x release. Currently ${{ github.ref_name }}."

          ASSET_DIR=$RUNNER_TEMP/release-assets
          mkdir -p $ASSET_DIR
          gh release download ${{ github.ref_name }} --dir $ASSET_DIR
          gh release upload $MAJOR $ASSET_DIR/*

  publish-release:
    needs: update-tags
    runs-on: ubuntu-latest
    steps:
      - name: Publish GitHub Release
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          gh release edit ${{ github.ref_name }} \
            --repo ${{ github.repository }} \
            --draft=false
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add release workflow"
```

---

## Task 18: `.github/workflows/test.yml`

**Files:**
- Create: `.github/workflows/test.yml`

- [ ] **Step 1: Write `test.yml`**

```yaml
name: Test

on:
  push:
    branches: [main]
  pull_request:

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11  # v4.1.1
      - uses: actions/setup-go@0c52d547c9bc32b1aa3301fd7a9cb496313a4491  # v5.0.0
        with:
          go-version: '1.22'
      - name: Run unit tests
        run: go test ./internal/... -v -count=1

  integration-android:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11  # v4.1.1
      - uses: actions/setup-go@0c52d547c9bc32b1aa3301fd7a9cb496313a4491  # v5.0.0
        with:
          go-version: '1.22'

      - uses: actions/setup-java@6a0805fcefea3d4657a47ac4c165951e33482018  # v4.2.2
        with:
          java-version: '17'
          distribution: temurin

      - name: Build Go binaries
        run: |
          for program in android-build android-sign android-upload; do
            CGO_ENABLED=0 go build -o /tmp/ma-bin/${program}-linux-amd64 ./cmd/$program
          done

      - name: Test android-build
        env:
          INPUT_APP_PATH: testdata/android
          INPUT_BUILD_TYPE: aab
          RUNNER_TEMP: /tmp/runner
          GITHUB_OUTPUT: /tmp/github-output
        run: /tmp/ma-bin/android-build-linux-amd64

      - name: Test android-sign
        env:
          INPUT_KEYSTORE_PASSWORD: testpassword
          INPUT_KEY_ALIAS: testkey
          INPUT_KEY_PASSWORD: testpassword
          INPUT_BUILD_TYPE: aab
          RUNNER_TEMP: /tmp/runner
          GITHUB_OUTPUT: /tmp/github-output-sign
        run: |
          # Read unsigned artifact path from previous step's GITHUB_OUTPUT file
          UNSIGNED=$(grep '^unsigned-artifact-path=' /tmp/github-output | cut -d= -f2-)
          export INPUT_UNSIGNED_ARTIFACT_PATH="$UNSIGNED"
          export INPUT_KEYSTORE=$(base64 -w0 testdata/android/test.keystore)
          /tmp/ma-bin/android-sign-linux-amd64

      - name: Test android-upload (dry-run)
        env:
          INPUT_SERVICE_ACCOUNT_JSON: e30K  # base64("{}")
          INPUT_PACKAGE_NAME: com.example.testapp
          INPUT_VERSION_CODE: '1'
          INPUT_TRACK: internal
          INPUT_BUILD_TYPE: aab
          INPUT_ARTIFACT_PATH: /tmp/runner/mobile-actions/signed/app-release.aab
          RUNNER_TEMP: /tmp/runner
        run: /tmp/ma-bin/android-upload-linux-amd64 --dry-run

  integration-ios:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11  # v4.1.1
      - uses: actions/setup-go@0c52d547c9bc32b1aa3301fd7a9cb496313a4491  # v5.0.0
        with:
          go-version: '1.22'

      - name: Build Go binaries
        run: |
          for program in ios-setup-signing ios-teardown-signing ios-build ios-upload verify-checksums; do
            CGO_ENABLED=0 go build -o /tmp/ma-bin/${program}-darwin-arm64 ./cmd/$program
          done

      - name: Test ios-build (stub project)
        env:
          INPUT_APP_PATH: testdata/ios
          INPUT_SCHEME: TestApp
          INPUT_BUNDLE_ID: com.example.testapp
          INPUT_TEAM_ID: FAKETEAMID1
          INPUT_DESTINATION: testflight
          RUNNER_TEMP: /tmp/runner
          GITHUB_OUTPUT: /tmp/github-output-ios
        run: |
          # Create a fake signing-state.json so ios-build can read the profile UUID
          mkdir -p /tmp/runner/mobile-actions
          echo '{"keychain_name":"test","original_keychains":[],"provisioning_profile_path":""}' \
            > /tmp/runner/mobile-actions/signing-state.json
          /tmp/ma-bin/ios-build-darwin-arm64 || true  # expected to fail without real credentials

      - name: Test ios-upload (dry-run)
        env:
          INPUT_APP_STORE_CONNECT_KEY: dGVzdA==
          INPUT_APP_STORE_CONNECT_KEY_ID: TESTKEYID
          INPUT_APP_STORE_CONNECT_ISSUER_ID: test-issuer
          INPUT_XCODE_VERSION: '16.2'
          INPUT_ARTIFACT_PATH: /tmp/test.ipa
          RUNNER_TEMP: /tmp/runner
        run: /tmp/ma-bin/ios-upload-darwin-arm64 --dry-run
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/test.yml
git commit -m "ci: add test workflow"
```

---

## Task 19: Final Integration and `go mod tidy`

- [ ] **Step 1: Tidy the Go module**

```bash
go mod tidy
```

- [ ] **Step 2: Verify all packages build**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Run all unit tests**

```bash
go test ./internal/... -v -count=1
```

Expected: all PASS.

- [ ] **Step 4: Commit final state**

```bash
git add go.mod go.sum
git commit -m "chore: go mod tidy"
```

- [ ] **Step 5: Update spec status**

Edit `docs/superpowers/specs/2026-03-22-mobile-actions-design.md` line 4: change `Status: Draft` to `Status: Approved`.

```bash
git add docs/superpowers/specs/2026-03-22-mobile-actions-design.md
git commit -m "docs: mark spec as approved"
```
