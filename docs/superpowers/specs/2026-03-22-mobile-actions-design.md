# mobile-actions Design Spec

**Date:** 2026-03-22
**Status:** Approved

## Overview

`mobile-actions` is a GitHub Actions repository that provides reusable composite actions for automating the full release pipeline of mobile apps (React Native, Flutter, bare Expo, or native Android/iOS) to the Google Play Store and Apple App Store.

It exposes two composite actions:
- `your-org/mobile-actions/android-upload@v1` — build, sign, and upload an Android AAB/APK to the Play Store
- `your-org/mobile-actions/ios-upload@v1` — build, sign, and upload an iOS IPA to App Store Connect or TestFlight

---

## Trigger

Releases are triggered in consumer repos by pushing a git tag matching `v*` (e.g., `v1.2.3`). Each consumer's workflow listens for this event and calls the composite action(s).

---

## Repository Structure

```
mobile-actions/
├── android/
│   └── action.yml                  # Composite action for Android
├── ios/
│   └── action.yml                  # Composite action for iOS
├── cmd/
│   ├── android-build/main.go       # Runs Gradle, produces unsigned AAB/APK
│   ├── android-sign/main.go        # Decodes keystore, signs artifact
│   ├── android-upload/main.go      # Uploads to Play Store via Google Play Developer API v3
│   ├── ios-setup-signing/main.go   # Creates temp keychain, installs p12 + provisioning profile
│   ├── ios-teardown-signing/main.go# Deletes keychain, restores search list, removes profile
│   ├── ios-build/main.go           # Generates ExportOptions.plist, xcodebuild archive + export
│   ├── ios-upload/main.go          # Uploads IPA via iTMSTransporter (Xcode-bundled CLI)
│   └── verify-checksums/main.go    # Cross-platform SHA-256 checksum verifier
├── internal/
│   ├── secrets/                    # Base64 decode → temp file in $RUNNER_TEMP, SIGTERM cleanup
│   ├── exec/                       # Subprocess runner with structured logging
│   └── actions/                    # GitHub Actions output helpers ($GITHUB_OUTPUT, mask, group)
├── testdata/
│   ├── android/                    # Minimal self-contained Android stub project
│   └── ios/                        # Minimal self-contained iOS stub project
├── verify-checksums.sha256         # Committed SHA-256 hashes of verify-checksums-* binaries
│                                   # (bootstrap trust anchor; updated on every release)
├── .gitignore                      # Includes bin/
├── .github/
│   └── workflows/
│       ├── build-binaries.yml      # Reusable (workflow_call): compile + attach binaries to release
│       ├── release.yml             # Triggered by exact version tag push
│       └── test.yml                # Integration tests on PRs and pushes to main
├── go.mod
└── go.sum
```

`bin/` is in `.gitignore` and never committed. Compiled binaries are attached as GitHub Release assets.

---

## Composite Action Interfaces

### Android (`android/action.yml`)

**Inputs:**

| Input | Description | Required | Default |
|---|---|---|---|
| `app-path` | Path to the Android app directory | No | `.` |
| `build-type` | `aab` or `apk` | No | `aab` |
| `gradle-task` | Override Gradle task (see auto-detection below) | No | auto |
| `java-version` | JDK version for `actions/setup-java` | No | `17` |
| `keystore` | Base64-encoded `.jks` keystore | Yes | — |
| `keystore-password` | Keystore password | Yes | — |
| `key-alias` | Key alias | Yes | — |
| `key-password` | Key password | Yes | — |
| `service-account-json` | Base64-encoded Google service account JSON | Yes | — |
| `package-name` | App package name (e.g. `com.myapp`) | Yes | — |
| `version-code` | Integer version code (must match the build) | Yes | — |
| `track` | Play Store track | No | `internal` |

**`gradle-task` auto-detection:** When not provided: `bundleRelease` for `build-type: aab`, `assembleRelease` for `build-type: apk`. Consumers with custom build variants must provide `gradle-task` explicitly.

**Signing prerequisite:** Consumers must NOT configure `signingConfigs` in `build.gradle` for the release build variant. The action expects Gradle to produce an unsigned artifact.

**Outputs:**

| Output | Description |
|---|---|
| `artifact-path` | Path to the signed artifact (written to `$RUNNER_TEMP/mobile-actions/signed/`) |

