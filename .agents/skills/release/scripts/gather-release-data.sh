#!/usr/bin/env bash
#
# gather-release-data.sh — Gathers PR data for release changelog generation.
#
# Uses gh CLI to fetch rich PR metadata (author, files, body) and classifies
# each PR into components (container, helm_operator, helm_crds) based on the
# files it changed. Outputs structured JSON to stdout.
#
# Progress messages go to stderr so stdout stays clean for piping/redirecting.

set -eou pipefail

CONTAINER_TAG_OVERRIDE=""
HELM_TAG_OVERRIDE=""
CRDS_TAG_OVERRIDE=""

while [[ $# -gt 0 ]]; do
  case $1 in
    --container-tag) CONTAINER_TAG_OVERRIDE="$2"; shift 2 ;;
    --helm-tag)      HELM_TAG_OVERRIDE="$2";      shift 2 ;;
    --crds-tag)      CRDS_TAG_OVERRIDE="$2";      shift 2 ;;
    -h|--help)
      cat >&2 <<EOF
Usage: gather-release-data.sh [OPTIONS]

Gathers merged PR data since the last release tags. Outputs JSON to stdout.

Options:
  --container-tag TAG   Override auto-detected container tag (ngrok-operator-*)
  --helm-tag TAG        Override auto-detected helm chart tag (helm-chart-ngrok-operator-*)
  --crds-tag TAG        Override auto-detected CRDs chart tag (helm-chart-ngrok-crds-*)
  -h, --help            Show this help
EOF
      exit 0 ;;
    *) echo "Unknown arg: $1" >&2; exit 1 ;;
  esac
done

log() { echo ">>> $*" >&2; }

die() { echo "{\"error\": \"$*\"}"; exit 1; }

find_latest_tag() {
  local pattern=$1
  git tag -l "$pattern" --sort=-version:refname | grep -vE 'alpha|beta|rc' | head -1
}

extract_pr_numbers() {
  local tag=$1
  git log --oneline "${tag}..origin/main" 2>/dev/null \
    | grep -oE '#[0-9]+' \
    | tr -d '#' \
    | sort -un
}

