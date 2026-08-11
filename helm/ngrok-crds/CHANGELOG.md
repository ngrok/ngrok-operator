# Changelog

All notable changes to the ngrok-crds helm chart will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).


## 0.4.0-rc.2
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-ngrok-crds-0.4.0-rc.1...helm-chart-ngrok-crds-0.4.0-rc.2

- Update CRDs Helm chart version to `0.4.0-rc.2`

### Fixed
- Regenerated `KubernetesOperator` CRD to include the legacy `enabledFeatures` string field used by the rollback-safe dual-write shim by @alex-bezek in [#865](https://github.com/ngrok/ngrok-operator/pull/865)

## 0.4.0-rc.1
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-ngrok-crds-0.3.0...helm-chart-ngrok-crds-0.4.0-rc.1

- Update CRDs Helm chart version to `0.4.0-rc.1`

### Breaking Changes
- Set up passive migrations to let CloudEndpoints reference TrafficPolicy resources under the new API group by @alex-bezek in [#823](https://github.com/ngrok/ngrok-operator/pull/823)
- Migrated default metadata handling from the legacy kubernetes-ingress-controller by @alex-bezek in [#838](https://github.com/ngrok/ngrok-operator/pull/838)
- Overhauled KubernetesOperator status: replaced phase enums with structured Conditions and added structured drain reporting by @alex-bezek in [#846](https://github.com/ngrok/ngrok-operator/pull/846)
- Cleaned up Domain and IPPolicy status: dropped spec-duplicated fields and the dead `Progressing` condition by @alex-bezek in [#851](https://github.com/ngrok/ngrok-operator/pull/851)
- Tightened CRD field validation, added `omitempty` on optional fields, and fixed Domain schema issues by @alex-bezek in [#843](https://github.com/ngrok/ngrok-operator/pull/843)
- Restricted `clientCertificateRefs` on AgentEndpoint to same-namespace secrets by @jonstacks in [#826](https://github.com/ngrok/ngrok-operator/pull/826)
- Support `map[string]string` for CRD metadata fields (K8SOP-295) by @alex-bezek in [#858](https://github.com/ngrok/ngrok-operator/pull/858)

### Added
- Support agent TLS termination on AgentEndpoint by @jonstacks in [#814](https://github.com/ngrok/ngrok-operator/pull/814)
- Added top-level `status.observedGeneration` to all CRDs by @alex-bezek in [#845](https://github.com/ngrok/ngrok-operator/pull/845)
- Added additional CRD status fields and validation by @alex-bezek in [#850](https://github.com/ngrok/ngrok-operator/pull/850)

### Changed
- Replaced `TrafficPolicy.status.policy` with `Ready`/`Valid` conditions and added printer columns by @alex-bezek in [#852](https://github.com/ngrok/ngrok-operator/pull/852)

## 0.3.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-ngrok-crds-0.2.1...helm-chart-ngrok-crds-0.3.0

Moving `0.3.0-rc.1` to `0.3.0`. See the `0.3.0-rc.1` notes below for changes.

## 0.3.0-rc.1
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-ngrok-crds-0.2.1...helm-chart-ngrok-crds-0.3.0-rc.1

- Update CRDs Helm chart version to `0.3.0-rc.1`

### Breaking Changes
- Renamed `URI` field to `URL` across CRDs for consistency by @sabrina-ngrok in [#779](https://github.com/ngrok/ngrok-operator/pull/779)

### Added
- Show Ready Condition's reason and message in `-o wide` output for CRDs by @alex-bezek in [#772](https://github.com/ngrok/ngrok-operator/pull/772)

### Fixed
- Fix ProxyProtocolVersion kubebuilder enum validation to use string values instead of integers by @copilot-swe-agent in [#792](https://github.com/ngrok/ngrok-operator/pull/792)
- Fix misc CRD annotation and validation issues by @alex-bezek in [#803](https://github.com/ngrok/ngrok-operator/pull/803)

## 0.2.1
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-ngrok-crds-0.2.0...helm-chart-ngrok-crds-0.2.1

- Update CRDs Helm chart version to `0.2.1`


### Added
- Updated `controller-gen` version and regenerated `CRDs` by @jonstacks in [#767](https://github.com/ngrok/ngrok-operator/pull/767)


## 0.2.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-ngrok-crds-0.1.0...helm-chart-ngrok-crds-0.2.0

- Update CRDs Helm chart version to `0.2.0`

### Breaking Changes
- Remove the deprecated cloud endpoint domain status by @alex-bezek in [#727](https://github.com/ngrok/ngrok-operator/pull/727)


### Added

- feat: support Domain `resolves_to` field by @andrew-harris-at-ngrok in [#746](https://github.com/ngrok/ngrok-operator/pull/746)
- Allow helm uninstall to be configured to handle cleaning up api resources by @alex-bezek in [#750](https://github.com/ngrok/ngrok-operator/pull/750)

### Fixed

- Fix invalid kubebuilder code gen markers by @alex-bezek in [#734](https://github.com/ngrok/ngrok-operator/pull/734)
- Add better kubebuilder type annotations to some status condition fields by @alex-bezek in [#728](https://github.com/ngrok/ngrok-operator/pull/728)

## 0.1.0

Initial release of the ngrok-crds Helm chart.

- feat: Split ngrok-operators CRDs into their own chart by @jonstacks in [#732](https://github.com/ngrok/ngrok-operator/pull/732)
