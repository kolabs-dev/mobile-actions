# Release Only on Success Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Only push the git tag and create the GitHub release after both tests and binary builds have succeeded — no draft releases.

**Architecture:** Decouple build from release creation. `build-binaries.yml` will build binaries and expose them as a GitHub Actions artifact instead of uploading to a pre-created release. `release.yml` will collect all that output and push the tag, create a published release, upload assets, commit `verify-checksums.sha256`, and update the floating major tag — all in a single final job that only runs after tests + build pass.

**Tech Stack:** GitHub Actions YAML, `gh` CLI, `actions/upload-artifact`, `actions/download-artifact`

---

## Current vs Target Flow

**Current (broken ordering):**
```
tests → tag (push) → create-release (draft) → build (upload to release) → update-tags → publish-release
```
Tag and draft release are created *before* binaries are built. A failed build leaves a dangling tag and draft release.

**Target (correct ordering):**
```
tests ─┐
       ├→ (both pass) → release (push tag, create published release, upload assets, commit, update floating tag)
build ─┘
```
Nothing is published until tests AND build both pass.

---

## File Map

| File | Change |
|------|--------|
| `.github/workflows/build-binaries.yml` | Remove `release-tag` input and `bootstrap-commit-sha` output; remove release upload + checksums commit steps; add artifact upload of `release-assets/` + `verify-checksums.sha256` |
| `.github/workflows/release.yml` | Remove `create-release`, `update-tags`, `publish-release` jobs; split `tag` into `compute-tag` (no push); add single `release` job that runs after compute-tag + build |

---

## Task 1: Modify `build-binaries.yml` — decouple build from release upload

**Files:**
- Modify: `.github/workflows/build-binaries.yml`

This job should only build binaries and expose them as an artifact. All release-related steps move to `release.yml`.

- [ ] **Step 1: Remove `release-tag` input and `bootstrap-commit-sha` output**

In `build-binaries.yml`, delete the `inputs` block and the `outputs` block entirely:

```yaml
# DELETE these blocks:
on:
  workflow_call:
    inputs:
      release-tag:
        required: true
        type: string
    outputs:
      bootstrap-commit-sha:
        description: 'SHA of commit with updated verify-checksums.sha256'
        value: ${{ jobs.aggregate.outputs.commit-sha }}
```

Replace with (no inputs, no outputs):

```yaml
on:
  workflow_call:
```

- [ ] **Step 2: Remove `commit-sha` output from the `aggregate` job**

Delete this from the `aggregate` job:

```yaml
    outputs:
      commit-sha: ${{ steps.commit.outputs.commit-sha }}
```

- [ ] **Step 3: Remove "Upload release assets" step from `aggregate` job**

Delete this entire step:

```yaml
      - name: Upload release assets
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          cd release-assets
          gh release upload ${{ inputs.release-tag }} * --repo ${{ github.repository }}
```

- [ ] **Step 4: Remove "Commit verify-checksums.sha256" step from `aggregate` job**

Delete this entire step:

```yaml
      - name: Commit verify-checksums.sha256
        id: commit
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git add verify-checksums.sha256
          git commit -m "chore: update verify-checksums hashes for ${{ inputs.release-tag }}"
          git push origin HEAD:main
          echo "commit-sha=$(git rev-parse HEAD)" >> $GITHUB_OUTPUT
```

- [ ] **Step 5: Add artifact upload step to `aggregate` job**

After the existing "Merge checksums and prepare release assets" step, add:

```yaml
      - name: Upload release assets artifact
        uses: actions/upload-artifact@5d5d22a31266ced268874388b861e4b58bb5c2f3  # v4.3.1
        with:
          name: release-assets
          path: |
            release-assets/
            verify-checksums.sha256
```

- [ ] **Step 6: Remove unused `GH_TOKEN` env from "Merge checksums" step**

The "Merge checksums and prepare release assets" step had `GH_TOKEN` only for the release upload. Remove it:

```yaml
      - name: Merge checksums and prepare release assets
        # DELETE: env: GH_TOKEN: ${{ github.token }}
        run: |
          ...
```

- [ ] **Step 7: Verify the final `build-binaries.yml` looks correct**

