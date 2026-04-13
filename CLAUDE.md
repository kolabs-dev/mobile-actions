# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`mobile-actions` is a GitHub Actions repository providing two reusable composite actions for automating mobile app releases:
- `kolabs-dev/mobile-actions/android-upload@v1` — Build, sign, and upload Android apps (AAB/APK) to Google Play Store
- `kolabs-dev/mobile-actions/android-promote@v1` — Promote an uploaded Android app to a Play Store track (alpha/beta/production)
- `kolabs-dev/mobile-actions/ios-upload@v1` — Build, sign, and upload iOS apps (IPA) to App Store Connect/TestFlight
- `kolabs-dev/mobile-actions/ios-promote@v1` — Submit an iOS build for App Store review

## Commands

**Unit tests:**
```bash
go test ./internal/... ./cmd/... -v -count=1
```

**Single test:**
```bash
go test ./internal/actions -v -count=1 -run TestSetOutput
```

**Build a binary (CGO must be disabled):**
```bash
CGO_ENABLED=0 go build -o bin/android-build ./cmd/android-build
```

**Build all binaries for a platform:**
```bash
for program in android-build android-sign android-upload android-promote ios-setup-signing ios-teardown-signing ios-build ios-upload ios-promote verify-checksums; do
  CGO_ENABLED=0 go build -o bin/$program ./cmd/$program
done
```

**Lint:**
```bash
go vet ./...
```

## Architecture

### Structure

Each step in the mobile CI pipeline is a **separate Go binary** in `cmd/`. They communicate via `$GITHUB_OUTPUT` and the filesystem. The composite actions in `android-upload/action.yml`, `android-promote/action.yml`, `ios-upload/action.yml`, and `ios-promote/action.yml` orchestrate the binaries.

### Shared packages (`internal/`)

- **`internal/actions/`** — GitHub Actions helpers: `SetOutput`, `AddMask`, `Group`/`EndGroup`, `Error`/`Warning`. These write to `$GITHUB_OUTPUT` and emit `::command::` annotations.
- **`internal/exec/`** — Subprocess wrapper: `Run()` streams stdout/stderr; `RunOutput()` captures stdout.
- **`internal/secrets/`** — Decodes base64 secrets to temp files under `$RUNNER_TEMP/mobile-actions/` and returns a cleanup function.
- **`internal/playstore/`** — Google Play API client and edit lifecycle helpers: `NewClient`, `CreateEdit`, `PromoteTrack`, `CommitEdit`.

### Android pipeline (`cmd/android-*/`)

1. **`android-build`** — Runs Gradle (`bundleRelease` for AAB, `assembleRelease` for APK), finds the unsigned artifact, outputs `unsigned-artifact-path`.
2. **`android-sign`** — Decodes keystore, uses `jarsigner` for AAB or `apksigner` (auto-discovered in `$ANDROID_SDK_ROOT/build-tools/`, highest semver) for APK. Outputs `artifact-path`.
3. **`android-upload`** — OAuth2 JWT with service account JSON, calls Google Play API (create edit → upload → update track → commit). Supports `--dry-run`.
4. **`android-promote`** — OAuth2 JWT with service account JSON, calls Google Play API (create edit → update track → commit). Promotes an already-uploaded version to alpha, beta, or production. Supports `--dry-run`.

### iOS pipeline (`cmd/ios-*/`)

1. **`ios-setup-signing`** — Creates a temp keychain (`mobile-actions-<GITHUB_RUN_ID>.keychain-db`), imports the `.p12` certificate, installs provisioning profile (parses CMS/plist to get UUID). Saves state to `$RUNNER_TEMP/mobile-actions/signing-state.json`.
2. **`ios-build`** — Auto-detects `.xcworkspace`, reads signing state JSON for profile UUID, generates `ExportOptions.plist` (method: `app-store` for both testflight and app-store destinations), runs `xcodebuild archive` + `xcodebuild -exportArchive`. Outputs `artifact-path`.
3. **`ios-upload`** — Writes `.p8` key to `~/.appstoreconnect/private_keys/`, calls `iTMSTransporter`. Supports `--dry-run`.
4. **`ios-teardown-signing`** — Reads signing state, deletes keychain, restores original keychain list, removes provisioning profile. Always non-fatal (logs warnings only).

**`ios-promote`** is a separate standalone action (not part of the ios-upload pipeline). It takes a build already processed in TestFlight and submits it for App Store review via the ASC API, without requiring a full build or upload.

### Binary distribution and trust

- Binaries are never committed. They are compiled with `CGO_ENABLED=0` and attached as GitHub Release assets for linux-amd64, darwin-arm64.
- `verify-checksums.sha256` (committed) contains SHA-256 hashes for the `verify-checksums` binaries only — this is the bootstrap trust anchor.
- The composite actions download all binaries via `gh release download`, verify with `verify-checksums`, then use the other binaries.
- `cmd/verify-checksums/` reads a checksums file (`<hash>  <filename>`) and verifies SHA-256 of each file.

### All-inputs via environment variables

All Go programs read inputs from `INPUT_*` env vars (GitHub Actions convention). This allows direct CLI invocation for testing without needing an actual Actions runner.

### CI workflows

- **`test.yml`** — Runs unit tests and integration tests. Triggers on `pull_request`. Also supports `workflow_call` so it can be called as a reusable workflow.
- **`release.yml`** — Triggers on `push: branches: [main]` (excluding commits that only touch `verify-checksums.sha256`). Calls `test.yml`, then computes and pushes the next semver tag, builds binaries for all platforms, creates and publishes the GitHub Release.
- **`build-binaries.yml`** — Reusable `workflow_call` workflow. Builds binaries for `linux/amd64`, `darwin/arm64`, `darwin/amd64`, aggregates checksums, commits `verify-checksums.sha256` to main, and uploads assets to the release.

### Release flow

Triggered by a push to `main` → runs all tests → computes next `vMAJOR.MINOR.PATCH` tag from `VERSION` → creates draft release → builds binaries for all platforms → aggregates checksums → commits updated `verify-checksums.sha256` to main → attaches assets → publishes release → updates floating major tag (e.g., `v1`). The `verify-checksums.sha256` commit does not re-trigger the workflow (`paths-ignore`).

## Key constraints

- All binaries must be compiled with `CGO_ENABLED=0`.
- Binaries are never committed to the repo.
- iOS teardown must always run (composite action uses `if: always()`).
- Integration tests for iOS expect `ios-build` to fail without real provisioning credentials — this is expected behavior.
- When modifying any binary in `cmd/`, write or update unit tests in the corresponding `cmd/<binary>/main_test.go`.
