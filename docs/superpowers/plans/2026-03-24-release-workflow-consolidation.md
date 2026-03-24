# Release Workflow Consolidation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the broken three-workflow chain (test → auto-tag → release) with a single `release.yml` that runs tests, tags, builds binaries, and publishes a GitHub release on every merge to main.

**Architecture:** `release.yml` triggers on `push: branches: [main]` with `paths-ignore: verify-checksums.sha256` to prevent infinite loops from the bot commit. All jobs share the computed tag via `$GITHUB_OUTPUT`, with every downstream job listing `tag` in its own `needs` array (required by GitHub Actions for output access). `build-binaries.yml` remains an unchanged reusable workflow.

**Tech Stack:** GitHub Actions YAML, Go (for binaries), `gh` CLI, `git`

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `.github/workflows/release.yml` | Rewrite | Full pipeline: test → tag → release → build → publish |
| `.github/workflows/test.yml` | Modify | PR-only testing (remove `push: branches: [main]`) |
| `.github/workflows/auto-tag.yml` | Delete | Logic absorbed into `release.yml` |
| `.github/workflows/build-binaries.yml` | No change | Reusable binary build workflow |

---

## Task 1: Simplify test.yml to PR-only

**Files:**
- Modify: `.github/workflows/test.yml`

- [ ] **Step 1: Remove the `push: branches: [main]` trigger**

Change the `on:` block from:

```yaml
on:
  push:
    branches: [main]
  pull_request:
```

to:

```yaml
on:
  pull_request:
```

- [ ] **Step 2: Verify YAML syntax**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/test.yml'))" && echo "OK"
```

Expected: `OK`

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/test.yml
git commit -m "ci: test workflow runs on PRs only"
```

---

## Task 2: Delete auto-tag.yml

**Files:**
- Delete: `.github/workflows/auto-tag.yml`

- [ ] **Step 1: Delete the file**

```bash
rm .github/workflows/auto-tag.yml
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/auto-tag.yml
git commit -m "ci: remove auto-tag workflow (absorbed into release)"
```

---

## Task 3: Rewrite release.yml

**Files:**
- Rewrite: `.github/workflows/release.yml`

- [ ] **Step 1: Replace the entire file with the new pipeline**