**Internal artifact handoff:** `android-build` writes the unsigned artifact path to `$GITHUB_OUTPUT` as `unsigned-artifact-path`. `android-sign` reads it via `INPUT_UNSIGNED_ARTIFACT_PATH`.

**Steps (internal):**
1. Download and verify binaries (see Binary Download section)
2. Run `actions/setup-java@<SHA>` with `java-version` and `distribution: temurin`
3. Run `android-build` → unsigned AAB/APK
4. Run `android-sign` → signed artifact
5. Run `android-upload` → Play Store

### iOS (`ios/action.yml`)

**Inputs:**

| Input | Description | Required | Default |
|---|---|---|---|
| `app-path` | Path to the iOS app directory | No | `.` |
| `scheme` | Xcode scheme name | Yes | — |
| `workspace` | Path to `.xcworkspace` (see auto-detection below) | No | auto-detected |
| `bundle-id` | App bundle identifier | Yes | — |
| `team-id` | Apple Developer Team ID (10-character alphanumeric) | Yes | — |
| `xcode-version` | Xcode version (must be pre-installed on runner) | No | `16.2` |
| `certificate` | Base64-encoded `.p12` certificate | Yes | — |
| `certificate-password` | P12 password | Yes | — |
| `provisioning-profile` | Base64-encoded `.mobileprovision` | Yes | — |
| `app-store-connect-key` | Base64-encoded `.p8` API key | Yes | — |
| `app-store-connect-key-id` | App Store Connect key ID | Yes | — |
| `app-store-connect-issuer-id` | App Store Connect issuer ID | Yes | — |
| `destination` | `testflight` or `app-store` | No | `testflight` |

**`workspace` auto-detection:** Searches `app-path` for `*.xcworkspace`. Exactly one found → use it. Multiple or none → fail with a descriptive error listing found paths. No `.xcodeproj` fallback.

**`xcode-version` default:** `16.2` corresponds to Xcode on `macos-latest` (macOS 15 Sequoia) as of early 2026. Consumers on `macos-13` must set a compatible version (e.g., `15.4`). If `/Applications/Xcode_<version>.app` does not exist, the action fails immediately listing available Xcode installations.

**Outputs:**

| Output | Description |
|---|---|
| `artifact-path` | Path to the exported `.ipa` (written to `$RUNNER_TEMP/mobile-actions/export/`) |

**Steps (internal):**
1. Download and verify binaries (see Binary Download section)
2. Select Xcode:
   ```bash
   XCODE_PATH="/Applications/Xcode_${{ inputs.xcode-version }}.app/Contents/Developer"
   sudo xcode-select -s "$XCODE_PATH"
   xcodebuild -version   # fails fast if path is wrong
   ```
3. Detect runner arch:
   ```bash
   RAW_ARCH=$(uname -m)
   ARCH=$( [ "$RAW_ARCH" = "x86_64" ] && echo "amd64" || echo "arm64" )
   ```
4. Run `ios-setup-signing` → writes signing state to `$RUNNER_TEMP/mobile-actions/signing-state.json`
5. Run `ios-build` → xcodebuild archive + export IPA
6. Run `ios-upload` → upload via iTMSTransporter
7. Run `ios-teardown-signing` (always runs, even on failure, via composite action `post:` mechanism — see Keychain Lifecycle)

---

## Go CLI Programs

Each program in `cmd/` is a focused CLI tool. All inputs arrive via `INPUT_*` environment variables.

**CGO policy:** All programs must be CGO-free (`CGO_ENABLED=0`). Enforced in the build script.

**Shared `internal/` packages:**
- `secrets` — decodes base64 to a temp file in `$RUNNER_TEMP/mobile-actions/`; registers `signal.NotifyContext` handler for `SIGTERM`/`SIGINT` to trigger cleanup before exit; masks values via `::add-mask`
- `exec` — wraps `os/exec`, streams stdout/stderr, wraps errors with context
- `actions` — writes to `$GITHUB_OUTPUT` file; writes `::add-mask`, `::group`/`::endgroup`, `::error`, `::warning` to stdout. Never uses deprecated `::set-output`.

**`--dry-run` flag:** `android-upload` and `ios-upload` skip the actual upload and log what would be sent. Used in integration tests.

