# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 0.12.2
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/kubernetes-ingress-controller-0.12.1...kubernetes-ingress-controller-0.12.2

### Added

- feat: Ability to specify cluster domain [#339](https://github.com/ngrok/ngrok-operator/pull/339). Thank you, @fr6nco !
- feat: Support for wildcard domains [#412](https://github.com/ngrok/ngrok-operator/pull/412)

### Changed

- chore: Clean up predicate filters [#409](https://github.com/ngrok/ngrok-operator/pull/409)
- refactor: Easier to read driver seed [#411](https://github.com/ngrok/ngrok-operator/pull/411)

### Fixed

- fix(store): Multiple ingress rules per ingress not working [#413](https://github.com/ngrok/ngrok-operator/pull/413)

## 0.12.1
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/kubernetes-ingress-controller-0.12.0...kubernetes-ingress-controller-0.12.1

### Fixed
- fix(service-controller): Updates not working [#406](https://github.com/ngrok/ngrok-operator/pull/406)
- fix: Deleting ngrok LoadBalancer services hanging [#404](https://github.com/ngrok/ngrok-operator/pull/404)

## 0.12.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/kubernetes-ingress-controller-0.11.0...kubernetes-ingress-controller-0.12.0

### Added

- feat: Auto-provision domain for TLS Edges [#386](https://github.com/ngrok/ngrok-operator/pull/386)
- feat: Support for Load Balancer services [#387](https://github.com/ngrok/ngrok-operator/pull/387)
- feat: Support TLS termination in modulesets for Load Balancer Services [388](https://github.com/ngrok/ngrok-operator/pull/388)

### Changed

- Switching over README to Operator [#351](https://github.com/ngrok/ngrok-operator/pull/351)
- chore: Remove custom code for non leader-elected controllers [#383](https://github.com/ngrok/ngrok-operator/pull/383)
- refactor: annotations parsers to handle client.Object instead of just networking.Ingress by [#384](https://github.com/ngrok/ngrok-operator/pull/384)
- chore: Turn on golangci-lint [#385](https://github.com/ngrok/ngrok-operator/pull/385)

### Fixed
- fix: TLSEdge not reconciling changes to hostports [#390](https://github.com/ngrok/ngrok-operator/pull/390)
- assign tunnel group lable by httproute namespace [#393](https://github.com/ngrok/ngrok-operator/pull/393)




## 0.11.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/kubernetes-ingress-controller-0.10.4...kubernetes-ingress-controller-0.11.0

### Added

- create policy kind [#361](https://github.com/ngrok/ngrok-operator/pull/361)
- initial policy controller update [#364](https://github.com/ngrok/ngrok-operator/pull/364)
- root-cas setting [#371](https://github.com/ngrok/ngrok-operator/pull/371)
  Takes an install option for --set rootCAs=host and plumb the isHostCA check into the caCerts for it to just get the host certs.
- feat: Add support for mutualTLS [#373](https://github.com/ngrok/ngrok-operator/pull/373)
- Add GatewayClass to cachestore [#376](https://github.com/ngrok/ngrok-operator/pull/376)
- Add extensionRef support for policy crd inclusion [#377](https://github.com/ngrok/ngrok-operator/pull/377
)



### Changed

- ngrok client api update [#367](https://github.com/ngrok/ngrok-operator/pull/367)
- switch edge kinds to raw json policy [#368](https://github.com/ngrok/ngrok-operator/pull/368)
- modules to traffic policy [#370](https://github.com/ngrok/ngrok-operator/pull/370)
- Update nix flake, go version, and Makefile dep versions [#379](https://github.com/ngrok/ngrok-operator/pull/379)

### Fixes

- fix: panics in oauth providers [#374](https://github.com/ngrok/ngrok-operator/pull/374)
- Handle non-existent backend IDs more gracefully [#380](https://github.com/ngrok/ngrok-operator/pull/380)
- Fixes not all reserved addrs being returned while iterating [#381](https://github.com/ngrok/ngrok-operator/pull/381)

## 0.10.4

### Added

- Add the `--api-url` option
  This can be used to set the endpoint for the ngrok API.
  It can be set through via the helm `apiURL` value.
- Set metadata for edges created by the gateway
- Add gateway to client info comment

### Changed

- Controller will now start without having session established. Any operations
  that require tunnels will return error, while it is trying to create a session.
  Its ready and health checks now depend on the status of this session - `ready`
  will not return `ok` until connection was established, and `health` check will
  return error if this connection had authentication issues.

### Fixed

- Search for backend service using the `HTTPRoute` namepace

## 0.10.3

### Added

- Support for Gateway api

## 0.10.2

### Added

- Support for [Traffic Policies](https://ngrok.com/docs/http/traffic-policy/) [#334](https://github.com/ngrok/ngrok-operator/pull/334)
- Support for [Application protocol](https://kubernetes.io/docs/concepts/services-networking/service/#application-protocol) on target services to support HTTP/2. [#323](https://github.com/ngrok/ngrok-operator/pull/323)

### Fixed

- The `Status.LoadBalancer[].Hostname` field is now propagated from `Domain` CNAME status updates. [#342](https://github.com/ngrok/ngrok-operator/pull/342)

## 0.10.1

### Fixed

- IPPolicy controller wasn't applying the attached rules, leaving the IP policy in its current state [#315](https://github.com/ngrok/ngrok-operator/pull/315)

## 0.10.0

### Added

- TLSEdge CRD, see the [TCP and TLS Edges Guide](https://github.com/ngrok/ngrok-operator/blob/main/docs/user-guide/tcp-tls-edges.md) for more details.

### Fixed

- Added support for TLS Renegotiation for backends that use it [#314](https://github.com/ngrok/ngrok-operator/pull/314)

## 0.9.1

### Fixed

- Send FQDN in SNI when using backend https [#304](https://github.com/ngrok/ngrok-operator/pull/304)

## 0.9.0

### Changed

- Update ngrok-go to 1.4.0 [#298](https://github.com/ngrok/ngrok-operator/pull/298)
- Tunnels are now unique in their respective namespace, not across the cluster [#281](https://github.com/ngrok/ngrok-operator/pull/281)
- The CRs that ingress controller creates are uniquely marked and managed by it. Other CRs created manually are no longer deleted when the ingress controller is not using them [#267](https://github.com/ngrok/ngrok-operator/issues/267); fixed for tunnel in [#285](https://github.com/ngrok/ngrok-operator/pull/285) and for https edges in [#286](https://github.com/ngrok/ngrok-operator/pull/286)
- Better error handling and retry, specifically for the case where we try to create an https edge for a domain which is not created yet [#283](https://github.com/ngrok/ngrok-operator/issues/283); fixed in [#288](https://github.com/ngrok/ngrok-operator/pull/288)
- Watch and apply ngrok module set CR changes [#287](https://github.com/ngrok/ngrok-operator/issues/287); fixed in [#290](https://github.com/ngrok/ngrok-operator/pull/290)
- Label https edges and tunnels with service UID to make them more unique within ngrok [#291](https://github.com/ngrok/ngrok-operator/issues/291); fixed in [#293](https://github.com/ngrok/ngrok-operator/pull/293) and [#302](https://github.com/ngrok/ngrok-operator/pull/302)

### Fixed

- The controller stopping at the first resource create [#270](https://github.com/ngrok/ngrok-operator/pull/270)
- Using `make deploy` now requires `NGROK_AUTHTOKEN` and `NGROK_API_KEY` to be set [#292](https://github.com/ngrok/ngrok-operator/pull/292)

## 0.8.1

### Fixed

- Handle special case for changing auth types that causes an error during state transition [#259](https://github.com/ngrok/ngrok-operator/pull/259)
- Handle IP Policy CRD state transitions in a safer way [#260](https://github.com/ngrok/ngrok-operator/pull/260)
- Better handling when changing pathType between 'Exact' and 'Prefix' [#262](https://github.com/ngrok/ngrok-operator/pull/262)

## 0.8.0

### Changed

- tunneldriver: plumb the version through ngrok-go [#228](https://github.com/ngrok/ngrok-operator/pull/228)
- Support HTTPS backends via service annotation [#238](https://github.com/ngrok/ngrok-operator/pull/238)

### Fixed

- Initialize route backends after module updates [#243](https://github.com/ngrok/ngrok-operator/pull/243)
- validate ip restriction rules, before creating the route [#241](https://github.com/ngrok/ngrok-operator/pull/241)
- Don't shadow remoteIPPolicies [#230](https://github.com/ngrok/ngrok-operator/pull/230)
- resolve some linter warnings [#229](https://github.com/ngrok/ngrok-operator/pull/229)

### Documentation

- Use direnv layout feature [#248](https://github.com/ngrok/ngrok-operator/pull/248)
- chore(readme): improve structure and content [#246](https://github.com/ngrok/ngrok-operator/pull/246)
- Added direnv and a nix devshell [#227](https://github.com/ngrok/ngrok-operator/pull/227)

### Testing Improvements

- fix route modules, using ngrokmoduleset instead [#239](https://github.com/ngrok/ngrok-operator/pull/239)
- Use raw yq output, split e2e runner from deployment [#235](https://github.com/ngrok/ngrok-operator/pull/235)
- Added e2e config init script [#234](https://github.com/ngrok/ngrok-operator/pull/234)
- Some updates to handle different cases for e2e run [#226](https://github.com/ngrok/ngrok-operator/pull/226).

## 0.7.0

### Changed

- Don't log errors on normal connection closing [#206](https://github.com/ngrok/ngrok-operator/pull/206).
- Updated `golang.org/x/net` to `0.9.0` [#215](https://github.com/ngrok/ngrok-operator/pull/215).

### Fixed

- Add support for named service ports [#222](https://github.com/ngrok/ngrok-operator/pull/222).

## 0.6.0

### Changed

- Added Ingress controller version to user-agent [#198](https://github.com/ngrok/ngrok-operator/pull/198).
- Don't default to development mode for logging [#199](https://github.com/ngrok/ngrok-operator/pull/199).

### Fixed

- Leaking TCP connections for every tunnel dial [#203](https://github.com/ngrok/ngrok-operator/pull/203).

## 0.5.0

### Changed

- Bumped go version to 1.20 [#167](https://github.com/ngrok/ngrok-operator/pull/167)
- Refactored Route Module Updates to be lazy [#168](https://github.com/ngrok/ngrok-operator/pull/168)
- Annotations for configuration have been removed in favor of grouping module configurations together in `NgrokModuleSet` custom resources [#170](https://github.com/ngrok/ngrok-operator/pull/170)

### Added

- Ran go mod tidy and added check to make sure its tidy before merge [#166](https://github.com/ngrok/ngrok-operator/pull/166)
- Added `NgrokModuleSet` CRD [#170](https://github.com/ngrok/ngrok-operator/pull/170)
- Added support for Circuit Breaker route module [#171](https://github.com/ngrok/ngrok-operator/pull/171)
- Added support for OIDC route module [#173](https://github.com/ngrok/ngrok-operator/pull/173)
- Added support for SAML route module [#186](https://github.com/ngrok/ngrok-operator/pull/186)
- Added support for OAuth route module [#192](https://github.com/ngrok/ngrok-operator/pull/192)

## 0.4.0

### Changed

- When no region override is passed to helm, the controller now does not default to the US and instead uses the closes geographic edge servers [#160](https://github.com/ngrok/ngrok-operator/pull/160)
- Ingress Class has Default set to false [#109](https://github.com/ngrok/ngrok-operator/pull/109)

### Added

- Allow controller name to be configured to support multiple ngrok ingress classes [#159](https://github.com/ngrok/ngrok-operator/pull/159)
- Allow the controller to be configured to only watch a single namespace [#157](https://github.com/ngrok/ngrok-operator/pull/157)
- Pass key/value pairs to helm that get added as json string metadata in ngrok api resources [#156](https://github.com/ngrok/ngrok-operator/pull/156)
- merge all ingress objects into a single store to derive Edges. [#129](https://github.com/ngrok/ngrok-operator/pull/129), [#10](https://github.com/ngrok/ngrok-operator/pull/10), [#131](https://github.com/ngrok/ngrok-operator/pull/131), [#137](https://github.com/ngrok/ngrok-operator/pull/137)
- Minimum TLS Version Route Module [#125](https://github.com/ngrok/ngrok-operator/pull/125)
- Webhook Verification Route Module [#122](https://github.com/ngrok/ngrok-operator/pull/122)
- Add/Remove Header Route Module [#121](https://github.com/ngrok/ngrok-operator/pull/121)
- Add IP Policy CRD and IP Policy Route Module [#120](https://github.com/ngrok/ngrok-operator/pull/120)
- Load certs from the directory `"/etc/ssl/certs/ngrok/"` for ngrok-go if present [#111](https://github.com/ngrok/ngrok-operator/pull/111)

### Fixed

- Fix bug from Driver and Store refactor so ingress status has CNAME Targets for custom domains updated correctly [#162](https://github.com/ngrok/ngrok-operator/pull/162)
- Reduce domain controller reconcile counts by not updating domains if they didn't change [#140](https://github.com/ngrok/ngrok-operator/pull/140)
- Remove routes from remote API when they are removed from the ingress object [#124](https://github.com/ngrok/ngrok-operator/pull/124)

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
