# mobile-actions

Reusable GitHub Actions for automating mobile app releases. Provides three composite actions:

- **`kolabs-dev/mobile-actions/android-upload@v1`** — Build, sign, and upload Android apps (AAB/APK) to Google Play Store
- **`kolabs-dev/mobile-actions/android-promote@v1`** — Promote an uploaded Android app to a Play Store track (alpha/beta/production)
- **`kolabs-dev/mobile-actions/ios-upload@v1`** — Build, sign, and upload iOS apps (IPA) to App Store Connect / TestFlight

## Android Upload Action

Builds a Gradle project, signs the artifact with your keystore, and uploads it to Google Play Store.

### Usage

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
```

### Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `app-path` | No | `.` | Path to the Android app directory |
| `build-type` | No | `aab` | Build type: `aab` or `apk` |
| `gradle-task` | No | | Override Gradle task (auto-detected if not set) |
| `java-version` | No | `17` | JDK version |
| `keystore` | **Yes** | | Base64-encoded `.jks` keystore |
| `keystore-password` | **Yes** | | Keystore password |
| `key-alias` | **Yes** | | Key alias |
| `key-password` | **Yes** | | Key password |
| `service-account-json` | **Yes** | | Base64-encoded Google service account JSON |
| `package-name` | **Yes** | | App package name (e.g. `com.myapp`) |
| `version-code` | **Yes** | | Integer version code |
| `track` | No | `internal` | Play Store track: `internal`, `alpha`, `beta`, or `production` |

### Outputs

| Output | Description |
|--------|-------------|
| `artifact-path` | Path to the signed artifact |

### Secrets setup

**Keystore** — encode your `.jks` file as base64:
```bash
base64 -i release.jks | tr -d '\n'
```

**Google service account** — encode your service account JSON as base64:
```bash
base64 -i service-account.json | tr -d '\n'
```

The service account needs the **Release Manager** role (or equivalent) in Google Play Console.

> First time? See the [Android setup guide](docs/setup/android-upload.md) for step-by-step instructions on creating a keystore, setting up a service account, and configuring GitHub Secrets.

### Full example

```yaml
name: Android Release

on:
  push:
    tags: ['v*']

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: kolabs-dev/mobile-actions/android-upload@v1
        with:
          app-path: android
          build-type: aab
          java-version: '17'
          keystore: ${{ secrets.ANDROID_KEYSTORE_BASE64 }}
          keystore-password: ${{ secrets.ANDROID_KEYSTORE_PASSWORD }}
          key-alias: ${{ secrets.ANDROID_KEY_ALIAS }}
          key-password: ${{ secrets.ANDROID_KEY_PASSWORD }}
          service-account-json: ${{ secrets.GOOGLE_SERVICE_ACCOUNT_JSON_BASE64 }}
          package-name: com.example.myapp
          version-code: ${{ github.run_number }}
          track: internal
```

---

## Android Promote Action

Promotes an already-uploaded Android app version to a Play Store track. Use this after `android-upload` to move a build from `internal` testing through to `production`.

### Usage

```yaml
- uses: kolabs-dev/mobile-actions/android-promote@v1
  with:
    service-account-json: ${{ secrets.GOOGLE_SERVICE_ACCOUNT_JSON_BASE64 }}
    package-name: com.example.myapp
    version-code: ${{ github.run_number }}
    target-track: production
```

### Inputs

| Input | Required | Description |
|-------|----------|-------------|
| `service-account-json` | **Yes** | Base64-encoded Google service account JSON |
| `package-name` | **Yes** | App package name (e.g. `com.myapp`) |
| `version-code` | **Yes** | Integer version code to promote |
| `target-track` | **Yes** | Play Store track: `alpha`, `beta`, or `production` |

### Full example

A two-step workflow that uploads to `internal` and then promotes to `production`:

```yaml
name: Android Release

on:
  push:
    tags: ['v*']

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: kolabs-dev/mobile-actions/android-upload@v1
        with:
          app-path: android
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

> **Note:** `target-track` accepts `alpha`, `beta`, or `production`. The `internal` track is only valid for upload, not promotion. To promote through multiple tracks, call the action twice.

> First time? See the [Android Promote setup guide](docs/setup/android-promote.md) for usage patterns including staged promotion and manual release gates.

---

## iOS Upload Action

Builds an Xcode project, signs the IPA with your certificate and provisioning profile, and uploads it to App Store Connect or TestFlight.

### Usage

