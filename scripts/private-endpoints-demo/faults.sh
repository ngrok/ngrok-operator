#!/usr/bin/env bash
# Live fault scenarios on the kind demo cluster (FT-3, FT-5, FT-6 in
# specs/private-endpoints/README.md). See "Fault scenarios" in
# specs/private-endpoints/demo.md. Each scenario mutates the cluster, prints
# what it does and expects, then one PASS/FAIL/OBSERVE line per finding.
# FAIL means "short of the correctness bar", and several are expected to FAIL
# on today's code.
# Usage: faults.sh origin-restart|rolling-restart|uninstall [--yes]|revoke
#
# Env:
#   PE_DEMO_PREFIX         hostname prefix (default: pe-demo namespace annotation)
#   PE_DEMO_NS             demo namespace (default pe-demo)
#   KUBE_NAMESPACE         operator namespace (default ngrok-operator)
#   HELM_RELEASE_NAME      operator release (default ngrok-operator)
#   FAULT_DOWN_SECONDS     origin-restart: how long the origin stays down (default 25, > one 10s poll)
#   FAULT_ORIGIN_MODE      origin-restart: endpoint (delete + re-apply the redis AgentEndpoint)
#                          or agent (scale the agent-manager to 0 and back) (default endpoint)
#   FAULT_RECOVER_TIMEOUT  seconds to wait for the PrivateEndpoint to be Ready again (default 120)
#   FAULT_SETTLE_SECONDS   uninstall: wait after disabling (default 30, three polls)
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$DIR/../.." && pwd)"
DEMO_NS="${PE_DEMO_NS:-pe-demo}"
OP_NS="${KUBE_NAMESPACE:-ngrok-operator}"
RELEASE="${HELM_RELEASE_NAME:-ngrok-operator}"
CHART="$ROOT/helm/ngrok-operator"
DOWN="${FAULT_DOWN_SECONDS:-25}"
ORIGIN_MODE="${FAULT_ORIGIN_MODE:-endpoint}"
RECOVER="${FAULT_RECOVER_TIMEOUT:-120}"
SETTLE="${FAULT_SETTLE_SECONDS:-30}"
CLIENT="pe-demo-client"
FWD_DEPLOY="$RELEASE-private-endpoint-forwarder"
AGENT_DEPLOY="$RELEASE-agent"
fails=0

result() {
  echo "$1  $2"
  [[ "$1" != FAIL ]] || fails=$((fails + 1))
}

# These scenarios delete things; refuse anything that isn't a kind cluster.
require_kind() {
  local ctx
  ctx="$(kubectl config current-context)"
  [[ "$ctx" == kind-* ]] || { echo "current context '$ctx' is not a kind cluster; refusing to inject faults" >&2; exit 1; }
  echo "==> context $ctx"
}

prefix() {
  if [[ -n "${PE_DEMO_PREFIX:-}" ]]; then
    echo "$PE_DEMO_PREFIX"
    return
  fi
  kubectl get ns "$DEMO_NS" -o jsonpath="{.metadata.annotations.k8s\.ngrok\.com/pe-demo-prefix}" 2>/dev/null || true
}

require_demo() {
  require_kind
  P="$(prefix)"
  [[ -n "$P" ]] || { echo "no demo prefix found on namespace $DEMO_NS; run make pe-demo-up first" >&2; exit 1; }
  kubectl -n "$DEMO_NS" get pod "$CLIENT" >/dev/null 2>&1 || { echo "no $DEMO_NS/$CLIENT pod; run make pe-demo-up first" >&2; exit 1; }
  REDIS_HOST="$P-redis.ngrok.direct"
  REDIS_SVC="$P-redis-ngrok-direct"
  WEB_HOST="$P-web.ngrok.direct"
}

# "<name> <uid> <clusterIP> <forwarderPort> <Ready>" of the PrivateEndpoint for a hostname, or empty.
pe_line() {
  kubectl -n "$OP_NS" get privateendpoints -o jsonpath='{range .items[*]}{.spec.hostname}{" "}{.metadata.name}{" "}{.metadata.uid}{" "}{.status.clusterIP}{" "}{.status.forwarderPort}{" "}{.status.conditions[?(@.type=="Ready")].status}{"\n"}{end}' |
    awk -v h="$1" '$1 == h {print $2, $3, $4, $5, $6}'
}

