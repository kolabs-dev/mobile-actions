# Android Promote Setup Guide

This guide explains how to use `kolabs-dev/mobile-actions/android-promote@v1` to promote an already-uploaded Android app to a Play Store track.

---

## Overview

The `android-promote` action moves an existing build to a higher track in the Play Store — for example, from `internal` testing to `alpha`, `beta`, or `production`. It does **not** build or upload a new binary; it only updates which track a version code is assigned to.

**Use it after `android-upload`** — or after any other process that has already uploaded a build to Google Play and given you a version code.

---

## Prerequisites

- A Google service account with Play Store access (see [Android setup guide](android.md#section-2-google-play-service-account))
- A build already uploaded to Google Play with a known version code
- The version must be in a track before you can promote it (e.g. uploaded to `internal` first)

---

## Inputs

| Input | Required | Description |
|-------|----------|-------------|
| `service-account-json` | **Yes** | Base64-encoded Google service account JSON (same account used for upload) |
| `package-name` | **Yes** | App package name (e.g. `com.myapp`) |
| `version-code` | **Yes** | Integer version code to promote |
| `target-track` | **Yes** | Play Store track to promote to: `alpha`, `beta`, or `production` |

> **Note:** The `internal` track is not a valid `target-track`. You promote **to** alpha, beta, or production. The `internal` track is the starting point — use `android-upload` with `track: internal` to get there.

---

## Usage

### Minimal usage

```yaml
- uses: kolabs-dev/mobile-actions/android-promote@v1
  with:
    service-account-json: ${{ secrets.GOOGLE_SERVICE_ACCOUNT_JSON_BASE64 }}
    package-name: com.example.myapp
    version-code: ${{ github.run_number }}
    target-track: production
```

### Upload and immediately promote

```yaml
- uses: kolabs-dev/mobile-actions/android-upload@v1
  with:
    keystore: ${{ secrets.ANDROID_KEYSTORE_BASE64 }}
    keystore-password: ${{ secrets.ANDROID_KEYSTORE_PASSWORD }}
    key-alias: ${{ secrets.ANDROID_KEY_ALIAS }}
    key-password: ${{ secrets.ANDROID_KEY_PASSWORD }}
    service-account-json: ${{ secrets.GOOGLE_SERVICE_ACCOUNT_JSON_BASE64 }}
    package-name: com.example.myapp
    version-code: ${{ github.run_number }}
    track: internal

- uses: kolabs-dev/mobile-actions/android-promote@v1
  with:
    service-account-json: ${{ secrets.GOOGLE_SERVICE_ACCOUNT_JSON_BASE64 }}
    package-name: com.example.myapp
    version-code: ${{ github.run_number }}
    target-track: production
```

### Staged promotion (internal → alpha → production)

To promote through multiple tracks, call `android-promote` twice:

```yaml
- uses: kolabs-dev/mobile-actions/android-upload@v1
  with:
    # ... upload inputs ...
    track: internal

- uses: kolabs-dev/mobile-actions/android-promote@v1
  with:
    service-account-json: ${{ secrets.GOOGLE_SERVICE_ACCOUNT_JSON_BASE64 }}
    package-name: com.example.myapp
    version-code: ${{ github.run_number }}
    target-track: alpha

- uses: kolabs-dev/mobile-actions/android-promote@v1
  with:
    service-account-json: ${{ secrets.GOOGLE_SERVICE_ACCOUNT_JSON_BASE64 }}
    package-name: com.example.myapp
    version-code: ${{ github.run_number }}
    target-track: production
```

### Promote from a separate workflow (manual gate)

A common pattern is to upload automatically on every merge, but require a manual trigger to promote to production:

```yaml
# .github/workflows/android-promote.yml
name: Promote to Production

on:
  workflow_dispatch:
    inputs:
      version-code:
        description: 'Version code to promote'
        required: true

jobs:
  promote:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: kolabs-dev/mobile-actions/android-promote@v1
        with:
          service-account-json: ${{ secrets.GOOGLE_SERVICE_ACCOUNT_JSON_BASE64 }}
          package-name: com.example.myapp
          version-code: ${{ inputs.version-code }}
          target-track: production
```

---

## Required Secret

The `android-promote` action only requires the service account secret — no keystore is needed since no binary is signed.

| Secret name | Value |
|---|---|
| `GOOGLE_SERVICE_ACCOUNT_JSON_BASE64` | Base64-encoded service account JSON key |

The service account must have the **Versions** permission for your app in Google Play Console. See [Section 2 of the Android setup guide](android.md#section-2-google-play-service-account) for setup instructions.
