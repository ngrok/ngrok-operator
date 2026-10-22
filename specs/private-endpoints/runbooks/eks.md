# Runbook: private endpoints on EKS (managed CoreDNS add-on)

Tracker T7 / decision D7. Goal: prove the rewrite works on EKS when applied through the
CoreDNS add-on's configuration values (`corefile`), survives add-on updates, and rolls back
cleanly. Only `.ngrok.direct` is mirrored (D9). Test cluster only. ~1–1.5 h.

`VERIFY:` marks claims taken from docs/research that this run should confirm or refute.
Paste results into a new findings.md section `## EKS — <date>` using the step IDs.

Run the [kind runbook](kind-e2e.md) first; this reuses its endpoints, `pes`/`nx` helpers and
checks B1–B12 / D / I1–I7.

## 0. Prereqs and variables

aws CLI v2 (creds able to describe/update add-ons, ECR push), jq, kubectl context on the
cluster, helm, docker, ngrok agent.

```sh
export AWS_REGION=<region> CLUSTER=<cluster>
export NS=ngrok-operator
export NGROK_ACCESS_TOKEN=<PAT> NGROK_AUTHTOKEN=<PAT>
export U=$(openssl rand -hex 3); echo "U=$U"
mkdir -p eks-pe && cd eks-pe       # all saved files land here
# pes() and nx() from kind-e2e.md §0
```

## 1. Inventory and baseline (before any change)

```sh
aws eks describe-cluster --name $CLUSTER --query 'cluster.{k8s:version,platform:platformVersion,vpc:resourcesVpcConfig.vpcId}'
aws eks list-addons --cluster-name $CLUSTER                     # E1: coredns listed?
aws eks describe-addon --cluster-name $CLUSTER --addon-name coredns > coredns-addon.before.json
jq '.addon | {addonVersion, status, configurationValues}' coredns-addon.before.json
kubectl -n kube-system get cm coredns -o jsonpath='{.data.Corefile}' > Corefile.cm.before; cat Corefile.cm.before
kubectl -n kube-system get deploy coredns -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
kubectl -n kube-system get ds -o name | grep -i -E 'node-?local' || echo "no NodeLocal DNSCache"
kubectl get nodes -o custom-columns=NAME:.metadata.name,ARCH:.status.nodeInfo.architecture
```

- If `coredns` is **not** a managed add-on, it's self-managed: stop and use the ConfigMap
  approach (kind script logic) instead; record that.
- If NodeLocal DNSCache exists, every DNS check below must also be run from a pod on each
  node pool, and note it (research flags it can bypass central CoreDNS — VERIFY).
- Note whether CoreDNS runs on Fargate (`kubectl -n kube-system get pods -l k8s-app=kube-dns -o wide`);
  out of scope, just record.

Baseline DNS from a netshoot pod (`kubectl run nettest --image=nicolaka/netshoot --restart=Never -- sleep infinity`):

```sh
N1=$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}'); echo $N1   # ip-x.ec2.internal or ip-….<region>.compute.internal
N1SHORT=${N1%%.*}
APEX=${N1#*.}; APEX1=${APEX#*.}     # us-west-2.compute.internal → compute.internal; ec2.internal → internal
nx "cat /etc/resolv.conf"
nx "getent hosts $N1; getent hosts $N1SHORT"
nx "dig $N1 | grep -E 'status|IN A'"
nx "dig $APEX SOA | grep status; dig $APEX1 SOA | grep status"
nx "dig instance-data.ec2.internal | grep -E 'status|IN A'"     # VERIFY: may not exist
nx "dig +short kubernetes.default.svc.cluster.local"
```

Collision inventory (anything that would be shadowed by a single-label `.internal` /
`.ngrok.direct` rewrite):

```sh
VPC=$(aws eks describe-cluster --name $CLUSTER --query cluster.resourcesVpcConfig.vpcId --output text)
aws route53 list-hosted-zones-by-vpc --vpc-id $VPC --vpc-region $AWS_REGION \
  --query 'HostedZoneSummaries[].Name'                             # flag "internal." or "ngrok.direct."
aws route53resolver list-resolver-rules --query 'ResolverRules[].DomainName'   # flag internal./ngrok.direct.
kubectl get svc -A -o jsonpath='{range .items[?(@.spec.externalName)]}{.metadata.namespace}/{.metadata.name} {.spec.externalName}{"\n"}{end}' | grep -E '\.internal$|\.ngrok\.direct$'
```