# "<uid> <clusterIP>" of a per-host Service, or empty.
svc_line() {
  kubectl -n "$OP_NS" get svc "$1" --ignore-not-found -o jsonpath='{.metadata.uid} {.spec.clusterIP}'
}

# Client loops run inside the client pod, print "<epoch> <event>" lines, and
# exit when the stop file appears (or after a hard cap, so nothing lingers).
STOP_FILE="/tmp/pe-fault-stop-$$"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# Args: host port cap stopfile. Resolves the name once and keeps dialing that IP
# (a DNS-caching app), next to a probe that resolves fresh every time.
# shellcheck disable=SC2016
CACHED_LOOP='
H=$1 PORT=$2 CAP=$3 STOP=$4
IP=$(dig +short "$H" | head -1)
echo "$(date +%s) resolved-once ${IP:-none}"
end=$((SECONDS + CAP))
while ((SECONDS < end)) && [[ ! -e $STOP ]]; do
  c=FAIL f=FAIL
  [[ -n $IP && $(printf "PING\r\n" | timeout 5 nc -w 3 "$IP" "$PORT" 2>&1) == *+PONG* ]] && c=OK
  fip=$(dig +short "$H" | head -1)
  [[ $(printf "PING\r\n" | timeout 5 nc -w 3 "$H" "$PORT" 2>&1) == *+PONG* ]] && f=OK
  echo "$(date +%s) cached-ip=$c fresh=$f fresh-ip=${fip:-none}"
  sleep 2
done
'

# Args: host port cap stopfile. One TCP connection held open with a PING every second.
# shellcheck disable=SC2016
HELD_LOOP='
H=$1 PORT=$2 CAP=$3 STOP=$4
end=$((SECONDS + CAP))
if ! exec 3<>"/dev/tcp/$H/$PORT"; then echo "$(date +%s) held CONNECT_FAIL"; exit; fi
echo "$(date +%s) held OPEN"
while ((SECONDS < end)) && [[ ! -e $STOP ]]; do
  printf "PING\r\n" >&3 2>/dev/null || { echo "$(date +%s) held WRITE_FAIL"; exit; }
  if IFS= read -r -t 5 line <&3; then echo "$(date +%s) held ${line%?}"; else echo "$(date +%s) held READ_FAIL (EOF or 5s timeout)"; exit; fi
  sleep 1
done
echo "$(date +%s) held STILL_OPEN"
'

# Args: host port cap stopfile. A new connection every 0.5s.
# shellcheck disable=SC2016
NEW_LOOP='
H=$1 PORT=$2 CAP=$3 STOP=$4
end=$((SECONDS + CAP))
while ((SECONDS < end)) && [[ ! -e $STOP ]]; do
  out=$(printf "PING\r\n" | timeout 5 nc -w 3 "$H" "$PORT" 2>&1 | tr -d "\r")
  if [[ $out == *+PONG* ]]; then echo "$(date +%s) new OK"; else echo "$(date +%s) new FAIL ${out:-no reply}"; fi
  sleep 0.5
done
'

# start_loop <name> <script> <host> <port> <cap>: runs in the background, output in $TMP/<name>.log.
start_loop() {
  kubectl -n "$DEMO_NS" exec -i "$CLIENT" -- bash -s -- "$3" "$4" "$5" "$STOP_FILE-$1" <<<"$2" >"$TMP/$1.log" 2>&1 &
}

stop_loop() {
  kubectl -n "$DEMO_NS" exec "$CLIENT" -- touch "$STOP_FILE-$1" >/dev/null 2>&1 || true
}

# wait_pe_ready <hostname> <timeout>: prints the PrivateEndpoint line once Ready.
wait_pe_ready() {
  local deadline=$((SECONDS + $2)) line
  while ((SECONDS < deadline)); do
    line="$(pe_line "$1")"
    if [[ "$(awk '{print $5}' <<<"$line")" == True ]]; then
      echo "$line"
      return
    fi
    sleep 2
  done
  return 1
}

