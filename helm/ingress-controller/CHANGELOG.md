# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 0.12.1

- Update to version 0.10.1 of the ingress controller, which includes:
  - IPPolicy controller wasn't applying the attached rules, leaving the IP policy in its current state [#315](https://github.com/ngrok/kubernetes-ingress-controller/pull/315)

## 0.12.0

- Update to version 0.10.0 of the ingress controller, this includes:
  - TLSEdge support - see the [TCP and TLS Edges Guide](https://github.com/ngrok/kubernetes-ingress-controller/blob/main/docs/user-guide/tcp-tls-edges.md) for more details.
  - A fix for renegotiating TLS backends

## 0.11.0

** Important ** This version of the controller changes the ownership model for https edge and tunnel CRs. To ease out the transition to the new ownership, make sure to run `migrate-edges.sh` and `migrate-tunnels.sh` scripts before installing the new version.

### Changed
- Specify IPPolicyRule action as an enum of (allow,deny) as part of [#260](https://github.com/ngrok/kubernetes-ingress-controller/pull/260)
- Handle special case for changing auth types that causes an error during state transition [#259](https://github.com/ngrok/kubernetes-ingress-controller/pull/259)
- Better handling when changing pathType between 'Exact' and 'Prefix' [#262](https://github.com/ngrok/kubernetes-ingress-controller/pull/262)
- Update ngrok-go to 1.4.0 [#298](https://github.com/ngrok/kubernetes-ingress-controller/pull/298)
- Tunnels are now unique in their respective namespace, not across the cluster [#281](https://github.com/ngrok/kubernetes-ingress-controller/pull/281)
- The CRs that ingress controller creates are uniquely marked and managed by it. Other CRs created manually are no longer deleted when the ingress controller is not using them [#267](https://github.com/ngrok/kubernetes-ingress-controller/issues/267); fixed for tunnel in [#285](https://github.com/ngrok/kubernetes-ingress-controller/pull/285) and for https edges in [#286](https://github.com/ngrok/kubernetes-ingress-controller/pull/286)
- Better error handling and retry, specifically for the case where we try to create an https edge for a domain which is not created yet [#283](https://github.com/ngrok/kubernetes-ingress-controller/issues/283); fixed in [#288](https://github.com/ngrok/kubernetes-ingress-controller/pull/288)
- Watch and apply ngrok module set CR changes [#287](https://github.com/ngrok/kubernetes-ingress-controller/issues/287); fixed in [#290](https://github.com/ngrok/kubernetes-ingress-controller/pull/290)
- Label https edges and tunnels with service UID to make them more unique within ngrok [#291](https://github.com/ngrok/kubernetes-ingress-controller/issues/291); fixed in [#293](https://github.com/ngrok/kubernetes-ingress-controller/pull/293) and [#302](https://github.com/ngrok/kubernetes-ingress-controller/pull/302)

### Added
- Add support for configuring pod affinities, pod disruption budget, and priorityClassName [#258](https://github.com/ngrok/kubernetes-ingress-controller/pull/258)
- The controller stopping at the first resource create [#270](https://github.com/ngrok/kubernetes-ingress-controller/pull/270)
- Using `make deploy` now requires `NGROK_AUTHTOKEN` and `NGROK_API_KEY` to be set [#292](https://github.com/ngrok/kubernetes-ingress-controller/pull/292)

## 0.10.0

### Added
- Support HTTPS backends via service annotation [#238](https://github.com/ngrok/kubernetes-ingress-controller/pull/238)

### Changed
- Normalize all ngrok `.io` TLD to `.app` TLD [#240](https://github.com/ngrok/kubernetes-ingress-controller/pull/240)
- Chart Icon

### Fixed
- Add namespace to secret [#244](https://github.com/ngrok/kubernetes-ingress-controller/pull/244). Thank you for the contribution, @vincetse!

## 0.9.0
### Added
- Add a 'podLabels' option to the helm chart [#212](https://github.com/ngrok/kubernetes-ingress-controller/pull/212).
- Permission to `get`,`list`, and `watch` `services` [#222](https://github.com/ngrok-kubernetes-ingress-controller/pull/222).

## 0.8.0
### Changed
- Log Level configuration to helm chart [#199](https://github.com/ngrok/kubernetes-ingress-controller/pull/199).
- Bump default controller image to use `0.6.0` release [#204](https://github.com/ngrok/kubernetes-ingress-controller/pull/204).

### Fixed
- update default-container annotation so logs work correctly [#197](https://github.com/ngrok/kubernetes-ingress-controller/pull/197)

## 0.7.0

### Added
- Update `NgrokModuleSet` and `HTTPSEdge` CRD to support SAML and OAuth

### Changed
- Update appVersion to `0.5.0` to match the latest release of the controller.

## 0.6.1
### Fixed
- Default the image tag to the chart's `appVersion` for predictable installs. Previously, the helm chart would default to the `latest` image tag which can have breaking changes, notably with CRDs.

## 0.6.0
### Changed
- Ingress Class has Default set to false [#109](https://github.com/ngrok/kubernetes-ingress-controller/pull/109)

### Added
- Allow controller name to be configured to support multiple ngrok ingress classes [#159](https://github.com/ngrok/kubernetes-ingress-controller/pull/159)
- Allow the controller to be configured to only watch a single namespace [#157](https://github.com/ngrok/kubernetes-ingress-controller/pull/157)
- Pass key/value pairs to helm that get added as json string metadata in ngrok api resources [#156](https://github.com/ngrok/kubernetes-ingress-controller/pull/156)
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
