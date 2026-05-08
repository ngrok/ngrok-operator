## 0.22.3
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-ngrok-operator-0.22.2...helm-chart-ngrok-operator-0.22.3

- Update ngrok-operator image version to `0.21.0`
- Update Helm chart version to `0.22.3`
- Update [ngrok-crds](../ngrok-crds/CHANGELOG.md) dependency version to `0.3.0`

### Fixed
- Fix "object has been modified" errors by adding RBAC `patch` permissions for finalizer updates on agent and controller roles by @alex-bezek in [#773](https://github.com/ngrok/ngrok-operator/pull/773)
