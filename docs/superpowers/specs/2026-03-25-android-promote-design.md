# Android Promote Action Design Spec

**Date:** 2026-03-25
**Status:** Approved

## Overview

Add a new `android-promote` composite action to `mobile-actions` that promotes an already-uploaded Android app version from the internal track to another Play Store track (closed testing, open testing, or production), without rebuilding or re-uploading the artifact.

Also rename the existing `android` and `ios` composite actions to `android-upload` and `ios-upload` respectively, for naming consistency.

---

## Motivation

The existing `android-upload` action always uploads to the `internal` track. After internal testing, the user wants to promote that version to a broader track manually via GitHub Actions — without visiting the Play Console dashboard and without rebuilding the `.aab`.

---

## Changes

### Action Directory Renames

| Old path | New path |
|---|---|
| `android/action.yml` | `android-upload/action.yml` |
| `ios/action.yml` | `ios-upload/action.yml` |

These are breaking changes. The `v1` floating tag is retained. All known consumers will be updated manually. The old `android/` and `ios/` directories are deleted — no stubs are kept.

### New Files

- `android-promote/action.yml` — new composite action
- `cmd/android-promote/main.go` — new Go binary
- `cmd/android-promote/main_test.go` — unit tests
- `internal/playstore/` — shared package (extracted from `android-upload`)

### Updated Files

- `cmd/android-upload/main.go` — refactored to use `internal/playstore`
- `.github/workflows/build-binaries.yml` — `android-promote` added to the `PROGRAMS` list
- `.github/workflows/test.yml` — dry-run integration step added for `android-promote`
- `CLAUDE.md` — binary list updated to include `android-promote`

---

## `android-promote` Composite Action

### Inputs

| Input | Required | Description |
|---|---|---|
| `service-account-json` | Yes | Base64-encoded Google service account JSON |
| `package-name` | Yes | App package name (e.g. `com.myapp`) |
| `version-code` | Yes | Integer version code to promote |
| `target-track` | Yes | Play Store track: `alpha`, `beta`, or `production` |

`target-track` is validated by the binary. Only `alpha`, `beta`, and `production` are accepted. Passing `internal` or any other value returns a descriptive error and exits non-zero.

### Outputs

None. Promote is a terminal action.

### Steps (internal)

1. Download and verify binaries (same pattern as `android-upload`)
2. Run `android-promote` binary

---

## `cmd/android-promote` Binary

Reads all inputs from `INPUT_*` environment variables. Supports a `--dry-run` flag that logs what would happen without calling the Play Store API.

### Input validation

- `INPUT_SERVICE_ACCOUNT_JSON`, `INPUT_PACKAGE_NAME`, `INPUT_VERSION_CODE`, `INPUT_TARGET_TRACK` are all required; missing any returns a descriptive error.
- `INPUT_TARGET_TRACK` must be one of `alpha`, `beta`, `production`; any other value returns: `target-track must be one of: alpha, beta, production`.
- `INPUT_VERSION_CODE` is passed as-is to the API (string). No integer validation is performed — same lenient behavior as `android-upload`. Invalid values will produce an API error.

### Dry-run

`--dry-run` is checked immediately after input validation, before the service account is decoded or any API call is made:

```
[dry-run] would promote version-code=<N> to track=<track> for package=<package>
```

### Play Store API flow

1. Decode `INPUT_SERVICE_ACCOUNT_JSON` (base64) in memory → `playstore.NewClient`
2. `playstore.CreateEdit` → `editId`
3. `playstore.PromoteTrack(client, packageName, editID, targetTrack, versionCode)` → `edits.tracks.update`
4. `playstore.CommitEdit` — with `changesNotSentForReview=true` retry on HTTP 400 (managed publishing accounts)

### `edits.tracks.update` request body

```json
{
  "track": "<target-track>",
  "releases": [{
    "versionCodes": ["<version-code>"],
    "status": "completed",
    "userFraction": 1.0
  }]
}
```

`userFraction: 1.0` is included to explicitly specify 100% rollout. The existing `android-upload` omits this field (defaulting to full rollout implicitly). Including it here is intentional and documents the rollout intent clearly. Note: for `alpha` (closed testing) and `beta` (open testing) tracks, the Play Store API ignores `userFraction` — staged rollouts only apply to `production`. The field is included uniformly across all tracks for simplicity.

---

## `internal/playstore` Shared Package

Extracted from `cmd/android-upload/main.go` to eliminate duplication. Exposes:

```go
func NewClient(serviceAccountB64 string) (*http.Client, error)
func CreateEdit(client *http.Client, packageName string) (string, error)
func PromoteTrack(client *http.Client, packageName, editID, track, versionCode string) error
func CommitEdit(client *http.Client, packageName, editID string) error
```