cmd_origin_restart() {
  require_demo
  cat <<EOF
== origin-restart (FT-3, R29)
Does:    stop the origin for $REDIS_HOST for ${DOWN}s (FAULT_ORIGIN_MODE=$ORIGIN_MODE), start it again.
         A client in $CLIENT resolves $REDIS_HOST once and keeps dialing that IP, next to a
         probe that resolves fresh every time.
Expects: the PrivateEndpoint, its Service and the ClusterIP survive a short origin restart, and
         the DNS-caching client works again once the origin is back (R29 "offline" != "retired").
Today:   expected FAIL. One poll without the endpoint deletes CR -> Service -> ClusterIP; the
         re-created Service usually gets a new IP, so the caching client never recovers.
EOF
  local before svc_before
  before="$(pe_line "$REDIS_HOST")"
  svc_before="$(svc_line "$REDIS_SVC")"
  [[ -n "$before" && -n "$svc_before" ]] || { echo "no PrivateEndpoint/Service for $REDIS_HOST yet; run make pe-demo-up first" >&2; exit 1; }
  echo "before:  PrivateEndpoint $before"
  echo "         Service $REDIS_SVC $svc_before"

  start_loop cached "$CACHED_LOOP" "$REDIS_HOST" 6379 $((DOWN + RECOVER + 120))
  sleep 6

  local t_stop t_start gone_at="" svc_gone_at="" t
  t_stop=$(date +%s)
  case "$ORIGIN_MODE" in
    endpoint)
      echo "==> $(date +%T) kubectl -n $DEMO_NS delete agentendpoint redis-direct"
      kubectl -n "$DEMO_NS" delete agentendpoint redis-direct --wait --timeout=60s
      ;;
    agent)
      echo "==> $(date +%T) kubectl -n $OP_NS scale deploy/$AGENT_DEPLOY --replicas=0"
      kubectl -n "$OP_NS" scale "deploy/$AGENT_DEPLOY" --replicas=0
      ;;
    *) echo "FAULT_ORIGIN_MODE must be endpoint or agent" >&2; exit 2 ;;
  esac
  echo "==> origin down for ${DOWN}s, watching the PrivateEndpoint and Service"
  while (($(date +%s) - t_stop < DOWN)); do
    t=$(($(date +%s) - t_stop))
    [[ -n "$gone_at" || -n "$(pe_line "$REDIS_HOST")" ]] || { gone_at=$t; echo "    +${t}s PrivateEndpoint gone"; }
    [[ -n "$svc_gone_at" || -n "$(svc_line "$REDIS_SVC")" ]] || { svc_gone_at=$t; echo "    +${t}s Service $REDIS_SVC gone"; }
    sleep 2
  done

  t_start=$(date +%s)
  case "$ORIGIN_MODE" in
    endpoint)
      echo "==> $(date +%T) re-applying the demo (demo.sh apply)"
      KUBE_NAMESPACE="$OP_NS" PE_DEMO_NS="$DEMO_NS" PE_DEMO_PREFIX="$P" "$DIR/demo.sh" apply >/dev/null
      ;;
    agent)
      echo "==> $(date +%T) kubectl -n $OP_NS scale deploy/$AGENT_DEPLOY --replicas=1"
      kubectl -n "$OP_NS" scale "deploy/$AGENT_DEPLOY" --replicas=1
      ;;
  esac
  local after svc_after
  if after="$(wait_pe_ready "$REDIS_HOST" "$RECOVER")"; then
    echo "    PrivateEndpoint Ready again $(($(date +%s) - t_start))s after restart"
  else
    echo "    PrivateEndpoint for $REDIS_HOST not Ready within ${RECOVER}s (FAULT_RECOVER_TIMEOUT)"
  fi
  # Give the caching client a few rounds against the recovered endpoint.
  sleep 12
  stop_loop cached
  wait || true
  svc_after="$(svc_line "$REDIS_SVC")"
  echo "after:   PrivateEndpoint ${after:-<none>}"
  echo "         Service $REDIS_SVC ${svc_after:-<none>}"
  echo "== client log ($CLIENT; epoch, then cached-IP dial, fresh-resolve dial)"
  cat "$TMP/cached.log"
  echo

  if [[ -z "$gone_at" ]]; then
    result OBSERVE "PrivateEndpoint never disappeared during the ${DOWN}s outage (raise FAULT_DOWN_SECONDS if no poll ran)"
  else
    result OBSERVE "PrivateEndpoint deleted ${gone_at}s into the outage; Service gone at: ${svc_gone_at:-never}${svc_gone_at:+s}"
  fi
  if [[ -n "$after" && "$(awk '{print $2}' <<<"$before")" == "$(awk '{print $2}' <<<"$after")" ]]; then
    result PASS "PrivateEndpoint kept across the restart (same UID)"
  else
    result FAIL "PrivateEndpoint deleted and re-created (UID $(awk '{print $2}' <<<"$before") -> $(awk '{print $2}' <<<"$after"))"
  fi
  if [[ -n "$svc_after" && "${svc_before% *}" == "${svc_after% *}" ]]; then
    result PASS "Service $REDIS_SVC kept (same UID)"
  else
    result FAIL "Service $REDIS_SVC deleted and re-created"
  fi
  if [[ -n "$svc_after" && "${svc_before#* }" == "${svc_after#* }" ]]; then
    result PASS "ClusterIP unchanged (${svc_before#* })"
  else
    result FAIL "ClusterIP changed ${svc_before#* } -> ${svc_after#* }"
  fi
  local last
  last="$(grep 'cached-ip=' "$TMP/cached.log" | tail -1 || true)"
  if [[ "$last" == *cached-ip=OK* ]]; then
    result PASS "DNS-caching client works again after the restart"
  else
    result FAIL "DNS-caching client still failing after the restart (last: ${last:-no samples})"
  fi
  if [[ "$last" == *fresh=OK* ]]; then
    result PASS "client that re-resolves works again"
  else
    result FAIL "client that re-resolves still failing (last: ${last:-no samples})"
  fi
}

