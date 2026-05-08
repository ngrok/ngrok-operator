## 0.21.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-0.20.3...ngrok-operator-0.21.0

### Added

- Add description and metadata annotations to ingress and gateway resources by @sabrina-ngrok in [#788](https://github.com/ngrok/ngrok-operator/pull/788)
- Show the Ready Condition's reason and message in `-o wide` output for CRDs by @alex-bezek in [#772](https://github.com/ngrok/ngrok-operator/pull/772)

### Changed

- RBAC overhaul: restructured RBAC across all three operator deployments (api-manager, agent-manager, bindings-forwarder) with per-component roles and namespace-scoped permissions by @alex-bezek in [#804](https://github.com/ngrok/ngrok-operator/pull/804)
- Dependency updates by @jonstacks in [#785](https://github.com/ngrok/ngrok-operator/pull/785)

### Fixed

- Fix multiple controller bugs: data races, swallowed errors, wrong error messages, and doc cleanup by @alex-bezek in [#803](https://github.com/ngrok/ngrok-operator/pull/803)
- Fix agent driver context propagation, channel safety, stale status conditions by @alex-bezek in [#802](https://github.com/ngrok/ngrok-operator/pull/802)
- Fix bindings safety: operator crashes, data races, and nil panics by @alex-bezek in [#801](https://github.com/ngrok/ngrok-operator/pull/801)
- Fix manager driver: lost status conditions, reconcile churn from nondeterministic ordering, and informer cache corruption by @alex-bezek in [#800](https://github.com/ngrok/ngrok-operator/pull/800)
- fix(ngrok): use ReconcileStatus for CloudEndpoint IsNotFound handling to be consistent with other controllers by @alex-bezek in [#799](https://github.com/ngrok/ngrok-operator/pull/799)
- Fix gateway and HTTPRoute controller validation logic, informer cache corruption, and cross-namespace matching by @alex-bezek in [#798](https://github.com/ngrok/ngrok-operator/pull/798)
- Fix KubernetesOperator controller: error swallowing, nil pointer panics, and unreachable dead code by @alex-bezek in [#797](https://github.com/ngrok/ngrok-operator/pull/797)
- fix: skip redundant ngrok API updates for CloudEndpoint by @jonstacks in [#806](https://github.com/ngrok/ngrok-operator/pull/806)
- Fix driver sync to not log errors for non-error scenarios by @alex-bezek in [#768](https://github.com/ngrok/ngrok-operator/pull/768)
- Fix object modified conflict errors with retry logic across all controllers by @alex-bezek in [#773](https://github.com/ngrok/ngrok-operator/pull/773)
- Fix ProxyProtocolVersion kubebuilder enum type mismatch by @copilot-swe-agent in [#792](https://github.com/ngrok/ngrok-operator/pull/792)
- Fix URI vs URL inconsistency in BoundEndpoint types by @sabrina-ngrok in [#779](https://github.com/ngrok/ngrok-operator/pull/779)
