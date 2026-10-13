<!-- path: specs/private-endpoints/research/06-open-questions.md -->
# Private endpoints: open questions and experiments

**Date:** 2026-10-12. **Status:** experiment backlog. None of the experiments below was executed while rebuilding this bundle. Use disposable/staging clusters and synthetic endpoints, not customer traffic.

The recorded kind findings are useful baseline evidence, not results for every provider, CNI, IP family, or upgrade path. [Recorded kind test findings](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/findings.md)

## Highest-priority correctness tests

| Question | Experiment | Acceptance evidence |
|---|---|---|
| Can a reused forwarder port select the wrong hostname? | Run two forwarders. Pause one watch, delete A, force its allocation to be reused by B, and send uniquely labeled TCP payloads through both old/new Service paths. Repeat under controller failover. | No request for B reaches A, or vice versa. Fail closed while state disagrees. Record CR versions, Service targetPorts, and replica table versions. |
| Can a stale Service IP resolve to an unrelated destination after reuse? | Keep a client DNS cache populated; delete A's Service, churn Services, and test the old address with a synthetic raw TCP server. | Explicit policy for IP lifetime/reuse and evidence that the supported path cannot silently reach an unrelated service. |
| Does the controller overwrite an unrelated Service? | Pre-create the deterministic Service name with a different owner, selector, and ports; reconcile the endpoint. | Existing object unchanged; clear conflict condition/event. |
| Can user-edited mirror state persist or redirect routing? | Mutate spec/status/labels using a deliberately delegated test role. Reconcile/restart. | RBAC/admission or reconciliation enforces the documented trust boundary; no unsupported adoption. |
| Is there an orphan cleanup gap? | Stop controllers, delete endpoint CRs, restart; repeat with label drift, finalizers, disable, and uninstall. | Managed orphan resources are found/handled according to a defined policy. |

These experiments follow from the reviewed separate Service/status/table update mechanisms, not from a claimed reproduction. [PrivateEndpoint controller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/privateendpoints/controller.go) [PrivateEndpoint poller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/privateendpoints/poller.go) [Private-endpoint forwarder syncer](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/privateendpoints/forwarder/syncer.go)

## Platform validation matrix

