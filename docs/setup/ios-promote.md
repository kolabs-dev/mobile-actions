# iOS Promote Setup Guide

This guide explains how to use `kolabs-dev/mobile-actions/ios-promote@v1` to submit an iOS build for App Store review.

---

## Overview

The `ios-promote` action takes a build that has already been processed in TestFlight and submits it for App Store review. It does **not** build or upload a new binary; it only initiates the review submission for a specific version and build number.

**Use it after `ios-upload` with `destination: testflight`** — or after any other process that has already delivered a build to TestFlight and given you a version and build number.

---

## Prerequisites

- An App Store Connect API key with App Manager role (see [iOS setup guide](ios-upload.md#section-2-app-store-connect-api-key))
- A build already uploaded to TestFlight with a known version string and build number
- The build must have passed Apple's processing step before it can be submitted for review

---

## Inputs

| Input | Required | Description |
|-------|----------|-------------|
| `app-store-connect-key` | **Yes** | Base64-encoded `.p8` API key (same key used for upload) |
| `app-store-connect-key-id` | **Yes** | App Store Connect key ID (e.g. `ABCD123456`) |
| `app-store-connect-issuer-id` | **Yes** | App Store Connect issuer ID (UUID format) |
| `bundle-id` | **Yes** | App bundle identifier (e.g. `com.myapp`) |
| `version` | **Yes** | Version string to submit (e.g. `1.2.3`) |
| `build-number` | **Yes** | Build number to submit (e.g. `42`) |

---

## Usage

### Minimal usage

```yaml
- uses: kolabs-dev/mobile-actions/ios-promote@v1
  with:
    app-store-connect-key: ${{ secrets.APP_STORE_CONNECT_KEY_BASE64 }}
    app-store-connect-key-id: ${{ secrets.APP_STORE_CONNECT_KEY_ID }}
    app-store-connect-issuer-id: ${{ secrets.APP_STORE_CONNECT_ISSUER_ID }}
    bundle-id: com.example.myapp
    version: 1.2.3
    build-number: ${{ github.run_number }}
```

### Upload to TestFlight and immediately submit for review

```yaml
- uses: kolabs-dev/mobile-actions/ios-upload@v1
  with:
    app-path: ios
    scheme: MyApp
    bundle-id: com.example.myapp
    team-id: ${{ secrets.APPLE_TEAM_ID }}
    certificate: ${{ secrets.IOS_CERTIFICATE_BASE64 }}
    certificate-password: ${{ secrets.IOS_CERTIFICATE_PASSWORD }}
    provisioning-profile: ${{ secrets.IOS_PROVISIONING_PROFILE_BASE64 }}
    app-store-connect-key: ${{ secrets.APP_STORE_CONNECT_KEY_BASE64 }}
    app-store-connect-key-id: ${{ secrets.APP_STORE_CONNECT_KEY_ID }}
    app-store-connect-issuer-id: ${{ secrets.APP_STORE_CONNECT_ISSUER_ID }}
    destination: testflight
    version: 1.2.3
    build-number: ${{ github.run_number }}

- uses: kolabs-dev/mobile-actions/ios-promote@v1
  with:
    app-store-connect-key: ${{ secrets.APP_STORE_CONNECT_KEY_BASE64 }}
    app-store-connect-key-id: ${{ secrets.APP_STORE_CONNECT_KEY_ID }}
    app-store-connect-issuer-id: ${{ secrets.APP_STORE_CONNECT_ISSUER_ID }}
    bundle-id: com.example.myapp
    version: 1.2.3
    build-number: ${{ github.run_number }}
```

### Submit from a separate workflow (manual gate)

A common pattern is to upload to TestFlight automatically on every merge, but require a manual trigger to submit for App Store review:

```yaml
# .github/workflows/ios-promote.yml
name: Submit for App Store Review

on:
  workflow_dispatch:
    inputs:
      version:
        description: 'Version string to submit (e.g. 1.2.3)'
        required: true
      build-number:
        description: 'Build number to submit'
        required: true

jobs:
  promote:
    runs-on: macos-15
    steps:
      - uses: actions/checkout@v4

      - uses: kolabs-dev/mobile-actions/ios-promote@v1
        with:
          app-store-connect-key: ${{ secrets.APP_STORE_CONNECT_KEY_BASE64 }}
          app-store-connect-key-id: ${{ secrets.APP_STORE_CONNECT_KEY_ID }}
          app-store-connect-issuer-id: ${{ secrets.APP_STORE_CONNECT_ISSUER_ID }}
          bundle-id: com.example.myapp
          version: ${{ inputs.version }}
          build-number: ${{ inputs.build-number }}
```

---

## Required Secrets

The `ios-promote` action only requires the App Store Connect API key — no certificate or provisioning profile is needed since no binary is built or signed.

| Secret name | Value |
|---|---|
| `APP_STORE_CONNECT_KEY_BASE64` | Base64-encoded `.p8` API key file |
| `APP_STORE_CONNECT_KEY_ID` | Key ID from App Store Connect (e.g. `ABCD123456`) |
| `APP_STORE_CONNECT_ISSUER_ID` | Issuer ID from App Store Connect (UUID format) |

See [Section 2 of the iOS setup guide](ios-upload.md#section-2-app-store-connect-api-key) for instructions on creating and encoding the API key.