```yaml
- uses: kolabs-dev/mobile-actions/ios-upload@v1
  with:
    scheme: MyApp
    bundle-id: com.example.myapp
    team-id: ABCD123456
    certificate: ${{ secrets.IOS_CERTIFICATE_BASE64 }}
    certificate-password: ${{ secrets.IOS_CERTIFICATE_PASSWORD }}
    provisioning-profile: ${{ secrets.IOS_PROVISIONING_PROFILE_BASE64 }}
    app-store-connect-key: ${{ secrets.APP_STORE_CONNECT_KEY_BASE64 }}
    app-store-connect-key-id: ${{ secrets.APP_STORE_CONNECT_KEY_ID }}
    app-store-connect-issuer-id: ${{ secrets.APP_STORE_CONNECT_ISSUER_ID }}
```

### Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `app-path` | No | `.` | Path to the iOS app directory |
| `scheme` | **Yes** | | Xcode scheme name |
| `workspace` | No | | Path to `.xcworkspace` (auto-detected if not set) |
| `bundle-id` | **Yes** | | App bundle identifier |
| `team-id` | **Yes** | | Apple Developer Team ID (10-character alphanumeric) |
| `xcode-version` | No | `16.2` | Xcode version (must be pre-installed on runner) |
| `certificate` | **Yes** | | Base64-encoded `.p12` certificate |
| `certificate-password` | **Yes** | | P12 password |
| `provisioning-profile` | **Yes** | | Base64-encoded `.mobileprovision` |
| `app-store-connect-key` | **Yes** | | Base64-encoded `.p8` API key |
| `app-store-connect-key-id` | **Yes** | | App Store Connect key ID |
| `app-store-connect-issuer-id` | **Yes** | | App Store Connect issuer ID |
| `destination` | No | `testflight` | Upload destination: `testflight` or `app-store` |

### Outputs

| Output | Description |
|--------|-------------|
| `artifact-path` | Path to the exported `.ipa` |

### Secrets setup

**Distribution certificate** — export your `.p12` from Keychain Access and encode it:
```bash
base64 -i Certificates.p12 | tr -d '\n'
```

**Provisioning profile** — encode your `.mobileprovision` file:
```bash
base64 -i MyApp.mobileprovision | tr -d '\n'
```

**App Store Connect API key** — create a key at [App Store Connect > Users and Access > Keys](https://appstoreconnect.apple.com/access/api), download the `.p8` file, and encode it:
```bash
base64 -i AuthKey_XXXXXXXXXX.p8 | tr -d '\n'
```

> First time? See the [iOS setup guide](docs/setup/ios-upload.md) for step-by-step instructions on creating a distribution certificate, provisioning profile, and App Store Connect API key.

### Full example

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

---

## Development

### Building binaries locally

All programs live under `cmd/`. Binaries must be compiled with `CGO_ENABLED=0` and are never committed to the repository.

Build a single binary:
```bash
CGO_ENABLED=0 go build -o bin/android-build ./cmd/android-build
```

Build all binaries for the current platform:
```bash
for program in android-build android-sign android-upload android-promote ios-setup-signing ios-teardown-signing ios-build ios-upload verify-checksums; do
  CGO_ENABLED=0 go build -o bin/$program ./cmd/$program
done
```

Run tests and lint:
```bash
go test ./internal/... ./cmd/... -v -count=1
go vet ./...
```

### Releasing a new version

Releases are fully automated. Every push to `main` triggers the [Release workflow](.github/workflows/release.yml), which runs all tests, then reads the major version from the `VERSION` file, computes the next `vMAJOR.MINOR.PATCH` tag (incrementing patch), builds and uploads binaries for all platforms, updates the floating `vMAJOR` tag, and publishes the GitHub Release. If any test fails, no tag is created and no release is published.

**Bumping the major version:**

Edit `VERSION` and commit to `main`:
```bash
echo "2" > VERSION
git add VERSION
git commit -m "chore: bump major version to v2"
git push origin main
# → auto-creates v2.0.0 and publishes a new release
```

The floating `v1` tag continues to point to the last `v1.x.x` release. Users pinned to `@v1` are unaffected.

> **Note:** `verify-checksums.sha256` and tag creation are handled automatically. Do not push tags manually.

---

## Security

All binaries are compiled with `CGO_ENABLED=0` and distributed as GitHub Release assets. Each release includes a `checksums.txt` file with SHA-256 hashes for all binaries. The composite actions verify checksums before executing any binary — the `verify-checksums.sha256` file committed to this repository serves as the trust anchor for bootstrap verification.
