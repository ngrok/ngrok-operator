## 0.20.4
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-0.20.3...ngrok-operator-0.20.4

### Added
- Add `k8s.ngrok.com/metadata` and `k8s.ngrok.com/description` annotations to Ingress and Gateway resources, propagated to generated CloudEndpoints and AgentEndpoints by @sabrina-ngrok in [#788](https://github.com/ngrok/ngrok-operator/pull/788)
- Show the Ready Condition's reason and message when using `kubectl get -o wide` for CRDs by @alex-bezek in [#772](https://github.com/ngrok/ngrok-operator/pull/772)

### Changed
- Reduce "object has been modified" errors by using patch for finalizers and RetryOnConflict for status updates by @alex-bezek in [#773](https://github.com/ngrok/ngrok-operator/pull/773)
- Update Go from 1.25.7 to 1.26.1 and update Go module dependencies by @jonstacks in [#785](https://github.com/ngrok/ngrok-operator/pull/785)
- Update full install manifest generation to trigger from main branch by @sabrina-ngrok in [#782](https://github.com/ngrok/ngrok-operator/pull/782)
- Add `go fix ./...` to Makefile and CI by @app/copilot-swe-agent in [#791](https://github.com/ngrok/ngrok-operator/pull/791)
- Consolidate nix setup and remove `nix develop --command` wrappers in CI by @jonstacks in [#795](https://github.com/ngrok/ngrok-operator/pull/795)
- Add Copilot setup steps for GitHub agent environment by @jonstacks in [#787](https://github.com/ngrok/ngrok-operator/pull/787)
- Add testing agent to help with testing by @jonstacks in [#793](https://github.com/ngrok/ngrok-operator/pull/793)
- Skip Trivy scanning job temporarily by @jonstacks in [#786](https://github.com/ngrok/ngrok-operator/pull/786)

### Fixed
- Fix driver sync to not log errors for non-error scenarios by replacing sentinel errors with channel-based signaling by @alex-bezek in [#768](https://github.com/ngrok/ngrok-operator/pull/768)
- Fix ProxyProtocolVersion kubebuilder enum type mismatch causing admission validation to reject valid string inputs by @app/copilot-swe-agent in [#792](https://github.com/ngrok/ngrok-operator/pull/792)
- Use ReconcileStatus for CloudEndpoint IsNotFound handling instead of silently discarding status update errors by @alex-bezek in [#799](https://github.com/ngrok/ngrok-operator/pull/799)
- Fix data race in ChannelHealthChecker.Alive(), swallowed API pagination errors in findReservedDomainByHostname, and incorrect error messages by @alex-bezek in [#803](https://github.com/ngrok/ngrok-operator/pull/803)
- Fix test flake in service controller due to race condition by @alex-bezek in [#784](https://github.com/ngrok/ngrok-operator/pull/784)
- Fix flaky GatewayClass finalizer test due to missing Eventually timeout by @app/copilot-swe-agent in [#789](https://github.com/ngrok/ngrok-operator/pull/789)
- Fix flaky agent endpoint controller test due to race condition in mock driver setup by @jonstacks in [#794](https://github.com/ngrok/ngrok-operator/pull/794)
