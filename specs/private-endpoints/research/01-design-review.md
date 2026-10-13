<!-- path: specs/private-endpoints/research/01-design-review.md -->
# Private endpoints: design review

**Scope:** revision-2 POC on `alex/private-endpoints-poc`. **Evidence:** branch source excerpts and recorded kind tests; platform documentation checked 2026-10-12. **Not performed:** execution, race testing, load testing, or an exhaustive PR audit.

**Severity:** High = possible wrong destination, ownership/security violation, or invalid platform-support claim. Medium = outage/operational limitation needing a defined contract. Low = documentation or polish. A high production consequence is not automatically a reason to stop a bounded POC.

## 1. What is sound

The design retains the hostname by allocating one local Service IP per hostname, then preserves the destination port through a dedicated listener mapping. It does not need to parse application protocols. Recorded tests demonstrate HTTP, HTTPS with verification, TLS passthrough, Redis/TCP, two ports on one hostname, and separation between the two suffixes. These are recorded tests, not tests rerun for this review. [Revision-2 design](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/design.md) [Recorded kind test findings](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/findings.md)

The API error behavior is sensible: finish listing before changing the mirrored resource set, and keep existing resources if listing fails. Pooled endpoint URLs collapse to one CR. [PrivateEndpoint poller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/privateendpoints/poller.go)

## 2. Findings

### H1. Existing-Service ownership is not checked

**Verified in reviewed code:** `CreateOrUpdate` selects a deterministic Service name, then assigns its labels, selector, and ports without first rejecting a Service owned by someone else. A pre-existing `foo-internal` could be redirected. [PrivateEndpoint controller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/privateendpoints/controller.go)

**Recommendation:** only mutate objects belonging to this installation and hostname. Report a conflict when ownership differs. Include an installation identity, not only a generic controller label. Namespace isolation reduces the likelihood but does not make takeover correct. Test with an unrelated Service before reconciliation.

### H2. Endpoint identity is not protected by eventual consistency alone

**Verified mechanism:** Services and CR status are written separately; forwarders independently rebuild a listener-port-to-target table. The table is consulted at accept time. [PrivateEndpoint controller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/privateendpoints/controller.go) [Private-endpoint forwarder syncer](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/privateendpoints/forwarder/syncer.go)

**Inference, not a reproduced bug:** after endpoint A is deleted, its listener port can be reassigned to B while a forwarder replica still maps that port to A. A new connection for B can reach the stale replica. Conversely, an old Service mapping can reach a replica already mapping the port to B. Uncached allocation avoids one allocator race but does not create a cross-resource or cross-replica transaction.

**Recommendation:** make "never connect to a different hostname" an invariant. Test deletion/reallocation under a paused watch. Consider stable assignments, a reuse quarantine, or an explicit assignment epoch/acknowledgment design. A fixed delay reduces exposure but is not a proof when watches can stall indefinitely. Dropping a connection is preferable to cross-routing raw TCP.

### H3. DNS capture has a larger boundary than endpoint discovery

