## 0.23.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-ngrok-operator-0.22.2...helm-chart-ngrok-operator-0.23.0

- Update ngrok-operator image version to `0.21.0`
- Update Helm chart version to `0.23.0`
- Update [ngrok-crds](../ngrok-crds/CHANGELOG.md) dependency version to `0.3.0`

### Added

- Add `k8s.ngrok.com/metadata` and `k8s.ngrok.com/description` annotations support for Ingress and Gateway resources by @sabrina-ngrok in [#788](https://github.com/ngrok/ngrok-operator/pull/788)

### Changed

- Overhauled RBAC: hand-managed Helm templates replace controller-gen markers, reorganized into per-component directories (`api-manager/`, `agent/`, `bindings-forwarder/`), added namespace-scoping support (Role instead of ClusterRole when `watchNamespace` is set), and removed stale CRD access roles by @alex-bezek in [#804](https://github.com/ngrok/ngrok-operator/pull/804)
- Renamed `endpointURI` to `endpointURL` in BoundEndpoint CRD by @sabrina-ngrok in [#779](https://github.com/ngrok/ngrok-operator/pull/779)

### Fixed

- Fixed object modified errors with retry-on-conflict handling by @alex-bezek in [#773](https://github.com/ngrok/ngrok-operator/pull/773)
