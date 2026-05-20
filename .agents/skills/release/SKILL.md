---
name: release
description: Prepare ngrok-operator release changes locally. Use when the user wants to cut a release, prepare a release candidate, update release versions, draft changelog entries, or rehearse the repo's release workflow without publishing artifacts.
license: Apache-2.0
disable-model-invocation: true
---

# Release Skill

This skill prepares local release changes for the ngrok Kubernetes Operator.

It covers three release artifacts:

| Component | Version source | Changelog | Tag pattern |
| --- | --- | --- | --- |
| Container | `VERSION` | `CHANGELOG.md` | `ngrok-operator-X.Y.Z` |
| Helm operator chart | `helm/ngrok-operator/Chart.yaml` `.version` | `helm/ngrok-operator/CHANGELOG.md` | `helm-chart-ngrok-operator-X.Y.Z` |
| CRDs chart | `helm/ngrok-crds/Chart.yaml` `.version` | `helm/ngrok-crds/CHANGELOG.md` | `helm-chart-ngrok-crds-X.Y.Z` |

Use the bundled scripts for deterministic steps. Keep the agent focused on judgment-heavy work: version suggestions, changelog wording, and deciding whether meta-only PRs should be excluded.

## Defaults

- Prefer a disposable dry-run worktree unless the user explicitly wants to use the current checkout:

  ```bash
  bash .agents/skills/release/scripts/create-dry-run-worktree.sh
  ```

- Do not commit, push, tag, or open a PR unless the user explicitly asks.
- If the current checkout is dirty and the user did not ask to work in place, stop and use the dry-run worktree.

## Workflow

### 1. Preflight

Run these checks in the target checkout:

```bash
git status --porcelain
gh auth status
cat VERSION
yq '.version' helm/ngrok-operator/Chart.yaml
yq '.version' helm/ngrok-crds/Chart.yaml
```

If the tree is dirty, stop unless the user explicitly asked to work in that checkout.

### 2. Gather release data

Run the data-gathering script and read the JSON:

```bash
bash .agents/skills/release/scripts/gather-release-data.sh > /tmp/release-data.json
```

Optional tag overrides:

```bash
bash .agents/skills/release/scripts/gather-release-data.sh \
  --container-tag ngrok-operator-0.20.0 \
  --helm-tag helm-chart-ngrok-operator-0.22.1 \
  --crds-tag helm-chart-ngrok-crds-0.2.1 \
  > /tmp/release-data.json
```

Summarize:

- PR counts per component
- breaking changes detected
- PR titles grouped by component
- meta-only PRs that you recommend excluding from changelogs

### 3. Suggest versions

This repo uses semver in the `0.y.z` range.

- Container:
  - breaking change or `feat:` PRs -> bump minor
  - fixes, chores, docs, CI only -> bump patch
- Helm operator chart:
  - always bump if releasing the operator chart
  - breaking Helm changes -> bump minor
  - otherwise bump patch
- CRDs chart:
  - only bump if PRs are `new_for` `helm_crds`
  - breaking CRD changes -> bump minor
  - otherwise bump patch

Ask the user to confirm or override the versions before editing files.

### 4. Handle release candidates

If the user wants an RC, use `-rc.N` with a dot before `N`.

Find the next RC number from existing tags for the relevant base version:

```bash
git tag -l 'ngrok-operator-<base-version>-rc.*' | sort -V | tail -1
git tag -l 'helm-chart-ngrok-operator-<base-version>-rc.*' | sort -V | tail -1
git tag -l 'helm-chart-ngrok-crds-<base-version>-rc.*' | sort -V | tail -1
```

Normalize user input like `RC1` or `rc1` to `-rc.1`.

### 5. Apply release edits

Only create a release branch if the user explicitly wants one:

```bash
git fetch origin main
git checkout -b release-ngrok-operator-<app-version>-helm-chart-<chart-version> origin/main
```

Update versions:

```bash
echo "<new-app-version>" > VERSION
yq -Y -i ".version = \"<new-chart-version>\"" helm/ngrok-operator/Chart.yaml
yq -Y -i ".appVersion = \"<new-app-version>\"" helm/ngrok-operator/Chart.yaml
```

If CRDs changed:

```bash
yq -Y -i ".version = \"<new-crds-version>\"" helm/ngrok-crds/Chart.yaml
yq -Y -i ".appVersion = \"<new-crds-version>\"" helm/ngrok-crds/Chart.yaml
yq -Y -i "(.dependencies[] | select(.name == \"ngrok-crds\") | .version) = \"<new-crds-version>\"" helm/ngrok-operator/Chart.yaml
```

### 6. Update changelogs

Match the existing changelog format already in the repo. Read the current files before writing.

- `CHANGELOG.md` uses container tags in the compare link.
- `helm/ngrok-operator/CHANGELOG.md` uses `helm-chart-ngrok-operator-*` tags and should mention chart/image version updates at the top of the new section.
- `helm/ngrok-crds/CHANGELOG.md` uses `helm-chart-ngrok-crds-*` tags and should mention the chart version update at the top.

Categorize entries using the PR title:

- `feat:` -> `Added`
- `fix:` -> `Fixed`
- `chore:`, `refactor:`, `ci:`, `docs:`, `test:`, `perf:` -> `Changed`
- explicit removals or deprecations -> `Removed`

Do not leave empty sections.

### 7. Validate generated files

After version and changelog updates, run:

```bash
make helm-update-snapshots helm-test
make manifest-bundle
```

If validation fails, fix the generated drift before finishing.

### 8. Final review

Show:

```bash
git status --short
git diff -- CHANGELOG.md helm/ngrok-operator/CHANGELOG.md helm/ngrok-crds/CHANGELOG.md VERSION helm/ngrok-operator/Chart.yaml helm/ngrok-crds/Chart.yaml manifest-bundle.yaml
```

Finish with:

- versions chosen
- excluded meta-only PRs, if any
- validation run
- remaining manual steps

Do not commit, push, tag, or open a PR unless the user explicitly asks.