cmd_rolling_restart() {
  require_demo
  cat <<EOF
== rolling-restart (FT-5, R24)
Does:    holds one TCP connection to $REDIS_HOST:6379 open (PING every 1s) and opens a new
         connection every 0.5s, then kubectl rollout restart deploy/$FWD_DEPLOY.
Expects: the held connection is drained, not cut, and no new connection fails during the rollout.
Today:   expected FAIL for the held connection (no preStop, no drain of in-flight connections,
         terminationGracePeriodSeconds only delays SIGKILL). New connections: OBSERVE; the new pod
         can go Ready before its private-dial session is up, and the old pod stops accepting
         before kube-proxy drops it.
EOF
  [[ -n "$(pe_line "$REDIS_HOST")" ]] || { echo "no PrivateEndpoint for $REDIS_HOST; run make pe-demo-up first" >&2; exit 1; }
  start_loop held "$HELD_LOOP" "$REDIS_HOST" 6379 600
  start_loop new "$NEW_LOOP" "$REDIS_HOST" 6379 600
  sleep 5
  local t0 t1
  t0=$(date +%s)
  echo "==> $(date +%T) kubectl -n $OP_NS rollout restart deploy/$FWD_DEPLOY"
  kubectl -n "$OP_NS" rollout restart "deploy/$FWD_DEPLOY"
  kubectl -n "$OP_NS" rollout status "deploy/$FWD_DEPLOY" --timeout=180s
  t1=$(date +%s)
  echo "    rollout took $((t1 - t0))s; watching 15s more"
  sleep 15
  stop_loop held
  stop_loop new
  wait || true
  echo "== held connection log (epoch, event)"
  cat "$TMP/held.log"
  echo "== new connections: failures only"
  grep 'new FAIL' "$TMP/new.log" || echo "(none)"
  echo

  local held_end n_ok n_fail
  held_end="$(tail -1 "$TMP/held.log")"
  if [[ "$held_end" == *STILL_OPEN* ]]; then
    result PASS "held connection survived the rollout"
  elif [[ "$held_end" =~ ^([0-9]+)\ (.*) ]]; then
    result FAIL "held connection cut $((BASH_REMATCH[1] - t0))s after rollout restart: ${BASH_REMATCH[2]}"
  else
    result FAIL "held connection: no usable log (${held_end:-empty})"
  fi
  n_ok="$(grep -c 'new OK' "$TMP/new.log" || true)"
  n_fail="$(grep -c 'new FAIL' "$TMP/new.log" || true)"
  if ((n_fail == 0)); then
    result PASS "no new connection failed during the rollout ($n_ok ok)"
  else
    result FAIL "$n_fail of $((n_ok + n_fail)) new connections failed around the rollout (rollout epoch $t0-$t1)"
  fi
}

