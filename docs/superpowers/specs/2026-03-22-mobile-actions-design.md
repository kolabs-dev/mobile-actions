# mobile-actions Design Spec

**Date:** 2026-03-22
**Status:** Approved

## Overview

`mobile-actions` is a GitHub Actions repository that provides reusable composite actions for automating the full release pipeline of mobile apps (React Native, Flutter, bare Expo, or native Android/iOS) to the Google Play Store and Apple App Store.

It exposes two composite actions:
- `your-org/mobile-actions/android@v1` — build, sign, and upload an Android AAB/APK to the Play Store
- `your-org/mobile-actions/ios@v1` — build, sign, and upload an iOS IPA to App Store Connect or TestFlight

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
│   ├── android-upload/main.go      # Uploads to Play Store via Google API
│   ├── ios-setup-signing/main.go   # Installs p12 cert + provisioning profile to keychain
│   ├── ios-build/main.go           # Runs xcodebuild archive + export IPA
│   └── ios-upload/main.go          # Uploads IPA to App Store Connect / TestFlight
├── internal/
│   ├── secrets/                    # Base64 decode → temp file, cleanup on exit
│   ├── exec/                       # Subprocess runner with structured logging
│   └── actions/                    # GitHub Actions output helpers (set-output, mask, group)
├── bin/
│   ├── linux/amd64/                # Android binaries (ubuntu-latest runner)
│   └── darwin/
│       ├── arm64/                  # iOS binaries (macos-latest, Apple Silicon)
│       └── amd64/                  # iOS binaries (macos-13, Intel)
├── .github/
│   └── workflows/
│       ├── build-binaries.yml      # Compiles all binaries and commits to bin/ on push to main
│       └── test.yml                # Integration tests against stub Android/iOS projects
├── go.mod
└── go.sum
```

---

## Composite Action Interfaces

### Android (`android/action.yml`)

**Inputs:**

| Input | Description | Required | Default |
|---|---|---|---|
| `app-path` | Path to the Android app directory | No | `.` |
| `build-type` | `aab` or `apk` | No | `aab` |
| `gradle-task` | Override default Gradle task | No | auto |
| `keystore` | Base64-encoded `.jks` keystore | Yes | — |
| `keystore-password` | Keystore password | Yes | — |
| `key-alias` | Key alias | Yes | — |
| `key-password` | Key password | Yes | — |
| `service-account-json` | Base64-encoded Google service account JSON | Yes | — |
| `package-name` | App package name (e.g. `com.myapp`) | Yes | — |
| `track` | Play Store track | No | `internal` |

**Outputs:**

| Output | Description |
|---|---|
| `artifact-path` | Path to the signed AAB/APK |

**Steps (internal):**
1. Select binary for `linux/amd64`
2. Run `android-build` → produces unsigned AAB/APK
3. Run `android-sign` → decodes keystore, signs artifact
4. Run `android-upload` → uploads to Play Store

### iOS (`ios/action.yml`)

**Inputs:**

| Input | Description | Required | Default |
|---|---|---|---|
| `app-path` | Path to the iOS app directory | No | `.` |
| `scheme` | Xcode scheme name | Yes | — |
| `workspace` | Path to `.xcworkspace` | No | auto-detected |
| `bundle-id` | App bundle identifier | Yes | — |
| `certificate` | Base64-encoded `.p12` certificate | Yes | — |
| `certificate-password` | P12 password | Yes | — |
| `provisioning-profile` | Base64-encoded `.mobileprovision` | Yes | — |
| `app-store-connect-key` | Base64-encoded `.p8` API key | Yes | — |
| `app-store-connect-key-id` | App Store Connect key ID | Yes | — |
| `app-store-connect-issuer-id` | App Store Connect issuer ID | Yes | — |
| `destination` | `testflight` or `app-store` | No | `testflight` |

**Outputs:**

| Output | Description |
|---|---|
| `artifact-path` | Path to the exported `.ipa` |

**Steps (internal):**
1. Detect runner arch (`uname -m`) to select `darwin/arm64` or `darwin/amd64` binary
2. Run `ios-setup-signing` → decodes cert + profile, installs to keychain
3. Run `ios-build` → `xcodebuild archive` + `xcodebuild -exportArchive`
4. Run `ios-upload` → uploads IPA via App Store Connect API

---

## Go CLI Programs

Each program in `cmd/` is a focused CLI tool with a single responsibility. All inputs are received via environment variables using the `INPUT_*` convention (GitHub Actions standard for composite action steps).

**Shared `internal/` packages:**

- `secrets` — decodes base64 input to a temp file; registers `os.Remove` cleanup on process exit; masks sensitive values via `::add-mask`
- `exec` — wraps `os/exec`, streams stdout/stderr to the runner log, wraps errors with contextual messages
- `actions` — writes GitHub Actions workflow commands to stdout: `::set-output`, `::add-mask`, `::group`/`::endgroup`, `::error`, `::warning`

**`--dry-run` flag:** All upload commands (`android-upload`, `ios-upload`) support a `--dry-run` flag that skips the actual API call and logs what would be sent. Used in integration tests.

---

## Binary Build Pipeline

A workflow (`.github/workflows/build-binaries.yml`) runs on every push to `main`:

```yaml
strategy:
  matrix:
    include:
      - { goos: linux,  goarch: amd64 }
      - { goos: darwin, goarch: arm64 }
      - { goos: darwin, goarch: amd64 }
```

For each matrix entry and each `cmd/` program:
```
GOOS=$goos GOARCH=$goarch go build -o bin/$goos/$goarch/$program ./cmd/$program
```

After building, the workflow commits the updated `bin/` directory back to `main` with a message like `chore: update compiled binaries`.

Inside `action.yml`, binaries are referenced via `${{ github.action_path }}`:
```yaml
- run: ${{ github.action_path }}/bin/linux/amd64/android-build
  env:
    INPUT_APP_PATH: ${{ inputs.app-path }}
```

For iOS, the action detects arch at runtime:
```yaml
- run: |
    ARCH=$(uname -m)
    ${{ github.action_path }}/bin/darwin/${ARCH}/ios-build
```

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
      - uses: your-org/mobile-actions/android@v1
        with:
          keystore: ${{ secrets.ANDROID_KEYSTORE }}
          keystore-password: ${{ secrets.KEYSTORE_PASSWORD }}
          key-alias: ${{ secrets.KEY_ALIAS }}
          key-password: ${{ secrets.KEY_PASSWORD }}
          service-account-json: ${{ secrets.PLAY_SERVICE_ACCOUNT }}
          package-name: com.myapp
          track: internal

  release-ios:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v4
      - uses: your-org/mobile-actions/ios@v1
        with:
          scheme: MyApp
          bundle-id: com.myapp
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

- **Floating major tags** (`v1`, `v2`) — updated via `git tag -f v1 && git push -f origin v1` on each release; consumers using `@v1` get the latest compatible version automatically
- **Exact tags** (`v1.0.0`) — for consumers who want pinned stability
- Breaking input/output changes require a major version bump

---

## Testing

- **Unit tests** — for all `internal/` packages: secrets decoding, exec helpers, actions output formatting
- **Integration tests** (`.github/workflows/test.yml`) — calls composite actions against minimal stub Android/iOS projects in a `testdata/` directory; uses `--dry-run` on upload steps to avoid real store submissions
- All tests run on PRs and pushes to `main`
