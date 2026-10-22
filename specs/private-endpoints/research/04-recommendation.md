<!-- path: specs/private-endpoints/research/04-recommendation.md -->
# Private endpoints: recommendation and staged plan

**Date:** 2026-10-12. **Status:** recommendation, not a committed product plan. The POC permits documented administrator setup, deferred pod identity, and concurrent SDK development. Those constraints are accepted here. [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md)

## Decision

Keep the revision-2 Service/private-dial data path. Ship documentation for known-compatible CoreDNS rewrite first. Prototype a small DNS service as the portability layer when customer-selected platforms require it. The saved research favored this DNS-service direction; it should be validated rather than declared an already-shipped replacement. [Revision-2 design](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/design.md)

Do not couple that DNS work to HTTP Host/SNI parsing. DNS can answer arbitrary supported hostnames with their existing Service IPs while the forwarder remains a byte-stream bridge. Once DNS owns the hostname mapping, Service names can be hash-based and need not encode the hostname. This removes the rewrite naming constraint, not every possible endpoint/certificate constraint.

## Alternatives

| Approach | Main benefit | Main cost | Recommendation |
|---|---|---|---|
| CoreDNS rewrite to Services | Minimal ngrok DNS code; uses native IP allocation | Specific DNS engine/path, restricted mechanical name mapping | Keep for the POC and validated CoreDNS deployments. |
| Product DNS service plus conditional forward | Encapsulates ngrok name rules; supports documented delegation interfaces | Another component, DNS correctness/availability, stable target reachability | Prototype as broad-support path. |
| Webhook sets pod `dnsConfig` | Workload-scoped enrollment without editing central CoreDNS | Admission installation, pod rollout, general resolver responsibility, resolver collisions | Secondary opt-in mode only if customers need it. |
| TUN/CNI/eBPF capture | Can use synthetic IPs and transparent routing | Privileged network integration and substantially broader maintenance | Not justified for the current POC. |

