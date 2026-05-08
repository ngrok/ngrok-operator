---
name: release
description: >
  Automates the ngrok-operator release process: gathers PR data, classifies changes
  by component (container, Helm chart, CRDs chart), generates changelogs, updates
  version files, and prepares the release branch. Use this skill whenever the user
  mentions releasing, cutting a release, bumping versions, updating changelogs,
  or preparing a release PR for the ngrok-operator. Also use when the user says
  "release", "cut a release", "prepare release", "update changelog", or "version bump".
---

# Release Skill — ngrok-operator

This skill orchestrates the full release preparation for the ngrok-operator. It replaces the old `scripts/release.sh` and `.github/agents/release-agent.agent.md`.

The key insight: **a shell script handles deterministic data gathering** (PR metadata, file classification, author attribution via `gh` CLI), and **you handle what AI is good at** — summarizing changes into readable changelogs and suggesting version bumps.

## The Three Components

| Component | Changelog | Version source | Tag pattern |
|-----------|-----------|---------------|-------------|
| Container (Go binary) | `CHANGELOG.md` | `VERSION` file | `ngrok-operator-X.Y.Z` |
| Helm operator chart | `helm/ngrok-operator/CHANGELOG.md` | `helm/ngrok-operator/Chart.yaml` `.version` | `helm-chart-ngrok-operator-X.Y.Z` |
| CRDs sub-chart | `helm/ngrok-crds/CHANGELOG.md` | `helm/ngrok-crds/Chart.yaml` `.version` | `helm-chart-ngrok-crds-X.Y.Z` |

A single PR often affects multiple components. The data-gathering script classifies by files changed — you don't need to figure this out yourself.

## Workflow

Follow these steps in order. Do NOT commit, push, or create a PR — stop after local changes.

### Step 1: Pre-flight checks

1. Verify git working tree is clean: `git status --porcelain`
2. Verify `gh` CLI is available and authenticated: `gh auth status`
3. Show current versions:
   - `cat VERSION`
   - `yq '.version' helm/ngrok-operator/Chart.yaml`
   - `yq '.version' helm/ngrok-crds/Chart.yaml`

If the tree isn't clean, stop and tell the user.

### Step 2: Gather release data

Run the data-gathering script. It lives relative to this skill file:

```bash
bash .claude/skills/release/scripts/gather-release-data.sh > /tmp/release-data.json
```

If the user wants to override the base tags (e.g., to capture a wider range):
```bash
bash .claude/skills/release/scripts/gather-release-data.sh \
  --container-tag ngrok-operator-0.19.0 \
  > /tmp/release-data.json
```

Read the output JSON. Present a summary to the user:
- Number of PRs per component
- Whether any breaking changes were detected
- List the PR titles grouped by component

### Step 3: Suggest version bumps

Analyze the PRs in the JSON to suggest next versions. Rules (this project uses semver, currently pre-1.0):

**Container version** (from `VERSION`):
- Breaking changes → bump minor (Y in 0.Y.Z)
- New features (`feat:` prefix in title) → bump minor
- Only fixes/chores/CI → bump patch (Z in 0.Y.Z)

**Helm operator chart version** (from `Chart.yaml .version`):
- Always bumped when releasing (it pins the container image version)
- Breaking Helm changes (removed values, renamed fields) → bump minor
- Otherwise → bump patch

**CRDs chart version** (from `helm/ngrok-crds/Chart.yaml .version`):
- Only bumped if PRs have `new_for` containing `"helm_crds"`
- Breaking CRD changes (removed fields, renamed CRDs) → bump minor
- Otherwise → bump patch
- If no CRD changes, skip entirely

Present your suggestions and ask the user to confirm or override. Wait for confirmation before proceeding.

**Release candidates (RC):** If the user asks for an RC, append `-rc.N` (with a dot before the number) to each version. The canonical format is always `-rc.1`, `-rc.2`, etc. — even if the user writes "RC1" or "rc1", normalize to `-rc.N`. To find the right N, check existing RC tags:
```bash
git tag -l 'ngrok-operator-<base-version>-rc.*' | sort -V | tail -1
```
If no RC exists yet, use `-rc.1`. Otherwise increment. Apply the RC suffix to all components being released.

When doing an RC:
- Branch name includes RC suffix: `release-ngrok-operator-0.21.0-rc.1-helm-chart-0.23.0-rc.1`
- VERSION file: `0.21.0-rc.1`
- Chart.yaml versions: `0.23.0-rc.1`
- Changelog entries: use the RC version as the section header (e.g., `## 0.21.0-rc.1`)

When the user later does the final (non-RC) release, the RC changelog entries get folded into the final version entry — don't duplicate them.

### Step 4: Create release branch

```bash
git fetch origin main
git checkout -b release-ngrok-operator-<app-ver>-helm-chart-<chart-ver> origin/main
```

### Step 5: Update version files

1. Write new container version:
   ```bash
   echo "<new-app-version>" > VERSION
   ```

2. Update Helm operator chart:
   ```bash
   yq -Y -i ".version = \"<new-chart-version>\"" helm/ngrok-operator/Chart.yaml
   yq -Y -i ".appVersion = \"<new-app-version>\"" helm/ngrok-operator/Chart.yaml
   ```

3. **If CRDs changed**, update CRDs chart:
   ```bash
   yq -Y -i ".version = \"<new-crds-version>\"" helm/ngrok-crds/Chart.yaml
   yq -Y -i ".appVersion = \"<new-crds-version>\"" helm/ngrok-crds/Chart.yaml
   ```

