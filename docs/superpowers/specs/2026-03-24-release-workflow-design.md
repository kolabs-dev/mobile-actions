# Release Workflow Consolidation

**Date:** 2026-03-24
**Status:** Approved

## Problem

The current CI/CD pipeline uses three separate workflows chained together via `workflow_run` and tag-push events:

1. `test.yml` triggers on push to main and PRs
2. `auto-tag.yml` triggers after `Test` completes on main, pushes a semver tag using `GITHUB_TOKEN`
3. `release.yml` triggers on the tag push — but never fires because GitHub suppresses workflow triggers from `GITHUB_TOKEN`-authenticated pushes

## Goal

A single `Release` workflow triggered on merge to main that: re-runs all tests, creates the version tag, builds and attaches binaries for all platforms, and publishes the GitHub release.

No PAT required.

## Design

### Trigger

```yaml
on:
  push:
    branches: [main]
```

### Job Graph

```
unit-tests ──────────┐
integration-android ─┼──► tag ──► create-release ──► build ──► update-tags ──► publish-release
integration-ios ─────┘
```

### Jobs

**`unit-tests`**
Runs `go test ./internal/... -v -count=1` on `ubuntu-latest`. Copied from `test.yml`.

**`integration-android`**
Builds android binaries, runs the full android build/sign/upload(dry-run) pipeline against the test project. Copied from `test.yml`.

**`integration-ios`**
Builds iOS binaries on `macos-latest`, runs ios-build (expected to fail without real credentials) and ios-upload dry-run. Copied from `test.yml`.

**`tag`**
`needs: [unit-tests, integration-android, integration-ios]`
Reads `VERSION`, finds the latest `vMAJOR.*.*` tag via `git tag -l`, increments patch, pushes the new tag using `GITHUB_TOKEN` (no PAT needed — the release is created directly in the next job rather than relying on the tag push to trigger another workflow). Outputs `tag` (e.g. `v1.2.3`) via `$GITHUB_OUTPUT`.

**`create-release`**
`needs: tag`
Creates a draft GitHub release using `gh release create` with the tag from `needs.tag.outputs.tag`.

**`build`**
`needs: create-release`
Calls `./.github/workflows/build-binaries.yml` with `release-tag: ${{ needs.tag.outputs.tag }}`. This reusable workflow builds binaries for `linux/amd64`, `darwin/arm64`, `darwin/amd64`, aggregates checksums, commits `verify-checksums.sha256` to main, and uploads all assets to the release. Outputs `bootstrap-commit-sha`.

**`update-tags`**
`needs: build`
Force-updates the floating major tag (e.g. `v1`) to the bootstrap commit SHA output by `build`. Recreates the `v1` GitHub release pointing to the latest versioned release assets.

**`publish-release`**
`needs: update-tags`
Marks the versioned release non-draft via `gh release edit --draft=false`.

### Passing the tag between jobs

The `tag` job computes and outputs the tag name:

```yaml
outputs:
  tag: ${{ steps.compute.outputs.tag }}
steps:
  - id: compute
    run: |
      ...
      echo "tag=$NEXT" >> $GITHUB_OUTPUT
```

Subsequent jobs reference it as `needs.tag.outputs.tag`. No cross-workflow event propagation is required.

### No PAT required

The root cause of the original bug was `auto-tag.yml` pushing a tag with `GITHUB_TOKEN` and expecting that to trigger `release.yml`. In the new design, the release pipeline is a single workflow — the tag push and the release steps are all within the same workflow run, so there is no cross-workflow trigger needed.

## File Changes

| File | Change |
|------|--------|
| `.github/workflows/release.yml` | Rewritten with the full pipeline |
| `.github/workflows/test.yml` | Remove `push: branches: [main]` trigger; keep `pull_request` only |
| `.github/workflows/auto-tag.yml` | Deleted |
| `.github/workflows/build-binaries.yml` | Unchanged (still a reusable `workflow_call`) |
