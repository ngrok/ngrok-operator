#!/usr/bin/env bash
# Canned kind demo for private endpoints. See specs/private-endpoints/demo.md.
# Usage: demo.sh preflight|apply|wait|status|shell|down
#
# Env:
#   NGROK_ACCESS_TOKEN  checked against the ngrok API by preflight
#   PE_DEMO_PREFIX      hostname prefix (default: namespace annotation, else pe-<user>)
#   PE_DEMO_NS          namespace for origins + client pod (default pe-demo)
#   KUBE_NAMESPACE      operator namespace (default ngrok-operator)
#   PE_DEMO_TIMEOUT     seconds to wait for Ready (default 180)
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEMO_NS="${PE_DEMO_NS:-pe-demo}"
OP_NS="${KUBE_NAMESPACE:-ngrok-operator}"
TIMEOUT="${PE_DEMO_TIMEOUT:-180}"
CLIENT="pe-demo-client"

# Runs before a cluster exists, so it must not touch kubectl. A revoked or
# wrong token otherwise shows up as a crashlooping manager and a hung wait.
cmd_preflight() {
  [[ -n "${NGROK_ACCESS_TOKEN:-}" ]] || { echo "NGROK_ACCESS_TOKEN is not set" >&2; exit 1; }
  echo "==> checking NGROK_ACCESS_TOKEN against the ngrok API"
  local out code body
  if ! out="$(curl -sS -m 15 -w '\n%{http_code}' \
    -H "Authorization: Bearer $NGROK_ACCESS_TOKEN" -H "Ngrok-Version: 2" \
    "https://api.ngrok.com/endpoints?limit=1")"; then
    echo "could not reach api.ngrok.com (see curl error above)" >&2
    exit 1
  fi
  code="${out##*$'\n'}"
  body="${out%$'\n'*}"
  case "$code" in
    200) echo "token OK" ;;
    401 | 403)
      echo "NGROK_ACCESS_TOKEN rejected by the ngrok API (HTTP $code, $(grep -o 'ERR_NGROK_[0-9]*' <<<"$body" | head -1 || true))." >&2
      echo "It must be a valid API key / PAT for the account. ngrok said: $(tr '\n' ' ' <<<"$body" | cut -c1-300)" >&2
      exit 1
      ;;
    *) echo "warning: unexpected HTTP $code from the ngrok API, continuing: $(tr '\n' ' ' <<<"$body" | cut -c1-300)" >&2 ;;
  esac
}

if [[ "${1:-}" == preflight ]]; then
  cmd_preflight
  exit 0
fi

# Prefix must be a DNS label starting with a letter and stay short enough for
# the .ngrok.direct label limit. Reuse the one the namespace was created with so
# status/down work from a different shell.
prefix() {
  if [[ -n "${PE_DEMO_PREFIX:-}" ]]; then
    echo "$PE_DEMO_PREFIX"
    return
  fi
  local p
  p="$(kubectl get ns "$DEMO_NS" -o jsonpath="{.metadata.annotations.k8s\.ngrok\.com/pe-demo-prefix}" 2>/dev/null || true)"
  if [[ -n "$p" ]]; then
    echo "$p"
    return
  fi
  p="$(printf '%s' "${USER:-}" | tr '[:upper:]' '[:lower:]' | tr -cd 'a-z0-9' | cut -c1-12)"
  [[ -n "$p" ]] || p="$(od -An -N3 -tx1 /dev/urandom | tr -d ' \n')"
  echo "pe-$p"
}

P="$(prefix)"
[[ "$P" =~ ^[a-z][a-z0-9-]{0,30}$ ]] || { echo "invalid PE_DEMO_PREFIX '$P' (lowercase DNS label, starts with a letter, <=31 chars)" >&2; exit 1; }

# Endpoints the poller must mirror.
urls() {
  echo "http://$P-web.ngrok.direct"
  echo "https://$P-web.ngrok.direct"
  echo "tls://$P-tls.ngrok.direct:443"
  echo "tcp://$P-redis.ngrok.direct:6379"
}

