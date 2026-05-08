## 0.22.3
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-ngrok-operator-0.22.2...helm-chart-ngrok-operator-0.22.3

- Update ngrok-operator image version to `0.20.4`
- Update Helm chart version to `0.22.3`
- Update [ngrok-crds](../ngrok-crds/CHANGELOG.md) dependency version to `0.2.2`

### Changed

- RBAC overhaul: restructured RBAC templates into per-component directories (agent, api-manager, bindings-forwarder) with namespace-scoped roles and dedicated service accounts by @alex-bezek in [#804](https://github.com/ngrok/ngrok-operator/pull/804)

### Fixed

- Fix object modified errors by adding RBAC permissions needed for conflict retry logic by @alex-bezek in [#773](https://github.com/ngrok/ngrok-operator/pull/773)