The current rewrite selects syntactically matching names, not only known ngrok endpoints. An unknown matching name is redirected into Kubernetes and can return NXDOMAIN. This intercepts unrelated single-label `.internal` names. [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md) [CoreDNS rewrite plugin](https://coredns.io/plugins/rewrite/)

**Platform-specific correction:** preserving `metadata.google.internal` is useful but insufficient as a universal GKE safety argument. Google also documents the single-label `metadata.internal` name. Do not enable `.internal` capture on GKE without enumerating and testing platform-dependent names. [Google: kube-dns configuration](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/kube-dns)

**Recommendation:** `.ngrok.direct` first; separate opt-in for `.internal`; collision preflight and a documented rollback. For a future DNS component, specify whether unknown `.internal` names fall through, and ensure the fallback resolver cannot forward the same query back into ngrok.

### H4. Rewrites cannot be the universal installation interface

GKE's native kube-dns interface is `stubDomains`; Cloud DNS is a different execution path. OpenShift's supported customization is the DNS Operator's `spec.servers`. EKS manages Corefile ownership through its add-on. [Google: kube-dns configuration](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/kube-dns) [Google: Cloud DNS for GKE](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/about-cloud-dns) [Red Hat: OpenShift DNS Operator](https://docs.redhat.com/en/documentation/openshift_container_platform/4.18/html/networking_operators/dns-operator) [AWS: managing CoreDNS on EKS](https://docs.aws.amazon.com/eks/latest/userguide/managing-coredns.html)

**Disposition:** accepted documentation-first work, not an objection to the POC. Define the product interface as "resolve this suffix through ngrok" rather than "patch CoreDNS". Do not claim portable support based only on successful kind runs.

### M1. Readiness means configured, not end-to-end verified

The controller marks a resource ready after assigning a Service IP and listener port. Forwarder process readiness checks initial table sync; the syncer replaces the table before attempting every listener bind. It is therefore possible to have configured status without a usable listener or successful private dial. [PrivateEndpoint controller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/privateendpoints/controller.go) [Private-endpoint forwarder syncer](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/privateendpoints/forwarder/syncer.go) [Forwarder command and readiness](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/cmd/private-endpoint-forwarder.go)

**Disposition:** an initial setup-to-use gap is explicitly acceptable. Document it. Improve failure visibility and ensure bind failures are not hidden by a previously successful initial sync. Do not make liveness depend on ngrok reachability, since a remote outage should not create restart loops.

### M2. Allocation and discovery are bounded

The reviewed range is 20000-20999: 1000 distinct listener assignments. This is a POC setting, not a Kubernetes port limit. One hostname consumes one Service; multiple endpoint ports on it consume additional listeners. Every forwarder replica needs the assignments, so adding replicas increases traffic capacity, not the endpoint-count ceiling. [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md) [PrivateEndpoint controller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/privateendpoints/controller.go) [Private-endpoint forwarder syncer](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/privateendpoints/forwarder/syncer.go)

At exhaustion, expose an understandable condition, preserve already working endpoints, and avoid infinite high-rate retries. Measure reconciliation latency, API reads/writes, Service and EndpointSlice count, file descriptors, and active connections. Listing all CRs on every hostname reconcile can become expensive; benchmark before replacing the simple allocator.

### M3. Namespace and installation ownership need a contract

**Inference:** cluster-wide suffix configuration can select only one canonical destination for a hostname. Two installations using different accounts or namespaces cannot independently own the same suffix without additional scoping. Treat one DNS owner per suffix/cluster as an initial constraint. The operator namespace is a trust boundary: write access to routing CRs or their status must not be treated as harmless configuration permission. [Revision-2 design](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/design.md) [Private-endpoint forwarder syncer](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/privateendpoints/forwarder/syncer.go)

### M4. Reconciliation must heal drift

The reviewed poller considers a desired CR present when its name exists; it does not restore an edited spec. Service deletion uses labels rather than a per-host owner resource. [PrivateEndpoint poller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/privateendpoints/poller.go) [PrivateEndpoint controller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/privateendpoints/controller.go)

**Recommendation:** either enforce immutable, operator-owned specs or reconcile them back to the source of truth. Test restart after deletion, finalizers, missing events, label drift, and disable/uninstall cleanup. Do not assume a deletion event is a durable garbage-collection mechanism.

### M5. Connection lifecycle belongs in the SDK, with a precise contract

The copy helper half-closes each direction and allows a five-second drain after one direction finishes. Copy errors are discarded by the reviewed helper. [Private-dial stream forwarding](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/privateendpoints/forwarder/privatedial_forward.go)

**Recommendation:** keep the operator adapter thin and upstream the generic join/forward behavior, as intended. Add tests for half-close followed by a response taking longer than five seconds, context cancellation, rolling shutdown, and reconnects. A generic TCP stream may legally continue in one direction after the other has ended; document any imposed timeout. QUIC-only egress and the SDK feature-branch pin are known POC dependencies, not newly discovered blockers. [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md)

## 3. DNS and networking details

**NodeLocal DNSCache - Medium:** the central DNS Service is not necessarily on the query path for external-looking names. Compare a query through the pod's configured resolver with a direct query to central CoreDNS. On GKE, NodeLocal explicitly handles stub-domain forwarding and requires both TCP and UDP reachability. [Kubernetes: NodeLocal DNSCache](https://kubernetes.io/docs/tasks/administer-cluster/nodelocaldns/) [Google: NodeLocal DNSCache](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/nodelocal-dns-cache)

**Search domains / ndots - Medium:** normal resolver calls can try search-expanded names before the absolute name. Test the application resolver, not only `dig`. Use a trailing dot in diagnostic DNS queries; do not prescribe trailing dots in application HTTPS URLs as a universal fix. Avoid a cluster-wide `ndots` change for one feature. [Kubernetes: DNS for Services and Pods](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/)

**Negative caching - Medium:** querying an endpoint before creation can extend the apparent provisioning delay. Positive caches can retain a deleted Service IP. CoreDNS caches positive, denial, and some failure responses according to configuration. Test positive and negative TTL behavior at every resolver hop, plus application caches. Do not claim a single global propagation time from the API polling interval. [CoreDNS cache plugin](https://coredns.io/plugins/cache/)

**IPv6 - High for an IPv6-support claim:** the current design exposes one `clusterIP` status value, while dual-stack Services can have multiple addresses. The branch notes also describe IPv4 wildcard listener binding. IPv4 kind results do not establish IPv6-only or dual-stack operation. Verify A/AAAA answers, Service address family, backend pod addresses, listeners, and private-dial gateway reachability independently. Return NODATA rather than fabricated addresses for unsupported record types. [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md) [Kubernetes: IPv4/IPv6 dual stack](https://kubernetes.io/docs/concepts/services-networking/dual-stack/)

**DNS security versus authorization - Medium:** not knowing a DNS name does not prevent direct Service or listener access. Kubernetes NetworkPolicy can still restrict pod/namespace/IP/port traffic; the feature does not disable NetworkPolicy. What is lost is straightforward remote-endpoint/workload attribution through a shared proxy. Do not describe DNS opt-in as an access-control boundary. [Kubernetes: NetworkPolicy](https://kubernetes.io/docs/concepts/services-networking/network-policies/)

**TLS wording - Low documentation correction:** `.internal` is not eligible for the ordinary vendor public-certificate story, but private-CA validation is possible for end-to-end TLS when clients trust that CA. Do not teach users that disabling verification is inherently required for all `.internal` usage. The recorded publicly trusted certificate test applies to the tested `.ngrok.direct` HTTPS endpoint; arbitrary deeper names need a separate certificate-coverage check. [Recorded kind test findings](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/findings.md)

## 4. Comparison with existing bindings

| Property | Existing bindings | Revision 2 | Conclusion |
|---|---|---|---|
| Polling/control-plane convergence | Poller and separate controllers | Poller, hostname controller, forwarder watch | Eventual activation is not novel. |
| Finite allocatable forwarding ports | `PortRange` and bitmap allocator | Fixed reviewed range and CR-status allocations | The class of scale limit already exists; exact maxima are not asserted equal. |
| Separate forwarder reconciliation | Watches BoundEndpoint changes | Watches PrivateEndpoint changes | Neither fact alone proves synchronized readiness across replicas. |
| Workload identity | Existing forwarder looks up source pod IP and constructs pod identity | Deferred private-dial metadata | An acknowledged functional gap, not a reason to reject the POC. |

Evidence for the old implementation: [Existing bindings poller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/bindings/boundendpoint_poller.go) [Existing bindings forwarder](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/bindings/forwarder_controller.go). Compare deployment defaults and churn tests before claiming equal capacity, equal identity guarantees, or identical race behavior.

## 5. Near-term priorities

First test Service ownership and wrong-destination races. Then document current readiness and capacity semantics. Validate DNS routes on the first customer-selected platforms. Keep pod identity/backend policy and the generic SDK work as explicit parallel tracks. Missing automated provider configuration is not a POC failure.

## Sources

Sources were checked on **2026-10-12** unless the entry states otherwise. Provider documentation is evidence of the extension mechanism, not evidence that ngrok was tested on that platform.

- [Revision-2 design](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/design.md) - Draft revision 2, 2026-09-29; not a released feature contract.
- [Recorded kind test findings](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/findings.md) - Recorded tests: 2026-09-29, Kubernetes 1.36.1, CoreDNS 1.14.2; tests were not rerun for this bundle.
- [PrivateEndpoint poller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/privateendpoints/poller.go) - Branch source reviewed in this conversation; moving branch link.
- [PrivateEndpoint controller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/privateendpoints/controller.go) - Branch source reviewed in this conversation; moving branch link.
- [Private-endpoint forwarder syncer](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/privateendpoints/forwarder/syncer.go) - Branch source reviewed in this conversation; moving branch link.
- [Branch notes and decision log](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/specs/private-endpoints/notes.md) - Branch excerpts reviewed in this conversation; POC notes dated 2026-09-29.
- [CoreDNS rewrite plugin](https://coredns.io/plugins/rewrite/) - Rolling documentation checked 2026-10-12; match deployed plugin version.
- [Google: kube-dns configuration](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/kube-dns) - Rolling documentation checked 2026-10-12; Standard-cluster scope.
- [Google: Cloud DNS for GKE](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/about-cloud-dns) - Rolling documentation checked 2026-10-12; custom stub-domain support described for Standard only.
- [Red Hat: OpenShift DNS Operator](https://docs.redhat.com/en/documentation/openshift_container_platform/4.18/html/networking_operators/dns-operator) - OpenShift Container Platform 4.18 documentation checked 2026-10-12.
- [AWS: managing CoreDNS on EKS](https://docs.aws.amazon.com/eks/latest/userguide/managing-coredns.html) - Rolling documentation checked 2026-10-12; add-on-version-specific schema applies.
- [Forwarder command and readiness](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/cmd/private-endpoint-forwarder.go) - Branch source reviewed in this conversation; moving branch link.
- [Private-dial stream forwarding](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/privateendpoints/forwarder/privatedial_forward.go) - Branch source reviewed in this conversation; moving branch link.
- [Kubernetes: NodeLocal DNSCache](https://kubernetes.io/docs/tasks/administer-cluster/nodelocaldns/) - Rolling documentation checked 2026-10-12; deployed Corefile remains authoritative.
- [Google: NodeLocal DNSCache](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/nodelocal-dns-cache) - Rolling documentation checked 2026-10-12; provider/version limitations apply.
- [Kubernetes: DNS for Services and Pods](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/) - Rolling documentation checked 2026-10-12.
- [CoreDNS cache plugin](https://coredns.io/plugins/cache/) - Rolling documentation checked 2026-10-12.
- [Kubernetes: IPv4/IPv6 dual stack](https://kubernetes.io/docs/concepts/services-networking/dual-stack/) - Rolling documentation checked 2026-10-12.
- [Kubernetes: NetworkPolicy](https://kubernetes.io/docs/concepts/services-networking/network-policies/) - Rolling documentation checked 2026-10-12; enforcement depends on the network implementation.
- [Existing bindings poller](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/bindings/boundendpoint_poller.go) - Read 2026-10-12; blob 8e43acefb522669df0dfea25860372a664d32de8.
- [Existing bindings forwarder](https://github.com/ngrok/ngrok-operator/blob/alex/private-endpoints-poc/internal/controller/bindings/forwarder_controller.go) - Read 2026-10-12; blob 04d7c38d6148817eae8d24a01c39b3d64b2f42ba.
