<!-- path: specs/private-endpoints/research/00-summary.md -->
# Private endpoints: research summary

**Date:** 2026-10-12. **Status:** reconstructed research draft, not a support announcement.

The earlier Deep Research result retained its summary and source list but not the seven downloadable file bodies. This bundle reconstructs the requested documents from that report, the branch excerpts reviewed in this conversation, and rechecked primary documentation. It is not a byte-for-byte recovery. No new live cluster tests were run.

## Recommendation

Keep revision 2's byte-stream data plane: one Service per hostname, endpoint ports mapped to forwarder listeners, and private dial to `hostname:port`. Keep CoreDNS rewrite for the kind POC and compatible clusters. For wider support, evaluate a **small DNS component plus administrator-configured conditional forwarding**, without restoring revision 1's HTTP Host/SNI inspection. This is a recommendation, not something the current branch already ships. [Revision-2 design](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/design.md)

Documentation-first is the agreed product direction. Do not grant the normal Helm release authority to rewrite cluster DNS. Start with `.ngrok.direct`; make `.internal` a separate, explicitly acknowledged choice. Preserve the plan to move generic stream-forwarding code into the SDK and to add per-connection workload identity later. [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md)

## What matters now

| Finding | Severity / disposition |
|---|---|
| CoreDNS rewrite is not a universal provider interface | High for a broad-support claim, not a POC blocker. GKE and OpenShift expose different integration surfaces. [Google: kube-dns configuration](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/kube-dns) [Red Hat: OpenShift DNS Operator](https://docs.redhat.com/en/documentation/openshift_container_platform/4.18/html/networking_operators/dns-operator) |
| `.internal` capture can collide with existing private DNS | High for customers using that suffix. Do not claim that excluding multi-label names protects every platform name: GKE also documents `metadata.internal`. [Google: kube-dns configuration](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/kube-dns) |
| Existing Service can be overwritten without an ownership check | High correctness risk. Refuse conflicting ownership rather than adopting it implicitly. [PrivateEndpoint controller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/privateendpoints/controller.go) |
| Forwarder-port reuse while another replica is stale can select the wrong target | High potential consequence; concurrency experiment required. This is different from a brief connection failure during initial setup. [PrivateEndpoint controller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/privateendpoints/controller.go) [Private-endpoint forwarder syncer](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/privateendpoints/forwarder/syncer.go) |
| Activation is eventually consistent | Accepted POC behavior. Document `Ready` semantics; do not invent an atomic readiness promise. [Recorded kind test findings](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/findings.md) |
| Port-range/listener limitations also exist in bindings | Confirmed structurally. Existing bindings has an allocatable range and separate forwarder reconciliation; exact capacity equivalence is not established. [Existing bindings poller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/bindings/boundendpoint_poller.go) [Existing bindings forwarder](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/bindings/forwarder_controller.go) |
| QUIC-only SDK and missing caller identity | Acknowledged concurrent development, not surprising architectural objections. [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md) |

## Platform direction

EKS: configure the managed add-on's full `corefile`, not an unmanaged patch. AKS: use `coredns-custom`. GKE Standard: investigate stub domains, separately testing NodeLocal and Cloud DNS forwarding reachability. OpenShift: use the DNS Operator API. K3s: use custom imports. RKE2: use version-matched HelmChartConfig values; exact DNS chart configuration remains a validation task. [AWS: managing CoreDNS on EKS](https://docs.aws.amazon.com/eks/latest/userguide/managing-coredns.html) [Microsoft: customize CoreDNS on AKS](https://learn.microsoft.com/en-us/azure/aks/coredns-custom) [Google: Cloud DNS for GKE](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/about-cloud-dns) [Google: NodeLocal DNSCache](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/nodelocal-dns-cache) [Red Hat: OpenShift DNS Operator](https://docs.redhat.com/en/documentation/openshift_container_platform/4.18/html/networking_operators/dns-operator) [K3s CoreDNS manifest](https://github.com/k3s-io/k3s/blob/82a6ddcd820bdfdd787455fae69e79afb5ef3102/manifests/coredns.yaml) [RKE2 HelmChartConfig customization](https://docs.rke2.io/add-ons/helm)

The closest prior art is Tailscale's explicit in-cluster nameserver plus admin-configured DNS forwarding. That validates a deployment boundary, not every implementation detail. [Tailscale: enable MagicDNS resolution in a cluster](https://tailscale.com/docs/kubernetes-operator/egress/enable-magicdns-resolution)

## Reading order

1. [Design review](01-design-review.md): severity, correctness, and bindings comparison.
2. [Prior art](02-prior-art.md): comparison matrix and limits of the evidence.
3. [Platform playbooks](03-platform-playbooks.md): ownership, configuration, rollback, and support gates.
4. [Recommendation](04-recommendation.md): staged delivery and alternatives.
5. [User documentation drafts](05-docs-outline-and-drafts.md): setup and troubleshooting pages.
6. [Open questions](06-open-questions.md): experiments before making support claims.

## Sources

Sources were checked on **2026-10-12** unless the entry states otherwise. Provider documentation is evidence of the extension mechanism, not evidence that ngrok was tested on that platform.

- [Revision-2 design](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/design.md) - Draft revision 2, 2026-09-29; not a released feature contract.
- [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md) - Branch excerpts reviewed in this conversation; POC notes dated 2026-09-29.
- [Google: kube-dns configuration](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/kube-dns) - Rolling documentation checked 2026-10-12; Standard-cluster scope.
- [Red Hat: OpenShift DNS Operator](https://docs.redhat.com/en/documentation/openshift_container_platform/4.18/html/networking_operators/dns-operator) - OpenShift Container Platform 4.18 documentation checked 2026-10-12.
- [PrivateEndpoint controller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/privateendpoints/controller.go) - Branch source reviewed in this conversation; moving branch link.
- [Private-endpoint forwarder syncer](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/privateendpoints/forwarder/syncer.go) - Branch source reviewed in this conversation; moving branch link.
- [Recorded kind test findings](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/findings.md) - Recorded tests: 2026-09-29, Kubernetes 1.36.1, CoreDNS 1.14.2; tests were not rerun for this bundle.
- [Existing bindings poller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/bindings/boundendpoint_poller.go) - Read 2026-10-12; blob 8e43acefb522669df0dfea25860372a664d32de8.
- [Existing bindings forwarder](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/bindings/forwarder_controller.go) - Read 2026-10-12; blob 04d7c38d6148817eae8d24a01c39b3d64b2f42ba.
- [AWS: managing CoreDNS on EKS](https://docs.aws.amazon.com/eks/latest/userguide/managing-coredns.html) - Rolling documentation checked 2026-10-12; add-on-version-specific schema applies.
- [Microsoft: customize CoreDNS on AKS](https://learn.microsoft.com/en-us/azure/aks/coredns-custom) - Page last updated 2025-10-22; checked 2026-10-12.
- [Google: Cloud DNS for GKE](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/about-cloud-dns) - Rolling documentation checked 2026-10-12; custom stub-domain support described for Standard only.
- [Google: NodeLocal DNSCache](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/nodelocal-dns-cache) - Rolling documentation checked 2026-10-12; provider/version limitations apply.
- [K3s CoreDNS manifest](https://github.com/k3s-io/k3s/blob/82a6ddcd820bdfdd787455fae69e79afb5ef3102/manifests/coredns.yaml) - Pinned commit checked 2026-10-12; includes coredns-custom .override and .server imports.
- [RKE2 HelmChartConfig customization](https://docs.rke2.io/add-ons/helm) - Page last updated 2026-10-07; checked 2026-10-12. Exact installed CoreDNS chart values not verified.
- [Tailscale: enable MagicDNS resolution in a cluster](https://tailscale.com/docs/kubernetes-operator/egress/enable-magicdns-resolution) - Page last validated 2026-06-05; checked 2026-10-12.
