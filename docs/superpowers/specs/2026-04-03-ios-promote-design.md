# ios-promote: Submit iOS Build for App Store Review

**Date:** 2026-04-03  
**Status:** Approved

## Overview

A new composite action `kolabs-dev/mobile-actions/ios-promote@v1` that takes a build already uploaded to TestFlight and submits it for App Store review via the App Store Connect REST API. No build or signing steps — pure API call, analogous to `android-promote`.

## Inputs

| Input | Required | Description |
|---|---|---|
| `app-store-connect-key` | yes | Base64-encoded `.p8` API key |
| `app-store-connect-key-id` | yes | App Store Connect key ID |
| `app-store-connect-issuer-id` | yes | App Store Connect issuer ID |
| `bundle-id` | yes | App bundle identifier (e.g. `com.myapp`) |
| `version` | yes | Version string (e.g. `1.2.3`) |
| `build-number` | yes | Build number (e.g. `42`) |

No Xcode, no signing credentials required.

## New Files

- `cmd/ios-promote/main.go` — Go binary entry point
- `cmd/ios-promote/main_test.go` — unit tests
- `internal/appstore/appstore.go` — ASC API client and promotion logic
- `internal/appstore/appstore_test.go` — unit tests using `httptest.Server`
- `ios-promote/action.yml` — composite action definition

## Architecture

### Binary (`cmd/ios-promote`)

Mirrors `cmd/android-promote` exactly:
- Reads inputs from `INPUT_*` env vars
- Supports `--dry-run` flag (logs what would be called, no API requests)
- Calls `appstore.PromoteBuild(client, bundleID, version, buildNumber)`
- Exits non-zero on error via `actions.Error`

### Package (`internal/appstore`)

**`NewClient(keyB64, keyID, issuerID string) (*Client, error)`**  
Decodes the base64 `.p8` key, parses the PKCS#8 RSA private key, stores credentials for JWT generation. No HTTP calls at construction time.

**`PromoteBuild(client *Client, bundleID, version, buildNumber string) error`**  
Orchestrates 5 ASC API calls (see below).

### Authentication

RS256 JWT signed with Go stdlib only (`crypto/rsa`, `crypto/x509`, `encoding/pem`). No new dependency.

JWT claims:
- `iss`: issuer ID
- `iat`: now (Unix)
- `exp`: now + 1200s (20 min, ASC maximum)
- `aud`: `"appstoreconnect-v1"`

JWT header includes `kid: keyID`.

Each request sends `Authorization: Bearer <jwt>`. A new JWT is generated per request (simpler than token caching given the short flow).

### ASC API Sequence

1. **Resolve app ID**  
   `GET /v1/apps?filter[bundleId]={bundleId}`  
   Returns the internal app ID needed for subsequent calls.

2. **Find build**  
   `GET /v1/builds?filter[app]={appId}&filter[version]={buildNumber}&filter[preReleaseVersion.version]={version}&filter[processingState]=VALID`  
   Fails with a clear message if no result: `build {version}({buildNumber}) not found or still processing`.

3. **Create App Store version**  
   `POST /v1/appStoreVersions`  
   Body: `{ data: { type: "appStoreVersions", attributes: { platform: "IOS", versionString: version }, relationships: { app: { data: { type: "apps", id: appId } } } } }`  
   If a version with that string already exists in `PREPARE_FOR_SUBMISSION` state, skip creation and reuse the existing version ID.  
   If it exists in any other non-editable state, surface the ASC API error with context.

4. **Attach build to version**  
   `PATCH /v1/appStoreVersions/{versionId}/relationships/build`  
   Body: `{ data: { type: "builds", id: buildId } }`

5. **Submit for review**  
   `POST /v1/appStoreVersionSubmissions`  
   Body: `{ data: { type: "appStoreVersionSubmissions", relationships: { appStoreVersion: { data: { type: "appStoreVersions", id: versionId } } } } }`

Fire-and-forget: exits after submission request is accepted. Does not poll review status.

### Composite Action (`ios-promote/action.yml`)

Same binary download and checksum verification pattern as `android-promote/action.yml`. Single step: run `ios-promote-${OS}-${ARCH}` binary with `INPUT_*` env vars.

## Error Handling

| Scenario | Behavior |
|---|---|
| Build not found / still processing | Error: `build {version}({buildNumber}) not found or still processing` |
| App version already in `PREPARE_FOR_SUBMISSION` | Reuse existing version, continue |
| App version in non-editable state | Error with ASC response body for context |
| Any non-2xx ASC response | Error includes status code + response body |

## Testing

Unit tests use `httptest.NewServer` to mock ASC API responses — same pattern as `internal/playstore/playstore_test.go`.

Test cases:
- Happy path end-to-end
- Build not found (empty results)
- Build still processing (`processingState` not `VALID`)
- App Store version already exists in `PREPARE_FOR_SUBMISSION` (reuse)
- Dry-run output (no HTTP calls made)

Integration tests for `cmd/ios-promote` follow the `INPUT_*` env var pattern of other binaries.
