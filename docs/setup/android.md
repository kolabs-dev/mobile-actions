# Android Setup Guide

This guide walks you through setting up the `kolabs-dev/mobile-actions/android@v1` action from scratch. By the end, you will have a GitHub Actions workflow that builds, signs, and uploads your Android app to Google Play automatically on every tagged release.

It assumes you have built an Android app but have never automated publishing before.

---

## Prerequisites

- A Google account with access to [Google Play Console](https://play.google.com/console)
- Your app already created in Google Play Console (even if you have not published anything yet)

> ⚠️ **Important: First upload must be done manually.** The Google Play API cannot publish an app's very first build — that initial upload must be done by hand through the Play Console web UI. Once at least one build has been uploaded manually, the API can take over for all future uploads. See [Google's instructions](https://support.google.com/googleplay/android-developer/answer/9859348) for how to create the first release.

---

## Section 1: Signing Keystore

**Inputs covered:** `keystore`, `keystore-password`, `key-alias`, `key-password`

### What is a keystore?

A keystore is a file that holds your app's signing identity — a cryptographic key that proves updates come from you. Google Play ties every app to the key it was first signed with. When you upload a new version, Google checks that it is signed with the same key.

> ⚠️ **Critical warning: Never lose your keystore.** If you lose this file and its passwords, you can never publish an update to your app. Back it up somewhere safe — a password manager, encrypted cloud storage, or an offline drive. Do not rely solely on your repo or your laptop.

### Generate a keystore

Run the following command in your terminal. Replace `my-key-alias` with a label of your choice (you will reference it later):

```bash
keytool -genkey -v -keystore release.jks -alias my-key-alias \
  -keyalg RSA -keysize 2048 -validity 10000
```

`keytool` will prompt you for:

- **Keystore password** — choose a strong password and save it (this is `keystore-password`)
- **Key password** — can be the same as the keystore password or different (this is `key-password`)
- Your name, organization, city, country — these are embedded in the certificate; fill them in as you like

The `-alias` value you typed (`my-key-alias` in the example) is the `key-alias` input.

### Encode the keystore for GitHub Secrets

GitHub Secrets only accept text, so encode the binary `.jks` file as a Base64 string:

```bash
base64 < release.jks | tr -d '\n'
```

Copy the output — this is the value for the `ANDROID_KEYSTORE_BASE64` secret.

**Official reference:** https://developer.android.com/studio/publish/app-signing

---

## Section 2: Google Play Service Account

**Input covered:** `service-account-json`

### What is a service account?

A service account is a non-human Google identity. Instead of logging in with your personal Google account during CI, you create a dedicated account that CI uses to upload builds on your behalf. You grant it only the permissions it needs (uploading releases) and nothing else.

### Step-by-step setup

1. Open [Google Play Console](https://play.google.com/console) and go to **Setup → API access**.
2. Link your Play Console to a Google Cloud project. If you do not have one yet, click **Create new project** — Google will create one automatically.
3. Under "Service accounts", click **Create new service account**.
4. Click the link that takes you to **Google Cloud Console**. There:
   1. Fill in a name for the service account (e.g. `github-actions-upload`).
   2. Click **Create and continue**, then skip the optional role fields and click **Done**.
   3. Find the service account in the list, click it, then go to the **Keys** tab.
   4. Click **Add key → Create new key**, choose **JSON**, and download the file.
5. Back in Play Console, refresh the service accounts list. Find the account you just created and click **Grant access**.
6. Assign the **Release Manager** role (or a custom role with at least the "Releases" permission), then click **Apply**.

### Encode the JSON key for GitHub Secrets

```bash
base64 < service-account.json | tr -d '\n'
```

Copy the output — this is the value for the `GOOGLE_SERVICE_ACCOUNT_JSON_BASE64` secret.

**Official reference:** https://developers.google.com/android-publisher/getting_started

---

## Section 3: Build-time Config Files

Some libraries and services require config files to be present in specific locations inside your `android/` directory before Gradle runs. These files are typically not committed to the repo (especially in public repos) because they contain API keys or project identifiers.

You must inject them as a workflow step **before** calling `mobile-actions/android`.

### Firebase (`google-services.json`)

Firebase requires `android/app/google-services.json` to be present at build time. The Google Services Gradle plugin reads it automatically — you do not need to change any Gradle config.

1. Copy the contents of your `google-services.json` and encode it:

   ```bash
   base64 < google-services.json | tr -d '\n'
   ```

2. Add the output as a GitHub Secret (e.g. `GOOGLE_SERVICES_JSON_BASE64`).

3. Add a step to your workflow **before** the `mobile-actions/android` step to decode and write the file:

   ```yaml
   - name: Inject google-services.json
     run: |
       echo "${{ secrets.GOOGLE_SERVICES_JSON_BASE64 }}" \
         | base64 -d > android/app/google-services.json
   ```

> ℹ️ If your repo is private and you are comfortable with the security trade-offs, you can commit `google-services.json` directly and skip this step.

### Other config files

The same pattern applies to any file that must exist at a known path before the build runs — for example:

- `android/app/google-services.json` (Firebase)
- `android/app/src/main/assets/google-services.json` (some SDK variants)
- `.env` files consumed by Gradle
- Any SDK-specific JSON or `.plist`-equivalent dropped into the project tree

Encode the file as Base64, store it as a secret, and decode it in the workflow before the action runs:

```yaml
- name: Inject <config-file>
  run: |
    echo "${{ secrets.<SECRET_NAME> }}" \
      | base64 -d > android/app/<config-file>
```

If you need to inject multiple files, add one step per file.

---

## Section 4: App Metadata

**Inputs covered:** `package-name`, `version-code`, `track`

### `package-name`

This is your app's unique identifier on Google Play — the `applicationId` in your `app/build.gradle` file. It looks like `com.example.myapp`. Copy it exactly as it appears there.

### `version-code`

The version code is a positive integer that must be higher than the last uploaded build. Google Play rejects any upload whose version code is not strictly greater than the previous one.

The easiest approach in CI is to use `${{ github.run_number }}`, which GitHub increments automatically with every workflow run:

```yaml
version-code: ${{ github.run_number }}
```

This means you never have to manage version codes manually.

> ⚠️ `run_number` works as a starting point but resets if the workflow is recreated. If you've ever manually uploaded a build with a higher version code, the API will reject uploads with a lower number. Consider managing `versionCode` in `build.gradle` directly for production workflows.

### `track`

The track controls where the build is published after upload:

| Track | Audience |
|---|---|
| `internal` | Only internal testers you add in Play Console — recommended for first setup |
| `alpha` | Closed testing (invited testers) |
| `beta` | Open testing (anyone can opt in) |
| `production` | Live on the Play Store for all users |

Start with `internal` until you have confirmed the full pipeline works end to end. The default value for `track` is `internal`, so you can omit this input once you understand the available options.

**Official reference:** https://support.google.com/googleplay/android-developer/answer/9859348

---

## GitHub Secrets Checklist

In your repository, go to **Settings → Secrets and variables → Actions → New repository secret** and create each of the following:

| Secret name | Value |
|---|---|
| `ANDROID_KEYSTORE_BASE64` | Base64-encoded `release.jks` |
| `ANDROID_KEYSTORE_PASSWORD` | Keystore password you chose during `keytool` |
| `ANDROID_KEY_ALIAS` | The `-alias` value you passed to `keytool` |
| `ANDROID_KEY_PASSWORD` | Key password you chose during `keytool` |
| `GOOGLE_SERVICE_ACCOUNT_JSON_BASE64` | Base64-encoded service account JSON key |

---

## Full Workflow Example

Create `.github/workflows/android-release.yml` in your repository:

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

      - uses: kolabs-dev/mobile-actions/android@v1
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

Push a tag (e.g. `git tag v1.0.0 && git push origin v1.0.0`) and the workflow will trigger automatically.
