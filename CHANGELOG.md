# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 0.17.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-0.16.1...ngrok-operator-0.17.0

### Added
- feat: Add a reclaimPolicy to domains by @jonstacks in [#641](https://github.com/ngrok/ngrok-operator/pull/641)
- Add gatewayclass to store by @jonstacks in [#650](https://github.com/ngrok/ngrok-operator/pull/650)
- feat(gateway-api): Set accepted conditions on gateways & httproutes by @jonstacks in [#651](https://github.com/ngrok/ngrok-operator/pull/651)
- feat(gateway-api): Add gateay status addresses by @jonstacks in [#653](https://github.com/ngrok/ngrok-operator/pull/653)
- feat(gateway-api): Validate gateway listeners hostname and port by @jonstacks in [#658](https://github.com/ngrok/ngrok-operator/pull/658)
- feat(cloud-endpoints): Add CloudEndpoint ID to printer column by @jonstacks in [#647](https://github.com/ngrok/ngrok-operator/pull/647)

### Changed
- bake ca into image by @Megalonia in [#626](https://github.com/ngrok/ngrok-operator/pull/626)
- chore(testing): Add tests for Domain Reconciler by @jonstacks in [#646](https://github.com/ngrok/ngrok-operator/pull/646)
- Combine binaries by @masonj5n in [#648](https://github.com/ngrok/ngrok-operator/pull/648)
- refactor: Update statuses concurrently and pre-calculate domains by @jonstacks in [#649](https://github.com/ngrok/ngrok-operator/pull/649)
- perf: Enable 'use-errors-new' revive linter by @jonstacks in [#659](https://github.com/ngrok/ngrok-operator/pull/659)
- chore(deprecation): Add deprecation notice to CRDs by @jonstacks in [#655](https://github.com/ngrok/ngrok-operator/pull/655). See [discussion](https://github.com/ngrok/ngrok-operator/discussions/654) for details.
- chore(deps): Updated to go 1.24 by @jonstacks in [#642](https://github.com/ngrok/ngrok-operator/pull/642) and [#661](https://github.com/ngrok/ngrok-operator/pull/661).
- chore: Use slices and maps from stdlib by @jonstacks in [#638](https://github.com/ngrok/ngrok-operator/pull/638)

### Fixed
- Periodically re-reconcile boundendpoints and refactor connectivity check by @masonj5n in [#628](https://github.com/ngrok/ngrok-operator/pull/628).
- fix: Hanging connections by @jonstacks in [#657](https://github.com/ngrok/ngrok-operator/pull/657)
- fix: httproute should enqueue requests when gateways are deleted by @jonstacks in [#660](https://github.com/ngrok/ngrok-operator/pull/660)

## 0.16.1
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-0.16.0...ngrok-operator-0.16.1

### Added

- enable more golangci linters by @Alice-Lilith in [#630](https://github.com/ngrok/ngrok-operator/pull/630)

### Removed

- remove generated bindings from ingress/gateway resources by @Alice-Lilith in [#635](https://github.com/ngrok/ngrok-operator/pull/635)

### Fixed

- fix(api-manager): panic when TCP/TLS Route CRDs are not installed. by @jonstacks in [#636](https://github.com/ngrok/ngrok-operator/pull/636)


## 0.16.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-0.15.1...ngrok-operator-0.16.0

### Added
- Consume IngressEndpoint from status by @masonj5n in [#607](https://github.com/ngrok/ngrok-operator/pull/607)
- add new mapping strategy by @Alice-Lilith in [#612](https://github.com/ngrok/ngrok-operator/pull/612)
- add support for TCPRoute & TLSRoute by @Alice-Lilith in [#621](https://github.com/ngrok/ngrok-operator/pull/621)
- Lb services reserve tcp addr for endpoints by @jonstacks in [#622](https://github.com/ngrok/ngrok-operator/pull/622)

### Changed
- unconditionally create operator with ngrok api by @masonj5n in [#614](https://github.com/ngrok/ngrok-operator/pull/614)
- only make tunnels for edges strategy by @Alice-Lilith in [#623](https://github.com/ngrok/ngrok-operator/pull/623)

### Fixed
- add deployment name and version to _update calls by @Megalonia in [#610](https://github.com/ngrok/ngrok-operator/pull/610)
- fix: Agent endpoint domain strategy by @jonstacks in [#613](https://github.com/ngrok/ngrok-operator/pull/613)
- fix: Empty TLS cert on creation when bindings are enabled by @jonstacks in [#615](https://github.com/ngrok/ngrok-operator/pull/615)
- fix: traffic-policy annotation not working when using mapping-strategy of edges by @jonstacks in [#616](https://github.com/ngrok/ngrok-operator/pull/616)
- set operator description from values.yaml description by @masonj5n in [#605](https://github.com/ngrok/ngrok-operator/pull/605)
- fix: Retry cloud endpoint creation on pooling state errors by @jonstacks in [#624](https://github.com/ngrok/ngrok-operator/pull/624)


## 0.15.1
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-0.15.0...ngrok-operator-0.15.1

### Changed
- chore(deps): Upgrade go to 1.24.1 by @jonstacks in [#601](https://github.com/ngrok/ngrok-operator/pull/601)
- chore(lint): Enable go-critic by @jonstacks in [#604](https://github.com/ngrok/ngrok-operator/pull/604)

### Fixed
- fix: HTTP/2 upstreams for edges & endpoints by @jonstacks in [#606](https://github.com/ngrok/ngrok-operator/pull/606). Fixes [#572](https://github.com/ngrok/ngrok-operator/issues/572).
- fix: add support for https agent endpoints from ingress/gwapi by @Alice-Lilith in [#599](https://github.com/ngrok/ngrok-operator/pull/599)

## 0.15.0

**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-0.14.3...ngrok-operator-0.15.0

### Added

- Gateway API to Endpoints conversion by @Alice-Lilith in [#588](https://github.com/ngrok/ngrok-operator/pull/588)

- Gateway API ReferenceGrant support (endpoints only) by @Alice-Lilith in [#589](https://github.com/ngrok/ngrok-operator/pull/589). When using the Gateway API configuration and translating to endpoints, ReferenceGrants will be required for cross-namespace references as dictated by Gateway API. You may opt-out of support for ReferenceGrants and always allow cross namespace references using the `--disable-reference-grants` in the pod arguments or using the helm value `gateway.disableReferenceGrants`.

- Support for the `k8s.ngrok.com/bindings` annotation (endpoints only) to set bindings on endpoints generated from Ingresses and Gateway API config by @Alice-Lilith in [#593](https://github.com/ngrok/ngrok-operator/pull/593).

- Implemented support for configuring upstream client certificates in the `AgentEndpoint` respirces and Gateway API Gateway Backend TLS config to configure client certificates to be sent to upstream services (endpoints only) bby @Alice-Lilith in [#594](https://github.com/ngrok/ngrok-operator/pull/594)

### Changed

- Endpoints are now the default for translations from `Ingress`, LoadBalancer Service, and Gateway API config by @Alice-Lilith in [#596](https://github.com/ngrok/ngrok-operator/pull/596) The `k8s.ngrok.com/mapping-strategy: endpoints` annotation is no longer required for endpoints conversion. Similarly, you may still use Edges instead with the `k8s.ngrok.com/mapping-strategy: edges` annotation, but Edges will be removed in a future release.

- Auto enable Gateway API support by @Alice-Lilith in [#592](https://github.com/ngrok/ngrok-operator/pull/592). Instead of needing to opt-in to Gateway API support, we now enable it by default if the Gateway API CRDs are detected. You may still opt-out using the `enable-feature-gateway` flag on the pod arguments or using the helm value `gateway.enabled`.

- Replaced binding name printcolumn with endpoint selectors for KubernetesOperator resources by @masonj5n in [#586](https://github.com/ngrok/ngrok-operator/pull/586)

- Updated notes.txt to reflect bindings API changes by @masonj5n in [#585](https://github.com/ngrok/ngrok-operator/pull/585)

- Support for latest bindings configuration (endpoints only) with the `k8s.ngrok.com/bindings` annotation to set bindings on endpoints generated from LoadBalancer Service resoruces by @Megalonia in [#590](https://github.com/ngrok/ngrok-operator/pull/590)

## 0.14.3
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-0.14.2...ngrok-operator-0.14.3

### Added

- Endpoint pooling SDK support and auto pooling for `AgentEndpoint` resources by @jonstacks in [#581](https://github.com/ngrok/ngrok-operator/pull/581)
- Endpoint pooling support for `CloudEndpoint` resources (default=false), also supported on `Ingress`/`Service` resources that create endpoints using the `"k8s.ngrok.com/mapping-strategy": "endpoints"` annotation when `"k8s.ngrok.com/pooling-enabled": "true"` annotation is supplied by @Alice-Lilith in [#582](https://github.com/ngrok/ngrok-operator/pull/582)

## 0.14.2
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-0.14.1...ngrok-operator-0.14.2

### Added

- Add conversion support from ingress to endpoints by @Alice-Lilith in [#562](https://github.com/ngrok/ngrok-operator/pull/562)
- feat: Add trafficpolicy package and conversion util by @jonstacks in [#564](https://github.com/ngrok/ngrok-operator/pull/564)
- feat: Copy domain status to cloud endpoint status by @jonstacks in [#566](https://github.com/ngrok/ngrok-operator/pull/566)
- feat: Opt-in to endpoints for Load balancer Services by @jonstacks in [#568](https://github.com/ngrok/ngrok-operator/pull/568)
- feat(ci): Use codecov for coverage reports by @jonstacks in [#571](https://github.com/ngrok/ngrok-operator/pull/571)

### Changed

- chore(deps): Update ngrok-api-go by @jonstacks in [#560](https://github.com/ngrok/ngrok-operator/pull/560)
- Change `allowed_urls` to `endpoint_selectors` by @masonj5n in [#573](https://github.com/ngrok/ngrok-operator/pull/573)
- chore(ci): Make codecov patch status informational for now as well by @jonstacks in [#577](https://github.com/ngrok/ngrok-operator/pull/577)
- update use endpoints annotation by @Alice-Lilith in [#579](https://github.com/ngrok/ngrok-operator/pull/579)

### Fixed

- fix(service-controller): Service controller uses configured cluster domain by @jonstacks in [#552](https://github.com/ngrok/ngrok-operator/pull/552)
- fix(ngrok-api-go): Update to client that doesn't panic for get_bound_endpoints by @jonstacks in [#561](https://github.com/ngrok/ngrok-operator/pull/561)
- fix: managerdriver tests not being run by @jonstacks in [#569](https://github.com/ngrok/ngrok-operator/pull/569)
- fix(ci): Disable bindings for e2e tests by @jonstacks in [#570](https://github.com/ngrok/ngrok-operator/pull/570)
- add newly created agent endpoints to the map by @Alice-Lilith in [#574](https://github.com/ngrok/ngrok-operator/pull/574)
- fix(httpsedges): HTTPS Edges should retry on hostport already in use by @jonstacks in [#576](https://github.com/ngrok/ngrok-operator/pull/576)

### Removed

- Remove binding name by @masonj5n in [#567](https://github.com/ngrok/ngrok-operator/pull/567)

## 0.14.1
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-0.14.0...ngrok-operator-0.14.1

### Fixed

- Fix http endpoint scheme by @jonstacks in [#549](https://github.com/ngrok/ngrok-operator/pull/549)


## 0.14.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-0.13.7...ngrok-operator-0.14.0

- Update ngrok-operator version to `0.14.0`
- Update Helm chart version to `0.17.0-rc.1`

### Added
- add agentendpoint crd by @Alice-Lilith in [#525](https://github.com/ngrok/ngrok-operator/pull/525)
- agent endpoints work continued by @Alice-Lilith in [#538](https://github.com/ngrok/ngrok-operator/pull/538)
- make protocol optional for agentendpoint upstreams by @Alice-Lilith in [#547](https://github.com/ngrok/ngrok-operator/pull/547)
- remove boilerplate type field from trafficPolicy field by @Alice-Lilith in [#548](https://github.com/ngrok/ngrok-operator/pull/548)

### Changed

- Error with invalid API key by @hjkatz in [#524](https://github.com/ngrok/ngrok-operator/pull/524)

### Fixed

- fix: Re-create tunnel if forwardsTo or appProto changes by @jonstacks in [#527](https://github.com/ngrok/ngrok-operator/pull/527)
- adjust bindings-forwarder deployment template by @masonj5n in [#529](https://github.com/ngrok/ngrok-operator/pull/529)
- skip no-op status and annotation updates for boundendpoint reconciliation by @masonj5n in [#537](https://github.com/ngrok/ngrok-operator/pull/537)
- fix endpoint url validation helper and add tests by @Alice-Lilith in [#544](https://github.com/ngrok/ngrok-operator/pull/544)
- fix(agent-endpoints): Delete agent endpoint instead of tunnel by @jonstacks in [#543](https://github.com/ngrok/ngrok-operator/pull/543)

### Internal / CI

- Add artifacthub badge by @hjkatz in [#513](https://github.com/ngrok/ngrok-operator/pull/513)
- feat: add chainsaw based e2e tests by @eddycharly in [#506](https://github.com/ngrok/ngrok-operator/pull/506)
- e2e updates / fixes 1 by @hjkatz in [#526](https://github.com/ngrok/ngrok-operator/pull/526)
- Trigger ci e2e with Makefile change by @hjkatz in [#528](https://github.com/ngrok/ngrok-operator/pull/528)
- Use correct namespace for debugging by @hjkatz in [#530](https://github.com/ngrok/ngrok-operator/pull/530)
- Ensure build-and-test runs on push events by @hjkatz in [#531](https://github.com/ngrok/ngrok-operator/pull/531)
- E2E 5, E5E by @hjkatz in [#532](https://github.com/ngrok/ngrok-operator/pull/532)
- Fix typo for changes to tests ; Add scripts/e2e.sh too by @hjkatz in [#534](https://github.com/ngrok/ngrok-operator/pull/534)
- Checkout fork PR HEAD for e2e tests by @hjkatz in [#535](https://github.com/ngrok/ngrok-operator/pull/535)
- Enable deny gate for 'safe to test' label by @hjkatz in [#539](https://github.com/ngrok/ngrok-operator/pull/539)
- Add found labels debug message by @hjkatz in [#540](https://github.com/ngrok/ngrok-operator/pull/540)
- feat: Use a merge group for e2e tests by @jonstacks in [#542](https://github.com/ngrok/ngrok-operator/pull/542)
- Add some e2e tests as a feature branch by @hjkatz in [#533](https://github.com/ngrok/ngrok-operator/pull/533)
- feat(ci): Update release script by @jonstacks in [#545](https://github.com/ngrok/ngrok-operator/pull/545)

### New Contributors
- @eddycharly made their first contribution in https://github.com/ngrok/ngrok-operator/pull/506
- @masonj5n made their first contribution in https://github.com/ngrok/ngrok-operator/pull/529
- @Alice-Lilith made their first contribution in https://github.com/ngrok/ngrok-operator/pull/525

## 0.13.7
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-0.13.6...ngrok-operator-0.13.7

- Update ngrok-operator version to `0.13.7`
- Update Helm chart version to `0.16.4`

### Fixed

- Use GPG Key name instead of ID

## 0.13.6
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-0.13.5...ngrok-operator-0.13.6

- Update ngrok-operator version to `0.13.6`
- Update Helm chart version to `0.16.3`

### Changed

- Updated GPG Key

## 0.13.5
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-0.13.4...ngrok-operator-0.13.5

- Update ngrok-operator version to `0.13.5`
- Update Helm chart version to `0.16.2`

### Added

- Sign ngrok-operator Helm chart with GPG key by @hjkatz in [#514](https://github.com/ngrok/ngrok-operator/pull/514)

### Fixed

- Update README.md with new rename by @hjkatz in [#516](https://github.com/ngrok/ngrok-operator/pull/516)

## 0.13.4
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-0.13.3...ngrok-operator-0.13.4

- Update ngrok-operator version to `0.13.4`
- Update Helm chart version to `0.16.1`

### Added

- Add `scripts/release.sh` and `make release` by @hjkatz in [#507](https://github.com/ngrok/ngrok-operator/pull/507) [#509](https://github.com/ngrok/ngrok-operator/pull/509) [#510](https://github.com/ngrok/ngrok-operator/pull/510)

## 0.13.3
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-0.13.1...ngrok-operator-0.13.3

### Added

- Add support for 1-click demo mode by @hjkatz in [#503](https://github.com/ngrok/ngrok-operator/pull/503)
- Enable automatic Helm releases for `ngrok/ngrok-operator` in `.github/workflows` by @hjkatz in (this PR)

### Fixed

- Hide `kind: KubernetesOperator` API registration behind the `bindings.enable` feature flag by @hjkatz in [#504](https://github.com/ngrok/ngrok-operator/pull/504)

## 0.13.2
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-0.13.1...ngrok-operator-0.13.2

### Added

- Support allowedURLs by @hjkatz in [#496](https://github.com/ngrok/ngrok-operator/pull/496)

### Fixed

- fix: Clear status and re-reconcile if httpsedge is not found by @jonstacks in [#501](https://github.com/ngrok/ngrok-operator/pull/501)
- Use the previously ingress in the error messages by @alex-bezek in [#500](https://github.com/ngrok/ngrok-operator/pull/500)

## 0.13.1
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/ngrok-operator-0.13.0...ngrok-operator-0.13.1

### Changed

- Use goroutine instead of errGroup by @hjkatz in [#497](https://github.com/ngrok/ngrok-operator/pull/497)
- Reduce polling interval to 10 seconds by @hjkatz in [#491](https://github.com/ngrok/ngrok-operator/pull/491)

### Fixed

- fix: domain stuck when ID is not found by @jonstacks in [#488](https://github.com/ngrok/ngrok-operator/pull/488)
- Ensure the TLS secret is valid otherwise upsert by @hjkatz in [#486](https://github.com/ngrok/ngrok-operator/pull/486)
- Use unique context for endpoint poller reconcile actions by @hjkatz in [#489](https://github.com/ngrok/ngrok-operator/pull/489)
- fix: Make sure we update the status by @jonstacks in [#493](https://github.com/ngrok/ngrok-operator/pull/493)
- Add more logging for binding forwarder mux handshake by @hjkatz in [#494](https://github.com/ngrok/ngrok-operator/pull/494)
- fix: Better migration path from the ngrok kuberntes-ingress-controller to the ngrok-operator by @jonstacks in [#495](https://github.com/ngrok/ngrok-operator/pull/495)

## 0.13.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/kubernetes-ingress-controller-0.12.2...ngrok-operator-0.13.0


### :warning: :warning: :warning: Notice :warning: :warning: :warning:

This version of the controller is not backwards compatible with previous versions and is only compatible with
version 0.16.0 of the `ngrok/ngrok-operator` helm chart and later. Using this version or later of the controller with the `ngrok/kubernetes-ingress-controller` helm chart will result in the controller not functioning correctly.

Even though we are in major version 0, and semver v2.0.0 allows that anything may change until a 1.0.0 release, we try not to break backwards compatibility. However, this change is necessary to support new features and improvements in the operator.

### Added

#### Kubernetes Operator

The operator installation will now be registered with the ngrok API. This will allow you to view the status of the operator in the ngrok dashboard, see what version of the operator is running, and power new features
in the future. This is powered by a new `KubernetesOperator` CRD that is created by the operator in its
own namespace when it starts up.

- Register operator by @jonstacks in [#457](https://github.com/ngrok/ngrok-operator/pull/457)
- Add status to KubernetesOperator by @hjkatz in [#467](https://github.com/ngrok/ngrok-operator/pull/467)
- fix: Add nil checks to prevent potential panics by @jonstacks in [#483](https://github.com/ngrok/ngrok-operator/pull/483)

#### Endpoint Bindings (private beta)

Endpoint bindings is a new feature that allows you to securely access a ngrok endpoint no matter where it is running. Specifically, Kubernetes bound endpoints allow you to project services running outside of your Kubernetes cluster or in other clusters into your cluster as native Kubernetes services.

- Add feature flag support for bindings by @hjkatz in [#424](https://github.com/ngrok/ngrok-operator/pull/424)
- feat: Initial bindings driver by @stacks in [#450](https://github.com/ngrok/ngrok-operator/pull/450)
- Modify EndpointBinding CRD to reflect cardinality of bound Endpoints by @hjkatz in [#452](https://github.com/ngrok/ngrok-operator/pull/452)
- Implement AggregateBindingEndpoints for interacting with the ngrok api by @hjkatz in [#453](https://github.com/ngrok/ngrok-operator/pull/453)
- Implement BindingEndpoint polling by @hjkatz in [#458](https://github.com/ngrok/ngrok-operator/pull/458)
- Implement EndpointBinding -> Services creation by @hjkatz in [#459](https://github.com/ngrok/ngrok-operator/pull/459)
- Implement port allocation by @hjkatz in [#460](https://github.com/ngrok/ngrok-operator/pull/460)
- Bindings forwarder by @jonstacks in [#465](https://github.com/ngrok/ngrok-operator/pull/465)
- Add endpoint status to EndpointBinding kubectl output by @hjkatz in [#464](https://github.com/ngrok/ngrok-operator/pull/464)
- chore: Update ngrok-api-go to pull in new changes by @jonstacks in [#468](https://github.com/ngrok/ngrok-operator/pull/468)
- Ensure endpoint poller does not start until k8sop is regestered with API by @hjkatz in [#470](https://github.com/ngrok/ngrok-operator/pull/470)
- Rename EndpointBinding to BoundEndpoint by @hjkatz in [#475](https://github.com/ngrok/ngrok-operator/pull/475)
- Implement Target Metadata by @hjkatz in [#477](https://github.com/ngrok/ngrok-operator/pull/477)
- Bindings forwarder implementation by @jonstacks in [#476](https://github.com/ngrok/ngrok-operator/pull/476)
- Ensure KubernetesOperator.Status.EnabledFeatures is set properly from the API by @hjkatz in [#480](https://github.com/ngrok/ngrok-operator/pull/480)
- Add equality tests for Target.Metadata by @hjkatz in [#482](https://github.com/ngrok/ngrok-operator/pull/482)
- feat: BoundEndpointPoller polls from the API by @jonstacks in [#481](https://github.com/ngrok/ngrok-operator/pull/481)


#### Cloud Endpoints (private beta)

[Cloud Endpoints](https://ngrok.com/docs/network-edge/cloud-endpoints/) can now be created and managed by the operator via a new `CloudEndpoint` CRD.

- Allow configuring ngrok Cloud Endpoints using CRDs by @alex-bezek in [#471](https://github.com/ngrok/ngrok-operator/pull/471)


### Changed

#### Ingress/Gateway

* Seed additional types when first starting by @alex-bezek in [#431](https://github.com/ngrok/ngrok-operator/pull/431).

#### Traffic Policy

Updates `TrafficPolicy` CRD and inline policy to support new phase-based names as well as the new `TrafficPolicy` API.

- update traffic policy for phase-based naming by @TheConcierge in [#456](https://github.com/ngrok/ngrok-operator/pull/456)


#### Splitting controllers into multiple manager instances

The controllers have been split into multiple manager instances to improve performance and scalability. This now allows the ngrok agent manager which handles traffic to run independently of the API managers which reconcile CRDs with the ngrok API. This change also allows for more fine-grained control over the controllers and their resources.

- refactor: Split the agent and API controllers by @jonstacks in [#446](https://github.com/ngrok/ngrok-operator/pull/446)

### Fixes

#### Gateway API

- fix: Add `GatewayClass` controller by @jonstacks in [#484](https://github.com/ngrok/ngrok-operator/pull/484)


### Documentation
* Update README.md to use ngrok Kubernetes Operator instead of ingress controller. by @stmcallister in [#433](https://github.com/ngrok/ngrok-operator/pull/433)

### New Contributors
- @TheConcierge made their first contribution in [#456](https://github.com/ngrok/ngrok-operator/pull/456)



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
- feat: Support TLS termination in modulesets for Load Balancer Services [#388](https://github.com/ngrok/ngrok-operator/pull/388)

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