Expected: no `internal.` zone/rule with single-label records; no ExternalName pointing at a
single-label `.internal` name. Record any hit (there is no `.internal` rule, D9, so it's
informational). The default `.ngrok.direct` rule only matters if a `ngrok.direct.` zone or rule shows
up.

## 2. Deploy operator and endpoints

Run from the repo root in a subshell so you stay in `eks-pe/`:

```sh
( cd /workspaces/ngrok-operator
REG=$(aws sts get-caller-identity --query Account --output text).dkr.ecr.$AWS_REGION.amazonaws.com
export IMG=$REG/ngrok-operator-pe
aws ecr create-repository --repository-name ngrok-operator-pe 2>/dev/null || true
aws ecr get-login-password | docker login --username AWS --password-stdin $REG
ARCH=$(kubectl get nodes -o jsonpath='{.items[0].status.nodeInfo.architecture}')   # mixed-arch groups: needs a multi-arch build, not covered
make docker-build DOCKER_ARCH=$ARCH IMG=$IMG && make docker-push IMG=$IMG
make manifests _helm_setup
helm upgrade ngrok-operator helm/ngrok-operator --install -n $NS --create-namespace \
  --set image.repository=$IMG --set image.tag=latest --set image.pullPolicy=Always \
  --set ngrok.credentials.accessToken=$NGROK_ACCESS_TOKEN --set ngrok.features.privateEndpoints.enabled=true )
kubectl -n $NS get pods
```

Start (outside the cluster) the backends and these agents from kind-e2e.md §2:
`http://pe-web-$U.ngrok.direct`, `https://pe-web-$U.ngrok.direct`,
`tcp://pe-redis-$U.ngrok.direct:6379`, and the negative control `http://pe-web-$U.internal`.

```sh
sleep 15; pes
nx "dig pe-web-$U.ngrok.direct | grep status"     # E2: before DNS change, expect NXDOMAIN
```

## 3. Discover the add-on schema and build the config

Don't assume the schema; read it:

```sh
V=$(jq -r .addon.addonVersion coredns-addon.before.json)
aws eks describe-addon-configuration --addon-name coredns --addon-version $V \
  --query configurationSchema --output text > coredns-schema.json
jq '.properties | keys' coredns-schema.json        # E3: expect "corefile" present
jq '.properties.corefile' coredns-schema.json
```

If `corefile` is absent for this add-on version: stop, record, try a newer add-on version.

Current values (may be empty, JSON, or YAML — VERIFY format; if YAML, convert with `yq -o json`):

```sh
jq -r '.addon.configurationValues // "{}"' coredns-addon.before.json > coredns-values.before.json
jq -r '.corefile // empty' coredns-values.before.json > Corefile.before
[ -s Corefile.before ] || cp Corefile.cm.before Corefile.before   # no custom corefile yet → start from live
diff Corefile.cm.before Corefile.before && echo "values match ConfigMap"
```

Insert rules inside the existing `.:53 {` block (same rules as the kind script; `cluster.local`
is the EKS default — check `kubernetes cluster.local …` in the Corefile):

```sh
NSD=$NS.svc.cluster.local
cat > ngrok-rules.txt <<EOF
    # ngrok-private-endpoints begin
    rewrite stop name regex ^([a-z][a-z0-9-]*)\.ngrok\.direct\.$ {1}-ngrok-direct.$NSD. answer auto
    # ngrok-private-endpoints end
EOF
awk 'NR==FNR{r=r $0 "\n"; next} {print} !done && /^\.:53[[:space:]]*\{/ {printf "%s", r; done=1}' \
  ngrok-rules.txt Corefile.before > Corefile.ngrok
diff Corefile.before Corefile.ngrok                # exactly 3 added lines after ".:53 {"
jq --rawfile corefile Corefile.ngrok '. + {corefile: $corefile}' coredns-values.before.json > coredns-values.ngrok.json
jq 'del(.corefile)' coredns-values.before.json; jq 'del(.corefile)' coredns-values.ngrok.json   # other settings unchanged
```

