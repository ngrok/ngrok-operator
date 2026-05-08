## 0.20.4
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-0.20.3...ngrok-operator-0.20.4

### Added

- Add description and metadata annotations to ingress and gateway endpoints by @sabrina-ngrok in [#788](https://github.com/ngrok/ngrok-operator/pull/788)

### Changed

- RBAC overhaul: restructured Helm RBAC templates into per-component roles with namespace-scoped permissions by @alex-bezek in [#804](https://github.com/ngrok/ngrok-operator/pull/804)
- Fix URI vs URL inconsistency in bindings by @sabrina-ngrok in [#779](https://github.com/ngrok/ngrok-operator/pull/779)
- chore(deps): Dependency updates by @jonstacks in [#785](https://github.com/ngrok/ngrok-operator/pull/785)

### Fixed

- Fix agent driver to correctly handle traffic policy merging and upstream URL construction by @alex-bezek in [#802](https://github.com/ngrok/ngrok-operator/pull/802)
- Fix bindings safety issues in bound endpoint poller and port allocator by @alex-bezek in [#801](https://github.com/ngrok/ngrok-operator/pull/801)
- Fix gateway controller issues with status handling and HTTPRoute reconciliation by @alex-bezek in [#798](https://github.com/ngrok/ngrok-operator/pull/798)
- Fix manager driver issues with gateway API translation and sync utilities by @alex-bezek in [#800](https://github.com/ngrok/ngrok-operator/pull/800)
- fix: skip redundant ngrok API updates for CloudEndpoint by @jonstacks in [#806](https://github.com/ngrok/ngrok-operator/pull/806)
- Fix KubernetesOperator controller reconciliation issues by @alex-bezek in [#797](https://github.com/ngrok/ngrok-operator/pull/797)
- fix(ngrok): use ReconcileStatus for CloudEndpoint IsNotFound handling by @alex-bezek in [#799](https://github.com/ngrok/ngrok-operator/pull/799)
- Fix misc quick wins: port allocator, domain controller, healthcheck, and CRD annotations by @alex-bezek in [#803](https://github.com/ngrok/ngrok-operator/pull/803)
- Fix object modified errors by adding conflict retry logic to controllers by @alex-bezek in [#773](https://github.com/ngrok/ngrok-operator/pull/773)
- Fix driver sync to not log errors for non-error scenarios by @alex-bezek in [#768](https://github.com/ngrok/ngrok-operator/pull/768)
- Show the Ready Condition's reason and message in `-o wide` for CRDs by @alex-bezek in [#772](https://github.com/ngrok/ngrok-operator/pull/772)
- Fix ProxyProtocolVersion kubebuilder enum type mismatch by @Copilot in [#792](https://github.com/ngrok/ngrok-operator/pull/792)
