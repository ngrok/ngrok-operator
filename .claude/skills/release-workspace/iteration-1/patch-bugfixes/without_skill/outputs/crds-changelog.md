## 0.2.2
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-ngrok-crds-0.2.1...helm-chart-ngrok-crds-0.2.2

- Update CRDs Helm chart version to `0.2.2`

### Changed

- Show the Ready Condition's reason and message in `-o wide` printer columns for all CRDs by @alex-bezek in [#772](https://github.com/ngrok/ngrok-operator/pull/772)
- Fix URI vs URL inconsistency in BoundEndpoint CRD by @sabrina-ngrok in [#779](https://github.com/ngrok/ngrok-operator/pull/779)

### Fixed

- Fix ProxyProtocolVersion kubebuilder enum type mismatch in AgentEndpoint CRD by @Copilot in [#792](https://github.com/ngrok/ngrok-operator/pull/792)
- Fix misc CRD annotation issues for IPPolicy and CloudEndpoint by @alex-bezek in [#803](https://github.com/ngrok/ngrok-operator/pull/803)
- Regenerated CRDs as part of RBAC overhaul by @alex-bezek in [#804](https://github.com/ngrok/ngrok-operator/pull/804)
