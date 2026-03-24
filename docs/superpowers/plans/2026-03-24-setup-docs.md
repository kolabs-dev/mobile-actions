# Setup Docs (Android & iOS) Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Write beginner-friendly setup guides for Android and iOS that walk first-time publishers through obtaining every credential the action needs, with links to official resources.

**Architecture:** Two new Markdown files under `docs/setup/` — one per platform. Each file explains every required input: what it is, why it's needed, how to generate it (step by step), and how to encode it for GitHub Secrets. No code changes required; this is docs-only.

**Tech Stack:** Markdown, GitHub Secrets, Google Play Console, Apple Developer Portal, App Store Connect.

---

## File Structure

- Create: `docs/setup/android.md` — Full setup guide for the Android action inputs
- Create: `docs/setup/ios.md` — Full setup guide for the iOS action inputs

---

## Chunk 1: Android Setup Guide

### Task 1: Create `docs/setup/android.md`

**Files:**
- Create: `docs/setup/android.md`

- [ ] **Step 1: Create the file with the full Android setup guide**

  The file should cover every required input in the order a new user would encounter them when setting up CI for the first time. Structure:

  1. Introduction — what the action does, what you need before starting
  2. Prerequisites — Google account, app created in Play Console, first manual upload done
  3. Section per credential group:
     - **Signing keystore** (`keystore`, `keystore-password`, `key-alias`, `key-password`)
     - **Google Play service account** (`service-account-json`)
     - **App metadata** (`package-name`, `version-code`, `track`)
  4. GitHub Secrets checklist
  5. Links to official resources

  Content details per section:

  **Signing keystore**
  - Explain what a keystore is (a file that holds your signing identity; Google uses it to verify updates come from you)
  - Warn: losing this file means you can never update the app — back it up securely
  - How to generate one with `keytool` (ship the exact command)
  - How to export base64: `base64 -i release.jks | tr -d '\n'`
  - What `key-alias` is (a label for the key inside the keystore — you chose it when running `keytool`)
  - Official link: https://developer.android.com/studio/publish/app-signing

  **Google Play service account**
  - Explain what a service account is (a non-human Google identity that CI uses to upload on your behalf)
  - Step-by-step: Google Play Console → Setup → API access → Link to a Google Cloud project → Create service account → Grant "Release Manager" role → Download JSON key
  - How to base64-encode: `base64 -i service-account.json | tr -d '\n'`
  - Official link: https://developers.google.com/android-publisher/getting_started

  **App metadata**
  - `package-name`: the application ID in `build.gradle` (`applicationId`)
  - `version-code`: must be higher than the last upload; recommend `${{ github.run_number }}` for simplicity
  - `track`: explain internal/alpha/beta/production; recommend starting with `internal`
  - Official link: https://support.google.com/googleplay/android-developer/answer/9859348

  **Prerequisites note:** Play Store requires a first manual upload before the API can upload subsequent builds. Link to the official publishing guide.

  **GitHub Secrets checklist** — list all secret names to create:
  - `ANDROID_KEYSTORE_BASE64`
  - `ANDROID_KEYSTORE_PASSWORD`
  - `ANDROID_KEY_ALIAS`
  - `ANDROID_KEY_PASSWORD`
  - `GOOGLE_SERVICE_ACCOUNT_JSON_BASE64`

- [ ] **Step 2: Verify the file renders correctly**

  Open the file and scan for broken links, missing code fences, or truncated sections.

- [ ] **Step 3: Commit**

  ```bash
  git add docs/setup/android.md
  git commit -m "docs: add Android setup guide for first-time publishers"
  ```

---

## Chunk 2: iOS Setup Guide

### Task 2: Create `docs/setup/ios.md`

**Files:**
- Create: `docs/setup/ios.md`

