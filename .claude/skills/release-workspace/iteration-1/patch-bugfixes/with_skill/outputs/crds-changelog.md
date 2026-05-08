## 0.2.2
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-ngrok-crds-0.2.1...helm-chart-ngrok-crds-0.2.2

- Update CRDs Helm chart version to `0.2.2`

### Breaking Changes
- Rename BoundEndpoint `spec.endpointURI` to `spec.endpointURL` for naming consistency. The old field is preserved as deprecated and will be removed in a future release by @sabrina-ngrok in [#779](https://github.com/ngrok/ngrok-operator/pull/779)

### Added
- Show the Ready Condition's reason and message in `-o wide` output for BoundEndpoint, Domain, IPPolicy, AgentEndpoint, and CloudEndpoint CRDs by @alex-bezek in [#772](https://github.com/ngrok/ngrok-operator/pull/772)

### Fixed
- Fix ProxyProtocolVersion kubebuilder enum type mismatch on AgentEndpoint CRD by @app/copilot-swe-agent in [#792](https://github.com/ngrok/ngrok-operator/pull/792)
- Remove leftover scaffolding comments from NgrokTrafficPolicy, IPPolicy, and CloudEndpoint CRD types by @alex-bezek in [#803](https://github.com/ngrok/ngrok-operator/pull/803)