| Platform / topic | Suggested experiment | What remains unresolved |
|---|---|---|
| EKS managed CoreDNS | Apply through add-on `corefile`, perform a staging add-on update, and compare effective Corefile plus application DNS. | Exact tested add-on versions, IaC ownership workflows, and any Auto Mode/Fargate-specific differences. [AWS: managing CoreDNS on EKS](https://docs.aws.amazon.com/eks/latest/userguide/managing-coredns.html) |
| AKS | Compare `.override` rewrite with `.server` delegation on the installed Corefile; update/restart the add-on. | Exact rewrite import interaction and which mode is supported in the chosen release. [Microsoft: customize CoreDNS on AKS](https://learn.microsoft.com/en-us/azure/aks/coredns-custom) |
| GKE kube-dns + NodeLocal | Query through each node-local resolver and central kube-dns; capture destination of forwarded requests. | Effective behavior by GKE version and dataplane. [Google: NodeLocal DNSCache](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/nodelocal-dns-cache) |
| GKE Cloud DNS | Test stubDomain forwarding to the chosen DNS frontend from the actual provider path, with and without NodeLocal. | Whether ClusterIP is reachable or a VPC-reachable frontend is required; firewall and source-range constraints. [Google: Cloud DNS for GKE](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/about-cloud-dns) [Google Cloud DNS forwarding zones](https://docs.cloud.google.com/dns/docs/zones/zones-overview#forwarding_zones) |
| GKE Autopilot | Obtain the documented allowed integration and run a disposable-cluster test; record admission and resolver behavior. | Supported custom-suffix path; Standard's ConfigMap recipe is not an Autopilot guarantee. [Google: Cloud DNS for GKE](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/about-cloud-dns) |
| OpenShift | Configure DNS Operator forwarding, query from pods/nodes, and perform a supported update. | DNS-server reachability from the platform resolver and supported SCC/CNI/topology combinations. [Red Hat: OpenShift DNS Operator](https://docs.redhat.com/en/documentation/openshift_container_platform/4.18/html/networking_operators/dns-operator) |
| K3s | Verify imports on oldest/newest proposed support versions; update K3s with customization installed. | Which released versions expose the inspected manifest's imports. [K3s CoreDNS manifest](https://github.com/k3s-io/k3s/blob/82a6ddcd820bdfdd787455fae69e79afb5ef3102/manifests/coredns.yaml) |
| RKE2 | Pin chart version; render HelmChartConfig with the chart's documented DNS values; inspect generated Corefile and test an update/rollback. | Exact supported chart keys and version matrix. [RKE2 HelmChartConfig customization](https://docs.rke2.io/add-ons/helm) |

## DNS behavior tests

**Name safety:** compare baseline and enabled results for existing `db.internal`, GKE `metadata.internal`, `metadata.google.internal`, mixed case, trailing dots, invalid/long labels, multi-label names, and unknown names. For a future DNS server, decide which suffixes are authoritative and which queries can fall through. Prove the fallback path cannot loop. [Google: kube-dns configuration](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/kube-dns) [CoreDNS rewrite plugin](https://coredns.io/plugins/rewrite/)

**Resolver behavior:** run application lookups from representative glibc, musl, Go, JVM, Node, and Python images; compare absolute DNS queries, normal search behavior, and app-level caching. Measure initial creation after a cached NXDOMAIN and deletion after a cached positive response. Record every TTL/cache layer rather than reporting only the poll interval. [Kubernetes: DNS for Services and Pods](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/) [CoreDNS cache plugin](https://coredns.io/plugins/cache/)

**IPv6:** use IPv4-only, IPv6-only, and dual-stack clusters. Inspect Service `clusterIPs`/families, backend addresses and listeners, A/AAAA behavior, and private-dial gateway reachability. Do not promote support based only on an AAAA response. [Kubernetes: IPv4/IPv6 dual stack](https://kubernetes.io/docs/concepts/services-networking/dual-stack/)

**DNS component contract:** test TCP/UDP, unsupported types, NODATA versus NXDOMAIN, truncation behavior, zero-CR startup, API outage, readiness, and stable DNS Service address across reinstall. Test query volume without making a remote endpoint lookup/dial for every query.

## Capacity, lifecycle, and SDK tests

Create 100, 300, 1000, and 1001 endpoint-port assignments in an isolated account/cluster; measure resource counts, API QPS, convergence, memory, descriptors, listener failures, and traffic while churning endpoints. More replicas should not be advertised as increasing the port-range capacity. Compare the existing bindings allocator and forwarder under the same workload, not only their configuration values. [PrivateEndpoint controller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/privateendpoints/controller.go) [Existing bindings poller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/bindings/boundendpoint_poller.go) [Existing bindings forwarder](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/bindings/forwarder_controller.go)

For the SDK boundary, exercise client half-close with a response delayed beyond five seconds; long-lived bidirectional TCP; context cancellation; daemon shutdown; remote session loss; token rotation; and QUIC-blocked networks. Decide the desired behavior before upstreaming a shared `Join`/`Forward` API. Those API names in the branch comments are proposals, not verified exported SDK methods. [Private-dial stream forwarding](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/privateendpoints/forwarder/privatedial_forward.go) [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md)

## Identity and installation questions

Agree on the future authenticated identity fields and trust model. Test pod-IP lookup under SNAT, host-network pods, node-local/Service datapaths, pod churn, and dual-stack. Reject ambiguous attribution rather than attaching another workload's identity. The current old-bindings implementation is a comparison baseline, not a proof that every source-address topology is safe. [Existing bindings forwarder](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/bindings/forwarder_controller.go)

Define whether multiple ngrok installations/accounts can coexist with one global suffix. Test namespace/account changes and uninstall ordering. Until a scoped DNS selection model exists, document one DNS owner per suffix/cluster as a proposed preview constraint.

## Prior-art evidence still missing

The recovered research did not establish detailed client-side Kubernetes DNS, privilege, platform, and identity behavior for Twingate, NetBird, ZeroTier, Teleport, or WARP. Obtain official deployment manifests and an end-to-end caller-flow example for each before filling those comparison cells. Server-side connector deployment alone is insufficient evidence for transparent pod egress. [Cloudflare Tunnel deployment on Kubernetes](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/deployment-guides/kubernetes/)

## Release support record

For each passing platform test, store exact Kubernetes/provider DNS/CNI/operator/SDK versions, manifests, sanitized resolver traces, source and destination IP families, create/delete timings, upgrade results, and rollback steps. Mark unsupported combinations explicitly. Keep implementation TODOs separate from product decisions already accepted for the POC.

## Sources

Sources were checked on **2026-10-12** unless the entry states otherwise. Provider documentation is evidence of the extension mechanism, not evidence that ngrok was tested on that platform.

- [Recorded kind test findings](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/findings.md) - Recorded tests: 2026-09-29, Kubernetes 1.36.1, CoreDNS 1.14.2; tests were not rerun for this bundle.
- [PrivateEndpoint controller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/privateendpoints/controller.go) - Branch source reviewed in this conversation; moving branch link.
- [PrivateEndpoint poller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/privateendpoints/poller.go) - Branch source reviewed in this conversation; moving branch link.
- [Private-endpoint forwarder syncer](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/privateendpoints/forwarder/syncer.go) - Branch source reviewed in this conversation; moving branch link.
- [AWS: managing CoreDNS on EKS](https://docs.aws.amazon.com/eks/latest/userguide/managing-coredns.html) - Rolling documentation checked 2026-10-12; add-on-version-specific schema applies.
- [Microsoft: customize CoreDNS on AKS](https://learn.microsoft.com/en-us/azure/aks/coredns-custom) - Page last updated 2025-10-22; checked 2026-10-12.
- [Google: NodeLocal DNSCache](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/nodelocal-dns-cache) - Rolling documentation checked 2026-10-12; provider/version limitations apply.
- [Google: Cloud DNS for GKE](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/about-cloud-dns) - Rolling documentation checked 2026-10-12; custom stub-domain support described for Standard only.
- [Google Cloud DNS forwarding zones](https://docs.cloud.google.com/dns/docs/zones/zones-overview#forwarding_zones) - Rolling documentation checked 2026-10-12; forwarding-target reachability must be validated.
- [Red Hat: OpenShift DNS Operator](https://docs.redhat.com/en/documentation/openshift_container_platform/4.18/html/networking_operators/dns-operator) - OpenShift Container Platform 4.18 documentation checked 2026-10-12.
- [K3s CoreDNS manifest](https://github.com/k3s-io/k3s/blob/82a6ddcd820bdfdd787455fae69e79afb5ef3102/manifests/coredns.yaml) - Pinned commit checked 2026-10-12; includes coredns-custom .override and .server imports.
- [RKE2 HelmChartConfig customization](https://docs.rke2.io/add-ons/helm) - Page last updated 2026-10-07; checked 2026-10-12. Exact installed CoreDNS chart values not verified.
- [Google: kube-dns configuration](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/kube-dns) - Rolling documentation checked 2026-10-12; Standard-cluster scope.
- [CoreDNS rewrite plugin](https://coredns.io/plugins/rewrite/) - Rolling documentation checked 2026-10-12; match deployed plugin version.
- [Kubernetes: DNS for Services and Pods](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/) - Rolling documentation checked 2026-10-12.
- [CoreDNS cache plugin](https://coredns.io/plugins/cache/) - Rolling documentation checked 2026-10-12.
- [Kubernetes: IPv4/IPv6 dual stack](https://kubernetes.io/docs/concepts/services-networking/dual-stack/) - Rolling documentation checked 2026-10-12.
- [Existing bindings poller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/bindings/boundendpoint_poller.go) - Read 2026-10-12; blob 8e43acefb522669df0dfea25860372a664d32de8.
- [Existing bindings forwarder](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/bindings/forwarder_controller.go) - Read 2026-10-12; blob 04d7c38d6148817eae8d24a01c39b3d64b2f42ba.
- [Private-dial stream forwarding](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/privateendpoints/forwarder/privatedial_forward.go) - Branch source reviewed in this conversation; moving branch link.
- [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md) - Branch excerpts reviewed in this conversation; POC notes dated 2026-09-29.
- [Cloudflare Tunnel deployment on Kubernetes](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/deployment-guides/kubernetes/) - Rolling documentation checked 2026-10-12; server-side Tunnel deployment, not proof of transparent pod egress.
