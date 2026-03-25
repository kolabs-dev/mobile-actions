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

These are breaking changes. The `v1` floating tag is retained. All known consumers will be updated manually.

### New Files

- `android-promote/action.yml` — new composite action
- `cmd/android-promote/main.go` — new Go binary
- `cmd/android-promote/main_test.go` — unit tests
- `internal/playstore/` — shared package (extracted from `android-upload`)

---

## `android-promote` Composite Action

### Inputs

| Input | Required | Description |
|---|---|---|
| `service-account-json` | Yes | Base64-encoded Google service account JSON |
| `package-name` | Yes | App package name (e.g. `com.myapp`) |
| `version-code` | Yes | Integer version code to promote |
| `target-track` | Yes | Play Store track: `alpha`, `beta`, or `production` |

### Outputs

None. Promote is a terminal action.

### Steps (internal)

1. Download and verify binaries (same pattern as `android-upload`)
2. Run `android-promote` binary

---

## `cmd/android-promote` Binary

Reads all inputs from `INPUT_*` environment variables. Supports a `--dry-run` flag that logs what would happen without calling the Play Store API.

### Play Store API flow

1. Decode service account JSON → OAuth2 HTTP client
2. `edits.insert` → `editId`
3. `edits.tracks.update` on `target-track`:
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
4. `edits.commit` — with `changesNotSentForReview=true` retry on HTTP 400 (managed publishing accounts)

### Dry-run

`--dry-run` skips all API calls and logs:
```
[dry-run] would promote version-code=<N> to track=<track> for package=<package>
```

---

## `internal/playstore` Shared Package

Extracted from `android-upload` to avoid duplication. Exposes:

```go
func NewClient(serviceAccountB64 string) (*http.Client, error)
func CreateEdit(client *http.Client, packageName string) (string, error)
func CommitEdit(client *http.Client, packageName, editID string) error
```

`android-upload` is updated to use this package. `android-promote` uses the same package plus its own `PromoteTrack` function.

---

## Binary Distribution

`android-promote` is added to the build matrix in `build-binaries.yml`, built for `linux-amd64` only (same as all other Android binaries).

Asset naming: `android-promote-linux-amd64`.

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

## Testing

### Unit tests (`cmd/android-promote/main_test.go`)

- `--dry-run` logs correctly and exits 0
- Missing required inputs return a descriptive error
- `edits.tracks.update` request body contains correct track, version code, status, and `userFraction: 1.0`
- `changesNotSentForReview` retry logic on HTTP 400

### Unit tests (`internal/playstore/`)

- OAuth2 client construction from valid/invalid service account JSON
- `CreateEdit` happy path and API error cases
- `CommitEdit` happy path, managed-publishing retry, and error cases

No integration test is added for promote — the unit tests with an HTTP test server are sufficient.

---

## Key Constraints

- Binary compiled with `CGO_ENABLED=0`
- All inputs via `INPUT_*` env vars
- `--dry-run` flag consistent with `android-upload` and `ios-upload`
- No artifact download or re-upload — promote only touches track metadata
