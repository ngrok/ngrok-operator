## 0.3.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-ngrok-crds-0.2.1...helm-chart-ngrok-crds-0.3.0

- Update CRDs Helm chart version to `0.3.0`

### Added

- Show Ready condition's reason and message in `-o wide` output for AgentEndpoint, CloudEndpoint, Domain, IPPolicy, and BoundEndpoint CRDs by @alex-bezek in [#772](https://github.com/ngrok/ngrok-operator/pull/772)

### Changed

- Renamed `endpointURI` to `endpointURL` in BoundEndpoint CRD. The old `endpointURI` field is deprecated but still accepted. by @sabrina-ngrok in [#779](https://github.com/ngrok/ngrok-operator/pull/779)

### Fixed

- Fixed ProxyProtocolVersion kubebuilder enum type mismatch in AgentEndpoint CRD — enum values now correctly emitted as strings instead of integers in [#792](https://github.com/ngrok/ngrok-operator/pull/792)
- Fixed CloudEndpoint status condition description to say "CloudEndpoint" instead of "AgentEndpoint" by @alex-bezek in [#803](https://github.com/ngrok/ngrok-operator/pull/803)
- Removed leftover kubebuilder scaffolding comments from IPPolicy status field by @alex-bezek in [#803](https://github.com/ngrok/ngrok-operator/pull/803)
