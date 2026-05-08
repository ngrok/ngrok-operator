## 0.21.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-0.20.3...ngrok-operator-0.21.0

### Breaking Changes
- Renamed BoundEndpoint `spec.endpointURI` to `spec.endpointURL` for naming consistency. The old field is preserved as deprecated and will be removed in a future release by @sabrina-ngrok in [#779](https://github.com/ngrok/ngrok-operator/pull/779)

### Added
- Added `k8s.ngrok.com/metadata` and `k8s.ngrok.com/description` annotations for Ingress and Gateway resources, allowing per-resource metadata and descriptions to be propagated to generated endpoints by @sabrina-ngrok in [#788](https://github.com/ngrok/ngrok-operator/pull/788)
- Added Ready condition reason and message columns visible via `kubectl get -o wide` for CRDs (CloudEndpoint, AgentEndpoint, Domain, IPPolicy, BoundEndpoint) by @alex-bezek in [#772](https://github.com/ngrok/ngrok-operator/pull/772)

### Changed
- Updated Go from 1.25.7 to 1.26.1 and updated go module dependencies by @jonstacks in [#785](https://github.com/ngrok/ngrok-operator/pull/785)
- Added `go fix ./...` to Makefile and CI as an early gate to prevent outdated Go idioms by @app/copilot-swe-agent in [#791](https://github.com/ngrok/ngrok-operator/pull/791)
- Consolidated nix setup across CI workflows and removed `nix develop --command` wrappers by @jonstacks in [#795](https://github.com/ngrok/ngrok-operator/pull/795)
- Added Copilot agent setup steps for improved agent-created PR quality by @jonstacks in [#787](https://github.com/ngrok/ngrok-operator/pull/787)
- Added testing agent to assist with test development by @jonstacks in [#793](https://github.com/ngrok/ngrok-operator/pull/793)
- Temporarily disabled Trivy image scanning job by @jonstacks in [#786](https://github.com/ngrok/ngrok-operator/pull/786)
- Fixed full install manifest generation to run on main branch by @sabrina-ngrok in [#782](https://github.com/ngrok/ngrok-operator/pull/782)

### Fixed
- Fixed driver sync to not log spurious errors for non-error requeue scenarios by @alex-bezek in [#768](https://github.com/ngrok/ngrok-operator/pull/768)
- Fixed "object has been modified" errors by switching finalizer operations to patch and adding RetryOnConflict to status updates by @alex-bezek in [#773](https://github.com/ngrok/ngrok-operator/pull/773)
- Fixed `ProxyProtocolVersion` kubebuilder enum marker to use string values instead of integers, fixing admission validation rejections by @app/copilot-swe-agent in [#792](https://github.com/ngrok/ngrok-operator/pull/792)
- Fixed CloudEndpoint controller to use `ReconcileStatus` for IsNotFound handling instead of silently discarding status update errors by @alex-bezek in [#799](https://github.com/ngrok/ngrok-operator/pull/799)
- Fixed data race in `ChannelHealthChecker.Alive()` using wrong mutex, swallowed API pagination errors in `findReservedDomainByHostname`, incorrect error messages in gateway translation and port allocator, and removed leftover scaffolding comments by @alex-bezek in [#803](https://github.com/ngrok/ngrok-operator/pull/803)
- Fixed flaky service controller test due to race on resource lookup by @alex-bezek in [#784](https://github.com/ngrok/ngrok-operator/pull/784)
- Fixed flaky GatewayClass finalizer test due to missing Eventually timeout by @app/copilot-swe-agent in [#789](https://github.com/ngrok/ngrok-operator/pull/789)
- Fixed flaky agent endpoint controller test due to race between mock setup and secret creation by @jonstacks in [#794](https://github.com/ngrok/ngrok-operator/pull/794)
