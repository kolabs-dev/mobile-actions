# Auto-Tag on Main Push Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automatically create a semver tag and GitHub Release on every push to `main` where all tests pass, using a `VERSION` file to pin the major version.

**Architecture:** A new `auto-tag.yml` workflow triggers via `workflow_run` after the `Test` workflow completes successfully on `main`. It reads the major version from a `VERSION` file, finds the latest existing tag for that major, increments the patch, and pushes a new `vMAJOR.MINOR.PATCH` tag — which the existing `release.yml` picks up and handles from there. No changes needed to `release.yml` or `build-binaries.yml`.

**Tech Stack:** GitHub Actions (bash), existing `release.yml` pipeline

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `VERSION` | Single integer — the major version (e.g. `1`) |
| Create | `.github/workflows/auto-tag.yml` | Triggers after `Test` workflow passes on `main`, computes next tag, pushes it |
| Modify | `README.md` | Document the VERSION file and auto-release process |

---

### Task 1: Add the VERSION file

**Files:**
- Create: `VERSION`

- [ ] **Step 1: Create the file**

```
1
```

One line, just the major version integer. No trailing newline required, but consistent format matters.

- [ ] **Step 2: Verify the file reads cleanly**

```bash
cat VERSION
# expected output: 1
major=$(cat VERSION | tr -d '[:space:]')
echo "Major: $major"
# expected output: Major: 1
```

- [ ] **Step 3: Commit**

```bash
git add VERSION
git commit -m "chore: add VERSION file for major version pinning"
```

---

### Task 2: Create the auto-tag workflow

**Files:**
- Create: `.github/workflows/auto-tag.yml`

This workflow:
1. Triggers via `workflow_run` — only runs when the `Test` workflow completes **successfully** on `main`
2. Reads the major version from `VERSION`
3. Lists all existing tags matching `vMAJOR.*.*`, sorts them by semver, picks the highest
4. Increments the patch component by 1 (or starts at `vMAJOR.0.0` if no tags exist yet for that major)
5. Creates and pushes the new tag — `release.yml` takes it from there

Using `workflow_run` instead of `push` means:
- If any test job fails, `auto-tag` never runs
- The `Test` workflow name in `test.yml` must match exactly — it is `Test`

- [ ] **Step 1: Write the workflow**

```yaml
name: Auto Tag

on:
  workflow_run:
    workflows: [Test]
    types: [completed]
    branches: [main]

concurrency:
  group: auto-tag
  cancel-in-progress: false

permissions:
  contents: write

jobs:
  tag:
    if: ${{ github.event.workflow_run.conclusion == 'success' }}
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11  # v4.1.1
        with:
          fetch-depth: 0

      - name: Compute and push next tag
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          MAJOR=$(cat VERSION | tr -d '[:space:]')

          # Find latest tag for this major, sorted by semver
          LATEST=$(git tag -l "v${MAJOR}.*.*" | sort -V | tail -1)

          if [ -z "$LATEST" ]; then
            NEXT="v${MAJOR}.0.0"
          else
            # Parse minor and patch from vMAJOR.MINOR.PATCH
            MINOR=$(echo "$LATEST" | cut -d. -f2)
            PATCH=$(echo "$LATEST" | cut -d. -f3)
            NEXT="v${MAJOR}.${MINOR}.$((PATCH + 1))"
          fi

          echo "Latest tag: ${LATEST:-none}"
          echo "Next tag:   $NEXT"

          git tag "$NEXT"
          git push origin "$NEXT"
```

- [ ] **Step 2: Verify YAML is valid**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/auto-tag.yml'))" && echo "Valid YAML"
# expected output: Valid YAML
```

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/auto-tag.yml
git commit -m "ci: auto-tag after tests pass on main using VERSION file"
```

---

### Task 3: Update README.md

**Files:**
- Modify: `README.md`

Add a "Releasing" section (or update the existing Development section) to explain:
- The `VERSION` file controls the major version
- Every push to `main` automatically tags and releases
- How to bump the major version

- [ ] **Step 1: Add the release documentation**

Find the existing `### Releasing a new version` subsection in `README.md` and replace it with:

```markdown
### Releasing a new version

Releases are fully automated. Every push to `main` that passes all tests triggers the auto-tag workflow, which reads the major version from the `VERSION` file, computes the next `vMAJOR.MINOR.PATCH` tag (incrementing patch), and pushes it. That tag then triggers the existing [Release workflow](.github/workflows/release.yml), which builds binaries, uploads release assets, updates the floating `vMAJOR` tag, and publishes the GitHub Release. If any test fails, no tag is created and no release is published.

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
```

- [ ] **Step 2: Verify the README renders correctly**

```bash
# Quick sanity check — look for the new section
grep -n "VERSION" README.md
# expected: at least 2 matches referencing the VERSION file
```

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document auto-release and VERSION file"
```

---

### Task 4: Smoke test on a branch

Before merging, verify the tagging logic works locally with a dry-run simulation.

- [ ] **Step 1: Simulate the tag computation locally**

```bash
MAJOR=$(cat VERSION | tr -d '[:space:]')
LATEST=$(git tag -l "v${MAJOR}.*.*" | sort -V | tail -1)

if [ -z "$LATEST" ]; then
  NEXT="v${MAJOR}.0.0"
else
  MINOR=$(echo "$LATEST" | cut -d. -f2)
  PATCH=$(echo "$LATEST" | cut -d. -f3)
  NEXT="v${MAJOR}.${MINOR}.$((PATCH + 1))"
fi

echo "Latest: ${LATEST:-none}"
echo "Would create: $NEXT"
```

Expected output (assuming tags like `v1.0.0` exist):
```
Latest: v1.0.0
Would create: v1.0.1
```

Or if no tags exist yet for this major:
```
Latest: none
Would create: v1.0.0
```

- [ ] **Step 2: Confirm output looks correct, then push to main**

```bash
git push origin main
```

Watch the Actions tab — `Test` workflow runs first; once it passes, `Auto Tag` is triggered, creates a tag, which then triggers `Release`. If `Test` fails, `Auto Tag` is skipped entirely.
