## 0.22.3
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-ngrok-operator-0.22.2...helm-chart-ngrok-operator-0.22.3

- Update ngrok-operator image version to `0.20.4`
- Update Helm chart version to `0.22.3`
- Update [ngrok-crds](../ngrok-crds/CHANGELOG.md) dependency version to `0.2.2`

### Fixed
- Add RBAC `patch` permissions on finalizer-managed resources to reduce "object has been modified" errors by @alex-bezek in [#773](https://github.com/ngrok/ngrok-operator/pull/773)
