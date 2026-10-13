<!-- path: specs/private-endpoints/research/03-platform-playbooks.md -->
# Private endpoints: platform playbooks

**Date:** 2026-10-12. **Audience:** operator/platform engineers. **Validation level:** documentation-derived, except the branch's recorded kind run. These are proposed integration playbooks, not tested support certifications. [Recorded kind test findings](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/findings.md)

## Modes and prerequisites

**R: current rewrite model.** Map a single-label ngrok hostname to its existing per-hostname Service. Requires a compatible CoreDNS extension point and a DNS query path that actually traverses it.

**D: proposed DNS-service model.** Delegate the suffix to a DNS component that answers from reconciled hostname-to-Service-IP state. This component is not present in revision 2. Do not run D snippets until such a server is deployed and tested. `NGROK_DNS_IP` below means its verified, stable, reachable DNS address, not a forwarder IP. No hypothetical Helm flag is implied. [Revision-2 design](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/design.md)

For every platform, identify the DNS provider, DNS owner, cluster domain, release namespace, NodeLocal configuration, and IP families. Record a known ordinary cluster name, a public name, a private ngrok endpoint, and provider metadata names before changes. Confirm TCP/53 and UDP/53 reachability for DNS; the current private-dial data path separately needs UDP/443. [Kubernetes: DNS for Services and Pods](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/) [Google: NodeLocal DNSCache](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/nodelocal-dns-cache) [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md)

## Shared configuration fragments

### R: insert inside the existing default CoreDNS server block

Example namespace `ngrok-operator`, example cluster domain `cluster.local`; replace both for the actual installation:

```text
rewrite stop name regex ^([a-z][a-z0-9-]*)\.ngrok\.direct\.$ {1}-ngrok-direct.ngrok-operator.svc.cluster.local. answer auto
```