---

### `ios-setup-signing` and `ios-teardown-signing` — Keychain Lifecycle

**Cleanup scope:** Keychain cleanup must persist until after `ios-build` and `ios-upload` complete — not until `ios-setup-signing` exits. Therefore, cleanup is split into a separate `ios-teardown-signing` binary invoked as a dedicated composite action step that always runs (using the composite action's `if: always()` condition on that step).

**`ios-setup-signing` steps:**
1. Save existing search list: `ORIGINAL=$(security list-keychains | xargs)`
2. Generate a random keychain password
3. Create: `security create-keychain -p <password> mobile-actions-$GITHUB_RUN_ID.keychain-db`
4. Unlock: `security unlock-keychain -p <password> mobile-actions-$GITHUB_RUN_ID.keychain-db`
5. Set timeout: `security set-keychain-settings -lut 21600 mobile-actions-$GITHUB_RUN_ID.keychain-db`
6. Prepend to search list (preserving existing): `security list-keychains -s mobile-actions-$GITHUB_RUN_ID.keychain-db $ORIGINAL`
7. Import p12: `security import <p12-path> -k mobile-actions-$GITHUB_RUN_ID.keychain-db -T /usr/bin/codesign -T /usr/bin/security`
8. Allow non-interactive access: `security set-key-partition-list -S apple-tool:,apple: -s -k <password> mobile-actions-$GITHUB_RUN_ID.keychain-db`
9. Unwrap provisioning profile: `security cms -D -i <mobileprovision-path>` → parse resulting plist XML with a Go plist library → extract `UUID`
10. Install profile: resolve home directory via `os.UserHomeDir()` (never `~` — Go does not expand tilde); copy decoded `.mobileprovision` to `$HOME/Library/MobileDevice/Provisioning Profiles/<uuid>.mobileprovision`
11. Write signing state to `$RUNNER_TEMP/mobile-actions/signing-state.json`. All paths are fully expanded (no `~`):
    ```json
    {
      "keychain_name": "mobile-actions-<run-id>.keychain-db",
      "original_keychains": ["/path/to/keychain1.keychain-db", "/path/to/keychain2.keychain-db"],
      "provisioning_profile_path": "/Users/runner/Library/MobileDevice/Provisioning Profiles/<uuid>.mobileprovision"
    }
    ```
    Note: `original_keychains` is stored as a JSON array (not a flat string) to correctly handle paths that contain spaces.

**`ios-teardown-signing` steps** (reads `signing-state.json`):
1. Delete keychain: `security delete-keychain <keychain_name>`
2. Restore search list: `security list-keychains -s <original_keychains[0]> <original_keychains[1]> ...` (each element passed as a separate argument)
3. Remove provisioning profile: `os.Remove(signing_state.provisioning_profile_path)`

The `SIGTERM`/`SIGINT` handlers in `ios-setup-signing` log a warning that teardown should be handled by `ios-teardown-signing`. They do not attempt keychain deletion (to avoid partial state if the process is killed mid-setup).

---

### `ios-build` — ExportOptions.plist and xcodebuild command

Generates `$RUNNER_TEMP/mobile-actions/ExportOptions.plist` before calling `xcodebuild -exportArchive`.

The provisioning profile UUID is extracted by running `security cms -D -i <mobileprovision-path>` to unwrap the CMS envelope, then parsing the resulting plist XML with a Go plist library to extract the `UUID` key.

Both `testflight` and `app-store` destinations use `method: app-store`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "...">
<plist version="1.0">
<dict>
  <key>method</key><string>app-store</string>
  <key>teamID</key><string>$TEAM_ID</string>
  <key>signingStyle</key><string>manual</string>
  <key>provisioningProfiles</key>
  <dict>
    <key>$BUNDLE_ID</key><string>$PROVISIONING_PROFILE_UUID</string>
  </dict>
  <key>destination</key><string>export</string>
  <key>uploadSymbols</key><true/>
</dict>
</plist>
```

**`xcodebuild` commands:**
```bash
# Archive (archive path derived from $SCHEME to avoid hardcoded names)
xcodebuild archive \
  -workspace "$WORKSPACE" \
  -scheme "$SCHEME" \
  -configuration Release \
  -destination "generic/platform=iOS" \
  -archivePath "$RUNNER_TEMP/mobile-actions/${SCHEME}.xcarchive"

# Export
xcodebuild -exportArchive \
  -archivePath "$RUNNER_TEMP/mobile-actions/${SCHEME}.xcarchive" \
  -exportOptionsPlist "$RUNNER_TEMP/mobile-actions/ExportOptions.plist" \
  -exportPath "$RUNNER_TEMP/mobile-actions/export/"
```

---

### `ios-upload` — iTMSTransporter

`ios-upload` wraps `iTMSTransporter`, the Xcode-bundled upload CLI available in all current Xcode versions including Xcode 16.

**Resolving the iTMSTransporter path:** After `xcode-select` has been run, `ios-upload` resolves the active Xcode developer path via `xcode-select -p` (e.g., `/Applications/Xcode_16.2.app/Contents/Developer`), then constructs the `iTMSTransporter` path from it:

```go
developerDir := execOutput("xcode-select", "-p")  // e.g. /Applications/Xcode_16.2.app/Contents/Developer
xcodeContents := filepath.Dir(developerDir)        // /Applications/Xcode_16.2.app/Contents
itmsPath := filepath.Join(xcodeContents,
  "SharedFrameworks/ContentDeliveryServices.framework/Versions/A/itms/bin/iTMSTransporter")
```

This avoids hardcoding a static path and correctly follows whatever Xcode version the `xcode-select` step configured.

**`.p8` key placement:** `iTMSTransporter` with `-apiKey` looks for the key file in `~/.appstoreconnect/private_keys/AuthKey_<KEY_ID>.p8`. `ios-upload` decodes the base64 key and writes it there using `os.UserHomeDir()` (never `~` — Go does not expand tilde). The file is masked via `::add-mask` and cleaned up after upload (registered in the SIGTERM handler and via defer).

The tool is invoked:
```bash
"$ITMS_PATH" \
  -m upload \
  -f "$IPA_PATH" \
  -apiKey "$KEY_ID" \
  -apiIssuer "$ISSUER_ID"
```

The `destination` input (`testflight` vs `app-store`) is determined by the provisioning profile and export method, not a separate iTMSTransporter flag.

`--dry-run` skips the `iTMSTransporter` invocation and logs the command that would have run.

---

### `android-sign` — Signing Tool and `apksigner` Resolution

**AAB** — signed with `jarsigner` (ships with JDK, required for upload key signing):
```bash
jarsigner -verbose -sigalg SHA256withRSA -digestalg SHA-256 \
  -keystore <keystore-path> -storepass <password> -keypass <key-password> \
  app-release.aab <key-alias>
```

**AAB output path:** `jarsigner` signs the AAB in-place. After signing, `android-sign` copies the signed file to `$RUNNER_TEMP/mobile-actions/signed/app-release.aab` using `os.Rename` (or `io.Copy` if cross-device). This final path is written to `$GITHUB_OUTPUT` as `artifact-path`.

**APK** — signed with `apksigner`. `apksigner` is not on `$PATH` by default on `ubuntu-latest`. `android-sign` resolves it by searching `$ANDROID_SDK_ROOT/build-tools/` for the highest-versioned subdirectory using a **semantic version sort** (not alphabetical — `9.0.0` must not rank above `35.0.0`). Uses `github.com/Masterminds/semver/v3`, which handles bare version strings (no `v` prefix required):
```go
import semver "github.com/Masterminds/semver/v3"

entries, _ := os.ReadDir(filepath.Join(os.Getenv("ANDROID_SDK_ROOT"), "build-tools"))
versions := make(semver.Collection, 0, len(entries))
nameFor := map[*semver.Version]string{}
for _, e := range entries {
    v, err := semver.NewVersion(e.Name())
    if err != nil { continue }
    versions = append(versions, v)
    nameFor[v] = e.Name()
}
sort.Sort(sort.Reverse(versions))  // descending
apksignerPath := filepath.Join(sdkRoot, "build-tools", nameFor[versions[0]], "apksigner")
```

```bash
"$APKSIGNER" sign \
  --ks <keystore-path> --ks-key-alias <alias> \
  --ks-pass pass:<password> --key-pass pass:<key-password> \
  --out $RUNNER_TEMP/mobile-actions/signed/app-release.apk \
  app-release-unsigned.apk
```

---

### `android-upload` — Play Store API

Uses the Google Play Developer Publishing API v3. The version code is provided explicitly via `INPUT_VERSION_CODE` (from the action's `version-code` input) — it is not parsed from the binary.

Edit lifecycle:
1. `edits.insert` → `editId`
2. `edits.bundles.upload` (AAB) or `edits.apks.upload` (APK)
3. `edits.tracks.update` with `releases: [{ versionCodes: ["$version_code"], status: "completed" }]`
4. `edits.commit`

HTTP 409 (version code conflict) → non-zero exit, error via `::error`. All other API errors similarly logged.

---

### `verify-checksums`

Cross-platform Go CLI. Reads a `checksums.txt` file (format: `<sha256hex>  <filename>` per line, one per binary) and verifies SHA-256 of each named file in a specified directory. Exits non-zero if any checksum fails or file is missing.

```
verify-checksums --file checksums.txt --dir $DEST
```

**Flag semantics:** `--file` is always resolved relative to the current working directory (not relative to `--dir`). `--dir` specifies where the files named in `checksums.txt` are located. In the binary download step, `cd $DEST` is run before invocation, so `checksums.txt` resolves to `$DEST/checksums.txt` and `--dir $DEST` also points to `$DEST` — both flags are consistent with the CWD convention.

---

## Binary Build Pipeline

Binaries are compiled and attached to GitHub Releases. Never committed (`bin/` is in `.gitignore`).

### Asset naming convention

Binaries are named `<program>-<os>-<arch>` (e.g., `android-build-linux-amd64`, `verify-checksums-darwin-arm64`). Release assets include:

```
android-build-linux-amd64
android-sign-linux-amd64
android-upload-linux-amd64
ios-setup-signing-darwin-arm64
ios-setup-signing-darwin-amd64
ios-teardown-signing-darwin-arm64
ios-teardown-signing-darwin-amd64
ios-build-darwin-arm64
ios-build-darwin-amd64
ios-upload-darwin-arm64
ios-upload-darwin-amd64
verify-checksums-linux-amd64
verify-checksums-darwin-arm64
verify-checksums-darwin-amd64
checksums.txt
```

### Build workflow (`build-binaries.yml`) — reusable via `workflow_call`

```yaml
on:
  workflow_call:
    inputs:
      release-tag:
        required: true
        type: string
    outputs:
      bootstrap-commit-sha:
        description: "SHA of commit containing updated verify-checksums.sha256"
        value: ${{ jobs.aggregate.outputs.commit-sha }}

permissions:
  contents: write
```

Matrix:
```yaml
strategy:
  matrix:
    include:
      - { goos: linux,  goarch: amd64, runs-on: ubuntu-latest }
      - { goos: darwin, goarch: arm64, runs-on: macos-latest  }
      - { goos: darwin, goarch: amd64, runs-on: macos-13      }
```

Go version is pinned (e.g., `go-version: '1.22'`).

Each matrix job builds and generates checksums:
```bash
# Build (note: <program>-<os>-<arch> naming)
for program in android-build android-sign android-upload \
               ios-setup-signing ios-teardown-signing ios-build ios-upload \
               verify-checksums; do
  CGO_ENABLED=0 GOOS=${{ matrix.goos }} GOARCH=${{ matrix.goarch }} \
    go build -o ${program}-${{ matrix.goos }}-${{ matrix.goarch }} ./cmd/$program
done

# Checksums (openssl available on both Linux and macOS)
for f in *-${{ matrix.goos }}-${{ matrix.goarch }}; do
  echo "$(openssl dgst -sha256 -r "$f" | awk '{print $1}')  $f"
done >> checksums-${{ matrix.goos }}-${{ matrix.goarch }}.txt
```

Each matrix job uploads its binaries and partial checksum file as a workflow artifact.

**Aggregation job** (depends on all matrix jobs via `needs`, `runs-on: ubuntu-latest`):
1. Downloads all matrix artifacts
2. Merges `checksums-*.txt` → `checksums.txt`
3. Generates `verify-checksums.sha256` (same format as `checksums.txt`, containing only lines for `verify-checksums-*` binaries)
4. Uploads all binaries + `checksums.txt` to the GitHub Release via `gh release upload ${{ inputs.release-tag }} ./*`
5. Commits and pushes `verify-checksums.sha256` to `main`, then outputs the new commit SHA:
   ```bash
   git config user.name "github-actions[bot]"
   git config user.email "github-actions[bot]@users.noreply.github.com"
   git add verify-checksums.sha256
   git commit -m "chore: update verify-checksums hashes for ${{ inputs.release-tag }}"
   git push origin HEAD:main
   echo "commit-sha=$(git rev-parse HEAD)" >> $GITHUB_OUTPUT
   ```

### `verify-checksums.sha256` format

Same format as `checksums.txt`: `<sha256hex>  <filename>` per line, using two spaces. Contains one line per `verify-checksums-*` binary. Example:
```
abc123...  verify-checksums-linux-amd64
def456...  verify-checksums-darwin-arm64
789ghi...  verify-checksums-darwin-amd64
```

---

### Binary download (inside `action.yml`)

```yaml
- name: Download mobile-actions binaries
  env:
    GH_TOKEN: ${{ github.token }}
  run: |
    RAW_ARCH=$(uname -m)
    ARCH=$( [ "$RAW_ARCH" = "x86_64" ] && echo "amd64" || echo "arm64" )
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    VERSION=$GITHUB_ACTION_REF
    DEST=$RUNNER_TEMP/mobile-actions-bin
    mkdir -p $DEST

    # Step 1: Download verify-checksums binary and checksums.txt
    gh release download "$VERSION" \
      --repo your-org/mobile-actions \
      --pattern "verify-checksums-${OS}-${ARCH}" \
      --pattern "checksums.txt" \
      --dir $DEST

    # Step 2: Bootstrap-verify verify-checksums using the committed hash file
    # verify-checksums.sha256 is available in ${{ github.action_path }} (the action's repo checkout)
    HASH_FILE=${{ github.action_path }}/verify-checksums.sha256
    cd $DEST
    if [ "$OS" = "linux" ]; then
      grep "verify-checksums-${OS}-${ARCH}" "$HASH_FILE" | sha256sum --check
    else
      grep "verify-checksums-${OS}-${ARCH}" "$HASH_FILE" | shasum -a 256 --check
    fi
    chmod +x verify-checksums-${OS}-${ARCH}

    # Step 3: Download all remaining binaries for this platform
    gh release download "$VERSION" \
      --repo your-org/mobile-actions \
      --pattern "*-${OS}-${ARCH}" \
      --dir $DEST

    # Step 4: Verify all binaries against checksums.txt
    ./verify-checksums-${OS}-${ARCH} --file checksums.txt --dir $DEST
    chmod +x $DEST/*-${OS}-${ARCH}
```

`GITHUB_ACTION_REF` is set by GitHub Actions to the ref after `@` in the `uses:` line (`v1`, `v1.0.0`, etc.). Both floating and exact tags must have a corresponding GitHub Release (see Release Workflow).

**Self-hosted runner prerequisites:** `gh` CLI must be installed and `GH_TOKEN` must be available. `$RUNNER_TEMP` must be writable and cleaned between jobs.

---

## Release Workflow (`release.yml`)

Triggered by pushing an exact version tag:

```yaml
on:
  push:
    tags: ['v[0-9]+.[0-9]+.[0-9]+']

concurrency:
  group: release
  cancel-in-progress: false

permissions:
  contents: write
```

Steps (sequential jobs):

**Job 1 — `create-release`:** Create a draft GitHub Release for the exact tag (e.g., `v1.0.0`).

**Job 2 — `build` (calls `build-binaries.yml`):**
```yaml
uses: ./.github/workflows/build-binaries.yml
with:
  release-tag: ${{ github.ref_name }}
secrets: inherit
```
Outputs: `bootstrap-commit-sha` (SHA of the commit containing the updated `verify-checksums.sha256`).

**Job 3 — `update-tags` (depends on `build`):** Update the floating major tag and its GitHub Release:
```bash
MAJOR=$(echo ${{ github.ref_name }} | cut -d. -f1)   # e.g. "v1"
BOOTSTRAP_SHA=${{ needs.build.outputs.bootstrap-commit-sha }}

# Update floating git tag to point to the bootstrap commit (not github.sha)
git tag -f $MAJOR $BOOTSTRAP_SHA
git push -f origin $MAJOR

# Recreate the floating-tag GitHub Release with the same binary assets
gh release delete $MAJOR --yes --cleanup-tag 2>/dev/null || true
gh release create $MAJOR \
  --title "$MAJOR (latest — ${{ github.ref_name }})" \
  --notes "Always points to the latest $MAJOR.x.x release. Currently ${{ github.ref_name }}."

# Download assets from the exact-version release and re-upload to the floating-tag release
ASSET_DIR=$RUNNER_TEMP/release-assets
mkdir -p $ASSET_DIR
gh release download ${{ github.ref_name }} --dir $ASSET_DIR
gh release upload $MAJOR $ASSET_DIR/*
```

**Job 4 — `publish-release` (depends on `update-tags`):** Remove the draft status from the `v1.0.0` release.

---

## Consumer Usage

```yaml
# .github/workflows/release.yml (in consumer repo)
on:
  push:
    tags: ['v*']

jobs:
  release-android:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: your-org/mobile-actions/android-upload@v1
        with:
          keystore: ${{ secrets.ANDROID_KEYSTORE }}
          keystore-password: ${{ secrets.KEYSTORE_PASSWORD }}
          key-alias: ${{ secrets.KEY_ALIAS }}
          key-password: ${{ secrets.KEY_PASSWORD }}
          service-account-json: ${{ secrets.PLAY_SERVICE_ACCOUNT }}
          package-name: com.myapp
          version-code: ${{ github.run_number }}   # or a manually managed value
          track: internal

  release-ios:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v4
      - uses: your-org/mobile-actions/ios-upload@v1
        with:
          scheme: MyApp
          bundle-id: com.myapp
          team-id: ABCD1234EF
          certificate: ${{ secrets.IOS_CERTIFICATE }}
          certificate-password: ${{ secrets.IOS_CERTIFICATE_PASSWORD }}
          provisioning-profile: ${{ secrets.IOS_PROVISIONING_PROFILE }}
          app-store-connect-key: ${{ secrets.ASC_KEY }}
          app-store-connect-key-id: ${{ secrets.ASC_KEY_ID }}
          app-store-connect-issuer-id: ${{ secrets.ASC_ISSUER_ID }}
          destination: testflight
```

---

## Versioning

- **Exact tags** (`v1.0.0`) — created by `release.yml`; have their own GitHub Release with binaries
- **Floating major tags** (`v1`) — updated by `release.yml` to point to the `bootstrap-commit-sha` (the commit containing the updated `verify-checksums.sha256`); have their own GitHub Release with the same binary assets
- **Concurrency:** `concurrency: group: release, cancel-in-progress: false` serializes all releases
- **Breaking changes:** Input/output breaking changes require a new major version
- **Sub-action pinning:** All sub-actions inside composite actions pinned to exact commit SHA (not mutable tag)

---

## Testing

**Unit tests:** All `internal/` packages — secrets decoding, SIGTERM behavior, `$GITHUB_OUTPUT` formatting, checksum verification, plist parsing, CMS unwrapping, `apksigner` path resolution.

**Integration tests** (`.github/workflows/test.yml`):
- Android: `runs-on: ubuntu-latest`
- iOS: `runs-on: macos-latest`

Stub projects in `testdata/android/` (includes `build.gradle`, `settings.gradle`, source file) and `testdata/ios/` (includes `Podfile`, `.xcworkspace`, scheme, Swift source) are self-contained and buildable.

Upload steps run with `--dry-run`. Signing steps use dedicated test credentials:
- Android: a non-production test keystore (generated for CI purposes only, no production signing authority) committed to the repo as `testdata/android/test.keystore`; its password is hardcoded in the test workflow (not a secret, since the keystore is non-production and committed publicly)
- iOS: non-production test `.p12` and `.mobileprovision` stored as repository secrets (`TEST_IOS_CERTIFICATE`, `TEST_IOS_PROVISIONING_PROFILE`)

**Self-hosted runner support:** `gh` CLI must be installed and `GH_TOKEN` available. `$RUNNER_TEMP` must be writable and cleaned between jobs. No other runtime dependencies required.