render() { sed -e "s/__PREFIX__/$P/g" -e "s/__NS__/$DEMO_NS/g" "$DIR/demo.yaml"; }

ensure_client() {
  if ! kubectl -n "$DEMO_NS" get pod "$CLIENT" >/dev/null 2>&1; then
    kubectl -n "$DEMO_NS" run "$CLIENT" --image=nicolaka/netshoot:latest --restart=Never -- sleep infinity >/dev/null
  fi
  kubectl -n "$DEMO_NS" wait --for=condition=Ready "pod/$CLIENT" --timeout="${TIMEOUT}s" >/dev/null
}

pe_table() {
  kubectl -n "$OP_NS" get privateendpoints -o custom-columns='URL:.spec.url,CLUSTER-IP:.status.clusterIP,FWD-PORT:.status.forwarderPort,READY:.status.conditions[?(@.type=="Ready")].status,REASON:.status.conditions[?(@.type=="Ready")].reason'
}

cmd_apply() {
  echo "==> applying demo origins + AgentEndpoints (prefix $P, namespace $DEMO_NS)"
  render | kubectl apply -f -
  # Dropped with .internal support; remove leftovers from older runs.
  kubectl -n "$DEMO_NS" delete agentendpoint redis-internal tls-internal web-internal --ignore-not-found
}

# AgentEndpoint names whose Ready condition is not True.
aeps_not_ready() {
  kubectl -n "$DEMO_NS" get agentendpoints -o jsonpath='{range .items[*]}{.metadata.name} {.status.conditions[?(@.type=="Ready")].status}{"\n"}{end}' 2>/dev/null |
    awk '$2 != "True" {printf " %s", $1}'
}

# Expected PrivateEndpoint URLs that are missing or not Ready.
pes_not_ready() {
  local table u
  table="$(pe_table 2>/dev/null || true)"
  for u in $(urls); do
    # The API may report http(s) URLs with or without the default port.
    awk -v u="$u" '($1 == u || $1 ":80" == u || $1 ":443" == u || u ":80" == $1 || u ":443" == $1) && $4 == "True" {f=1} END {exit !f}' <<<"$table" || printf ' %s' "$u"
  done
}

logs_tail() {
  local name="$1" selector="$2"
  echo
  echo "== $name logs (last 30 lines)"
  kubectl -n "$OP_NS" logs -l "$selector" --all-containers --tail=30 2>&1 || true
  echo "== $name logs, previous container (if it restarted)"
  kubectl -n "$OP_NS" logs -l "$selector" --all-containers --tail=30 --previous 2>/dev/null || echo "(none)"
}

diagnose() {
  echo
  echo "======== diagnostics ========"
  echo "== AgentEndpoints ($DEMO_NS)"
  kubectl -n "$DEMO_NS" get agentendpoints -o jsonpath='{range .items[*]}{.metadata.name}  {.spec.url}{"\n"}{range .status.conditions[*]}    {.type}={.status} {.reason}: {.message}{"\n"}{end}{end}' 2>&1 || true
  echo
  echo "== PrivateEndpoints ($OP_NS)"
  pe_table 2>&1 || true
  kubectl -n "$OP_NS" get privateendpoints -o jsonpath='{range .items[*]}{.spec.url}{"\n"}{range .status.conditions[*]}    {.type}={.status} {.reason}: {.message}{"\n"}{end}{end}' 2>&1 || true
  echo
  echo "== per-host Services ($OP_NS)"
  kubectl -n "$OP_NS" get svc -l ngrok.com/managed-by=private-endpoint-poller 2>&1 || true
  echo
  echo "== operator pods ($OP_NS)"
  kubectl -n "$OP_NS" get pods -o wide 2>&1 || true
  echo
  echo "== demo pods ($DEMO_NS)"
  kubectl -n "$DEMO_NS" get pods -o wide 2>&1 || true
  logs_tail api-manager app.kubernetes.io/component=controller
  echo "== api-manager poller/error lines (last 15 of the last 1000 lines)"
  kubectl -n "$OP_NS" logs -l app.kubernetes.io/component=controller --tail=1000 2>&1 | grep -iE 'privateendpoint|error' | tail -15 || true
  logs_tail forwarder app.kubernetes.io/component=private-endpoint-forwarder
  if [[ -n "$(aeps_not_ready)" ]]; then
    logs_tail agent app.kubernetes.io/component=agent
  fi
  echo "======== end diagnostics ========"
  echo "See 'Troubleshooting' in specs/private-endpoints/demo.md"
}