4. **If CRDs changed**, update the dependency version in the operator chart:
   ```bash
   yq -Y -i "(.dependencies[] | select(.name == \"ngrok-crds\") | .version) = \"<new-crds-version>\"" helm/ngrok-operator/Chart.yaml
   ```

### Step 6: Handle meta-only PRs

Some PRs only touch CI workflows, developer docs, AI agent configs, nix tooling, or other files that don't affect the released artifacts. The gather script marks these with `"meta_only": true`.

**Review each meta-only PR** and decide: include or exclude from the changelog.

Exclude when the PR clearly doesn't affect users:
- CI workflow tweaks (`.github/workflows/` only)
- AI agent/copilot setup (`.github/agents/`, `.github/copilot/`)
- Nix devshell changes (`flake.nix`, `flake.lock`)
- Internal docs or specs (`docs/developer-guide/`, `specs/`)
- Makefile/tooling changes that don't affect the build output

Include when the change could matter to users even indirectly:
- Go version bumps (affects the built binary)
- Security docs users might reference
- Generated manifest changes (`deploy/`, `manifest-bundle/`)
- Test infrastructure changes that indicate behavior changes

**Always err on the side of including** — it's better to document something trivial than to miss something that matters.

**Present excluded PRs to the user** as a separate list with reasons, so they can override your decision:
```
Excluded from changelogs (meta-only):
- #786: Temporarily disabled Trivy scanning — CI workflow only
- #795: Consolidated nix setup — dev tooling only
```

### Step 7: Generate changelogs

For each component where `summary.*_prs > 0` in the release data JSON, generate a changelog entry. Use ONLY the PRs where `new_for` includes that component. Skip PRs you excluded in Step 6.

#### Changelog format

Match the existing format exactly. Read the current changelogs to see recent examples.

**Container changelog** (`CHANGELOG.md`):
```markdown
## <version>
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-<prev>...ngrok-operator-<new>

### Added
- <description> by @<author> in [#<num>](https://github.com/ngrok/ngrok-operator/pull/<num>)

### Changed
- <description> by @<author> in [#<num>](https://github.com/ngrok/ngrok-operator/pull/<num>)

### Fixed
- <description> by @<author> in [#<num>](https://github.com/ngrok/ngrok-operator/pull/<num>)
```

**Helm operator changelog** (`helm/ngrok-operator/CHANGELOG.md`):
```markdown
## <chart-version>
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-ngrok-operator-<prev>...helm-chart-ngrok-operator-<new>

- Update ngrok-operator image version to `<app-version>`
- Update Helm chart version to `<chart-version>`
- Update [ngrok-crds](../ngrok-crds/CHANGELOG.md) dependency version to `<crds-version>` (only if CRDs updated)

### Added
...
```

**CRDs changelog** (`helm/ngrok-crds/CHANGELOG.md`):
```markdown
## <crds-version>
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-ngrok-crds-<prev>...helm-chart-ngrok-crds-<new>

- Update CRDs Helm chart version to `<crds-version>`

### Added
...
```

#### Categorization rules

Use the PR title's conventional commit prefix to categorize:
- `feat:` or `feat(...):`  → **Added**
- `fix:` or `fix(...):`    → **Fixed**
- `chore:`, `refactor:`, `ci:`, `docs:`, `test:`, `perf:` → **Changed**
- Explicit removals or deprecations → **Removed**
- If no prefix, read the title and use your best judgment

If a PR has `has_breaking_changes: true`, add a **Breaking Changes** subsection at the top (before Added).

#### Writing the entry descriptions

- Write concise, user-facing descriptions. Don't just copy the PR title verbatim — clean it up.
- Strip the conventional commit prefix (`fix(ngrok):` → just describe the fix).
- Keep the `by @author in [#num](url)` attribution format.
- Only include sections that have entries. Don't leave empty `### Added` headers.

#### Inserting into the file

Insert the new version section **after the header block** (the "# Changelog" line and the format description) and **before the first existing `## X.Y.Z` entry**. Do not overwrite or modify existing entries.

### Step 8: Update Helm snapshots

```bash
make helm-update-snapshots helm-test
```

If `helm-test` fails, investigate and fix. Common cause: snapshot drift from version changes.

### Step 9: Regenerate manifest bundle

```bash
make manifest-bundle
```

This regenerates `manifest-bundle.yaml` at the repo root with the updated Helm chart versions. CI will fail if this is stale.

### Step 10: Done

Show the user:
1. A summary of files changed (`git status`)
2. The diff of changelog entries (`git diff CHANGELOG.md helm/ngrok-operator/CHANGELOG.md helm/ngrok-crds/CHANGELOG.md`)
3. Remind them: "Review the changes. When satisfied, commit and push."

**Do NOT** commit, push, or create a PR. The user handles that.

## Important notes

- The `gather-release-data.sh` script writes progress to stderr and JSON to stdout. Always redirect stdout to a file.
- Tag comparison links use the tag prefix for that component (`ngrok-operator-` for container, `helm-chart-ngrok-operator-` for helm chart, `helm-chart-ngrok-crds-` for CRDs).
- The `appVersion` in `helm/ngrok-operator/Chart.yaml` must match the container version in `VERSION`.
- When the CRDs chart is bumped, you must also update its dependency version in `helm/ngrok-operator/Chart.yaml` under the `dependencies` section.
- Refer to `docs/developer-guide/releasing.md` for the full release documentation.
