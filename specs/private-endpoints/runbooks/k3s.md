# Runbook: private endpoints on k3s (`coredns-custom` imports)

Tracker T7 / decision D7 (k3s = ngrok devboxes). Goal: apply the rewrite through k3s's
`coredns-custom` ConfigMap (never the packaged `coredns` ConfigMap), confirm it survives a
k3s restart, roll back. The main path uses defaults (D8: `.ngrok.direct` only). §4b opts in to
`.internal` and checks what it captures on this devbox (R15). ~45 min.

`VERIFY:` marks claims from research/docs that this run should confirm or refute. Paste
results into a new findings.md section `## k3s — <date>` using the step IDs.

Run the [kind runbook](kind-e2e.md) first; this reuses its endpoints, `pes`/`nx` helpers and
checks B1–B12 / D / I1–I7.

## 0. Prereqs and variables

kubectl context on the k3s cluster, shell on the server node (for `k3s`/`ctr`), helm, docker,
jq, ngrok agent.

```sh
export NS=ngrok-operator
export NGROK_ACCESS_TOKEN=<PAT> NGROK_AUTHTOKEN=<PAT>
export U=$(openssl rand -hex 3); echo "U=$U"
mkdir -p k3s-pe && cd k3s-pe
# pes() and nx() from kind-e2e.md §0
```

## 1. Inventory and baseline

```sh
k3s --version                        # on the node; K1
kubectl version | grep Server
kubectl -n kube-system get deploy coredns -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
kubectl -n kube-system get cm coredns -o jsonpath='{.data.Corefile}' | tee Corefile.before
kubectl -n kube-system get cm coredns -o jsonpath='{.data.NodeHosts}' | tee NodeHosts.before
kubectl -n kube-system get deploy coredns -o json \
  | jq '.spec.template.spec.volumes[] | select(.configMap.name=="coredns-custom")'
kubectl -n kube-system get cm coredns-custom -o yaml > coredns-custom.before.yaml 2>&1; cat coredns-custom.before.yaml
```

K2 — required extension points. Expected in `Corefile.before`:

