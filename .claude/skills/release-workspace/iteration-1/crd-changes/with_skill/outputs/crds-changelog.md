## 0.3.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-ngrok-crds-0.2.1...helm-chart-ngrok-crds-0.3.0

- Update CRDs Helm chart version to `0.3.0`

### Breaking Changes
- Rename BoundEndpoint `spec.endpointURI` to `spec.endpointURL` for naming consistency with ngrok API conventions. The old field is preserved as deprecated and will be removed in a future release. by @sabrina-ngrok in [#779](https://github.com/ngrok/ngrok-operator/pull/779)

### Added
- Add Ready condition reason and message columns to CRDs for `kubectl get -o wide` output (AgentEndpoint, CloudEndpoint, Domain, IPPolicy, BoundEndpoint) by @alex-bezek in [#772](https://github.com/ngrok/ngrok-operator/pull/772)

### Fixed
- Fix ProxyProtocolVersion kubebuilder enum on AgentEndpoint CRD to use quoted string values instead of integers, fixing admission validation rejection of valid inputs by @app/copilot-swe-agent in [#792](https://github.com/ngrok/ngrok-operator/pull/792)
- Remove leftover kubebuilder scaffolding comments from IPPolicy, CloudEndpoint, and NgrokTrafficPolicy types and regenerate CRDs by @alex-bezek in [#803](https://github.com/ngrok/ngrok-operator/pull/803)