uninstall_leftovers() {
  echo "-- PrivateEndpoints: $(kubectl -n "$OP_NS" get privateendpoints --no-headers 2>/dev/null | wc -l)"
  echo "-- per-host Services:"
  kubectl -n "$OP_NS" get svc -l ngrok.com/managed-by=private-endpoint-poller --no-headers 2>&1 || true
  echo "-- forwarder objects:"
  kubectl -n "$OP_NS" get deploy,sa,role,rolebinding -o name 2>&1 | grep -E 'private-endpoint' || echo none
  echo "-- CRD: $(kubectl get crd privateendpoints.ngrok.com -o name 2>&1)"
  echo "-- CoreDNS blocks: $(kubectl -n kube-system get cm coredns -o jsonpath='{.data.Corefile}' | grep -c 'ngrok-private-endpoints begin' || true)"
}

# "<hostname> <name> <uid> <clusterIP> <forwarderPort>" for every PrivateEndpoint, sorted.
pe_snapshot() {
  kubectl -n "$OP_NS" get privateendpoints -o jsonpath='{range .items[*]}{.spec.hostname}{" "}{.metadata.name}{" "}{.metadata.uid}{" "}{.status.clusterIP}{" "}{.status.forwarderPort}{"\n"}{end}' | sort
}

helm_set_enabled() {
  helm upgrade "$RELEASE" "$CHART" -n "$OP_NS" --reuse-values --set "ngrok.features.privateEndpoints.enabled=$1" --wait --timeout 5m
}