`NewClient` decodes the base64 service account JSON directly in memory using `google.JWTConfigFromJSON` — no temp file is written. This differs from the original `android-upload` implementation which used `secrets.DecodeToFile`. The refactored `android-upload` will call `playstore.NewClient` directly, dropping the temp-file path for the service account.

`CommitEdit` contains the `changesNotSentForReview=true` retry logic currently in `android-upload`.

`android-upload` is updated to use this package for its edit lifecycle. `PromoteTrack` is defined here (not in `cmd/android-promote`) so it is independently testable and reusable.

---

## Binary Distribution

`android-promote` is added to the `PROGRAMS` list in `build-binaries.yml`:

```bash
PROGRAMS="android-build android-sign android-upload android-promote ios-setup-signing ios-teardown-signing ios-build ios-upload verify-checksums"
```

The existing build matrix (linux-amd64, darwin-arm64) builds all programs for all platforms — `android-promote` follows the same pattern. This produces `android-promote-linux-amd64` and `android-promote-darwin-arm64` as release assets.

---

## Integration Test

A dry-run step is added to `test.yml` in the `integration-android` job, consistent with the existing `android-upload` dry-run step:

```yaml
- name: Build Go binaries
  run: |
    for program in android-build android-sign android-upload android-promote; do
      CGO_ENABLED=0 go build -o /tmp/ma-bin/${program}-linux-amd64 ./cmd/$program
    done

- name: Test android-promote (dry-run)
  env:
    INPUT_SERVICE_ACCOUNT_JSON: e30K  # base64("{}")
    INPUT_PACKAGE_NAME: com.example.testapp
    INPUT_VERSION_CODE: '1'
    INPUT_TARGET_TRACK: production
  run: /tmp/ma-bin/android-promote-linux-amd64 --dry-run
```

---

## Consumer Usage

```yaml
jobs:
  upload:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: kolabs-dev/mobile-actions/android-upload@v1
        with:
          version-code: ${{ github.run_number }}
          package-name: com.myapp
          service-account-json: ${{ secrets.PLAY_SERVICE_ACCOUNT }}
          keystore: ${{ secrets.ANDROID_KEYSTORE }}
          keystore-password: ${{ secrets.KEYSTORE_PASSWORD }}
          key-alias: ${{ secrets.KEY_ALIAS }}
          key-password: ${{ secrets.KEY_PASSWORD }}
          track: internal

  promote:
    needs: upload
    runs-on: ubuntu-latest
    environment: production        # GitHub approval gate
    steps:
      - uses: actions/checkout@v4
      - uses: kolabs-dev/mobile-actions/android-promote@v1
        with:
          service-account-json: ${{ secrets.PLAY_SERVICE_ACCOUNT }}
          package-name: com.myapp
          version-code: ${{ github.run_number }}
          target-track: production
```

The `environment: production` field on the promote job acts as the manual approval gate — reviewers approve or reject in the GitHub Actions UI before the promote step runs.

---

## Unit Tests

### `cmd/android-upload/main_test.go` — migration

The existing tests in `cmd/android-upload/main_test.go` directly call package-private functions (`commitEdit`, `createEdit`, `updateTrack`, `checkStatus`) and override the `playBaseURL` package-level variable. After the refactor, these symbols move to `internal/playstore`. The existing `android-upload` tests are migrated to `internal/playstore/playstore_test.go`. The `cmd/android-upload/main_test.go` is rewritten to only test the `run()` function (input validation and dry-run path), using the same HTTP test server injection pattern as before but via the `playstore` package.

### `cmd/android-promote/main_test.go`

- `--dry-run` logs correctly and exits 0
- Missing required inputs return a descriptive error
- Invalid `target-track` value returns the expected error message
- `edits.tracks.update` request body contains correct track, version code, status `"completed"`, and `userFraction: 1.0`
- `changesNotSentForReview` retry logic on HTTP 400 (via shared `CommitEdit`)

### `internal/playstore/playstore_test.go`

- `NewClient` constructs client from valid base64 service account JSON
- `NewClient` returns descriptive error on invalid base64 or malformed JSON
- `CreateEdit` happy path and API error cases
- `PromoteTrack` sends correct request body; handles API errors
- `CommitEdit` happy path, managed-publishing retry, and error cases

---

## Key Constraints

- Binary compiled with `CGO_ENABLED=0`
- All inputs via `INPUT_*` env vars
- `--dry-run` flag consistent with `android-upload` and `ios-upload`; guard applied before service account decode
- No artifact download or re-upload — promote only touches track metadata
- `target-track` validated to `alpha`, `beta`, or `production` only