`.ngrok.direct` only (D9).

## 4. Apply via the add-on

```sh
aws eks update-addon --cluster-name $CLUSTER --addon-name coredns --addon-version $V \
  --configuration-values file://coredns-values.ngrok.json --resolve-conflicts PRESERVE \
  | tee update1.json
aws eks wait addon-active --cluster-name $CLUSTER --addon-name coredns
aws eks describe-update --name $CLUSTER --addon-name coredns --update-id $(jq -r .update.id update1.json) \
  --query 'update.{status:status,errors:errors}'                       # E4: Successful, no errors
kubectl -n kube-system get cm coredns -o jsonpath='{.data.Corefile}' | grep -A3 'ngrok-private-endpoints begin'
kubectl -n kube-system logs -l k8s-app=kube-dns --tail=50 | grep -iE 'reload|error|unknown'
```

Expected: ConfigMap contains the block. CoreDNS picks it up via `reload` (VERIFY the EKS
Corefile has `reload`; ConfigMap volume sync can take ~1 min) or pods roll. If after 2 min
`nx "dig +short pe-web-$U.ngrok.direct"` is still empty, `kubectl -n kube-system rollout restart deploy/coredns`
and record that a restart was needed. Watch for `Unknown directive 'rewrite'` (VERIFY the EKS
CoreDNS build includes `rewrite`; it's a default plugin).

## 5. Verify

```sh
# E5: traffic (kind-e2e B1, B2, B4, B5, B7–B12; B10 IP differs on EKS)
nx "curl -s -o /dev/null -w '%{http_code}\n' http://pe-web-$U.ngrok.direct"
nx "curl -s -o /dev/null -w '%{http_code} ssl_verify=%{ssl_verify_result}\n' https://pe-web-$U.ngrok.direct"
nx "printf 'PING\r\n' | nc -w 3 pe-redis-$U.ngrok.direct 6379"
# E5b: .internal is not mirrored or resolved (D9)
kubectl -n $NS get privateendpoints -o jsonpath='{range .items[*]}{.spec.url}{"\n"}{end}' | grep -c "pe-web-$U.internal"
nx "dig pe-web-$U.internal | grep status"
# E6: every CoreDNS replica has it
for ip in $(kubectl -n kube-system get pods -l k8s-app=kube-dns -o jsonpath='{.items[*].status.podIP}'); do
  echo "$ip $(nx "dig @$ip +short pe-web-$U.ngrok.direct")"; done
# E7: EKS names unaffected with the default rule (compare to §1 baseline)
nx "getent hosts $N1; getent hosts $N1SHORT"
nx "dig $N1 | grep -E 'status|IN A'"
nx "dig $APEX SOA | grep status; dig $APEX1 SOA | grep status"
nx "dig instance-data.ec2.internal | grep -E 'status|IN A'"
nx "time getent hosts pe-web-$U.ngrok.direct"  # search list includes <region>.compute.internal → extra upstream NXDOMAIN
```

Expected: `200`, `200 ssl_verify=0`, `+PONG`; E5b `0` and upstream `NXDOMAIN`; every CoreDNS
IP returns the same Service IP; E7 identical to baseline, apexes included (no `.internal`
rule).

Also run kind-e2e §4 D (musl/glibc `getent` + curl) here; EKS resolv.conf has an extra search
domain, so record the `time` numbers.

**UDP/443 egress (E8).** Private dial is QUIC-only. If B2/B4 time out but DNS is right:

```sh
kubectl -n $NS logs -l app.kubernetes.io/component=private-endpoint-forwarder --tail=100 | grep -iE 'dial|quic|timeout|error'
```

Check node security group egress, subnet NACLs, and any egress firewall/proxy (AWS Network
Firewall, Squid, etc.) for UDP 443 to `quic.connect-endpoint.ngrok.com`. Default EKS node
SGs allow all egress (VERIFY for this cluster). Record which path the cluster uses (NAT GW /
firewall) and whether anything had to be opened. Also note whether the quic-go receive-buffer
warning appears.

## 6. Survives add-on updates

Find another version compatible with the cluster:

```sh
K8S=$(aws eks describe-cluster --name $CLUSTER --query cluster.version --output text)
aws eks describe-addon-versions --addon-name coredns --kubernetes-version $K8S \
  --query 'addons[0].addonVersions[].addonVersion' --output text
V2=<adjacent version>
```

**E9 — version change, no config passed:**

```sh
aws eks update-addon --cluster-name $CLUSTER --addon-name coredns --addon-version $V2 --resolve-conflicts PRESERVE | tee update2.json
aws eks wait addon-active --cluster-name $CLUSTER --addon-name coredns
aws eks describe-addon --cluster-name $CLUSTER --addon-name coredns --query 'addon.configurationValues' --output text | grep -c ngrok-private-endpoints
kubectl -n kube-system get cm coredns -o jsonpath='{.data.Corefile}' | grep -c ngrok-private-endpoints
nx "curl -s -o /dev/null -w '%{http_code} ssl_verify=%{ssl_verify_result}\n' https://pe-web-$U.ngrok.direct"
```

Expected (VERIFY, this is the point): configurationValues persist across a version change →
counts `2`/`2`, `200`.

**E10 — direct ConfigMap edits don't survive, add-on values do:**

```sh
kubectl -n kube-system get cm coredns -o json \
  | jq '.data.Corefile |= sub("\\.:53 \\{"; ".:53 {\n    # manual-edit-canary")' | kubectl apply -f -
aws eks update-addon --cluster-name $CLUSTER --addon-name coredns --addon-version $V --resolve-conflicts OVERWRITE | tee update3.json
aws eks wait addon-active --cluster-name $CLUSTER --addon-name coredns
kubectl -n kube-system get cm coredns -o jsonpath='{.data.Corefile}' | grep -cE 'manual-edit-canary|ngrok-private-endpoints'
```

Expected (VERIFY): canary gone, ngrok block present (count `2`). This is why docs must say
"use the add-on values, not the ConfigMap". If the canary survives, record it; if the ngrok
block is gone, that's a blocker.

## 7. Rollback

```sh
aws eks update-addon --cluster-name $CLUSTER --addon-name coredns --addon-version $V \
  --configuration-values file://coredns-values.before.json --resolve-conflicts PRESERVE | tee update4.json
aws eks wait addon-active --cluster-name $CLUSTER --addon-name coredns
kubectl -n kube-system get cm coredns -o jsonpath='{.data.Corefile}' | diff Corefile.cm.before - && echo "Corefile restored"
```

If `configurationValues` was originally empty, pass `'{}'` (VERIFY that `{}` resets the
Corefile to the add-on default; if not, try passing the original `Corefile.cm.before` as
`corefile`). Then:

```sh
nx "dig pe-web-$U.ngrok.direct | grep status"     # NXDOMAIN (after cache TTL)
nx "getent hosts $N1; dig $APEX SOA | grep status" # same as baseline
helm uninstall ngrok-operator -n $NS
kubectl -n $NS get svc -l ngrok.com/managed-by=private-endpoint-poller   # VERIFY: Services gone or orphaned? (no finalizer/ownerRef)
```

Stop the agents. Delete `nettest`. If the add-on version changed during §6, return it to `$V`.

## Record results

Paste into findings.md `## EKS — <date>`:

1. Cluster: k8s/platform version, region, add-on `$V`/`$V2`, CoreDNS image, node arch, NodeLocal y/n, Fargate y/n.
2. E1 managed add-on y/n; `configurationValues` before (redact nothing secret — it shouldn't contain any).
3. E3 schema: `.properties | keys` and `.properties.corefile`.
4. `diff Corefile.before Corefile.ngrok`.
5. E4 update status; whether CoreDNS reloaded on its own or needed a restart (and how long).
6. E5/E6 outputs; kind-e2e D results with timings.
7. §1 baseline vs E7 side by side; Route 53 zones/resolver rules found.
7b. E5b output.
8. E8 egress path, anything opened, forwarder log excerpt.
9. E9/E10 counts and verdict.
10. Rollback diff result; what happened to operator Services after uninstall.
11. Every VERIFY: confirmed / refuted / not reached.
12. Update README.md: T7 (EKS part), any `.ngrok.direct` collisions found.