This is adapted from the POC rule and documented CoreDNS syntax. Do not append a second `.:53` block. Keep the existing `kubernetes`, `ready`, `health`, forwarding, and cache configuration. An `.internal` rule is deliberately omitted from the recommended starting configuration. [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md) [CoreDNS rewrite plugin](https://coredns.io/plugins/rewrite/)

### D: add a separate conditional-forwarding block

```text
ngrok.direct:53 {
    errors
    cache 5
    forward . <NGROK_DNS_IP>
}
```

Replace the placeholder with an actual DNS listener address. The five-second cache is a proposed starting choice, not a documented ngrok default. Existing resolver layers may impose their own caching. This must not point back to a resolver that delegates the same zone here. The pattern uses the documented Kubernetes/CoreDNS conditional-forwarding interface. [Kubernetes: customizing DNS](https://kubernetes.io/docs/tasks/administer-cluster/dns-custom-nameservers/) [CoreDNS cache plugin](https://coredns.io/plugins/cache/)

## 1. EKS with the managed CoreDNS add-on

**Supported integration surface:** the add-on's `configurationValues.corefile`; inspect the installed add-on schema. AWS explicitly warns that direct edits to `data.Corefile` can be overwritten on an add-on update. Both R and D are possible Corefile configurations, subject to query-path testing. [AWS: managing CoreDNS on EKS](https://docs.aws.amazon.com/eks/latest/userguide/managing-coredns.html) [AWS CLI EKS configuration examples](https://docs.aws.amazon.com/cli/latest/userguide/cli_eks_code_examples.html)

**Procedure:** export the existing add-on settings and Corefile; add R inside the root block or D outside it; retain the entire remainder. Merge the new Corefile into the existing configuration JSON rather than dropping settings such as replicas/resources. Apply through the owning IaC system, or the documented EKS update API when it is the owner. See the executable draft in [user docs](05-docs-outline-and-drafts.md#eks-managed-coredns).

**Permissions:** read/patch relevant Kubernetes objects as needed, plus AWS permission to describe configuration and update the add-on. These are separate from the application Helm installer's namespace rights. The normal ngrok controller should receive neither AWS add-on administration nor kube-system write access solely for DNS setup.

**Verify:** add-on health, CoreDNS logs/readiness, pod lookup through the ordinary resolver, lookup from each relevant node pool, normal Kubernetes DNS, and endpoint traffic. Repeat after an add-on update in a test cluster.

**Rollback:** remove only the ngrok stanza/rule from the add-on-owned Corefile and submit that configuration through the same owner. Do not restore an old entire ConfigMap while EKS/IaC continues asserting another value.

**Automation recommendation:** generate a configuration diff or Terraform input; do not mutate automatically in the initial release. Fargate, EKS Auto Mode, and node-local variants need their own exercised configurations before a support claim.

## 2. AKS

**Supported surface:** `kube-system/coredns-custom`, keys ending `.server` or `.override`. Microsoft documents both rewrite and custom forwarding; the managed main Corefile is not the customer edit surface. The docs specify a CoreDNS rolling restart to activate the examples. [Microsoft: customize CoreDNS on AKS](https://learn.microsoft.com/en-us/azure/aks/coredns-custom)

**D:** put the shared forwarding block in `data.ngrok.server`. Preserve other keys. **R candidate:** put the rewrite rule in a uniquely named `.override` key only after confirming its import location in the current Corefile. The exact ngrok rewrite through that import is an integration test, not a completed result. A standalone `.server` rewrite also needs an explicit route to a resolver capable of answering the rewritten Kubernetes name; an upstream VPC resolver is not automatically that resolver.

**Permissions:** scoped write to the custom ConfigMap and ability to restart/read the CoreDNS Deployment. **Ownership/upgrade:** use the documented custom surface, never the generated main configuration; keep the custom ConfigMap in customer IaC. No promise that a future provider release leaves every plugin behavior unchanged.

**Verify/rollback:** inspect imported configuration and pod resolver behavior; remove only `ngrok.server`/the owned override, restart CoreDNS, and retest baseline names. **Automation:** an explicit helper could eventually own one key; defer it until existing-key collision and upgrade tests pass.

## 3. GKE Standard with kube-dns

**Supported surface:** JSON `stubDomains` in `kube-system/kube-dns`. CoreDNS rewrite syntax cannot be inserted into kube-dns configuration. Use D, or a separately designed replacement-DNS deployment; do not replace managed cluster DNS merely to preserve a rewrite implementation. [Google: kube-dns configuration](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/kube-dns)

Merge this entry into existing `data.stubDomains` JSON:

```json
{"ngrok.direct": ["<NGROK_DNS_IP>"]}
```

Do not replace other suffix mappings or change `upstreamNameservers`. kube-dns reloads the ConfigMap. When NodeLocal is enabled, test its effective path too; GKE documents propagation of custom stub domains to node-local caches. [Google: kube-dns configuration](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/kube-dns) [Google: NodeLocal DNSCache](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/nodelocal-dns-cache)

**Permissions:** ConfigMap get/update/patch for the DNS administrator. **Upgrade/ownership:** this is the documented input surface; keep it in IaC and exercise a node/control-plane upgrade. Avoid edits to the managed kube-dns Deployment.

**Verify:** both TCP and UDP lookup to the ngrok DNS target, then the pod's usual resolver, plus GKE metadata names. **Rollback:** remove only `ngrok.direct` from the JSON map. **Automation:** possible later for this narrowly owned key, but concurrent writers and JSON merge behavior require tests.

## 4. GKE Standard with Cloud DNS for GKE

Google documents custom stub domains through the kube-dns ConfigMap for **Standard** clusters and says Cloud DNS translates them into forwarding zones. This differs from treating a cluster-local CoreDNS plugin as the resolver. [Google: Cloud DNS for GKE](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/about-cloud-dns) [Google: troubleshoot Cloud DNS](https://docs.cloud.google.com/kubernetes-engine/docs/troubleshooting/cloud-dns)

**D, with a reachability gate:** the same desired `stubDomains` entry can describe the integration, but do not assume a Service ClusterIP is a valid reachable target from the actual Cloud DNS forwarding path. Establish whether the query goes directly from NodeLocal or via Cloud DNS, and validate the forwarding target/network requirements. A VPC-reachable DNS frontend may be required; that is an inference to test, not an already implemented ngrok architecture. [Google: NodeLocal DNSCache](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/nodelocal-dns-cache) [Google Cloud DNS forwarding zones](https://docs.cloud.google.com/dns/docs/zones/zones-overview#forwarding_zones)

**Scope caution:** Google documents this custom DNS configuration as affecting nodes and pods, unlike kube-dns's pod-only configuration. Plan the baseline and rollback accordingly. [Google: Cloud DNS for GKE](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/about-cloud-dns)

**Permissions:** DNS ConfigMap access, potentially cloud-network/DNS administration for the chosen reachable target. **Verify:** node and pod resolution, effective forwarding zones, firewall paths, and metadata names. **Rollback:** remove the zone input through its owner; do not hand-edit generated Google-managed zones as a competing controller.

**Automation:** not until a supported target topology is proven. Do not label this ready solely because `stubDomains` is accepted.

## 5. GKE Autopilot

**Support gate, not an executable ngrok playbook yet.** The checked Cloud DNS documentation scopes custom `stubDomains` support to Standard. NodeLocal being enabled in Autopilot does not by itself establish that the same ConfigMap customization is permitted/supported there. [Google: Cloud DNS for GKE](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/about-cloud-dns) [Google: NodeLocal DNSCache](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/nodelocal-dns-cache)

**Next experiment:** use a disposable Autopilot cluster and the documented support path to determine the supported private-suffix integration; record admission behavior, resolver path, and required permissions. Do not instruct users to replace system DNS or deploy privileged node routing as a workaround. No automatic configuration is recommended before this is resolved.

## 6. OpenShift

**Supported surface:** `dns.operator/default`, `spec.servers`. The DNS Operator renders and manages the underlying CoreDNS configuration; editing `openshift-dns/dns-default` is not the proposed ownership model. Use D via conditional forwarding. [Red Hat: OpenShift DNS Operator](https://docs.redhat.com/en/documentation/openshift_container_platform/4.18/html/networking_operators/dns-operator)

Append an entry while preserving all existing `spec.servers`:

```yaml
- name: ngrok-private
  zones:
    - ngrok.direct
  forwardPlugin:
    policy: Sequential
    upstreams:
      - <NGROK_DNS_IP>
```

These are documented DNS Operator fields with an ngrok-specific zone/name. **Permissions:** cluster-level DNS Operator resource update, normally a cluster administrator. Do not add this to the ngrok runtime role.

**Verify:** `oc describe clusteroperators/dns`, generated configuration, DNS-to-target reachability, pod queries, endpoint traffic. **Rollback:** delete only the named `spec.servers` entry and let the DNS Operator reconcile. **Upgrade:** the custom resource is the supported persisted input; test target OpenShift/CNI combinations and upgrades. **Automation:** later, only with explicit admin approval and ownership/conflict checks; not direct ConfigMap editing.

## 7. K3s

The inspected pinned manifest mounts `coredns-custom` and imports `*.override` within the main block and `*.server` at top level. It also marks the main Corefile as K3s-managed. Use those imports rather than editing the packaged source. Verify the installed cluster has the same extension points; the pinned source does not prove every older release does. [K3s CoreDNS manifest](https://github.com/k3s-io/k3s/blob/82a6ddcd820bdfdd787455fae69e79afb5ef3102/manifests/coredns.yaml) [K3s packaged-component management](https://docs.k3s.io/installation/packaged-components)

**R:** put the rewrite rule into `ngrok.override`. **D:** put the forwarding block into `ngrok.server`. Do not configure both for the same suffix. **Permissions:** custom ConfigMap write and CoreDNS rollout/read access. **Verify:** inspect the mounted custom configuration, restart/reload behavior, normal resolver path, and traffic. **Rollback:** remove just the owned key and reload/restart. **Automation:** a narrow opt-in key owner is plausible after supported versions are identified.

## 8. RKE2

**Supported customization framework:** `HelmChartConfig` with the same name/namespace as the managed chart; `spec.valuesContent` is the documented payload. The package's precise CoreDNS value keys are chart-version-dependent and were not verified here. Do not publish an invented `customCorefile`, `servers`, or `extraConfig` setting. [RKE2 HelmChartConfig customization](https://docs.rke2.io/add-ons/helm)

**Procedure:** inspect `helmchart/rke2-coredns` and the installed chart version; read that chart's values/templates; create a matching HelmChartConfig containing the verified customization. Preserve its existing contents. Render the candidate and inspect the resulting Corefile before applying. Prefer a separate forwarding block when the chart supports it.

**Permissions:** HelmChartConfig administration in kube-system, plus the chosen configuration owner (possibly server-manifest management). **Upgrade:** persist in HelmChartConfig, not generated resources. **Rollback:** remove only the ngrok customization from the persisted owner and resource. RKE2 documents that deleting a server-manifest file alone does not remove its live resources. **Automation:** blocked on a versioned chart test matrix. [RKE2 HelmChartConfig customization](https://docs.rke2.io/add-ons/helm)

## 9. kubeadm / self-managed CoreDNS / kind

**Supported surface:** administrator-owned CoreDNS ConfigMap. R and D are ordinary CoreDNS configuration changes, but the installed plugins and any node-local path still need verification. Use the shared fragments above, retain existing configuration, and reload/restart according to the cluster's CoreDNS deployment. [Kubernetes: customizing DNS](https://kubernetes.io/docs/tasks/administer-cluster/dns-custom-nameservers/)

**Permissions:** scoped ConfigMap and rollout permissions. **Upgrade:** whoever upgrades CoreDNS must retain the custom input; ownership by a Helm chart or GitOps controller takes precedence over a manual edit. **Rollback:** remove only ngrok configuration through that owner.

For kind, the branch records an idempotent `make kind-private-endpoints-dns` script and successful revision-2 tests. That script is a development convenience, not a safe generic installer for all cluster layouts. It enables both suffixes, unlike the `.ngrok.direct`-first recommendation in these playbooks. [Recorded kind test findings](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/findings.md) [kind DNS configuration script](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/scripts/kind-private-endpoints-dns.sh)

## Shared acceptance checklist

Pass A and AAAA behavior, TCP and UDP DNS, unknown names, normal cluster/public/metadata resolution, per-node resolver routing, endpoint create/delete, provider update, and rollback. Test application lookup with normal search domains in addition to absolute `dig`. A successful DNS answer alone does not test private-dial authentication, QUIC egress, TLS verification, or TCP stream correctness.

## Sources

Sources were checked on **2026-10-12** unless the entry states otherwise. Provider documentation is evidence of the extension mechanism, not evidence that ngrok was tested on that platform.

- [Recorded kind test findings](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/findings.md) - Recorded tests: 2026-09-29, Kubernetes 1.36.1, CoreDNS 1.14.2; tests were not rerun for this bundle.
- [Revision-2 design](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/design.md) - Draft revision 2, 2026-09-29; not a released feature contract.
- [Kubernetes: DNS for Services and Pods](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/) - Rolling documentation checked 2026-10-12.
- [Google: NodeLocal DNSCache](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/nodelocal-dns-cache) - Rolling documentation checked 2026-10-12; provider/version limitations apply.
- [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md) - Branch excerpts reviewed in this conversation; POC notes dated 2026-09-29.
- [CoreDNS rewrite plugin](https://coredns.io/plugins/rewrite/) - Rolling documentation checked 2026-10-12; match deployed plugin version.
- [Kubernetes: customizing DNS](https://kubernetes.io/docs/tasks/administer-cluster/dns-custom-nameservers/) - Rolling documentation checked 2026-10-12; inspect installed DNS version.
- [CoreDNS cache plugin](https://coredns.io/plugins/cache/) - Rolling documentation checked 2026-10-12.
- [AWS: managing CoreDNS on EKS](https://docs.aws.amazon.com/eks/latest/userguide/managing-coredns.html) - Rolling documentation checked 2026-10-12; add-on-version-specific schema applies.
- [AWS CLI EKS configuration examples](https://docs.aws.amazon.com/cli/latest/userguide/cli_eks_code_examples.html) - Checked 2026-10-12; includes describe-addon-configuration and corefile schema.
- [Microsoft: customize CoreDNS on AKS](https://learn.microsoft.com/en-us/azure/aks/coredns-custom) - Page last updated 2025-10-22; checked 2026-10-12.
- [Google: kube-dns configuration](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/kube-dns) - Rolling documentation checked 2026-10-12; Standard-cluster scope.
- [Google: Cloud DNS for GKE](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/about-cloud-dns) - Rolling documentation checked 2026-10-12; custom stub-domain support described for Standard only.
- [Google: troubleshoot Cloud DNS](https://docs.cloud.google.com/kubernetes-engine/docs/troubleshooting/cloud-dns) - Rolling documentation checked 2026-10-12.
- [Google Cloud DNS forwarding zones](https://docs.cloud.google.com/dns/docs/zones/zones-overview#forwarding_zones) - Rolling documentation checked 2026-10-12; forwarding-target reachability must be validated.
- [Red Hat: OpenShift DNS Operator](https://docs.redhat.com/en/documentation/openshift_container_platform/4.18/html/networking_operators/dns-operator) - OpenShift Container Platform 4.18 documentation checked 2026-10-12.
- [K3s CoreDNS manifest](https://github.com/k3s-io/k3s/blob/82a6ddcd820bdfdd787455fae69e79afb5ef3102/manifests/coredns.yaml) - Pinned commit checked 2026-10-12; includes coredns-custom .override and .server imports.
- [K3s packaged-component management](https://docs.k3s.io/installation/packaged-components) - Rolling documentation checked 2026-10-12.
- [RKE2 HelmChartConfig customization](https://docs.rke2.io/add-ons/helm) - Page last updated 2026-10-07; checked 2026-10-12. Exact installed CoreDNS chart values not verified.
- [kind DNS configuration script](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/scripts/kind-private-endpoints-dns.sh) - Behavior described and tested in branch notes/findings.