```yaml
name: Release

on:
  push:
    branches: [main]
    paths-ignore:
      - 'verify-checksums.sha256'

concurrency:
  group: release
  cancel-in-progress: false

permissions:
  contents: write

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11  # v4.1.1
      - uses: actions/setup-go@0c52d547c9bc32b1aa3301fd7a9cb496313a4491  # v5.0.0
        with:
          go-version: '1.26'
      - name: Run unit tests
        run: go test ./internal/... -v -count=1

  integration-android:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11  # v4.1.1
      - uses: actions/setup-go@0c52d547c9bc32b1aa3301fd7a9cb496313a4491  # v5.0.0
        with:
          go-version: '1.26'

      - uses: actions/setup-java@6a0805fcefea3d4657a47ac4c165951e33482018  # v4.2.2
        with:
          java-version: '17'
          distribution: temurin

      - name: Build Go binaries
        run: |
          for program in android-build android-sign android-upload; do
            CGO_ENABLED=0 go build -o /tmp/ma-bin/${program}-linux-amd64 ./cmd/$program
          done

      - name: Download Gradle wrapper JAR
        run: |
          curl -fsSL -o testdata/android/gradle/wrapper/gradle-wrapper.jar \
            "https://github.com/gradle/gradle/raw/refs/tags/v8.4.0/gradle/wrapper/gradle-wrapper.jar"

      - name: Test android-build
        id: android-build
        env:
          INPUT_APP_PATH: testdata/android
          INPUT_BUILD_TYPE: aab
          RUNNER_TEMP: /tmp/runner
        run: /tmp/ma-bin/android-build-linux-amd64

      - name: Test android-sign
        env:
          INPUT_UNSIGNED_ARTIFACT_PATH: ${{ steps.android-build.outputs.unsigned-artifact-path }}
          INPUT_KEYSTORE_PASSWORD: testpassword
          INPUT_KEY_ALIAS: testkey
          INPUT_KEY_PASSWORD: testpassword
          INPUT_BUILD_TYPE: aab
          RUNNER_TEMP: /tmp/runner
        run: |
          export INPUT_KEYSTORE=$(base64 -w0 testdata/android/test.keystore)
          /tmp/ma-bin/android-sign-linux-amd64

      - name: Test android-upload (dry-run)
        env:
          INPUT_SERVICE_ACCOUNT_JSON: e30K  # base64("{}")
          INPUT_PACKAGE_NAME: com.example.testapp
          INPUT_VERSION_CODE: '1'
          INPUT_TRACK: internal
          INPUT_BUILD_TYPE: aab
          INPUT_ARTIFACT_PATH: /tmp/runner/mobile-actions/signed/app-release.aab
          RUNNER_TEMP: /tmp/runner
        run: /tmp/ma-bin/android-upload-linux-amd64 --dry-run

  integration-ios:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11  # v4.1.1
      - uses: actions/setup-go@0c52d547c9bc32b1aa3301fd7a9cb496313a4491  # v5.0.0
        with:
          go-version: '1.26'

      - name: Build Go binaries
        run: |
          for program in ios-setup-signing ios-teardown-signing ios-build ios-upload verify-checksums; do
            CGO_ENABLED=0 go build -o /tmp/ma-bin/${program}-darwin-arm64 ./cmd/$program
          done

      - name: Test ios-build (stub project)
        env:
          INPUT_APP_PATH: testdata/ios
          INPUT_SCHEME: TestApp
          INPUT_BUNDLE_ID: com.example.testapp
          INPUT_TEAM_ID: FAKETEAMID1
          INPUT_DESTINATION: testflight
          RUNNER_TEMP: /tmp/runner
          GITHUB_OUTPUT: /tmp/github-output-ios
        run: |
          mkdir -p /tmp/runner/mobile-actions
          echo '{"keychain_name":"test","original_keychains":[],"provisioning_profile_path":""}' \
            > /tmp/runner/mobile-actions/signing-state.json
          /tmp/ma-bin/ios-build-darwin-arm64 || true  # expected to fail without real credentials

      - name: Test ios-upload (dry-run)
        env:
          INPUT_APP_STORE_CONNECT_KEY: dGVzdA==
          INPUT_APP_STORE_CONNECT_KEY_ID: TESTKEYID
          INPUT_APP_STORE_CONNECT_ISSUER_ID: test-issuer
          INPUT_XCODE_VERSION: '16.2'
          INPUT_ARTIFACT_PATH: /tmp/test.ipa
          RUNNER_TEMP: /tmp/runner
        run: /tmp/ma-bin/ios-upload-darwin-arm64 --dry-run

  tag:
    needs: [unit-tests, integration-android, integration-ios]
    runs-on: ubuntu-latest
    outputs:
      tag: ${{ steps.compute.outputs.tag }}
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11  # v4.1.1
        with:
          fetch-depth: 0

      - name: Compute and push next tag
        id: compute
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          MAJOR=$(cat VERSION | tr -d '[:space:]')

          LATEST=$(git tag -l "v${MAJOR}.*.*" | sort -V | tail -1)

          if [ -z "$LATEST" ]; then
            NEXT="v${MAJOR}.0.0"
          else
            MINOR=$(echo "$LATEST" | cut -d. -f2)
            PATCH=$(echo "$LATEST" | cut -d. -f3)
            NEXT="v${MAJOR}.${MINOR}.$((PATCH + 1))"
          fi

          echo "Latest tag: ${LATEST:-none}"
          echo "Next tag:   $NEXT"

          git tag "$NEXT"
          git push origin "$NEXT"
          echo "tag=$NEXT" >> $GITHUB_OUTPUT

  create-release:
    needs: tag
    runs-on: ubuntu-latest
    steps:
      - name: Create draft GitHub Release
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          gh release create ${{ needs.tag.outputs.tag }} \
            --repo ${{ github.repository }} \
            --title "${{ needs.tag.outputs.tag }}" \
            --draft \
            --generate-notes

  build:
    needs: [create-release, tag]
    uses: ./.github/workflows/build-binaries.yml
    with:
      release-tag: ${{ needs.tag.outputs.tag }}
    secrets: inherit

  update-tags:
    needs: [build, tag]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11  # v4.1.1
        with:
          fetch-depth: 0

      - name: Update floating major tag and release
        env:
          GH_TOKEN: ${{ github.token }}
          BOOTSTRAP_SHA: ${{ needs.build.outputs.bootstrap-commit-sha }}
        run: |
          MAJOR=$(echo ${{ needs.tag.outputs.tag }} | cut -d. -f1)

          git tag -f $MAJOR $BOOTSTRAP_SHA
          git push -f origin $MAJOR

          gh release delete $MAJOR --yes --cleanup-tag 2>/dev/null || true
          gh release create $MAJOR \
            --title "$MAJOR (latest — ${{ needs.tag.outputs.tag }})" \
            --notes "Always points to the latest $MAJOR.x.x release. Currently ${{ needs.tag.outputs.tag }}."

          ASSET_DIR=$RUNNER_TEMP/release-assets
          mkdir -p $ASSET_DIR
          gh release download ${{ needs.tag.outputs.tag }} --dir $ASSET_DIR
          gh release upload $MAJOR $ASSET_DIR/*

  publish-release:
    needs: [update-tags, tag]
    runs-on: ubuntu-latest
    steps:
      - name: Publish GitHub Release
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          gh release edit ${{ needs.tag.outputs.tag }} \
            --repo ${{ github.repository }} \
            --draft=false
```

