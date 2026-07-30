#!/usr/bin/env bash
# Build + push a linux/amd64 operator image and Helm chart to ttl.sh, then print
# a helm upgrade --reuse-values command that points at the new artifacts.
#
# Usage:
#   ./scripts/ttl-release.sh
#
# Optional env overrides:
#   HELM_RELEASE   Release name (default: ngrok-operator-compute-local)
#   KUBE_NAMESPACE Namespace (default: ngrok-compute-local)
#   IMAGE_TTL      ttl.sh image tag / expiry (default: 24h)
#   NAME_PREFIX    Artifact name prefix (default: ngrok-operator-laverya)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

HELM_RELEASE="${HELM_RELEASE:-ngrok-operator-compute-local}"
KUBE_NAMESPACE="${KUBE_NAMESPACE:-ngrok-compute-local}"
IMAGE_TTL="${IMAGE_TTL:-24h}"
NAME_PREFIX="${NAME_PREFIX:-ngrok-operator-laverya}"

GITSHA="$(git rev-parse --short HEAD)"
GIT_COMMIT="$(git rev-parse HEAD)"
DATETIME="$(date -u +%Y%m%d%H%M%S)"
CHART_BASE_VERSION="$(awk '/^version:/{print $2; exit}' helm/ngrok-operator/Chart.yaml)"
VERSION="${CHART_BASE_VERSION}-${DATETIME}"

IMG="ttl.sh/${NAME_PREFIX}-${GITSHA}:${IMAGE_TTL}"
CHART_OCI="oci://ttl.sh/${NAME_PREFIX}-${GITSHA}"
CHART_REF="${CHART_OCI}/ngrok-operator"

if ! command -v podman >/dev/null 2>&1; then
  echo "error: podman is required" >&2
  exit 1
fi
if ! command -v helm >/dev/null 2>&1; then
  echo "error: helm is required" >&2
  exit 1
fi

echo "=== Building image ${IMG} ==="
podman build \
  --platform linux/amd64 \
  --build-arg GIT_COMMIT="${GIT_COMMIT}" \
  -t "${IMG}" .

echo "=== Pushing image ==="
podman push "${IMG}"

echo "=== Packaging chart ${VERSION} ==="
make _helm_setup
OUT="$(mktemp -d)"
trap 'rm -rf "${OUT}"' EXIT
helm package ./helm/ngrok-operator --version "${VERSION}" -d "${OUT}"
helm push "${OUT}"/ngrok-operator-*.tgz "${CHART_OCI}"

echo
echo "Released:"
echo "  Image: ${IMG}"
echo "  Chart: ${CHART_REF}:${VERSION}"
echo
echo "Upgrade command:"
cat <<EOF
helm upgrade ${HELM_RELEASE} \\
  ${CHART_REF} \\
  --version ${VERSION} \\
  --namespace ${KUBE_NAMESPACE} \\
  --reuse-values \\
  --set image.registry=ttl.sh \\
  --set image.repository=${NAME_PREFIX}-${GITSHA} \\
  --set image.tag=${IMAGE_TTL} \\
  --set image.pullPolicy=Always \\
  --set-string podAnnotations.redeployTimestamp="\$(date +%s)"
EOF
