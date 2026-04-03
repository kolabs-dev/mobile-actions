# iOS Setup Guide

This guide walks you through setting up the `kolabs-dev/mobile-actions/ios-upload@v1` action from scratch. By the end, you will have a GitHub Actions workflow that builds, signs, and uploads your iOS app to TestFlight or the App Store automatically on every tagged release.

It assumes you have built an iOS app but have never automated publishing before.

---

## Prerequisites

- An active [Apple Developer Program](https://developer.apple.com/programs/) membership ($99/year). This is required to distribute apps outside of Xcode directly.
- Your app record already created in [App Store Connect](https://developer.apple.com/app-store/submitting/) before your first upload. The action cannot create the app record for you — it must already exist.

---

## Section 1: Distribution Certificate & Provisioning Profile

**Inputs covered:** `certificate`, `certificate-password`, `provisioning-profile`

### What is the difference between a certificate and a provisioning profile?

- **Certificate** — your signing identity. It proves that builds come from you (or your team). Apple checks this signature before allowing any app onto a device or into the App Store.
- **Provisioning profile** — ties your certificate to a specific app (by bundle identifier), your team, and a set of entitlements (capabilities your app uses). Think of it as the permission slip that says "this certificate is allowed to sign this specific app."

Both files are required to build a distributable `.ipa`.

### Create a Distribution Certificate

1. Open the [Apple Developer Portal Certificates page](https://developer.apple.com/account/resources/certificates/list).
2. Under **Certificates, Identifiers & Profiles**, go to **Certificates** and click **+**.
3. Under Distribution, choose **Apple Distribution**. This certificate type works for both App Store submissions and TestFlight.
4. You need to upload a Certificate Signing Request (CSR). To generate one:
   1. Open **Keychain Access** on your Mac.
   2. Go to **Keychain Access → Certificate Assistant → Request a Certificate from a Certificate Authority**.
   3. Enter your email address, leave the CA Email Address blank, and choose **Saved to disk**.
   4. Save the `.certSigningRequest` file somewhere you can find it.
5. Back in the Developer Portal, upload the `.certSigningRequest` file and click **Continue**.
6. Download the resulting `.cer` file.
7. Double-click the `.cer` file to install it in Keychain Access.

### Export the Certificate as a `.p12` File

The action needs the certificate in `.p12` format (a bundle that includes both the certificate and its private key).

1. Open **Keychain Access** and find the certificate you just installed (look in **My Certificates** — it should appear with a disclosure arrow showing a private key underneath).
2. Right-click the certificate entry and choose **Export**.
3. Choose the file format **Personal Information Exchange (.p12)**.
4. Set a strong password when prompted — this is your `certificate-password`. Save it, you will need it for the GitHub Secret.

### Create an App Store Provisioning Profile

1. In the Apple Developer Portal, go to **Profiles** and click **+**.
2. Under **Distribution**, choose **App Store Connect**.
3. Select your **App ID** — this must match your app's bundle identifier (e.g. `com.example.myapp`).
4. Select the **Distribution certificate** you just created.
5. Give the profile a name (e.g. `MyApp AppStore`) and click **Generate**.
6. Download the `.mobileprovision` file.

### Encode Both Files for GitHub Secrets

GitHub Secrets only accept text, so encode both binary files as Base64 strings:

```bash
base64 < Certificates.p12 | tr -d '\n'
base64 < MyApp.mobileprovision | tr -d '\n'
```

Copy each output separately — they become the values for `IOS_CERTIFICATE_BASE64` and `IOS_PROVISIONING_PROFILE_BASE64`.

**Official references:**
- https://developer.apple.com/support/certificates/
- https://developer.apple.com/documentation/xcode/distributing-your-app-for-beta-testing-and-releases

---

## Section 2: App Store Connect API Key

**Inputs covered:** `app-store-connect-key`, `app-store-connect-key-id`, `app-store-connect-issuer-id`

### What is an App Store Connect API key?

It is a JWT-based key that lets CI authenticate to App Store Connect without a password or interactive login. You create it once in App Store Connect, and CI uses it to upload builds on your behalf.

### Step-by-step setup

1. Go to [App Store Connect](https://appstoreconnect.apple.com) → **Users and Access** → **Integrations** → **App Store Connect API**.
2. Click **Generate API Key**.
3. Give it a name (e.g. `GitHub Actions Upload`) and choose the role **App Manager**.
4. Click **Generate**.

> ⚠️ **Download the `.p8` file immediately.** This file can only be downloaded once. If you close the page without downloading it, you will need to create a new key. Store the file somewhere safe before proceeding.

5. In the keys table, note the **Key ID** displayed next to your key — this is `app-store-connect-key-id` (a 10-character alphanumeric string).
6. At the top of the same page, note the **Issuer ID** — this is `app-store-connect-issuer-id` (a UUID).

### Encode the `.p8` key for GitHub Secrets

```bash
base64 < AuthKey_XXXXXXXXXX.p8 | tr -d '\n'
```

Replace `XXXXXXXXXX` with your actual Key ID. Copy the output — this is the value for `APP_STORE_CONNECT_KEY_BASE64`.

**Official reference:** https://developer.apple.com/documentation/appstoreconnectapi/creating_api_keys_for_app_store_connect_api

---

## Section 3: App Identity

**Inputs covered:** `scheme`, `bundle-id`, `team-id`

### `scheme`

The Xcode scheme name controls which target gets built. To find it, open your project in Xcode and go to **Product → Scheme** — the listed schemes are what you can pass here. The scheme name usually matches your app target name (e.g. `MyApp`).

### `bundle-id`

The bundle identifier (`CFBundleIdentifier`) is your app's unique name in the Apple ecosystem. You can find it in Xcode by selecting your app target → **General** tab → **Bundle Identifier** (e.g. `com.example.myapp`).

This must exactly match the App ID registered in the Apple Developer Portal. If there is a mismatch, the build will fail during signing.

### `team-id`

The team ID is a 10-character alphanumeric string that identifies your Apple Developer account or organization. You can find it in:
- [Apple Developer Portal → Membership](https://developer.apple.com/account/#!/membership/) — listed as "Team ID"
- Xcode → your app target → **Signing & Capabilities** → **Team** dropdown (hover over the team name to see the ID)

---

## Section 4: Upload Destination

**Input covered:** `destination`

The `destination` input controls where your build is sent after a successful upload.

- `testflight` (default): uploads to TestFlight for internal or external beta testing. Builds are reviewed quickly by Apple (usually within a few hours for internal testers) and can be distributed to testers before going public. This is the recommended starting point.
- `app-store`: submits the build directly for App Store review. Use this only when you are ready for a public release.

Start with `testflight` until you have confirmed the full pipeline works end to end.

---

## GitHub Secrets Checklist

In your repository, go to **Settings → Secrets and variables → Actions → New repository secret** and create each of the following:

| Secret name | Value |
|---|---|
| `IOS_CERTIFICATE_BASE64` | Base64-encoded `.p12` certificate |
| `IOS_CERTIFICATE_PASSWORD` | Password you set when exporting the `.p12` |
| `IOS_PROVISIONING_PROFILE_BASE64` | Base64-encoded `.mobileprovision` file |
| `APP_STORE_CONNECT_KEY_BASE64` | Base64-encoded `.p8` API key file |
| `APP_STORE_CONNECT_KEY_ID` | Key ID from App Store Connect (e.g. `ABCD123456`) |
| `APP_STORE_CONNECT_ISSUER_ID` | Issuer ID from App Store Connect (UUID format) |
| `APPLE_TEAM_ID` | Your 10-character Apple Developer Team ID — Required — store as a secret or hardcode the value directly in the workflow's `with:` block. |

---

## Full Workflow Example

Create `.github/workflows/ios-release.yml` in your repository:

```yaml
name: iOS Release

on:
  push:
    tags: ['v*']

jobs:
  release:
    runs-on: macos-15
    steps:
      - uses: actions/checkout@v4

      - uses: kolabs-dev/mobile-actions/ios-upload@v1
        with:
          app-path: ios
          scheme: MyApp
          bundle-id: com.example.myapp
          team-id: ${{ secrets.APPLE_TEAM_ID }}
          xcode-version: '16.2'
          certificate: ${{ secrets.IOS_CERTIFICATE_BASE64 }}
          certificate-password: ${{ secrets.IOS_CERTIFICATE_PASSWORD }}
          provisioning-profile: ${{ secrets.IOS_PROVISIONING_PROFILE_BASE64 }}
          app-store-connect-key: ${{ secrets.APP_STORE_CONNECT_KEY_BASE64 }}
          app-store-connect-key-id: ${{ secrets.APP_STORE_CONNECT_KEY_ID }}
          app-store-connect-issuer-id: ${{ secrets.APP_STORE_CONNECT_ISSUER_ID }}
          destination: testflight
```

Push a tag (e.g. `git tag v1.0.0 && git push origin v1.0.0`) and the workflow will trigger automatically.
