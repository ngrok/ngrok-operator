<!-- path: specs/private-endpoints/research/02-prior-art.md -->
# Private endpoints: prior art

**Date:** 2026-10-12. **Scope:** architectural comparison, not a feature-ranking or universal platform-support matrix. "Not verified" means this research does not establish the field; it does not mean the product cannot do it. Sources and their version scope are listed at the end.

## Comparison matrix

| Product / mechanism | DNS mechanism | IP model | Privilege / control-plane owner | Platforms established here | Caller identity established here |
|---|---|---|---|---|---|
| Tailscale Kubernetes operator | `DNSConfig` deploys a nameserver; cluster DNS forwards `.ts.net` | Nameserver has a ClusterIP; answers direct clients toward configured egress proxies | Operator installation plus admin DNS change | Kubernetes; guide addresses CoreDNS and links kube-dns setup | Per-pod identity propagation not established by this DNS guide. [Tailscale: enable MagicDNS resolution in a cluster](https://tailscale.com/docs/kubernetes-operator/egress/enable-magicdns-resolution) |
| Consul DNS | Forward `.consul` to Consul DNS | Addresses reflect catalog/service-discovery mode | Consul installation and cluster DNS configuration | Kubernetes/CoreDNS and kube-dns configuration documented | DNS records alone do not establish authenticated caller identity. [Consul: DNS forwarding in Kubernetes](https://developer.hashicorp.com/consul/docs/manage/dns/forwarding/k8s) |
| Istio DNS proxy / ServiceEntry | DNS capture in mesh dataplane; registry-backed answers | Synthetic VIP allocation is available for service entries | Mesh/CNI or sidecar installation and interception | Istio-enabled Kubernetes; defaults depend on mode/version | Requires separate mesh identity/security analysis; not a property of the DNS answer. [Istio DNS proxying](https://istio.io/latest/docs/ops/configuration/traffic-management/dns-proxy/) |
| Linkerd multicluster | Mirrored Kubernetes Services and normal service discovery | Local representations route via the multicluster transport | Mesh and multicluster-controller installation | Linked Kubernetes clusters | Identity behavior depends on gateway versus direct-pod mode; do not equate them. [Linkerd multi-cluster communication](https://linkerd.io/2-edge/features/multicluster/) |
| Cilium ClusterMesh Global Services | Normal Kubernetes names plus global Service backend discovery | Local Service addresses with cross-cluster backends | Cilium and ClusterMesh network administration | Cilium-equipped clusters; development docs reviewed | No assertion of ngrok-like per-connection metadata from this source. [Cilium Global Services](https://docs.cilium.io/en/latest/network/clustermesh/global-services/) |
| Submariner Lighthouse | Separate DNS server; CoreDNS delegates `clusterset.local` | Answers built from ServiceImport/EndpointSlice state | Submariner/Lighthouse installation plus DNS integration | Submariner-connected Kubernetes clusters | Discovery does not itself authenticate a workload. [Submariner Lighthouse service discovery](https://submariner.io/getting-started/architecture/service-discovery/) |
| Multi-Cluster Services API | Standardizes service export/import concepts and discovery expectations | Implementation-dependent | Implementing controller and network system | Kubernetes implementations of MCS | No end-to-end identity transport specified by the overview. [Kubernetes SIG Multicluster: Services API](https://multicluster.sigs.k8s.io/concepts/multicluster-services-api/) |
| Cloudflare Tunnel on Kubernetes | Documented deployment of a connector toward cluster applications | Existing origin Service addresses | Workload deployment and Tunnel credentials | Kubernetes deployment guide | Tunnel deployment alone is not proof of pod-originated identity/egress DNS. [Cloudflare Tunnel deployment on Kubernetes](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/deployment-guides/kubernetes/) |
| Cloudflare WARP / client-side routing | Not sufficiently verified in this recovered research | Not verified | Not verified | Do not claim zero-sidecar pod support from Tunnel docs | Not verified |
| AWS PrivateLink interface endpoints | Private DNS can map a familiar service name to interface endpoints | VPC subnet IPs on endpoint interfaces | AWS VPC/IAM configuration; not a CoreDNS plugin | AWS VPC clients with connectivity | Endpoint connectivity is not itself Kubernetes pod identity. [AWS PrivateLink DNS and endpoint addresses](https://docs.aws.amazon.com/vpc/latest/privatelink/privatelink-access-aws-services.html) |
| AWS VPC Lattice | Service-network associations provide private service access | AWS-managed networking; detailed allocation not audited here | AWS service-network/VPC administration | Associated AWS networking environments | Kubernetes caller attribution not established by this source. [AWS VPC Lattice service-network associations](https://docs.aws.amazon.com/vpc-lattice/latest/ug/service-network-associations.html) |
| GCP Private Service Connect | Private connectivity integrated with consumer networking; DNS depends on endpoint variant | Consumer-side private endpoint model | Google Cloud network administration | GCP VPC clients with appropriate connectivity | Pod identity transport not established by overview. [Google Private Service Connect overview](https://docs.cloud.google.com/vpc/docs/private-service-connect) |
| Twingate | Exact Kubernetes client DNS path not verified | Not verified | Not verified | Not verified | Not verified |
| NetBird | Exact Kubernetes client DNS path not verified | Not verified | Not verified | Not verified | Not verified |
| ZeroTier | Exact Kubernetes client DNS path not verified | Not verified | Not verified | Not verified | Not verified |
| Teleport | A comparable transparent pod DNS/egress mode not verified | Not verified | Not verified | Not verified | Not verified |

The unverified rows deliberately preserve the limits of the recovered research rather than filling cells from product reputation. A future research pass should inspect client-side Kubernetes deployment instructions, not just connector charts or generic identity documentation.

## What to copy

### 1. A nameserver can be a small integration boundary

Tailscale is the closest match: its guide explains why an HTTPS client needs the original MagicDNS hostname, deploys an in-cluster nameserver, and delegates a suffix to that nameserver. It also requires the egress proxy to exist; DNS is not a substitute for installing the data path. [Tailscale: enable MagicDNS resolution in a cluster](https://tailscale.com/docs/kubernetes-operator/egress/enable-magicdns-resolution)

**Recommendation:** copy the split between product-owned name-to-address state and administrator-owned delegation. Do not assume we must copy its proxy/IP allocation model. Keep ngrok's Service-per-hostname model unless tests show a need to change it.

### 2. Separate discovery from transport

Lighthouse constructs DNS answers from imported Kubernetes resources and owns a dedicated discovery suffix. Consul similarly uses an explicit DNS forwarding boundary. [Submariner Lighthouse service discovery](https://submariner.io/getting-started/architecture/service-discovery/) [Consul: DNS forwarding in Kubernetes](https://developer.hashicorp.com/consul/docs/manage/dns/forwarding/k8s)

**Recommendation:** keep DNS as a view of reconciled routing state. It should not perform a private dial for every A/AAAA query, and DNS failure should not mutate the resource set. Unknown-name behavior must be designed rather than inherited accidentally from a generic authoritative server.

### 3. Synthetic VIPs solve a different complexity trade-off

Istio DNS proxying can allocate addresses and associate them with registry entries, which addresses ambiguous destinations without assigning a Kubernetes Service for every name. [Istio DNS proxying](https://istio.io/latest/docs/ops/configuration/traffic-management/dns-proxy/)

**Recommendation:** note the scaling option but do not copy it into this POC. ngrok currently benefits from native Service routing and avoids owning interception, synthetic-subnet routing, and CNI-specific behavior. Moving from Service IPs to arbitrary synthetic IPs requires a real packet-routing design, not just a different DNS response.

### 4. Cloud private connectivity validates stable names, not universal installation

PrivateLink allows applications to retain familiar AWS service names while resolving them to private endpoint interfaces. PSC provides private consumer-to-producer connectivity in GCP. [AWS PrivateLink DNS and endpoint addresses](https://docs.aws.amazon.com/vpc/latest/privatelink/privatelink-access-aws-services.html) [Google Private Service Connect overview](https://docs.cloud.google.com/vpc/docs/private-service-connect)

**Recommendation:** copy the user-facing distinction between service identity (hostname) and local access address. Avoid implying equivalent installation friction: these products depend on their cloud network control planes, whereas ngrok must integrate into customer-owned Kubernetes DNS.

## What to avoid

Do not label every server-side Kubernetes connector an equivalent client-egress product. A deployment that exposes a cluster application proves neither that arbitrary pods can resolve private names nor that their traffic is transparently captured. Cloudflare's Tunnel Kubernetes guide is relevant to deployment operations, but it is not enough to fill a WARP client-DNS matrix. [Cloudflare Tunnel deployment on Kubernetes](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/deployment-guides/kubernetes/)

Do not infer end-to-end workload identity from mesh presence, DNS permissions, or a shared gateway token. Compare the identity of the original caller, the proxy, and the session separately. For ngrok, the planned per-connection pod metadata and Traffic Policy enforcement remain an explicit future requirement. [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md)

Do not interpret Kubernetes-native as zero operational cost. MCS standardizes API concepts; it still requires a working implementation and intercluster network. Likewise, an ngrok CRD and Service do not configure managed DNS automatically. [Kubernetes SIG Multicluster: Services API](https://multicluster.sigs.k8s.io/concepts/multicluster-services-api/)

## Follow-up research questions

For Twingate, NetBird, ZeroTier, Teleport, and WARP, require a concrete official client-side Kubernetes manifest, resolver trace, privilege inventory, and policy identity example before filling the blank cells. Record whether the mode requires a sidecar, host-level routing, a CNI, per-service config, or application proxy variables. The comparison is useful only when those differences are explicit.

## Sources

Sources were checked on **2026-10-12** unless the entry states otherwise. Provider documentation is evidence of the extension mechanism, not evidence that ngrok was tested on that platform.

- [Tailscale: enable MagicDNS resolution in a cluster](https://tailscale.com/docs/kubernetes-operator/egress/enable-magicdns-resolution) - Page last validated 2026-06-05; checked 2026-10-12.
- [Consul: DNS forwarding in Kubernetes](https://developer.hashicorp.com/consul/docs/manage/dns/forwarding/k8s) - Rolling documentation checked 2026-10-12.
- [Istio DNS proxying](https://istio.io/latest/docs/ops/configuration/traffic-management/dns-proxy/) - Rolling latest documentation checked 2026-10-12; defaults depend on Istio version and mode.
- [Linkerd multi-cluster communication](https://linkerd.io/2-edge/features/multicluster/) - Edge documentation checked 2026-10-12; not a stable-version support guarantee.
- [Cilium Global Services](https://docs.cilium.io/en/latest/network/clustermesh/global-services/) - Page identifies 1.21.0-dev; checked 2026-10-12. Do not treat development examples as stable support.
- [Submariner Lighthouse service discovery](https://submariner.io/getting-started/architecture/service-discovery/) - Rolling project documentation checked 2026-10-12.
- [Kubernetes SIG Multicluster: Services API](https://multicluster.sigs.k8s.io/concepts/multicluster-services-api/) - Rolling SIG documentation checked 2026-10-12; an API, not a complete networking implementation.
- [Cloudflare Tunnel deployment on Kubernetes](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/deployment-guides/kubernetes/) - Rolling documentation checked 2026-10-12; server-side Tunnel deployment, not proof of transparent pod egress.
- [AWS PrivateLink DNS and endpoint addresses](https://docs.aws.amazon.com/vpc/latest/privatelink/privatelink-access-aws-services.html) - Rolling documentation checked 2026-10-12; interface endpoints for AWS services.
- [AWS VPC Lattice service-network associations](https://docs.aws.amazon.com/vpc-lattice/latest/ug/service-network-associations.html) - Rolling documentation checked 2026-10-12; not a Kubernetes identity integration audit.
- [Google Private Service Connect overview](https://docs.cloud.google.com/vpc/docs/private-service-connect) - Rolling documentation checked 2026-10-12; endpoint/backend variants differ.
- [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md) - Branch excerpts reviewed in this conversation; POC notes dated 2026-09-29.
