# ios-upload: Replace iTMSTransporter with altool

**Date:** 2026-04-04  
**Status:** Approved

## Problem

Xcode 26.3 ships a stub `iTMSTransporter` binary that exits with code 1 and prints
`"iTMSTransporter is now part of Transporter."` GitHub-hosted macOS runners do not have
Transporter.app installed (it requires the Mac App Store), so uploads fail. The fix in
v1.0.18 added a Transporter.app check but the fallback (`xcrun -find iTMSTransporter`)
still resolves the stub.

## Solution

Replace all iTMSTransporter usage in `cmd/ios-upload/main.go` with `xcrun altool`.
`altool` is bundled with Xcode alongside iTMSTransporter and supports the same API key
auth. It is confirmed present in Xcode 26.10.1.

## Scope

Single file: `cmd/ios-upload/main.go`. No changes to `action.yml`, `internal/`, or any
other binary.

## Design

### `resolveAltool() (string, error)`

Replaces `resolveITMSTransporter()`. Calls `intexec.RunOutput("xcrun", "-find", "altool")`.
Returns the resolved path on success, or:

```
altool not found via xcrun — ensure Xcode is installed
```

No fallback chain. One tool, one resolution path.

### `run()` changes

| Before | After |
|---|---|
| `resolveITMSTransporter()` | `resolveAltool()` |
| `actions.Group("Uploading IPA via iTMSTransporter")` | `actions.Group("Uploading IPA via altool")` |
| `itmsPath, "-m", "upload", "-f", ipaPath, "-apiKey", keyID, "-apiIssuer", issuerID` | `altoolPath, "--upload-app", "-f", ipaPath, "--apiKey", keyID, "--apiIssuer", issuerID` |
| dry-run log uses iTMSTransporter args | dry-run log uses altool args |

`installP8Key()` is unchanged — altool reads from `~/.appstoreconnect/private_keys/AuthKey_<keyID>.p8`, the same convention iTMSTransporter used.

### Error wrapping

The error returned on failure changes from `"iTMSTransporter upload: ..."` to `"altool upload: ..."`.

## Tests

Existing tests cover `installP8Key` only and are unaffected. A `TestResolveAltool`
test is added that skips when `xcrun` is not present, matching the existing test style.

## Backward compatibility

`altool` has been bundled with Xcode since before iTMSTransporter was deprecated. No
`action.yml` changes. Runner requirements are unchanged.