cmd_wait() {
  local deadline=$((SECONDS + TIMEOUT)) aeps pes last=0
  echo "==> waiting up to ${TIMEOUT}s for origins, AgentEndpoints, and PrivateEndpoints (PE_DEMO_TIMEOUT)"
  if ! kubectl -n "$DEMO_NS" rollout status deploy/web deploy/redis deploy/tls --timeout="${TIMEOUT}s"; then
    echo "timed out waiting for demo origins" >&2
    diagnose >&2
    exit 1
  fi
  while :; do
    aeps="$(aeps_not_ready)"
    pes="$(pes_not_ready)"
    [[ -z "$aeps" && -z "$pes" ]] && break
    if ((SECONDS > deadline)); then
      echo "timed out after ${TIMEOUT}s" >&2
      [[ -z "$aeps" ]] || echo "  AgentEndpoints not Ready:$aeps" >&2
      [[ -z "$pes" ]] || echo "  PrivateEndpoints missing/not Ready:$pes" >&2
      diagnose >&2
      exit 1
    fi
    if ((SECONDS - last >= 15)); then
      echo "  [$((deadline - SECONDS))s left] AgentEndpoints not Ready:${aeps:- none}; PrivateEndpoints missing/not Ready (poll ~10s):${pes:- none}"
      last=$SECONDS
    fi
    sleep 5
  done
  ensure_client
  echo "==> ready. Try: make pe-demo-shell   or   make pe-demo-test"
}

cmd_status() {
  echo "== prefix $P (other private endpoints in the account show up too)"
  echo "== PrivateEndpoints ($OP_NS)"
  pe_table
  echo
  echo "== per-host Services ($OP_NS)"
  kubectl -n "$OP_NS" get svc -l ngrok.com/managed-by=private-endpoint-poller
  echo
  echo "== forwarder pods ($OP_NS)"
  kubectl -n "$OP_NS" get pods -l app.kubernetes.io/component=private-endpoint-forwarder -o wide
  echo
  echo "== demo AgentEndpoints + pods ($DEMO_NS)"
  kubectl -n "$DEMO_NS" get agentendpoints,pods 2>/dev/null || echo "(no $DEMO_NS namespace; run make pe-demo-up)"
}

cmd_shell() {
  ensure_client
  cat <<EOF
Demo endpoints (prefix $P):
  curl http://$P-web.ngrok.direct
  curl -v https://$P-web.ngrok.direct        # public cert, verification on
  echo | openssl s_client -connect $P-tls.ngrok.direct:443 -servername $P-tls.ngrok.direct 2>/dev/null | grep subject=
  printf 'PING\\r\\n' | nc -w 3 $P-redis.ngrok.direct 6379
  dig +noall +answer $P-web.ngrok.direct
EOF
  kubectl -n "$DEMO_NS" exec -it "$CLIENT" -- bash
}

cmd_down() {
  echo "==> deleting $DEMO_NS (AgentEndpoints are released by the agent-manager first)"
  kubectl -n "$DEMO_NS" delete agentendpoints --all --ignore-not-found --wait --timeout="${TIMEOUT}s" || true
  kubectl delete ns "$DEMO_NS" --ignore-not-found --wait --timeout="${TIMEOUT}s"
}

case "${1:-}" in
  apply) cmd_apply ;;
  wait) cmd_wait ;;
  status) cmd_status ;;
  shell) cmd_shell ;;
  down) cmd_down ;;
  *) echo "usage: $0 preflight|apply|wait|status|shell|down" >&2; exit 2 ;;
esac
