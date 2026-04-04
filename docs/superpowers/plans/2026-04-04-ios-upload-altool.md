# ios-upload: Replace iTMSTransporter with altool

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the broken `iTMSTransporter` binary in `cmd/ios-upload/main.go` with `xcrun altool`, which is bundled with Xcode and supports the same API key authentication.

**Architecture:** Delete `resolveITMSTransporter()` and add `resolveAltool()` that shells out to `xcrun -find altool`. Update the upload invocation to use altool's argument format. `installP8Key()` is unchanged — altool reads from the same `~/.appstoreconnect/private_keys/` path.

**Tech Stack:** Go 1.26, `internal/exec` (`RunOutput`, `Run`), `internal/actions` (`Group`/`EndGroup`, `Error`), `xcrun altool`

---

## Files

- Modify: `cmd/ios-upload/main.go` — remove `resolveITMSTransporter`, add `resolveAltool`, update `run()`
- Modify: `cmd/ios-upload/main_test.go` — add `TestResolveAltool`

---

### Task 1: Add failing test for `resolveAltool`

**Files:**
- Modify: `cmd/ios-upload/main_test.go`

- [ ] **Step 1: Add the test to `main_test.go`**

Append after the last test in the file (after `TestInstallP8Key_FilePermissions`):

```go
func TestResolveAltool_ReturnsPath(t *testing.T) {
	if _, err := exec.LookPath("xcrun"); err != nil {
		t.Skip("xcrun not available")
	}
	path, err := resolveAltool()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}
	if !strings.Contains(path, "altool") {
		t.Fatalf("expected path to contain 'altool', got %q", path)
	}
}
```

Also add `"os/exec"` to the import block (it already imports `"os"`, `"strings"`, etc. — add `"os/exec"` alongside them):

```go
import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)
```

- [ ] **Step 2: Run the test to confirm it fails (function not defined yet)**

```bash
go test ./cmd/ios-upload/ -v -count=1 -run TestResolveAltool
```

Expected: compile error — `undefined: resolveAltool`

---

### Task 2: Replace `resolveITMSTransporter` with `resolveAltool` in `main.go`

**Files:**
- Modify: `cmd/ios-upload/main.go`

- [ ] **Step 1: Replace the `resolveITMSTransporter` function with `resolveAltool`**

Remove the entire `resolveITMSTransporter` function (lines 68–97) and replace with:

```go
func resolveAltool() (string, error) {
	path, err := intexec.RunOutput("xcrun", "-find", "altool")
	if err != nil || path == "" {
		return "", fmt.Errorf("altool not found via xcrun — ensure Xcode is installed")
	}
	return path, nil
}
```

- [ ] **Step 2: Update `run()` to call `resolveAltool` and use altool argument format**

In `run()`, replace:

```go
itmsPath, err := resolveITMSTransporter()
if err != nil {
    return err
}
```

with:

```go
altoolPath, err := resolveAltool()
if err != nil {
    return err
}
```

Replace the dry-run log line:

```go
fmt.Printf("[dry-run] would run: %s -m upload -f %s -apiKey %s -apiIssuer %s (key at %s)\n",
    itmsPath, ipaPath, keyID, issuerID, p8Path)
```

with:

```go
fmt.Printf("[dry-run] would run: %s --upload-app -f %s --apiKey %s --apiIssuer %s (key at %s)\n",
    altoolPath, ipaPath, keyID, issuerID, p8Path)
```

Replace the group label and Run call:

```go
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
```

with:

```go
actions.Group("Uploading IPA via altool")
err = intexec.Run(altoolPath,
    "--upload-app",
    "-f", ipaPath,
    "--apiKey", keyID,
    "--apiIssuer", issuerID,
)
actions.EndGroup()
if err != nil {
    return fmt.Errorf("altool upload: %w", err)
}
```

- [ ] **Step 3: Verify the file compiles**

```bash
CGO_ENABLED=0 go build ./cmd/ios-upload/
```

Expected: no output, exit 0. Delete the produced binary if any:

```bash
rm -f ios-upload
```

---

### Task 3: Run all tests and commit

**Files:** none new

- [ ] **Step 1: Run the new test**

```bash
go test ./cmd/ios-upload/ -v -count=1 -run TestResolveAltool
```

Expected output (on a machine with Xcode):
```
=== RUN   TestResolveAltool_ReturnsPath
--- PASS: TestResolveAltool_ReturnsPath (0.Xs)
PASS
```

Expected output (on a machine without xcrun, e.g. Linux CI):
```
=== RUN   TestResolveAltool_ReturnsPath
    main_test.go:XX: xcrun not available
--- SKIP: TestResolveAltool_ReturnsPath (0.00s)
PASS
```

- [ ] **Step 2: Run the full test suite**

```bash
go test ./internal/... ./cmd/... -v -count=1
```

Expected: all existing tests pass, new test passes or skips. No failures.

- [ ] **Step 3: Lint**

```bash
go vet ./...
```

Expected: no output, exit 0.

- [ ] **Step 4: Commit**

```bash
git add cmd/ios-upload/main.go cmd/ios-upload/main_test.go
git commit -m "fix: replace iTMSTransporter with altool in ios-upload"
```
