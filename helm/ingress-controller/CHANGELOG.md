# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 0.6.0
### Changed
- Ingress Class has Default set to false [#109](https://github.com/ngrok/kubernetes-ingress-controller/pull/109)

### Added
- Allow controller name to be configured to support multiple ngrok ingress classes [#159](https://github.com/ngrok/kubernetes-ingress-controller/pull/159)
- Allow the controller to be configured to only watch a single namespace [#157](https://github.com/ngrok/kubernetes-ingress-controller/pull/157)
- Add IP Policy CRD and IP Policy Route Module [#120](https://github.com/ngrok/kubernetes-ingress-controller/pull/120)
- Load certs from the directory `"/etc/ssl/certs/ngrok/"` for ngrok-go if present [#111](https://github.com/ngrok/kubernetes-ingress-controller/pull/111)

## 0.5.0
### Changed
- Renamed chart from `ngrok-ingress-controller` to `kubernetes-ingress-controller`.
- Added CRDs for `domains`, `tcpedges`, and `httpsedges`.

## 0.4.0
### Added
- `serverAddr` flag to override the ngrok tunnel server address
- `extraVolumes` to add an arbitrary set of volumes to the controller pod
- `extraVolumeMounts` to add an arbitrary set of volume mounts to the controller container

## 0.3.1
### Fixed
- Fixes rendering of `NOTES.txt` when installing via helm

## 0.3.0
### Changed

- Moved from calling ngrok-agent sidecar to using the ngrok-go library in the controller process.
- Moved `apiKey` and `authtoken` to `credentials.apiKey` and `credentials.authtoken` respectively.
- `credentialSecrets.name` is now `credentials.secret.name`
- Changed replicas to 1 by default to work better for default/demo setup.

## 0.2.0
### Added

- Support for different values commonly found in helm charts

# 0.1.0

TODO
