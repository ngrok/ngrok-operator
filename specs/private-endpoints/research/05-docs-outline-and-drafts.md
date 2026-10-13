<!-- path: specs/private-endpoints/research/05-docs-outline-and-drafts.md -->
# Private endpoints: user documentation outline and drafts

**Editorial status:** 2026-10-12 draft. Do not publish these as generally available setup instructions. The current branch has the revision-2 forwarder and kind/CoreDNS rewrite path, not the proposed standalone DNS service. Pages marked **DNS-service prerequisite** require that future component and platform validation first. [Revision-2 design](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/design.md)

## Documentation outline

1. Overview and choosing `.ngrok.direct` versus `.internal`.
2. Install the operator preview and verify the private-endpoint resources.
3. Configure DNS: self-managed/kind, EKS, AKS, GKE Standard kube-dns, GKE Standard Cloud DNS, OpenShift, K3s, RKE2; separate Autopilot support-status page.
4. Troubleshoot resolution, forwarding, TLS, and update/rollback behavior.
5. Security, endpoint discovery scope, supported names, capacity, IP families, and uninstall.

The following pages are full draft content within this file. Placeholders must be replaced and platform tests completed before release.

---

## Draft: overview, installation, and security

Private endpoints let applications use an endpoint's original hostname while traffic passes through an in-cluster forwarder. The preview discovers private endpoints in the configured account and creates local Services automatically. DNS setup is a separate cluster-administrator task. [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md)

Use `.ngrok.direct` for the first deployment and for the tested public-certificate HTTPS path. `.internal` can overlap with existing private names and needs a separate administrator decision. Do not disable TLS verification in production merely to make a DNS test pass. For end-to-end TLS using private names, a private CA can be part of the application's trust design; that is not a claim that ngrok supplies such certificates automatically. [Recorded kind test findings](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/findings.md)

For the current POC, follow the repository's development installation rather than assuming a published Helm chart version includes the feature:

```sh
kind create cluster --name pe-poc
# Supply NGROK_ACCESS_TOKEN securely in the environment first.
KIND_CLUSTER_NAME=pe-poc make deploy_with_private_endpoints
kubectl -n ngrok-operator get privateendpoints
```

The feature flag is `privateEndpoints.enabled`; token wiring and the CRD must be present in the installed branch/chart. Never paste PAT values into troubleshooting reports. The repository's kind setup command `make kind-private-endpoints-dns` enables both suffixes; do not use it unchanged on a shared production cluster. [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md)

