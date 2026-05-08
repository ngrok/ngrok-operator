## 0.3.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-ngrok-crds-0.2.1...helm-chart-ngrok-crds-0.3.0

- Update CRDs Helm chart version to `0.3.0`

### Breaking Changes
- Renamed BoundEndpoint `spec.endpointURI` to `spec.endpointURL` for naming consistency. The old field is preserved as deprecated and will be removed in a future release by @sabrina-ngrok in [#779](https://github.com/ngrok/ngrok-operator/pull/779)

### Added
- Added Ready condition reason and message columns visible via `kubectl get -o wide` for CRDs (CloudEndpoint, AgentEndpoint, Domain, IPPolicy, BoundEndpoint) by @alex-bezek in [#772](https://github.com/ngrok/ngrok-operator/pull/772)

### Fixed
- Fixed `ProxyProtocolVersion` kubebuilder enum marker to use string values instead of integers by @app/copilot-swe-agent in [#792](https://github.com/ngrok/ngrok-operator/pull/792)
- Removed leftover kubebuilder scaffolding comments from IPPolicy, CloudEndpoint, and NgrokTrafficPolicy CRD types by @alex-bezek in [#803](https://github.com/ngrok/ngrok-operator/pull/803)
