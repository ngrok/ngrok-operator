## 0.23.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-ngrok-operator-0.22.2...helm-chart-ngrok-operator-0.23.0

- Update ngrok-operator image version to `0.21.0`
- Update Helm chart version to `0.23.0`
- Update [ngrok-crds](../ngrok-crds/CHANGELOG.md) dependency version to `0.3.0`

### Fixed
- Added RBAC `patch` permissions on finalizer-managed resources to fix "object has been modified" errors during concurrent reconciliation by @alex-bezek in [#773](https://github.com/ngrok/ngrok-operator/pull/773)