**Security:** any workload that can reach the forwarder can use its shared authority for the discovered endpoints in the current POC. Pod identity and Traffic Policy authorization are future work. Namespace/pod network policy can restrict access to the forwarder, but hiding DNS records is not authorization, and a Service name is not proof of caller identity. [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md) [Kubernetes: NetworkPolicy](https://kubernetes.io/docs/concepts/services-networking/network-policies/)

---

## Draft: DNS setup prerequisites

Choose the procedure matching the **actual DNS provider and owner**, not only the cloud name. Obtain permission to change the appropriate DNS input. The application installer's namespace access may not include this permission.

Before editing anything, record the current configuration and establish baseline resolution from a representative workload. Use an organization-approved diagnostic pod containing `dig` and `curl`; this document does not require downloading a particular third-party image.

```sh
export APP_NS=default
export TEST_POD=dns-debug       # Existing, approved diagnostic pod
export PE_NS=ngrok-operator
export HOST=foo.ngrok.direct    # Replace with your actual endpoint hostname
kubectl -n "$APP_NS" exec "$TEST_POD" -- cat /etc/resolv.conf
kubectl -n "$APP_NS" exec "$TEST_POD" -- dig kubernetes.default.svc.cluster.local.
kubectl -n "$APP_NS" exec "$TEST_POD" -- dig "$HOST."
kubectl -n "$PE_NS" get privateendpoints
```

Replace `cluster.local` if the cluster uses another domain. Compare with the application container's resolver settings when they differ. Kubernetes supports custom pod DNS settings, so one successful diagnostic pod is not proof for every workload. [Kubernetes: DNS for Services and Pods](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/)

For a **DNS-service prerequisite** page, obtain the stable address of the deployed ngrok-aware DNS server and verify it over TCP and UDP. Do not substitute the TCP forwarder Service: it is not a DNS server. This future component must already know the endpoint-to-Service-IP mappings. [Revision-2 design](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/design.md)

---

## Self-managed CoreDNS and kind

**Current POC path.** Administrator-managed CoreDNS can be customized through its ConfigMap. The commands below use the common names `kube-system/coredns` and `deployment/coredns`; verify them first. [Kubernetes: customizing DNS](https://kubernetes.io/docs/tasks/administer-cluster/dns-custom-nameservers/)

```sh
kubectl -n kube-system get configmap coredns -o yaml > coredns.before.yaml
kubectl -n kube-system edit configmap coredns
```

Inside the existing `.:53` block, add this single line, replacing the release namespace and cluster domain if needed:

```text
rewrite stop name regex ^([a-z][a-z0-9-]*)\.ngrok\.direct\.$ {1}-ngrok-direct.ngrok-operator.svc.cluster.local. answer auto
```

Keep every existing plugin and upstream configuration. Do not add a second root server block. The rule follows the branch's naming scheme and CoreDNS's response rewrite mechanism. [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md) [CoreDNS rewrite plugin](https://coredns.io/plugins/rewrite/)

```sh
kubectl -n kube-system rollout restart deployment/coredns
kubectl -n kube-system rollout status deployment/coredns
```

Run the verification block below. **Rollback:** edit the same input and remove only the ngrok rule, then restart and repeat baseline tests. If Helm/GitOps owns CoreDNS, change that input instead of fighting its reconciliation.

---

## EKS managed CoreDNS

**Current rewrite path; conditional-forwarding variant requires a DNS service.** AWS manages the Corefile via add-on configuration. Persist the change in the add-on's `corefile` value, not only in the Kubernetes ConfigMap. [AWS: managing CoreDNS on EKS](https://docs.aws.amazon.com/eks/latest/userguide/managing-coredns.html)

When Terraform or another system owns the add-on, make the equivalent change there. For an administrator using AWS CLI directly, export existing settings first:

```sh
export CLUSTER=your-cluster
aws eks describe-addon --cluster-name "$CLUSTER" --addon-name coredns > coredns-addon.before.json
ADDON_VERSION=$(jq -r '.addon.addonVersion' coredns-addon.before.json)
aws eks describe-addon-configuration --addon-name coredns --addon-version "$ADDON_VERSION" > coredns-schema.json
jq -r '.addon.configurationValues // "{}"' coredns-addon.before.json > coredns-values.before.json
kubectl -n kube-system get configmap coredns -o jsonpath='{.data.Corefile}' > Corefile.ngrok
```

Inspect the schema for `corefile`. Edit `Corefile.ngrok` to include the rewrite line from the self-managed page inside its existing root block. Keep `ready` and the remaining provider configuration. The following merges rather than discards the existing add-on settings:

```sh
jq --rawfile corefile Corefile.ngrok '. + {corefile: $corefile}' coredns-values.before.json > coredns-values.ngrok.json
aws eks update-addon --cluster-name "$CLUSTER" --addon-name coredns --configuration-values file://coredns-values.ngrok.json --resolve-conflicts PRESERVE
aws eks describe-addon --cluster-name "$CLUSTER" --addon-name coredns
```

The command/API names and `corefile` schema are documented by AWS; the file-generation workflow is this draft's adaptation. Do not switch to `OVERWRITE` merely to suppress a conflict without reviewing what would change. [AWS CLI EKS configuration examples](https://docs.aws.amazon.com/cli/latest/userguide/cli_eks_code_examples.html) [AWS CLI: update-addon](https://docs.aws.amazon.com/cli/latest/reference/eks/update-addon.html)

Verify add-on/CoreDNS health and application resolution; test after a staging add-on upgrade. **Rollback:** remove the added rule from the currently owned Corefile, regenerate the merged JSON, and submit it through the same API/IaC owner. Only restore the whole saved settings file when no unrelated configuration has changed since the backup.

---

## AKS

**Preferred portable variant: DNS-service prerequisite.** Use the `coredns-custom` ConfigMap, not the provider-managed main Corefile. Microsoft documents `.server` and `.override` import keys. [Microsoft: customize CoreDNS on AKS](https://learn.microsoft.com/en-us/azure/aks/coredns-custom)

```sh
kubectl -n kube-system get configmap coredns-custom -o yaml > coredns-custom.before.yaml
```

If the ConfigMap does not exist, create it before editing:

```sh
kubectl -n kube-system create configmap coredns-custom
```

Do not run the create command when it already exists. Edit the ConfigMap and add only this key under `data`, preserving others:

```yaml
ngrok.server: |
  ngrok.direct:53 {
      errors
      cache 5
      forward . <NGROK_DNS_IP>
  }
```

```sh
kubectl -n kube-system edit configmap coredns-custom
kubectl -n kube-system rollout restart deployment/coredns
kubectl -n kube-system rollout status deployment/coredns
```

Replace the placeholder before saving. Verify ordinary and ngrok resolution. **Rollback:** remove only `data.ngrok.server`, restart CoreDNS, and retest. Keep unrelated custom entries.

**Current rewrite candidate:** an `ngrok.override` key containing the rewrite rule may avoid a separate DNS server when imported into the expected main block. Confirm the installed import and live-test the ngrok rule before documenting it as supported. Do not enable both variants for the same suffix.

---

## GKE Standard with kube-dns

**DNS-service prerequisite.** Native kube-dns does not accept CoreDNS rewrite configuration. Add a stub-domain entry in its ConfigMap. [Google: kube-dns configuration](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/kube-dns)

```sh
kubectl -n kube-system get configmap kube-dns -o yaml > kube-dns.before.yaml
kubectl -n kube-system edit configmap kube-dns
```

Under `data.stubDomains`, merge the ngrok suffix into the existing JSON. For an otherwise empty mapping, the field is:

```yaml
stubDomains: |
  {"ngrok.direct": ["<NGROK_DNS_IP>"]}
```

Do not replace other mappings; do not change the cluster's global `upstreamNameservers`. The DNS server must be reachable through the actual resolver path. GKE documents automatic pickup of stub-domain settings by its NodeLocal caches and the need for TCP as well as UDP DNS. [Google: NodeLocal DNSCache](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/nodelocal-dns-cache)

Verify ngrok resolution, Kubernetes DNS, public DNS, `metadata.internal`, and `metadata.google.internal`, comparing against the saved baseline. **Rollback:** remove only the ngrok JSON key. Keep any pre-existing `internal` mapping. Do not make `.internal` delegation part of this procedure. [Google: kube-dns configuration](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/kube-dns)

---

## GKE Standard with Cloud DNS

**DNS-service prerequisite and unresolved target-topology validation.** Google supports custom stub domains via the kube-dns ConfigMap on Standard clusters, translating the values into Cloud DNS forwarding zones. This is not evidence that every in-cluster ClusterIP is reachable from every resulting DNS path. [Google: Cloud DNS for GKE](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/about-cloud-dns) [Google: troubleshoot Cloud DNS](https://docs.cloud.google.com/kubernetes-engine/docs/troubleshooting/cloud-dns)

First establish where a representative pod's query is forwarded. Validate that the proposed target is reachable from that path, not merely from another application pod. A VPC-reachable DNS frontend may be necessary; do not publish a ClusterIP-only recipe until the target is proven. Preserve provider-managed forwarding resources. [Google Cloud DNS forwarding zones](https://docs.cloud.google.com/dns/docs/zones/zones-overview#forwarding_zones) [Google: NodeLocal DNSCache](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/nodelocal-dns-cache)

After that validation, back up and edit `kube-system/kube-dns` as in the previous page and merge the same `ngrok.direct` entry, using the validated target address. Verify from both nodes and pods, because Google's documented custom configuration has a wider scope than kube-dns-only pod configuration. [Google: Cloud DNS for GKE](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/about-cloud-dns)

**Rollback:** remove the ngrok stub-domain input through the same owner, confirm the generated forwarding behavior changes, and repeat node/pod baseline queries. Do not delete unrelated cloud DNS zones.

### GKE Autopilot status

These Standard instructions are not an Autopilot support claim. The checked Cloud DNS documentation limits this custom stub-domain facility to Standard, while NodeLocal presence alone proves no permission to change it. Collect the supported provider procedure and pass a disposable-cluster test before publishing an Autopilot page with write commands. [Google: Cloud DNS for GKE](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/about-cloud-dns) [Google: NodeLocal DNSCache](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/nodelocal-dns-cache)

---

## OpenShift

**DNS-service prerequisite.** Configure the DNS Operator input, not its generated CoreDNS ConfigMap. The following fields are from the OpenShift 4.18 DNS Operator interface. [Red Hat: OpenShift DNS Operator](https://docs.redhat.com/en/documentation/openshift_container_platform/4.18/html/networking_operators/dns-operator)

```sh
oc get dns.operator/default -o yaml > openshift-dns.before.yaml
oc edit dns.operator/default
```

Append this entry to existing `spec.servers`:

```yaml
- name: ngrok-private
  zones:
    - ngrok.direct
  forwardPlugin:
    policy: Sequential
    upstreams:
      - <NGROK_DNS_IP>
```

Replace the target and preserve existing entries. This requires DNS Operator administration, normally a cluster-administrator role.

```sh
oc describe clusteroperators/dns
oc -n openshift-dns get configmap dns-default -o yaml
```

Verify DNS from representative pods and ordinary cluster resolution. **Rollback:** remove only the `ngrok-private` entry from `spec.servers`, allow the DNS Operator to reconcile, and repeat baseline checks. Do not set the DNS Operator unmanaged merely to retain a manual Corefile edit.

---

## K3s

**Current rewrite candidate with verified custom-import prerequisites.** The inspected K3s manifest uses `coredns-custom` imports. Verify them in the installed Corefile before continuing. [K3s CoreDNS manifest](https://github.com/k3s-io/k3s/blob/82a6ddcd820bdfdd787455fae69e79afb5ef3102/manifests/coredns.yaml)

```sh
kubectl -n kube-system get configmap coredns -o yaml
kubectl -n kube-system get configmap coredns-custom -o yaml > k3s-coredns-custom.before.yaml
```

Create `coredns-custom` if it is absent, then add the following key without removing existing keys:

```yaml
ngrok.override: |
  rewrite stop name regex ^([a-z][a-z0-9-]*)\.ngrok\.direct\.$ {1}-ngrok-direct.ngrok-operator.svc.cluster.local. answer auto
```

```sh
kubectl -n kube-system edit configmap coredns-custom
kubectl -n kube-system rollout restart deployment/coredns
kubectl -n kube-system rollout status deployment/coredns
```

Replace namespace/domain in the rule as necessary. Verify resolution and traffic. Do not edit the K3s-managed packaged Corefile. **Rollback:** remove only `ngrok.override` and repeat the restart/checks. A future DNS-service deployment can instead use the `ngrok.server` block from the AKS page; do not configure both modes for the same suffix. [K3s packaged-component management](https://docs.k3s.io/installation/packaged-components)

---

## RKE2

**Draft support gate: exact installed chart values must be verified.** RKE2's supported customization mechanism is a `HelmChartConfig` matching the packaged chart's name and namespace. This research verified that envelope, not a universal CoreDNS custom-server value key. [RKE2 HelmChartConfig customization](https://docs.rke2.io/add-ons/helm)

```sh
kubectl -n kube-system get helmchart rke2-coredns -o yaml
kubectl -n kube-system get helmchartconfig rke2-coredns -o yaml
```

Record the chart/version and existing configuration. Follow the chart reference linked from RKE2's documentation to identify the value that produces an extra CoreDNS server block or root-block override. Render that exact chart with the candidate values and inspect the Corefile. Stop if no documented extension is available for the installed version; do not patch the generated ConfigMap as an unexplained workaround.

The resulting administrator-owned manifest uses `apiVersion: helm.cattle.io/v1`, `kind: HelmChartConfig`, name `rke2-coredns`, namespace `kube-system`, and `spec.valuesContent`. Populate the latter only with chart-verified values. Validate DNS, upgrade behavior, and rollback in staging before release documentation. [RKE2 HelmChartConfig customization](https://docs.rke2.io/add-ons/helm)

**Rollback:** remove only the ngrok values from the persisted manifest and live HelmChartConfig. Merely deleting a file in the RKE2 server manifests directory is not a live-resource deletion procedure. This page intentionally supplies no fabricated chart value or untested installation one-liner. [RKE2 HelmChartConfig customization](https://docs.rke2.io/add-ons/helm)

---

## Draft: verification and troubleshooting

### 1. Confirm discovery and Service wiring

```sh
kubectl -n "$PE_NS" get privateendpoints -o wide
kubectl -n "$PE_NS" get services
kubectl -n "$PE_NS" get endpointslices
kubectl -n "$PE_NS" get pods -l app.kubernetes.io/component=private-endpoint-forwarder
```

Use `kubectl describe` on the relevant PrivateEndpoint. Check the hostname/port, assigned ClusterIP, condition reason, and forwarder port. An unsupported single-label/length format will not be fixed by restarting DNS. A resource marked Ready does not prove successful external authentication or that every forwarder replica has synchronized. [PrivateEndpoint controller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/privateendpoints/controller.go) [Forwarder command and readiness](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/cmd/private-endpoint-forwarder.go)

### 2. Separate DNS from forwarding

From the diagnostic pod:

```sh
kubectl -n "$APP_NS" exec "$TEST_POD" -- dig "$HOST." A
kubectl -n "$APP_NS" exec "$TEST_POD" -- dig "$HOST." AAAA
kubectl -n "$APP_NS" exec "$TEST_POD" -- dig +tcp "$HOST." A
kubectl -n "$APP_NS" exec "$TEST_POD" -- getent hosts "$HOST"
```

`getent` is useful when available because it exercises the container's usual resolver rather than only a direct DNS query. Inspect `/etc/resolv.conf` and compare search-expanded requests with the absolute-name lookup. Application runtimes may have their own caches. [Kubernetes: DNS for Services and Pods](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/) [CoreDNS cache plugin](https://coredns.io/plugins/cache/)

For an HTTPS endpoint on 443, set `SERVICE_IP` to its real assigned Service address and bypass only DNS while keeping the hostname for TLS:

```sh
export SERVICE_IP=10.96.12.34  # Example only: replace with the real Service IP
kubectl -n "$APP_NS" exec "$TEST_POD" -- curl --connect-timeout 5 --max-time 15 --resolve "$HOST:443:$SERVICE_IP" "https://$HOST/"
```

The command is a diagnostic adaptation, not an ngrok API. If this succeeds while ordinary hostname access fails, focus on DNS. If both fail, inspect Services, listener synchronization, network policy, token wiring, QUIC egress, and the remote endpoint. Do not add `-k` as the fix for an unexpected certificate error.

### 3. Detect NodeLocal bypass

Read the pod's resolver address. Compare its answer with a direct query to central CoreDNS and, when present, the ngrok DNS service. Direct-query success with ordinary-query failure suggests a different DNS path, policy, or cache. Test on multiple nodes. Do not conclude that a central rewrite is active for clients just because `dig @CENTRAL_DNS_IP` works. [Kubernetes: NodeLocal DNSCache](https://kubernetes.io/docs/tasks/administer-cluster/nodelocaldns/) [Google: NodeLocal DNSCache](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/nodelocal-dns-cache)

### 4. Interpret common failures

| Symptom | Investigate next |
|---|---|
| NXDOMAIN just after endpoint creation | Discovery/status, name restrictions, negative caches. |
| SERVFAIL | DNS server startup/sync, loop, inaccessible conditional upstream, malformed configuration. |
| Correct IP but connection refused | Service target port, backend listener, stale forwarder replica. |
| Correct IP but timeout | Network policy/routing, missing Service port, UDP/443 restrictions, remote endpoint. |
| TLS hostname mismatch | Original hostname, configured endpoint scheme, certificate coverage, accidental Service-name URL. |
| Works with direct DNS query, fails from application | NodeLocal path, pod DNS policy, search domains, application cache. |
| Works until a provider update | Wrong configuration owner or version-specific plugin/schema behavior. |
| Some calls hit the wrong endpoint | Treat as a serious routing/assignment bug; stop the test and capture assignment/watch state. |

This table is diagnostic reasoning, not a guarantee that each symptom has only one cause. The branch records timeouts after a Service port is removed and separate creation/deletion propagation. [Recorded kind test findings](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/findings.md)

### 5. Logs and safe support data

```sh
kubectl -n "$PE_NS" logs -l app.kubernetes.io/component=private-endpoint-forwarder --tail=200
kubectl -n "$PE_NS" get events --sort-by=.metadata.creationTimestamp
```

Collect component versions, sanitized conditions, Service ports, resolver settings, node pool, timestamps, and the effective DNS owner/configuration diff. Redact credentials and sensitive endpoint names. A successful DNS response is not proof of SDK connection health. [Forwarder command and readiness](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/cmd/private-endpoint-forwarder.go)

### 6. Known limitations and removal

The current POC has single-label name restrictions, a reviewed 1000-listener range, shared caller authority, no verified IPv6 support, QUIC-only egress, and eventually consistent activation. These are preview boundaries, not platform defaults. [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md)

Before removing the feature, move dependent application traffic. Remove the DNS customization through its owner, verify baseline DNS and account for cached answers, then remove ngrok DNS/forwarding components and review leftover managed resources. Do not delete a shared `coredns-custom` or `kube-dns` ConfigMap wholesale. Capture and test rollback before the first production change.

## Sources

Sources were checked on **2026-10-12** unless the entry states otherwise. Provider documentation is evidence of the extension mechanism, not evidence that ngrok was tested on that platform.

- [Revision-2 design](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/design.md) - Draft revision 2, 2026-09-29; not a released feature contract.
- [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md) - Branch excerpts reviewed in this conversation; POC notes dated 2026-09-29.
- [Recorded kind test findings](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/findings.md) - Recorded tests: 2026-09-29, Kubernetes 1.36.1, CoreDNS 1.14.2; tests were not rerun for this bundle.
- [Kubernetes: NetworkPolicy](https://kubernetes.io/docs/concepts/services-networking/network-policies/) - Rolling documentation checked 2026-10-12; enforcement depends on the network implementation.
- [Kubernetes: DNS for Services and Pods](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/) - Rolling documentation checked 2026-10-12.
- [Kubernetes: customizing DNS](https://kubernetes.io/docs/tasks/administer-cluster/dns-custom-nameservers/) - Rolling documentation checked 2026-10-12; inspect installed DNS version.
- [CoreDNS rewrite plugin](https://coredns.io/plugins/rewrite/) - Rolling documentation checked 2026-10-12; match deployed plugin version.
- [AWS: managing CoreDNS on EKS](https://docs.aws.amazon.com/eks/latest/userguide/managing-coredns.html) - Rolling documentation checked 2026-10-12; add-on-version-specific schema applies.
- [AWS CLI EKS configuration examples](https://docs.aws.amazon.com/cli/latest/userguide/cli_eks_code_examples.html) - Checked 2026-10-12; includes describe-addon-configuration and corefile schema.
- [AWS CLI: update-addon](https://docs.aws.amazon.com/cli/latest/reference/eks/update-addon.html) - AWS CLI v2 reference checked 2026-10-12.
- [Microsoft: customize CoreDNS on AKS](https://learn.microsoft.com/en-us/azure/aks/coredns-custom) - Page last updated 2025-10-22; checked 2026-10-12.
- [Google: kube-dns configuration](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/kube-dns) - Rolling documentation checked 2026-10-12; Standard-cluster scope.
- [Google: NodeLocal DNSCache](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/nodelocal-dns-cache) - Rolling documentation checked 2026-10-12; provider/version limitations apply.
- [Google: Cloud DNS for GKE](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/about-cloud-dns) - Rolling documentation checked 2026-10-12; custom stub-domain support described for Standard only.
- [Google: troubleshoot Cloud DNS](https://docs.cloud.google.com/kubernetes-engine/docs/troubleshooting/cloud-dns) - Rolling documentation checked 2026-10-12.
- [Google Cloud DNS forwarding zones](https://docs.cloud.google.com/dns/docs/zones/zones-overview#forwarding_zones) - Rolling documentation checked 2026-10-12; forwarding-target reachability must be validated.
- [Red Hat: OpenShift DNS Operator](https://docs.redhat.com/en/documentation/openshift_container_platform/4.18/html/networking_operators/dns-operator) - OpenShift Container Platform 4.18 documentation checked 2026-10-12.
- [K3s CoreDNS manifest](https://github.com/k3s-io/k3s/blob/82a6ddcd820bdfdd787455fae69e79afb5ef3102/manifests/coredns.yaml) - Pinned commit checked 2026-10-12; includes coredns-custom .override and .server imports.
- [K3s packaged-component management](https://docs.k3s.io/installation/packaged-components) - Rolling documentation checked 2026-10-12.
- [RKE2 HelmChartConfig customization](https://docs.rke2.io/add-ons/helm) - Page last updated 2026-10-07; checked 2026-10-12. Exact installed CoreDNS chart values not verified.
- [PrivateEndpoint controller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/privateendpoints/controller.go) - Branch source reviewed in this conversation; moving branch link.
- [Forwarder command and readiness](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/cmd/private-endpoint-forwarder.go) - Branch source reviewed in this conversation; moving branch link.
- [CoreDNS cache plugin](https://coredns.io/plugins/cache/) - Rolling documentation checked 2026-10-12.
- [Kubernetes: NodeLocal DNSCache](https://kubernetes.io/docs/tasks/administer-cluster/nodelocaldns/) - Rolling documentation checked 2026-10-12; deployed Corefile remains authoritative.
