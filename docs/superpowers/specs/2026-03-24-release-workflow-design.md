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

### Trigger and Concurrency

```yaml
on:
  push:
    branches: [main]
    paths-ignore:
      - 'verify-checksums.sha256'

concurrency:
  group: release
  cancel-in-progress: false  # never cancel a release mid-build
```

`paths-ignore: verify-checksums.sha256` filters the bot commit at the trigger level — the workflow is never queued when only that file changes. This eliminates the infinite loop without a guard job or `[skip ci]`. `cancel-in-progress: false` ensures a release already in progress is never interrupted by a subsequent push.

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
Checks out with `fetch-depth: 0` (required so `git tag -l` can enumerate all existing tags). Reads `VERSION`, finds the latest `vMAJOR.*.*` tag via `git tag -l`, increments patch, pushes the new tag using `GITHUB_TOKEN`. If no prior tag exists for the major version, starts at `v{MAJOR}.0.0`. Outputs `tag` (e.g. `v1.2.3`) via `$GITHUB_OUTPUT`.

No PAT is needed — the release is created directly in the next job rather than relying on the tag push to trigger another workflow.

**`create-release`**
`needs: tag`
Creates a draft GitHub release using `gh release create` with `needs.tag.outputs.tag`.

**`build`**
`needs: [create-release, tag]`
Calls `./.github/workflows/build-binaries.yml` with `release-tag: ${{ needs.tag.outputs.tag }}`. This reusable workflow builds binaries for `linux/amd64`, `darwin/arm64`, `darwin/amd64`, aggregates checksums, commits `verify-checksums.sha256` to main, and uploads all assets to the release. Outputs `bootstrap-commit-sha`.

Note: `tag` is included in `needs` so its output is accessible directly (GitHub Actions only exposes outputs from jobs listed in a job's own `needs` array).

**`update-tags`**
`needs: [build, tag]`
Force-updates the floating major tag (e.g. `v1`) to the bootstrap commit SHA output by `build`. Recreates the `v1` GitHub release. References tag via `needs.tag.outputs.tag`.

**`publish-release`**
`needs: [update-tags, tag]`
Marks the versioned release non-draft via `gh release edit ${{ needs.tag.outputs.tag }} --draft=false`.

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

Every downstream job that needs the tag value must list `tag` in its own `needs` array. `github.ref_name` cannot be used — the trigger is `push: branches: [main]`, so it resolves to `main`, not the version tag.

### No PAT required

The root cause of the original bug was `auto-tag.yml` pushing a tag with `GITHUB_TOKEN` and expecting that to trigger `release.yml`. In the new design, the release pipeline is a single workflow — the tag push and the release steps are all within the same workflow run, so there is no cross-workflow trigger needed.

### Infinite Loop Prevention

`build-binaries.yml` commits `verify-checksums.sha256` directly to `main` (`git push origin HEAD:main`). Without mitigation, this bot commit would trigger `release.yml` again, creating an infinite loop.

**Mitigation:** `paths-ignore: verify-checksums.sha256` on the workflow trigger (see above). Since `build-binaries.yml` only ever commits that single file, the bot push is filtered at the trigger level — no workflow run is queued, no runner time is consumed. No changes to `build-binaries.yml` are needed.

## File Changes

| File | Change |
|------|--------|
| `.github/workflows/release.yml` | Rewritten with the full pipeline |
| `.github/workflows/test.yml` | Remove `push: branches: [main]` trigger; keep `pull_request` only |
| `.github/workflows/auto-tag.yml` | Deleted |
| `.github/workflows/build-binaries.yml` | Unchanged |

## Notes

- Status check names will differ between PRs (`Test / unit-tests`) and post-merge (`Release / unit-tests`). This is expected and intentional.
- First release when no prior tag exists: `git tag -l "v${MAJOR}.*.*"` returns empty, so `NEXT` is set to `v{MAJOR}.0.0`. This is handled by the existing tagging logic inherited from `auto-tag.yml`.