classify_file() {
  local file=$1
  case "$file" in
    cmd/*|internal/*|pkg/*|api/*|main.go|go.mod|go.sum)
      echo "container" ;;
    Dockerfile|scripts/build.sh|VERSION)
      echo "container" ;;
    helm/ngrok-operator/*)
      echo "helm_operator" ;;
    helm/ngrok-crds/templates/*)
      echo "helm_crds" ;;
    helm/ngrok-crds/*)
      echo "helm_crds" ;;
    .github/*|.devcontainer/*|.claude/*|.agents/*|CLAUDE.md|CLAUDE.local.md)
      echo "meta" ;;
    docs/*|specs/*|CONTRIBUTING.md|SECURITY.md|README.md|LICENSE)
      echo "meta" ;;
    flake.nix|flake.lock|tools/*|Makefile|.golangci*|.editorconfig)
      echo "meta" ;;
    tests/*)
      echo "meta" ;;
    deploy/*|manifest-bundle.yaml)
      echo "meta" ;;
    *)
      echo "container" ;;
  esac
}

command -v gh  &>/dev/null || die "gh CLI not found. Install it or enter the nix devshell."
command -v jq  &>/dev/null || die "jq not found. Install it or enter the nix devshell."
gh auth status &>/dev/null 2>&1 || die "gh not authenticated. Run: gh auth login"

REPO=$(gh repo view --json nameWithOwner -q '.nameWithOwner' 2>/dev/null || echo "ngrok/ngrok-operator")

log "Fetching latest from origin..."
git fetch origin --quiet

CONTAINER_TAG="${CONTAINER_TAG_OVERRIDE:-$(find_latest_tag 'ngrok-operator-*')}"
HELM_TAG="${HELM_TAG_OVERRIDE:-$(find_latest_tag 'helm-chart-ngrok-operator-*')}"
CRDS_TAG="${CRDS_TAG_OVERRIDE:-$(find_latest_tag 'helm-chart-ngrok-crds-*')}"

[[ -n "$CONTAINER_TAG" ]] || die "No container tag found matching ngrok-operator-*"
[[ -n "$HELM_TAG" ]] || die "No helm chart tag found matching helm-chart-ngrok-operator-*"
[[ -n "$CRDS_TAG" ]] || die "No CRDs chart tag found matching helm-chart-ngrok-crds-*"

container_version="${CONTAINER_TAG#ngrok-operator-}"
helm_version="${HELM_TAG#helm-chart-ngrok-operator-}"
crds_version="${CRDS_TAG#helm-chart-ngrok-crds-}"

log "Tags detected:"
log "  Container:     $CONTAINER_TAG  (v$container_version)"
log "  Helm operator: $HELM_TAG  (v$helm_version)"
log "  CRDs chart:    $CRDS_TAG  (v$crds_version)"

container_pr_nums=$(extract_pr_numbers "$CONTAINER_TAG")
helm_pr_nums=$(extract_pr_numbers "$HELM_TAG")
crds_pr_nums=$(extract_pr_numbers "$CRDS_TAG")

all_pr_nums=$(printf '%s\n' $container_pr_nums $helm_pr_nums $crds_pr_nums | grep -v '^$' | sort -un)

if [[ -z "$all_pr_nums" ]]; then
  log "No PRs found since last release tags."
  jq -n \
    --arg repo "$REPO" \
    --arg gathered_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --arg ct "$CONTAINER_TAG" --arg cv "$container_version" \
    --arg ht "$HELM_TAG" --arg hv "$helm_version" \
    --arg dt "$CRDS_TAG" --arg dv "$crds_version" \
    '{
      repo: $repo,
      gathered_at: $gathered_at,
      tags: {
        container: { tag: $ct, version: $cv },
        helm_operator: { tag: $ht, version: $hv },
        helm_crds: { tag: $dt, version: $dv }
      },
      summary: {
        total_prs: 0,
        container_prs: 0,
        helm_operator_prs: 0,
        helm_crds_prs: 0,
        has_breaking_changes: false
      },
      prs: []
    }'
  exit 0
fi

pr_count=$(echo "$all_pr_nums" | wc -l | tr -d ' ')
log "Found $pr_count unique PR references. Fetching data from GitHub..."

tmp_prs=$(mktemp)
trap 'rm -f "$tmp_prs"' EXIT

echo "[" > "$tmp_prs"
first=true

for pr_num in $all_pr_nums; do
  log "  PR #$pr_num ..."

  pr_data=$(gh pr view "$pr_num" --repo "$REPO" \
    --json number,title,author,body,files,labels,mergedAt 2>/dev/null || true)
  [[ -n "$pr_data" ]] || continue

  merged_at=$(echo "$pr_data" | jq -r '.mergedAt // empty')
  [[ -n "$merged_at" && "$merged_at" != "null" && "$merged_at" != "0001-01-01T00:00:00Z" ]] || continue

  has_container=false
  has_helm=false
  has_crds=false
  has_meta=false

  while IFS= read -r file; do
    [[ -n "$file" ]] || continue
    component=$(classify_file "$file")
    case "$component" in
      container) has_container=true ;;
      helm_operator) has_helm=true ;;
      helm_crds) has_crds=true ;;
      meta) has_meta=true ;;
    esac
  done < <(echo "$pr_data" | jq -r '.files[].path')

  meta_only=false
  if [[ "$has_meta" == true && "$has_container" == false && "$has_helm" == false && "$has_crds" == false ]]; then
    meta_only=true
  fi

  components=$(jq -n \
    --argjson c "$has_container" \
    --argjson h "$has_helm" \
    --argjson d "$has_crds" \
    --argjson m "$has_meta" \
    '[if $c then "container" else empty end,
      if $h then "helm_operator" else empty end,
      if $d then "helm_crds" else empty end,
      if $m then "meta" else empty end]')

  new_for="[]"
  if [[ "$has_container" == true ]] && echo "$container_pr_nums" | grep -qw "$pr_num"; then
    new_for=$(echo "$new_for" | jq '. + ["container"]')
  fi
  if [[ "$has_helm" == true ]] && echo "$helm_pr_nums" | grep -qw "$pr_num"; then
    new_for=$(echo "$new_for" | jq '. + ["helm_operator"]')
  fi
  if [[ "$has_crds" == true ]] && echo "$crds_pr_nums" | grep -qw "$pr_num"; then
    new_for=$(echo "$new_for" | jq '. + ["helm_crds"]')
  fi

  body=$(echo "$pr_data" | jq -r '.body // ""')
  has_breaking=false
  if echo "$body" | grep -qiE 'breaking.change'; then
    breaking_section=$(echo "$body" | sed -n '/[Bb]reaking [Cc]hange/,/^##/p' | tail -n +2 | head -10)
    cleaned=$(echo "$breaking_section" \
      | grep -viE '^\s*$' \
      | grep -viE '^\s*\*?are there any breaking' \
      | grep -viE '^\s*\*?(none|no|n/?a)[.,;:!*\s]' \
      | grep -viE '^\s*\*?(none|no|n/?a)\s*\*?\s*$' \
      | grep -viE 'there shouldn.t be' \
      || true)
    if [[ -n "$cleaned" ]]; then
      has_breaking=true
    fi
  fi

  pr_json=$(echo "$pr_data" | jq \
    --argjson components "$components" \
    --argjson new_for "$new_for" \
    --argjson breaking "$has_breaking" \
    --argjson meta_only "$meta_only" \
    '{
      number: .number,
      title: .title,
      author: .author.login,
      body: .body,
      files: [.files[].path],
      labels: [.labels[].name],
      merged_at: .mergedAt,
      components: $components,
      new_for: $new_for,
      has_breaking_changes: $breaking,
      meta_only: $meta_only
    }')

  if [[ "$first" == true ]]; then
    first=false
  else
    echo "," >> "$tmp_prs"
  fi
  echo "$pr_json" >> "$tmp_prs"
done

echo "]" >> "$tmp_prs"

jq -n \
  --arg repo "$REPO" \
  --arg gathered_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg ct "$CONTAINER_TAG" --arg cv "$container_version" \
  --arg ht "$HELM_TAG" --arg hv "$helm_version" \
  --arg dt "$CRDS_TAG" --arg dv "$crds_version" \
  --slurpfile prs "$tmp_prs" \
  '{
    repo: $repo,
    gathered_at: $gathered_at,
    tags: {
      container: { tag: $ct, version: $cv },
      helm_operator: { tag: $ht, version: $hv },
      helm_crds: { tag: $dt, version: $dv }
    },
    summary: {
      total_prs: ($prs[0] | length),
      container_prs: ($prs[0] | [.[] | select(.new_for | index("container"))] | length),
      helm_operator_prs: ($prs[0] | [.[] | select(.new_for | index("helm_operator"))] | length),
      helm_crds_prs: ($prs[0] | [.[] | select(.new_for | index("helm_crds"))] | length),
      meta_only_prs: ($prs[0] | [.[] | select(.meta_only)] | length),
      has_breaking_changes: ($prs[0] | any(.has_breaking_changes))
    },
    prs: $prs[0]
  }'