- [ ] **Step 2: Verify YAML syntax**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml'))" && echo "OK"
```

Expected: `OK`

- [ ] **Step 3: Confirm no references to `github.ref_name` remain**

The old `release.yml` used `github.ref_name` throughout (it triggered on a tag push, so `ref_name` was the tag). The new trigger is `push: branches: [main]`, so `github.ref_name` resolves to `main`. Verify it is gone:

```bash
grep -n "ref_name" .github/workflows/release.yml
```

Expected: no output.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: consolidate release pipeline into single workflow"
```

---

## Task 4: Verify the complete picture

- [ ] **Step 1: Confirm auto-tag.yml is gone**

```bash
ls .github/workflows/
```

Expected output contains: `build-binaries.yml  release.yml  test.yml`
Does NOT contain: `auto-tag.yml`

- [ ] **Step 2: Confirm test.yml has no push-to-main trigger**

```bash
grep -n "push" .github/workflows/test.yml
```

Expected: no output (only `pull_request` trigger remains).

- [ ] **Step 3: Confirm release.yml has paths-ignore**

```bash
grep -A2 "paths-ignore" .github/workflows/release.yml
```

Expected:
```
    paths-ignore:
      - 'verify-checksums.sha256'
```

- [ ] **Step 4: Confirm all downstream jobs include `tag` in their needs**

```bash
grep "needs:" .github/workflows/release.yml
```

Verify `build`, `update-tags`, and `publish-release` all include `tag` in their `needs` list.

- [ ] **Step 5: Confirm `bootstrap-commit-sha` output key matches between workflows**

```bash
grep "bootstrap-commit-sha" .github/workflows/release.yml .github/workflows/build-binaries.yml
```

Expected: the key appears in both files and matches exactly.

- [ ] **Step 6: Push branch and open PR to trigger a test run**

```bash
git push origin HEAD
```

Open a PR and confirm `Test / unit-tests`, `Test / integration-android`, `Test / integration-ios` all pass on the PR. These come from `test.yml` (PR-only).

After merging to main, confirm `Release` workflow runs and completes all jobs through `publish-release`.