- [ ] **Step 1: Create the file with the full iOS setup guide**

  Structure mirrors the Android guide. Cover every required input:

  1. Introduction — what the action does, what you need before starting
  2. Prerequisites — Apple Developer Program membership ($99/year), app record created in App Store Connect
  3. Section per credential group:
     - **Distribution certificate & provisioning profile** (`certificate`, `certificate-password`, `provisioning-profile`)
     - **App Store Connect API key** (`app-store-connect-key`, `app-store-connect-key-id`, `app-store-connect-issuer-id`)
     - **App identity** (`scheme`, `bundle-id`, `team-id`)
     - **Destination** (`destination`)
  4. GitHub Secrets checklist
  5. Links to official resources

  Content details per section:

  **Distribution certificate & provisioning profile**
  - Explain the certificate (your signing identity; proves builds come from you) vs. the provisioning profile (ties the certificate to a specific app/team/entitlements)
  - How to create a Distribution certificate:
    1. Apple Developer Portal → Certificates, Identifiers & Profiles → Certificates → +
    2. Choose "Apple Distribution" (works for both App Store and TestFlight)
    3. Upload a CSR (Certificate Signing Request) generated in Keychain Access
    4. Download the `.cer` and install it in Keychain Access
    5. Export from Keychain Access as `.p12` (right-click → Export → Personal Information Exchange)
  - How to create an App Store provisioning profile:
    1. Apple Developer Portal → Profiles → +
    2. Choose "App Store Connect" distribution type
    3. Select your App ID and the Distribution certificate
    4. Download the `.mobileprovision` file
  - How to base64-encode both files:
    ```bash
    base64 -i Certificates.p12 | tr -d '\n'
    base64 -i MyApp.mobileprovision | tr -d '\n'
    ```
  - Official links:
    - https://developer.apple.com/support/certificates/
    - https://developer.apple.com/documentation/xcode/distributing-your-app-for-beta-testing-and-releases

  **App Store Connect API key**
  - Explain what it is (a JWT-based key that lets CI authenticate to App Store Connect without a password)
  - Step-by-step: App Store Connect → Users and Access → Integrations → App Store Connect API → Generate API Key → choose "Developer" or "App Manager" role → download the `.p8` file (only downloadable once — save it)
  - Note the Key ID (shown in the table) and Issuer ID (shown at the top of the Keys page)
  - How to base64-encode: `base64 -i AuthKey_XXXXXXXXXX.p8 | tr -d '\n'`
  - Official link: https://developer.apple.com/documentation/appstoreconnectapi/creating_api_keys_for_app_store_connect_api

  **App identity**
  - `scheme`: the Xcode scheme name (open Xcode → Product menu → Scheme → shows available schemes; usually the app target name)
  - `bundle-id`: the CFBundleIdentifier in `Info.plist` / Xcode target settings (e.g. `com.example.myapp`)
  - `team-id`: 10-character alphanumeric string visible in Apple Developer Portal → Membership, or in Xcode target signing settings
  - Official link: https://developer.apple.com/documentation/appstoreconnectapi

  **Destination**
  - `testflight`: upload to TestFlight for internal/external testing
  - `app-store`: submit directly for App Store review
  - Recommend starting with `testflight`

  **Prerequisites note:**
  - Apple Developer Program membership required
  - App record must exist in App Store Connect before first upload
  - Link to: https://developer.apple.com/app-store/submitting/

  **GitHub Secrets checklist** — list all secret names to create:
  - `IOS_CERTIFICATE_BASE64`
  - `IOS_CERTIFICATE_PASSWORD`
  - `IOS_PROVISIONING_PROFILE_BASE64`
  - `APP_STORE_CONNECT_KEY_BASE64`
  - `APP_STORE_CONNECT_KEY_ID`
  - `APP_STORE_CONNECT_ISSUER_ID`
  - `APPLE_TEAM_ID` (optional — can be hardcoded)

- [ ] **Step 2: Verify the file renders correctly**

  Scan for broken links, missing code fences, or truncated sections.

- [ ] **Step 3: Commit**

  ```bash
  git add docs/setup/ios.md
  git commit -m "docs: add iOS setup guide for first-time publishers"
  ```

---

## Chunk 3: Link from README

### Task 3: Add links to setup guides from README.md

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add "Setup guides" section links in README**

  After the "Secrets setup" subsection in each platform section of the README, add a note pointing to the detailed guide:

  For Android (after the existing "Secrets setup" subsection):
  ```markdown
  > First time? See the [Android setup guide](docs/setup/android.md) for step-by-step instructions on creating a keystore, setting up a service account, and configuring GitHub Secrets.
  ```

  For iOS (after the existing "Secrets setup" subsection):
  ```markdown
  > First time? See the [iOS setup guide](docs/setup/ios.md) for step-by-step instructions on creating a distribution certificate, provisioning profile, and App Store Connect API key.
  ```

- [ ] **Step 2: Commit**

  ```bash
  git add README.md
  git commit -m "docs: link to setup guides from README"
  ```