These trade-offs are engineering judgments. DNS and pod configuration mechanics are documented in [Kubernetes: DNS for Services and Pods](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/); CoreDNS rewriting in [CoreDNS rewrite plugin](https://coredns.io/plugins/rewrite/). Tailscale provides direct prior art for the separate-nameserver/delegation boundary. [Tailscale: enable MagicDNS resolution in a cluster](https://tailscale.com/docs/kubernetes-operator/egress/enable-magicdns-resolution)

### Why a webhook is not a free DNS substitute

A pod's nameserver list is not suffix-based split DNS. Merely adding another resolver does not mean the pod will ask it only for `ngrok.direct`. A reliable custom-resolver design must explicitly handle cluster names, search paths, upstream resolution, and failures. Admission installation still requires cluster-level privileges, and already running pods do not gain a new resolver from mutating future Pod creations. [Kubernetes: DNS for Services and Pods](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/)

**Recommendation:** only explore this when namespace opt-in is a stronger requirement than "every pod works after a one-time admin change." Use deliberate selection, a documented rollout, and safe behavior when the webhook is unavailable. DNS enrollment is not authorization.

## DNS service contract to prove

The proposed DNS component should answer from the existing reconciled Kubernetes state; it should not have a PAT merely to resolve names. It should support TCP and UDP, normalize case/trailing dots, return only configured address families, distinguish NXDOMAIN from NODATA, and expose health plus sync status. Define TTLs and stale-state behavior explicitly.

Prefer `.ngrok.direct` as the first delegated suffix. An `.internal` mode must have a separately documented collision and fallback policy. Do not forward unknown names to the same resolver that forwards the zone to this server. On GKE, validate both single-label and multi-label metadata names. [Google: kube-dns configuration](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/kube-dns)

A stable DNS frontend address is part of the installation contract. ClusterIP lifetime and Cloud DNS forwarding reachability cannot be hand-waved. Test upgrades, DNS-component deletion/recreation, and rollback before publishing provider-specific commands. [Google: Cloud DNS for GKE](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/about-cloud-dns) [Google Cloud DNS forwarding zones](https://docs.cloud.google.com/dns/docs/zones/zones-overview#forwarding_zones)

## Staged delivery

### Stage 0: finish the bounded POC

Keep kind and documented CoreDNS setup. Clearly label current readiness/capacity semantics and QUIC-only transport. Test Service ownership and wrong-destination reuse. Do not block on automatic cloud DNS edits or a completed caller-identity backend. [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md) [PrivateEndpoint controller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/privateendpoints/controller.go)

### Stage 1: documentation-first preview

Choose platforms based on actual customer demand. Publish supported configuration inputs, permission requirements, troubleshooting, rollback, and an honest tested matrix. Treat DNS setup as a cluster-admin task separate from the application Helm release. Keep the normal operator RBAC narrow.

### Stage 2: portable DNS prototype

Implement only the hostname-to-Service-address function. Validate at least one CoreDNS delegation path, GKE Standard's real stub path, and OpenShift DNS Operator integration. Add IPv6 only when the Service, listener, DNS, and upstream transport path all pass. Maintain a single data-plane implementation rather than two unrelated forwarders. [Google: kube-dns configuration](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/kube-dns) [Red Hat: OpenShift DNS Operator](https://docs.redhat.com/en/documentation/openshift_container_platform/4.18/html/networking_operators/dns-operator) [Kubernetes: IPv4/IPv6 dual stack](https://kubernetes.io/docs/concepts/services-networking/dual-stack/)

### Stage 3: targeted opt-in automation

Prefer a rendered patch or configuration export first. An automation adapter must identify the owner/version, preserve unrelated settings, reject conflicts, be idempotent, and remove only what it owns. EKS should integrate with add-on/IaC configuration, not mutate a generated ConfigMap. [AWS: managing CoreDNS on EKS](https://docs.aws.amazon.com/eks/latest/userguide/managing-coredns.html)

### Parallel tracks

**SDK:** move generic connection accept/dial/join, cancellation, half-close, error reporting, and drain semantics upstream; keep Kubernetes discovery, mapping, and identity extraction in the operator. The branch has already isolated the copy helper. [Private-dial stream forwarding](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/privateendpoints/forwarder/privatedial_forward.go)

**Identity:** propagate trustworthy original-caller metadata per connection and enforce endpoint policy through Traffic Policy. Validate source IP attribution under the supported Service/CNI paths; do not mistake a shared private-dial session identity for pod identity. Existing bindings offers a useful baseline implementation to compare. [Existing bindings forwarder](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/bindings/forwarder_controller.go)

## Exit criteria

A platform is supported only after resolution and traffic work through the actual application resolver, both positive/negative behavior is understood, ordinary platform DNS survives, the owner retains configuration through an update, and rollback works. Broad support is a test result, not a property conferred by publishing a Helm chart.

## Sources

Sources were checked on **2026-10-12** unless the entry states otherwise. Provider documentation is evidence of the extension mechanism, not evidence that ngrok was tested on that platform.

- [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md) - Branch excerpts reviewed in this conversation; POC notes dated 2026-09-29.
- [Revision-2 design](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/design.md) - Draft revision 2, 2026-09-29; not a released feature contract.
- [Kubernetes: DNS for Services and Pods](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/) - Rolling documentation checked 2026-10-12.
- [CoreDNS rewrite plugin](https://coredns.io/plugins/rewrite/) - Rolling documentation checked 2026-10-12; match deployed plugin version.
- [Tailscale: enable MagicDNS resolution in a cluster](https://tailscale.com/docs/kubernetes-operator/egress/enable-magicdns-resolution) - Page last validated 2026-06-05; checked 2026-10-12.
- [Google: kube-dns configuration](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/kube-dns) - Rolling documentation checked 2026-10-12; Standard-cluster scope.
- [Google: Cloud DNS for GKE](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/about-cloud-dns) - Rolling documentation checked 2026-10-12; custom stub-domain support described for Standard only.
- [Google Cloud DNS forwarding zones](https://docs.cloud.google.com/dns/docs/zones/zones-overview#forwarding_zones) - Rolling documentation checked 2026-10-12; forwarding-target reachability must be validated.
- [PrivateEndpoint controller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/privateendpoints/controller.go) - Branch source reviewed in this conversation; moving branch link.
- [Red Hat: OpenShift DNS Operator](https://docs.redhat.com/en/documentation/openshift_container_platform/4.18/html/networking_operators/dns-operator) - OpenShift Container Platform 4.18 documentation checked 2026-10-12.
- [Kubernetes: IPv4/IPv6 dual stack](https://kubernetes.io/docs/concepts/services-networking/dual-stack/) - Rolling documentation checked 2026-10-12.
- [AWS: managing CoreDNS on EKS](https://docs.aws.amazon.com/eks/latest/userguide/managing-coredns.html) - Rolling documentation checked 2026-10-12; add-on-version-specific schema applies.
- [Private-dial stream forwarding](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/privateendpoints/forwarder/privatedial_forward.go) - Branch source reviewed in this conversation; moving branch link.
- [Existing bindings forwarder](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/bindings/forwarder_controller.go) - Read 2026-10-12; blob 04d7c38d6148817eae8d24a01c39b3d64b2f42ba.