cmd_uninstall() {
  local yes=false
  [[ "${1:-}" != --yes ]] || yes=true
  cat <<EOF
== uninstall (FT-6, R16)
Does:    helm upgrade $RELEASE --reuse-values --set ngrok.features.privateEndpoints.enabled=false, waits ${SETTLE}s,
         lists leftovers (kind-e2e §5 F), dials the demo from the client, then re-enables and checks
         the leftovers are picked back up. Does not run 'helm uninstall' (kind-e2e §5 F3).
Expects: disabling cleans up PrivateEndpoints, per-host Services and (with a note) DNS; re-enable
         recovers.
Today:   expected FAIL on cleanup (no ownerRefs/finalizers; CRs and Services are orphaned, DNS still
         answers with a ClusterIP that has no endpoints). Re-enable expected PASS (same CRs, IPs, ports).
EOF
  if [[ "$yes" != true ]]; then
    cat <<EOF

Dry run. Rerun with --yes (make pe-demo-faults FAULT=uninstall FAULT_ARGS=--yes) to execute:
  helm upgrade $RELEASE $CHART -n $OP_NS --reuse-values --set ngrok.features.privateEndpoints.enabled=false --wait --timeout 5m
  sleep $SETTLE
  kubectl -n $OP_NS get privateendpoints
  kubectl -n $OP_NS get svc -l ngrok.com/managed-by=private-endpoint-poller
  kubectl -n $OP_NS get deploy,sa,role,rolebinding -o name | grep private-endpoint
  kubectl -n kube-system get cm coredns -o jsonpath='{.data.Corefile}' | grep -c 'ngrok-private-endpoints begin'
  kubectl -n $DEMO_NS exec $CLIENT -- curl -sS -m 10 -o /dev/null -w '%{http_code} %{time_total}s' http://<prefix>-web.ngrok.direct
  helm upgrade $RELEASE $CHART -n $OP_NS --reuse-values --set ngrok.features.privateEndpoints.enabled=true --wait --timeout 5m
  kubectl -n $OP_NS rollout status deploy/$FWD_DEPLOY --timeout=180s
  ./scripts/private-endpoints-demo/demo.sh wait
  make pe-demo-test
EOF
    return
  fi
  require_demo
  local before during after
  echo "== leftovers before"
  uninstall_leftovers
  before="$(pe_snapshot)"

  echo "==> $(date +%T) disabling private endpoints"
  helm_set_enabled false
  echo "==> waiting ${SETTLE}s (FAULT_SETTLE_SECONDS)"
  sleep "$SETTLE"
  echo "== leftovers while disabled"
  uninstall_leftovers
  during="$(pe_snapshot)"
  echo "== dialing http://$WEB_HOST from $CLIENT while disabled"
  local dig_out curl_out
  dig_out="$(kubectl -n "$DEMO_NS" exec "$CLIENT" -- dig +short "$WEB_HOST" | tr '\n' ' ' || true)"
  curl_out="$(kubectl -n "$DEMO_NS" exec "$CLIENT" -- bash -c "curl -sS -m 10 -o /dev/null -w '%{http_code} %{time_total}s' http://$WEB_HOST 2>&1; echo ' rc='\$?" | tr '\n' ' ' || true)"
  echo "    dig: ${dig_out:-<empty>}"
  echo "    curl: $curl_out"
  local n_pe n_svc fwd
  n_pe="$(kubectl -n "$OP_NS" get privateendpoints --no-headers 2>/dev/null | wc -l)"
  n_svc="$(kubectl -n "$OP_NS" get svc -l ngrok.com/managed-by=private-endpoint-poller --no-headers 2>/dev/null | wc -l)"
  fwd="$(kubectl -n "$OP_NS" get deploy,sa,role,rolebinding -o name | grep -E 'private-endpoint' || true)"

  echo "==> $(date +%T) re-enabling private endpoints"
  helm_set_enabled true
  kubectl -n "$OP_NS" rollout status "deploy/$FWD_DEPLOY" --timeout=180s
  local wait_rc=0
  KUBE_NAMESPACE="$OP_NS" PE_DEMO_NS="$DEMO_NS" PE_DEMO_PREFIX="$P" "$DIR/demo.sh" wait || wait_rc=$?
  after="$(pe_snapshot)"
  # The re-created forwarder can take a few seconds past rollout to connect
  # and land in the Service's endpoints, so retry before calling it a failure.
  local code
  for _ in $(seq 1 15); do
    code="$(kubectl -n "$DEMO_NS" exec "$CLIENT" -- curl -sS -m 10 -o /dev/null -w '%{http_code}' "http://$WEB_HOST" 2>&1 || true)"
    [[ "$code" == 200 ]] && break
    sleep 2
  done
  echo

  if ((n_pe == 0)); then result PASS "disable removed all PrivateEndpoints"; else result FAIL "disable left $n_pe PrivateEndpoints"; fi
  if ((n_svc == 0)); then result PASS "disable removed all per-host Services"; else result FAIL "disable left $n_svc per-host Services"; fi
  if [[ -z "$fwd" ]]; then result PASS "disable removed forwarder Deployment/SA/Roles (Helm-owned)"; else result FAIL "forwarder objects left: $(tr '\n' ' ' <<<"$fwd")"; fi
  if [[ "$before" == "$during" ]]; then
    result OBSERVE "leftover PrivateEndpoints unchanged while disabled (nobody reconciles them)"
  fi
  result OBSERVE "while disabled, $WEB_HOST resolved to '${dig_out% }' and curl gave: $curl_out"
  result OBSERVE "CoreDNS block is admin-owned and stays (remove by hand; see kind-e2e §5 F3)"
  if ((wait_rc == 0)); then result PASS "re-enable: demo PrivateEndpoints Ready again"; else result FAIL "re-enable: demo PrivateEndpoints not Ready (demo.sh wait rc=$wait_rc)"; fi
  if [[ "$before" == "$after" ]]; then
    result PASS "re-enable reused the same PrivateEndpoints, ClusterIPs and forwarder ports"
  else
    result OBSERVE "PrivateEndpoints differ before vs after (other accounts' endpoints can come and go):"
    diff <(echo "$before") <(echo "$after") || true
  fi
  if [[ "$code" == 200 ]]; then result PASS "http://$WEB_HOST returns 200 after re-enable"; else result FAIL "http://$WEB_HOST after re-enable: $code"; fi
}

