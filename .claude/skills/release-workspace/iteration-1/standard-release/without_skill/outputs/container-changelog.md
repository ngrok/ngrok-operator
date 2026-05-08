## 0.21.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-0.20.3...ngrok-operator-0.21.0

### Added

- Add `k8s.ngrok.com/metadata` and `k8s.ngrok.com/description` annotations to Ingress and Gateway resources, propagated to generated CloudEndpoints and AgentEndpoints by @sabrina-ngrok in [#788](https://github.com/ngrok/ngrok-operator/pull/788)
- Show the Ready condition's reason and message in `-o wide` output for all CRDs by @alex-bezek in [#772](https://github.com/ngrok/ngrok-operator/pull/772)

### Changed

- Overhauled RBAC across all operator deployments: hand-managed Helm templates replace controller-gen markers, reorganized into per-component directories, and added namespace-scoping support when `watchNamespace` is set by @alex-bezek in [#804](https://github.com/ngrok/ngrok-operator/pull/804)
- Renamed `endpointURI` to `endpointURL` in BoundEndpoint CRD (old field deprecated, still accepted) by @sabrina-ngrok in [#779](https://github.com/ngrok/ngrok-operator/pull/779)
- Dependency updates by @jonstacks in [#785](https://github.com/ngrok/ngrok-operator/pull/785)

### Fixed

- Fixed CloudEndpoint controller to skip redundant ngrok API updates when nothing has changed, reducing unnecessary API calls by @jonstacks in [#806](https://github.com/ngrok/ngrok-operator/pull/806)
- Fixed CloudEndpoint to use ReconcileStatus for IsNotFound handling, matching the standard pattern used by other controllers by @alex-bezek in [#799](https://github.com/ngrok/ngrok-operator/pull/799)
- Fixed Gateway controller: inverted validation logic, informer cache corruption from mutating cached objects, and cross-namespace HTTPRoute matching by @alex-bezek in [#798](https://github.com/ngrok/ngrok-operator/pull/798)
- Fixed KubernetesOperator controller: error swallowing in `findExisting`, nil pointer panics for optional Binding/Deployment specs, and unreachable TLS secret validation by @alex-bezek in [#797](https://github.com/ngrok/ngrok-operator/pull/797)
- Fixed manager driver: lost Gateway listener status conditions from value copies, nondeterministic Gateway/Ingress status ordering causing spurious updates, and informer cache corruption in cert ref translation by @alex-bezek in [#800](https://github.com/ngrok/ngrok-operator/pull/800)
- Fixed agent driver: context propagation in Forward(), panic on double channel close, stale TrafficPolicyApplied condition, and mergeEnabled logic by @alex-bezek in [#802](https://github.com/ngrok/ngrok-operator/pull/802)
- Fixed bindings package: replaced panic calls with error returns in port allocator, fixed data race on portAllocator reassignment, and nil pointer in forwarder logging by @alex-bezek in [#801](https://github.com/ngrok/ngrok-operator/pull/801)
- Fixed data race in ChannelHealthChecker, swallowed API pagination errors in findReservedDomainByHostname, and several minor error message bugs by @alex-bezek in [#803](https://github.com/ngrok/ngrok-operator/pull/803)
- Fixed ProxyProtocolVersion kubebuilder enum type mismatch (integer vs string) in AgentEndpoint CRD in [#792](https://github.com/ngrok/ngrok-operator/pull/792)
- Fixed driver sync to not log errors for non-error scenarios by @alex-bezek in [#768](https://github.com/ngrok/ngrok-operator/pull/768)
- Fixed object modified errors with retry-on-conflict handling by @alex-bezek in [#773](https://github.com/ngrok/ngrok-operator/pull/773)