- `import /etc/coredns/custom/*.override` **inside** the `.:53 { … }` block
- `import /etc/coredns/custom/*.server` at top level
- `kubernetes cluster.local …` (else use the cluster's domain in the rules below)
- `reload`

and the Deployment mounts ConfigMap `coredns-custom` (`optional: true`) at `/etc/coredns/custom`.
VERIFY: research saw this in a pinned recent k3s manifest; older k3s releases may lack the
`.server` import or the mount. If the `.override` import or mount is missing, stop and record
the version: this mechanism isn't available there.

Collision baseline (netshoot pod: `kubectl run nettest --image=nicolaka/netshoot --restart=Never -- sleep infinity`):

```sh
nx "cat /etc/resolv.conf"                      # note any *.internal search domains (cloud devboxes)
grep -E '(^|[[:space:]])[a-z][a-z0-9-]*\.(internal|ngrok\.direct)([[:space:]]|$)' NodeHosts.before || echo "no single-label NodeHosts hits"
kubectl get svc -A -o jsonpath='{range .items[?(@.spec.externalName)]}{.metadata.namespace}/{.metadata.name} {.spec.externalName}{"\n"}{end}' | grep -E '\.internal$|\.ngrok\.direct$'
nx "dig +short kubernetes.default.svc.cluster.local"
N1=$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}'); nx "getent hosts $N1"
```

For each search domain in resolv.conf ending in `.internal` (e.g. GCE `c.<proj>.internal google.internal`),
also `nx "dig <domain> SOA | grep status"` — single-label ones (`google.internal`) get captured
by the `.internal` rule if you opt in (§4b). A NodeHosts hit would be shadowed (rewrite runs
before `hosts`). Record all hits; any `.internal` hit means don't opt in on this devbox (D8).

## 2. Deploy operator and endpoints

Build on the k3s host (or build elsewhere and copy the tarball) and import into k3s containerd:

```sh
( cd /workspaces/ngrok-operator
  make docker-build manifests _helm_setup             # IMG defaults to ngrok-operator-local
  docker save ngrok-operator-local:latest | sudo k3s ctr images import -   # every node that may run the pods
  helm upgrade ngrok-operator helm/ngrok-operator --install -n $NS --create-namespace \
    --set image.repository=ngrok-operator-local --set image.tag=latest --set image.pullPolicy=Never \
    --set ngrok.credentials.accessToken=$NGROK_ACCESS_TOKEN --set ngrok.features.privateEndpoints.enabled=true )
kubectl -n $NS get pods
```

Start (outside the cluster) backends + agents from kind-e2e.md §2:
`http://pe-web-$U.ngrok.direct`, `https://pe-web-$U.ngrok.direct`,
`tcp://pe-redis-$U.ngrok.direct:6379`, and the negative control `http://pe-web-$U.internal`.

```sh
sleep 15; pes
nx "dig pe-web-$U.ngrok.direct | grep status"     # K3: NXDOMAIN before the DNS change
```

## 3. Apply via `coredns-custom`

```sh
cat > ngrok.override <<EOF
# ngrok-private-endpoints
rewrite stop name regex ^([a-z][a-z0-9-]*)\.ngrok\.direct\.$ {1}-ngrok-direct.$NS.svc.cluster.local. answer auto
EOF
cat ngrok.override
if kubectl -n kube-system get cm coredns-custom >/dev/null 2>&1; then
  kubectl -n kube-system patch cm coredns-custom --type merge \
    -p "$(jq -n --rawfile v ngrok.override '{data:{"ngrok.override":$v}}')"
  echo no > created-by-us
else
  kubectl -n kube-system create cm coredns-custom --from-file=ngrok.override=ngrok.override
  echo yes > created-by-us
fi
kubectl -n kube-system get cm coredns-custom -o jsonpath='{.data}' | jq 'keys'   # other keys still there
```

This is the default (D8): `.ngrok.direct` only; §4b adds the `.internal` line. If another `*.override` key already has `rewrite` rules, note it: rule order across imported
files depends on glob order (VERIFY).

**K4 — does it load without a restart?** Wait 90s, then:

```sh
nx "dig +short pe-web-$U.ngrok.direct"
```

Record yes/no. VERIFY: CoreDNS `reload` hashes the Corefile, so changes only in imported
files may not trigger a reload. Then restart regardless:

```sh
kubectl -n kube-system rollout restart deploy/coredns
kubectl -n kube-system rollout status deploy/coredns --timeout=120s
kubectl -n kube-system logs -l k8s-app=kube-dns --tail=30 | grep -iE 'error|unknown|plugin'
```

## 4. Verify

```sh
# K5: traffic + DNS (kind-e2e B1, B2, B4–B10)
nx "curl -s -o /dev/null -w '%{http_code}\n' http://pe-web-$U.ngrok.direct"
nx "curl -s -o /dev/null -w '%{http_code} ssl_verify=%{ssl_verify_result}\n' https://pe-web-$U.ngrok.direct"
nx "printf 'PING\r\n' | nc -w 3 pe-redis-$U.ngrok.direct 6379"
nx "dig +noall +answer pe-web-$U.ngrok.direct; dig nope-$U.ngrok.direct | grep status; dig metadata.google.internal | grep status"
# K5b: .internal not mirrored or resolved by default (kind-e2e B11, B12)
kubectl -n $NS get privateendpoints -o jsonpath='{range .items[*]}{.spec.url}{"\n"}{end}' | grep -c "pe-web-$U.internal"
nx "dig pe-web-$U.internal | grep status"
# K6: collision re-check vs §1
nx "dig +short kubernetes.default.svc.cluster.local; getent hosts $N1"
```

Expected: `200`, `200 ssl_verify=0`, `+PONG`, answer carries the queried name, `NXDOMAIN`,
upstream `NXDOMAIN` (not SERVFAIL); K5b `0` and upstream `NXDOMAIN`; cluster/node names and
`.internal` search domains same as baseline. Also run kind-e2e §4 D (musl/glibc).

### 4b. Opt-in `.internal` and capture check (D8, R15)

Skip if §1 found `.internal` hits; record that instead.

```sh
( cd /workspaces/ngrok-operator
  helm upgrade ngrok-operator helm/ngrok-operator -n $NS --reuse-values --set ngrok.features.privateEndpoints.includeInternal=true )
printf '%s\n' 'rewrite stop name regex ^([a-z][a-z0-9-]*)\.internal\.$ {1}-internal.'"$NS"'.svc.cluster.local. answer auto' >> ngrok.override
kubectl -n kube-system patch cm coredns-custom --type merge \
  -p "$(jq -n --rawfile v ngrok.override '{data:{"ngrok.override":$v}}')"
kubectl -n kube-system rollout restart deploy/coredns
kubectl -n kube-system rollout status deploy/coredns --timeout=120s
```

Start `tcp://pe-redis-$U.internal:6379` too (kind-e2e §4b), then:

```sh
# K10: .internal works (kind-e2e I1, I2, I4–I6)
sleep 15; pes | grep -E "NAME|\.internal"
nx "curl -s -o /dev/null -w '%{http_code}\n' http://pe-web-$U.internal"
nx "printf 'PING\r\n' | nc -w 3 pe-redis-$U.internal 6379"
nx "dig +noall +answer pe-web-$U.internal; dig nope-$U.internal | grep status; dig metadata.google.internal | grep status"
# K11 (R15): capture vs §1 baseline
nx "dig +short kubernetes.default.svc.cluster.local; getent hosts $N1"
```

For each `.internal` search domain from §1, rerun `nx "dig <domain> SOA | grep status"`.
Expected: `.internal` CRs `READY=True`, `200`, `+PONG`, `nope` `NXDOMAIN`, multi-label
`.internal` names unchanged. Single-label search domains (`google.internal`) now `NXDOMAIN`:
record each before/after and whether anything broke.

Back to the default before §5:

```sh
sed -i '/-internal\./d' ngrok.override; cat ngrok.override      # only the .ngrok.direct rule left
kubectl -n kube-system patch cm coredns-custom --type merge \
  -p "$(jq -n --rawfile v ngrok.override '{data:{"ngrok.override":$v}}')"
kubectl -n kube-system rollout restart deploy/coredns
( cd /workspaces/ngrok-operator
  helm upgrade ngrok-operator helm/ngrok-operator -n $NS --reuse-values --set ngrok.features.privateEndpoints.includeInternal=false )
sleep 15; pes | grep -cE "\.internal"            # 0
```

Stop the extra `.internal` agent.

**UDP/443 egress (K7).** QUIC-only. If DNS is right but traffic times out, check
forwarder logs (`kubectl -n $NS logs -l app.kubernetes.io/component=private-endpoint-forwarder --tail=100`)
and the devbox's host firewall / cloud egress rules for UDP 443 to
`quic.connect-endpoint.ngrok.com`. Record whether the quic-go receive-buffer warning shows
(`sysctl net.core.rmem_max` on the node).

## 5. Persistence

**K8 — k3s restart re-applies packaged manifests:**

```sh
sudo systemctl restart k3s          # on the server node
kubectl -n kube-system rollout status deploy/coredns --timeout=180s
kubectl -n kube-system get cm coredns-custom -o jsonpath='{.data.ngrok\.override}' | head -2
kubectl -n kube-system get cm coredns -o jsonpath='{.data.Corefile}' | diff Corefile.before - && echo "packaged Corefile unchanged"
nx "curl -s -o /dev/null -w '%{http_code} ssl_verify=%{ssl_verify_result}\n' https://pe-web-$U.ngrok.direct"
```

Expected: `ngrok.override` still present, packaged Corefile unchanged, `200 ssl_verify=0`.

K9 (optional, only on a throwaway devbox): k3s upgrade/downgrade one minor via the install
script with `INSTALL_K3S_VERSION`, then repeat K8 + K2. VERIFY: `coredns-custom` is not part
of the packaged manifest, so it should survive; the import lines must still exist in the new
version's Corefile.

## 6. Rollback

```sh
if grep -q yes created-by-us; then
  kubectl -n kube-system delete cm coredns-custom
else
  kubectl -n kube-system patch cm coredns-custom --type json -p '[{"op":"remove","path":"/data/ngrok.override"}]'
fi
kubectl -n kube-system rollout restart deploy/coredns
kubectl -n kube-system rollout status deploy/coredns --timeout=120s
nx "dig pe-web-$U.ngrok.direct | grep status"     # NXDOMAIN
nx "dig +short kubernetes.default.svc.cluster.local; getent hosts $N1"   # baseline
kubectl -n kube-system get cm coredns-custom -o yaml 2>&1 | diff coredns-custom.before.yaml - && echo "custom ConfigMap restored"
helm uninstall ngrok-operator -n $NS
```

(`diff` will show metadata noise like resourceVersion; check that `data` matches.) Stop
agents, delete `nettest`, and remove the imported image if wanted
(`sudo k3s ctr images rm docker.io/library/ngrok-operator-local:latest`).

## Record results

Paste into findings.md `## k3s — <date>`:

1. K1 versions (k3s, k8s, CoreDNS image), single/multi-node, host OS/cloud.
2. K2: the import lines, volume mount, cluster domain — present/absent.
3. §1 collision baseline: resolv.conf, NodeHosts/ExternalName/search-domain hits.
4. Whether `coredns-custom` pre-existed and its keys.
5. K4 reload-without-restart result and time.
6. K5/K5b/K6 outputs; kind-e2e D results.
6b. §4b: K10 outputs, K11 and search-domain SOA before/after, or why it was skipped.
7. K7 egress notes.
8. K8 (and K9 if run) results.
9. Rollback result.
10. Every VERIFY: confirmed / refuted / not reached.
11. Update README.md: T7 (k3s part), R4/R15 rows (k3s `.internal` opt-in verdict).