cmd_revoke() {
  local p
  p="$(prefix)"
  p="${p:-<prefix>}"
  cat <<EOF
== revoke (FT-5, R28): manual
Today the same NGROK_ACCESS_TOKEN (Helm ngrok.credentials.accessToken) drives the api-manager, the
agent and the forwarder, so revoking it hits all three. Nothing here is automated: it needs the
dashboard.

Expects: the forwarder notices a terminal auth failure and stops claiming Ready; after a valid
         token is in the Secret, a container restart recovers it.
Today:   the R28 fix in cmd/private-endpoint-forwarder-manager.go: on a terminal session end (401/403 on
         reconnect) the forwarder logs "private dial session ended permanently, exiting so the
         pod restarts" and exits. The restarted container re-reads the token, cannot Connect,
         and sits not-Ready (no crashloop). Caveat: if the token was already bad at startup the
         SDK never marks the session terminal, so it just stays not-Ready with no restart.
         The manager may still crashloop (see demo.md Troubleshooting).

1. Terminals to open first:
   kubectl -n $OP_NS get pods -w
   kubectl -n $OP_NS logs -f deploy/$FWD_DEPLOY
   kubectl -n $OP_NS get pods -l app.kubernetes.io/component=private-endpoint-forwarder -w \\
     -o custom-columns='NAME:.metadata.name,READY:.status.containerStatuses[0].ready,RESTARTS:.status.containerStatuses[0].restartCount'
   kubectl -n $DEMO_NS exec -it $CLIENT -- bash -c \\
     'while :; do printf "%s " "\$(date +%T)"; curl -sS -m 5 -o /dev/null -w "%{http_code} %{time_total}s\n" http://$p-web.ngrok.direct 2>&1; sleep 2; done'
2. In the ngrok dashboard, revoke the API key / authtoken the cluster uses (or rotate: create a
   new one, then revoke the old). Note the time.
3. Watch for ~2 minutes. Record: when the "ended permanently" log line appears (it needs a
   reconnect to see the 401/403), RESTARTS going 0 -> 1 and then staying put, READY false after
   the restart, curl result (code vs error, how long each takes), manager restarts.
4. Restore: put a valid token in the Secret, then restart the container (the token is read
   from the Secret at container start):
   kubectl -n $OP_NS get deploy/$FWD_DEPLOY -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="NGROK_ACCESS_TOKEN")].valueFrom.secretKeyRef}'
   kubectl -n $OP_NS patch secret <name> --type merge -p '{"stringData":{"<key>":"<new token>"}}'
   kubectl -n $OP_NS rollout restart deploy/$FWD_DEPLOY
   (or: export NGROK_ACCESS_TOKEN=<new token>; make pe-demo-up, which redeploys everything)
   make pe-demo-test
   Record whether READY goes true only after the first private dial Connect succeeds.

Result: OBSERVE (manual). Expected: one restart, then not-Ready until a valid token; no crashloop.
EOF
}

case "${1:-}" in
  origin-restart) cmd_origin_restart ;;
  rolling-restart) cmd_rolling_restart ;;
  uninstall) cmd_uninstall "${2:-}" ;;
  revoke) cmd_revoke ;;
  *) echo "usage: $0 origin-restart|rolling-restart|uninstall [--yes]|revoke" >&2; exit 2 ;;
esac

if ((fails > 0)); then
  echo "RESULT: $fails FAIL (expected for several scenarios on today's code; see demo.md Fault scenarios)"
  exit 1
fi
