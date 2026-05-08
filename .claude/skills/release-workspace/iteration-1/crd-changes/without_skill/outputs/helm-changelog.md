## 0.23.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-ngrok-operator-0.22.2...helm-chart-ngrok-operator-0.23.0

- Update ngrok-operator image version to `0.21.0`
- Update Helm chart version to `0.23.0`
- Update [ngrok-crds](../ngrok-crds/CHANGELOG.md) dependency version to `0.3.0`

### Changed

- RBAC overhaul: restructured RBAC across all three operator deployments (api-manager, agent-manager, bindings-forwarder) with per-component roles, namespace-scoped permissions, and dedicated CRD editor/viewer ClusterRoles by @alex-bezek in [#804](https://github.com/ngrok/ngrok-operator/pull/804)

### Fixed

- Fix object modified conflict errors with retry logic across all controllers by @alex-bezek in [#773](https://github.com/ngrok/ngrok-operator/pull/773)
