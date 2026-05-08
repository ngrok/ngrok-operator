## 0.21.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-0.20.3...ngrok-operator-0.21.0

### Breaking Changes
- Renamed BoundEndpoint `spec.endpointURI` to `spec.endpointURL` for naming consistency. The old field is preserved as deprecated and will be removed in a future release. by @sabrina-ngrok in [#779](https://github.com/ngrok/ngrok-operator/pull/779)

### Added
- Show the Ready condition's reason and message when using `kubectl get -o wide` on CRDs (AgentEndpoint, CloudEndpoint, Domain, IPPolicy, BoundEndpoint) by @alex-bezek in [#772](https://github.com/ngrok/ngrok-operator/pull/772)
- Support `k8s.ngrok.com/metadata` and `k8s.ngrok.com/description` annotations on Ingress and Gateway resources, propagating metadata and descriptions to generated endpoints by @sabrina-ngrok in [#788](https://github.com/ngrok/ngrok-operator/pull/788)

### Changed
- Update Go from 1.25.7 to 1.26.1 and update go module dependencies by @jonstacks in [#785](https://github.com/ngrok/ngrok-operator/pull/785)
- Skip trivy image scan job temporarily by @jonstacks in [#786](https://github.com/ngrok/ngrok-operator/pull/786)
- Add copilot setup steps workflow for GitHub Codespaces parity by @jonstacks in [#787](https://github.com/ngrok/ngrok-operator/pull/787)
- Add `go fix ./...` to Makefile and CI pipeline by @app/copilot-swe-agent in [#791](https://github.com/ngrok/ngrok-operator/pull/791)
- Add testing agent to improve copilot-assisted test authoring by @jonstacks in [#793](https://github.com/ngrok/ngrok-operator/pull/793)
- Consolidate nix setup and remove `nix develop --command` wrappers in CI by @jonstacks in [#795](https://github.com/ngrok/ngrok-operator/pull/795)
- Fix manifest-bundle.yaml generation to trigger on main branch by @sabrina-ngrok in [#782](https://github.com/ngrok/ngrok-operator/pull/782)

### Fixed
- Fix driver sync to not log errors for non-error scenarios when sync is debounced by @alex-bezek in [#768](https://github.com/ngrok/ngrok-operator/pull/768)
- Fix "object has been modified" errors by using patch for finalizers and retry-on-conflict for status updates by @alex-bezek in [#773](https://github.com/ngrok/ngrok-operator/pull/773)
- Fix ProxyProtocolVersion kubebuilder enum to use string values instead of integers by @app/copilot-swe-agent in [#792](https://github.com/ngrok/ngrok-operator/pull/792)
- Use ReconcileStatus for CloudEndpoint IsNotFound handling instead of silently discarding status update errors by @alex-bezek in [#799](https://github.com/ngrok/ngrok-operator/pull/799)
- Fix data race in ChannelHealthChecker.Alive(), propagate API pagination errors in findReservedDomainByHostname, and correct several error messages by @alex-bezek in [#803](https://github.com/ngrok/ngrok-operator/pull/803)
- Fix test flake in service controller due to race condition by @alex-bezek in [#784](https://github.com/ngrok/ngrok-operator/pull/784)
- Fix flaky GatewayClass finalizer test due to missing Eventually timeout by @app/copilot-swe-agent in [#789](https://github.com/ngrok/ngrok-operator/pull/789)
- Fix flakey agent endpoint controller test due to mock driver result ordering by @jonstacks in [#794](https://github.com/ngrok/ngrok-operator/pull/794)
