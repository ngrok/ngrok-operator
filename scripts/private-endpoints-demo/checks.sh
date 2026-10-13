#!/usr/bin/env bash
# Runs inside the netshoot client pod (piped in by demo.sh test). Prints one
# PASS/FAIL line per check and exits nonzero if any check fails.
# Usage: checks.sh <prefix> <operator-namespace> [include-internal]
set -uo pipefail

P="$1"
OP_NS="$2"
INCLUDE_INTERNAL="${3:-false}"
RETRIES="${RETRIES:-6}"
fails=0

# check <name> <expected-substring> <command...>: retries to ride out the poll
# interval and CoreDNS negative cache right after endpoints come up.
check() {
  local name="$1" want="$2" out=""
  shift 2
  for _ in $(seq 1 "$RETRIES"); do
    out="$("$@" 2>&1)"
    if [[ "$out" == *"$want"* ]]; then
      echo "PASS  $name"
      return
    fi
    sleep 3
  done
  echo "FAIL  $name: want \"$want\", got: $(printf '%s' "$out" | tr '\n' ' ' | cut -c1-200)"
  fails=$((fails + 1))
}

http_code() { curl -sS -m 10 -o /dev/null -w '%{http_code}' "$1"; }
https_verified() { curl -sS -m 10 -o /dev/null -w '%{http_code} ssl_verify=%{ssl_verify_result}' "$1"; }
tls_subject() { echo | openssl s_client -connect "$1:443" -servername "$1" 2>/dev/null | grep '^subject=' | tr -d ' '; }
redis_ping() { printf 'PING\r\n' | nc -w 3 "$1" 6379; }
dns_status() { dig "$1" | grep -o 'status: [A-Z]*'; }
# Empty answer covers both NXDOMAIN and an upstream that answers nothing.
dns_answer() {
  local a
  a="$(dig +short "$1" | tr '\n' ' ')"
  [[ -z "$a" ]] && echo "no-answer" || echo "answer: $a"
}
# The rewritten name must answer with the per-host Service's ClusterIP.
same_ip() {
  local a b
  a="$(dig +short "$1" | head -1)"
  b="$(dig +short "$2" | head -1)"
  [[ -n "$a" && "$a" == "$b" ]] && echo "same-ip $a" || echo "different: '$a' vs '$b'"
}

nope="$P-nope-$RANDOM"

check "dns: $P-web.ngrok.direct -> Service IP" "same-ip" same_ip "$P-web.ngrok.direct" "$P-web-ngrok-direct.$OP_NS.svc.cluster.local"
check "dns: $P-tls.ngrok.direct -> Service IP" "same-ip" same_ip "$P-tls.ngrok.direct" "$P-tls-ngrok-direct.$OP_NS.svc.cluster.local"
check "dns: unknown $nope.ngrok.direct is NXDOMAIN" "NXDOMAIN" dns_status "$nope.ngrok.direct"
check "dns: cluster DNS still works" "status: NOERROR" dns_status "kubernetes.default.svc.cluster.local"
check "http://$P-web.ngrok.direct" "200" http_code "http://$P-web.ngrok.direct"
check "body: http://$P-web.ngrok.direct" "hello from $P-web" curl -sS -m 10 "http://$P-web.ngrok.direct"
check "https://$P-web.ngrok.direct (cert verified)" "200 ssl_verify=0" https_verified "https://$P-web.ngrok.direct"
check "tls://$P-tls.ngrok.direct (origin cert passthrough)" "CN=$P-tls.ngrok.direct" tls_subject "$P-tls.ngrok.direct"
check "tcp://$P-redis.ngrok.direct:6379 PING" "+PONG" redis_ping "$P-redis.ngrok.direct"
if [[ "$INCLUDE_INTERNAL" == true ]]; then
  check "dns: $P-web.internal -> Service IP" "same-ip" same_ip "$P-web.internal" "$P-web-internal.$OP_NS.svc.cluster.local"
  check "http://$P-web.internal" "200" http_code "http://$P-web.internal"
else
  check "dns: $P-web.internal does not resolve (.internal not mirrored)" "no-answer" dns_answer "$P-web.internal"
fi

if ((fails > 0)); then
  echo "RESULT: $fails check(s) FAILED"
  exit 1
fi
echo "RESULT: all checks passed"