The file should now have:
- `on: workflow_call:` with no inputs or outputs at the top level
- `aggregate` job with no `outputs:` block
- `aggregate` job steps: checkout → download artifacts → merge checksums → upload artifact
- No `gh` CLI usage anywhere in the file

---

## Task 2: Modify `release.yml` — restructure job ordering

**Files:**
- Modify: `.github/workflows/release.yml`

- [ ] **Step 1: Rename `tag` job to `compute-tag` and remove git push**

The current `tag` job pushes the tag immediately after tests. Change it to only compute the tag, not push it. Rename from `tag:` to `compute-tag:` and remove the push lines:

```yaml
  compute-tag:
    needs: tests
    runs-on: ubuntu-latest
    outputs:
      tag: ${{ steps.compute.outputs.tag }}
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11  # v4.1.1
        with:
          fetch-depth: 0

      - name: Compute next tag
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
          echo "tag=$NEXT" >> $GITHUB_OUTPUT
```

Note: `GH_TOKEN` env is no longer needed here since we removed the push, but keeping it is harmless. Remove it for cleanliness.

- [ ] **Step 2: Update `build` job — remove dependency on `create-release`, remove `release-tag` input**

Change the `build` job from:

```yaml
  build:
    needs: [create-release, tag]
    uses: ./.github/workflows/build-binaries.yml
    with:
      release-tag: ${{ needs.tag.outputs.tag }}
    secrets: inherit
```

To:

```yaml
  build:
    needs: tests
    uses: ./.github/workflows/build-binaries.yml
    secrets: inherit
```

`build` now runs directly after `tests` (in parallel with `compute-tag`), with no inputs.

- [ ] **Step 3: Delete `create-release` job entirely**

Remove:

```yaml
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
```

- [ ] **Step 4: Delete `update-tags` job entirely**

Remove the entire `update-tags` job.

- [ ] **Step 5: Delete `publish-release` job entirely**

Remove the entire `publish-release` job.

- [ ] **Step 6: Add the new `release` job**

Add this job after the `build` job:

```yaml
  release:
    needs: [compute-tag, build]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11  # v4.1.1
        with:
          fetch-depth: 0

      - name: Download release assets
        uses: actions/download-artifact@87c55149d96e628cc2ef7e6fc2aab372015aec85  # v4.1.3
        with:
          name: release-assets
          path: release-assets/

      - name: Push tag
        run: |
          git tag ${{ needs.compute-tag.outputs.tag }}
          git push origin ${{ needs.compute-tag.outputs.tag }}

      - name: Create and publish GitHub Release
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          gh release create ${{ needs.compute-tag.outputs.tag }} \
            --repo ${{ github.repository }} \
            --title "${{ needs.compute-tag.outputs.tag }}" \
            --generate-notes \
            release-assets/*

      - name: Commit verify-checksums.sha256
        run: |
          cp release-assets/verify-checksums.sha256 verify-checksums.sha256
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git add verify-checksums.sha256
          git commit -m "chore: update verify-checksums hashes for ${{ needs.compute-tag.outputs.tag }}"
          git push origin HEAD:main

      - name: Update floating major tag and release
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          MAJOR=$(echo ${{ needs.compute-tag.outputs.tag }} | cut -d. -f1)
          BOOTSTRAP_SHA=$(git rev-parse HEAD)

          git tag -f $MAJOR $BOOTSTRAP_SHA
          git push -f origin $MAJOR

          gh release delete $MAJOR --yes --cleanup-tag 2>/dev/null || true
          gh release create $MAJOR \
            --title "$MAJOR (latest — ${{ needs.compute-tag.outputs.tag }})" \
            --notes "Always points to the latest $MAJOR.x.x release. Currently ${{ needs.compute-tag.outputs.tag }}."

          gh release upload $MAJOR release-assets/*
```

- [ ] **Step 7: Verify the final `release.yml` job graph**

The dependency graph should be:

```
tests ──────────────┬── compute-tag ─┐
                    │                ├── release
                    └── build ───────┘
```

Jobs: `tests`, `compute-tag` (needs tests), `build` (needs tests), `release` (needs compute-tag + build)

No `create-release`, `update-tags`, or `publish-release` jobs.

- [ ] **Step 8: Commit**

```bash
git add .github/workflows/release.yml .github/workflows/build-binaries.yml
git commit -m "fix: only create tag and release after tests and build succeed"
```
