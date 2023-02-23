# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 0.4.0
### Changed
- Ingress Class has Default set to false [#109](https://github.com/ngrok/kubernetes-ingress-controller/pull/109)


### Added
- Load certs from the directory `"/etc/ssl/certs/ngrok/"` for ngrok-go if present [#111](https://github.com/ngrok/kubernetes-ingress-controller/pull/111)
- Add IP Policy CRD and IP Policy Route Module [#120](https://github.com/ngrok/kubernetes-ingress-controller/pull/120)
- Add/Remove Header Route Module [#121](https://github.com/ngrok/kubernetes-ingress-controller/pull/121)
- Webhook Verification Route Module [#122](https://github.com/ngrok/kubernetes-ingress-controller/pull/122)
- Minimum TLS Version Route Module [#125](https://github.com/ngrok/kubernetes-ingress-controller/pull/125)
- Specify IP Policy by CRD name in addition to policy ID [#127](https://github.com/ngrok/kubernetes-ingress-controller/pull/127)
- merge all ingress objects into a single store to derive Edges. [#129](https://github.com/ngrok/kubernetes-ingress-controller/pull/129), [#10](https://github.com/ngrok/kubernetes-ingress-controller/pull/10), [#131](https://github.com/ngrok/kubernetes-ingress-controller/pull/131), [#137](https://github.com/ngrok/kubernetes-ingress-controller/pull/137)

### Fixed
- Remove routes from remote API when they are removed from the ingress object [#124](https://github.com/ngrok/kubernetes-ingress-controller/pull/124)
- Reduce domain controller reconcile counts by not updating domains if they didn't change  [#140](https://github.com/ngrok/kubernetes-ingress-controller/pull/140)

## 0.3.0
### Changed
- Renamed docker image from `ngrok/ngrok-ingress-controller` to `ngrok/kubernetes-ingress-controller`.
- Added new controllers for `domains`, `tcpedges`, and `httpsedges`.
- Updated go dependencies
- Moved `main.go` to root of project to match what `kubebuilder` expects.
- Updated `Makefile` to match what `kubebuilder` currently outputs.
- Created `serverAddr` flag and plumbed it through to `ngrok-go`
- Read environment variable `NGROK_API_ADDR` for an override to the ngrok API address.

## 0.2.0
### Changed

- Moved from calling ngrok-agent sidecar to using the ngrok-go library in process.

## 0.1.X

### Initial Alpha Releases

The ngrok ingress controller is currently in alpha. Releases will have varying features with breaking changes.
