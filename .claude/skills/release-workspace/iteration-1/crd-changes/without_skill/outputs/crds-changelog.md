## 0.3.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-ngrok-crds-0.2.1...helm-chart-ngrok-crds-0.3.0

- Update CRDs Helm chart version to `0.3.0`

### Added

- Show the Ready Condition's reason and message in `-o wide` output for AgentEndpoint, CloudEndpoint, Domain, IPPolicy, and BoundEndpoint CRDs by @alex-bezek in [#772](https://github.com/ngrok/ngrok-operator/pull/772)

### Fixed

- Fix ProxyProtocolVersion kubebuilder enum type mismatch in AgentEndpoint CRD by @copilot-swe-agent in [#792](https://github.com/ngrok/ngrok-operator/pull/792)
- Fix URI vs URL field naming inconsistency in BoundEndpoint CRD by @sabrina-ngrok in [#779](https://github.com/ngrok/ngrok-operator/pull/779)
- Fix various CRD type annotations and kubebuilder markers for IPPolicy and CloudEndpoint by @alex-bezek in [#803](https://github.com/ngrok/ngrok-operator/pull/803)
