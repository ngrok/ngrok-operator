# Changelog

All notable changes to the helm chart will be documented in this file. Please see the top-level [CHANGELOG.md](../../CHANGELOG.md) for changes to the controller itself.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 0.21.1
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.21.0...helm-chart-0.21.1

- Update ngrok-operator image version to `0.19.1`
- Update Helm chart version to `0.21.1`

### Added

- feat(helm): Add support for setting pod terminationGracePeriodSeconds by @jonstacks in [#701](https://github.com/ngrok/ngrok-operator/pull/701)


## 0.21.1-rc.1
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.21.0...helm-chart-0.21.1-rc.1

- Update ngrok-operator image version to `0.19.1`
- Update Helm chart version to `0.21.1-rc.1`

### Added

- feat(helm): Add support for setting pod terminationGracePeriodSeconds by @jonstacks in [#701](https://github.com/ngrok/ngrok-operator/pull/701)


## 0.21.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.20.0...helm-chart-0.21.0

- Update ngrok-operator image version to `0.19.0`
- Update Helm chart version to `0.21.0`

### Added

- Add status and conditions to AgentEndpoint CRD by @alex-bezek in [#673](https://github.com/ngrok/ngrok-operator/pull/673)
- Configure status and conditions for Domain CRD to track provisioning process by @alex-bezek in [#678](https://github.com/ngrok/ngrok-operator/pull/678)
- Add status and conditions to CloudEndpoint CRD by @alex-bezek in [#682](https://github.com/ngrok/ngrok-operator/pull/682)
- Add status and conditions to IPPolicy CRD by @sabrina-ngrok in [#684](https://github.com/ngrok/ngrok-operator/pull/684)
- Add status and conditions to BoundEndpoint CRD by @alex-bezek in [#688](https://github.com/ngrok/ngrok-operator/pull/688)

### Changed

- chore(helm): Migrate to bitnami OCI library chart by @jonstacks in [#676](https://github.com/ngrok/ngrok-operator/pull/676)

## 0.20.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.20.0-rc.1...helm-chart-0.20.0

Warning: The ngrok-operator image version `0.18.0` and above include a breaking change due to the
sunsetting of some ngrok platform features. Before you upgrade to this version of the image,
please see https://github.com/ngrok/ngrok-operator/discussions/654.

### Removed
- [Breaking Change!] Remove Deprecated CRDs by @jonstacks in [#664](https://github.com/ngrok/ngrok-operator/pull/664).

## 0.20.0-rc.1
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.19.0...helm-chart-0.20.0-rc.1

Warning: The ngrok-operator image version `0.18.0` and above include a breaking change due to the
sunsetting of some ngrok platform features. Before you upgrade to this version of the image,
please see https://github.com/ngrok/ngrok-operator/discussions/654.

### Removed
- [Breaking Change!] Remove Deprecated CRDs by @jonstacks in [#664](https://github.com/ngrok/ngrok-operator/pull/664).

### Changed
- Update ngrok-operator image version to `0.18.0`

## 0.19.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.18.1...helm-chart-0.19.0

### Added

- feat: Add a default domain reclaim policy by @jonstacks in [#656](https://github.com/ngrok/ngrok-operator/pull/656)

### Changed

- bake ca into image by @Megalonia in [#626](https://github.com/ngrok/ngrok-operator/pull/626)
- update helm notes by @Alice-Lilith in [#652](https://github.com/ngrok/ngrok-operator/pull/652)

## 0.18.1
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.18.0...helm-chart-0.18.1

- Update ngrok-operator image to `0.16.1`

## 0.18.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.18.0-rc.1...helm-chart-0.18.0

* Moving 0.18.0-rc.1 to 0.18.0. See the 0.18.0-rc.1 notes for changes.

## 0.18.0-rc.1
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.17.1...helm-chart-0.18.0-rc.1

- Update ngrok-operator image to `0.16.0`

### Added
- add support for TCPRoute & TLSRoute by @Alice-Lilith in [#621](https://github.com/ngrok/ngrok-operator/pull/621)

## 0.17.1
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.17.0...helm-chart-0.17.1

- Update ngrok-operator image to `0.15.1`

### Added
- Add IngressEndpoint to KubernetesOperatorStatus by @masonj5n in [#591](https://github.com/ngrok/ngrok-operator/pull/591)

### Fixed
- fix(helm-chart): Resource requests for agent & bindings forwarder by @jonstacks in [#602](https://github.com/ngrok/ngrok-operator/pull/602). Fixes [#598](https://github.com/ngrok/ngrok-operator/issues/598)

### Removed
- remove bindings.description by @masonj5n in [#603](https://github.com/ngrok/ngrok-operator/pull/603)

## 0.17.0

**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.17.0-rc.4...helm-chart-0.17.0

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

## 0.17.0-rc.4

**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.17.0-rc.2...helm-chart-0.17.0-rc.4

### Added

- Endpoint pooling SDK support and auto pooling for `AgentEndpoint` resources by @jonstacks in [#581](https://github.com/ngrok/ngrok-operator/pull/581)
- Endpoint pooling support for `CloudEndpoint` resources (default=false), also supported on `Ingress`/`Service` resources that create endpoints using the `"k8s.ngrok.com/mapping-strategy": "endpoints"` annotation when `"k8s.ngrok.com/pooling-enabled": "true"` annotation is supplied by @Alice-Lilith in [#582](https://github.com/ngrok/ngrok-operator/pull/582)

## 0.17.0-rc.3

**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.17.0-rc.2...helm-chart-0.17.0-rc.3

### Added

- feat(helm): Add support for nodeSelectors, tolerations, and topologyS… by @jonstacks in [#559](https://github.com/ngrok/ngrok-operator/pull/559)
- feat: Copy domain status to cloud endpoint status by @jonstacks in [#566](https://github.com/ngrok/ngrok-operator/pull/566)
- feat: Opt-in to endpoints for Load balancer Services by @jonstacks in [#568](https://github.com/ngrok/ngrok-operator/pull/568)

### Changed

- Change allowed_urls to endpoint_selectors by @masonj5n in [#573](https://github.com/ngrok/ngrok-operator/pull/573)

### Fixed

- fix(helm): .Values.podLabels should not be included in the deployment selectors by @jonstacks in [#558](https://github.com/ngrok/ngrok-operator/pull/558)

### Removed

- Remove binding name by @masonj5n in [#567](https://github.com/ngrok/ngrok-operator/pull/567)

## 0.17.0-rc.2
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.17.0-rc.1...helm-chart-0.17.0-rc.2

### Changed

- Update ngrok-operator version to `0.14.1`
- Update Helm chart version to `0.17.0-rc.2`

## 0.17.0-rc.1
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.16.4...helm-chart-0.17.0-rc.1

- Update ngrok-operator version to `0.14.0`
- Update Helm chart version to `0.17.0-rc.1`

### Added

- add agentendpoint crd by @Alice-Lilith in [#525](https://github.com/ngrok/ngrok-operator/pull/525)
- agent endpoints work continued by @Alice-Lilith in [#538](https://github.com/ngrok/ngrok-operator/pull/538)
- make protocol optional for agentendpoint upstreams by @Alice-Lilith in [#547](https://github.com/ngrok/ngrok-operator/pull/547)
- remove boilerplate type field from trafficPolicy field by @Alice-Lilith in [#548](https://github.com/ngrok/ngrok-operator/pull/548)

### Fixed

- adjust bindings-forwarder deployment template by @masonj5n in [#529](https://github.com/ngrok/ngrok-operator/pull/529)


## 0.16.4
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.16.3...helm-chart-0.16.4

- Update ngrok-operator version to `0.13.7`
- Update Helm chart version to `0.16.4`

### Fixed

- Use GPG Key name instead of ID

## 0.16.3
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.16.2...helm-chart-0.16.3

- Update ngrok-operator version to `0.13.6`
- Update Helm chart version to `0.16.3`

### Changed

- Updated GPG Key

## 0.16.2
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.16.1...helm-chart-0.16.2

- Update ngrok-operator version to `0.13.5`
- Update Helm chart version to `0.16.2`

### Added

- Sign ngrok-operator Helm chart with GPG key by @hjkatz in [#514](https://github.com/ngrok/ngrok-operator/pull/514)

## 0.16.1
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.16.0...helm-chart-0.16.1

- Update ngrok-operator version to `0.13.4`
- Update Helm chart version to `0.16.1`

### Changed

- Update NOTES.txt for new feature sets by @hjkatz in [#508](https://github.com/ngrok/ngrok-operator/pull/508)

## 0.16.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.16.0...helm-chart-0.15.0

### Added

- Add support for 1-click demo mode by @hjkatz in [#503](https://github.com/ngrok/ngrok-operator/pull/503)

### Changed

- Bump image version to `0.13.3`

## 0.16.0-rc.3
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.16.0-rc.2...helm-chart-0.16.0-rc.3

### Added

- Support allowedURLs by @hjkatz in [#496](https://github.com/ngrok/ngrok-operator/pull/496)

### Changed

- Bump image version to `0.13.2`

## 0.16.0-rc.2
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.16.0-rc.1...helm-chart-0.16.0-rc.2

### Added
- Temporarily vendor ngrok intermediate CA for bindings by @hjkatz in [#487](https://github.com/ngrok/ngrok-operator/pull/487)


## 0.16.0-rc.1
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.14.3...helm-chart-0.16.0-rc.1

### :warning: :warning: :warning: Notice :warning: :warning: :warning:

This release is a release candidate for the upcoming 0.16.0 release. The helm chart has been renamed to `ngrok/ngrok-operator`. Please test this release in a non-production environment before upgrading your production environment. Documentation for migrating from `ngrok/kubernetes-ingress-controller` to `ngrok/ngrok-operator` can be found [here](/docs/deployment-guide/migrating.md).

### Added

#### Kubernetes Operator

The operator installation will now be registered with the ngrok API. This will allow you to view the status of the operator in the ngrok dashboard, see what version of the operator is running, and power new features
in the future. This is powered by a new `KubernetesOperator` CRD that is created by the operator in its
own namespace when it starts up.

- Register operator by @jonstacks in [#457](https://github.com/ngrok/ngrok-operator/pull/457)
- Add status to KubernetesOperator by @hjkatz in [#467](https://github.com/ngrok/ngrok-operator/pull/467)

#### Endpoint Bindings (private beta)

Endpoint bindings is a new feature that allows you to securely access a ngrok endpoint no matter where it is running. Specifically, Kubernetes bound endpoints allow you to project services running outside of your Kubernetes cluster or in other clusters into your cluster as native Kubernetes services.

- Add feature flag support for bindings by @hjkatz in [#424](https://github.com/ngrok/ngrok-operator/pull/424)
- Modify EndpointBinding CRD to reflect cardinality of bound Endpoints by @hjkatz in [#452](https://github.com/ngrok/ngrok-operator/pull/452)
- Implement AggregateBindingEndpoints for interacting with the ngrok api by @hjkatz in [#453](https://github.com/ngrok/ngrok-operator/pull/453)
- Implement BindingEndpoint polling by @hjkatz in [#458](https://github.com/ngrok/ngrok-operator/pull/458)
- Implement EndpointBinding -> Services creation by @hjkatz in [#459](https://github.com/ngrok/ngrok-operator/pull/459)
- Implement port allocation by @hjkatz in [#460](https://github.com/ngrok/ngrok-operator/pull/460)
- Bindings forwarder by @jonstacks in [#465](https://github.com/ngrok/ngrok-operator/pull/465)
- Add endpoint status to EndpointBinding kubectl output by @hjkatz in [#464](https://github.com/ngrok/ngrok-operator/pull/464)
- Ensure endpoint poller does not start until k8sop is regestered with API by @hjkatz in [#470](https://github.com/ngrok/ngrok-operator/pull/470)
- Rename EndpointBinding to BoundEndpoint by @hjkatz in [#475](https://github.com/ngrok/ngrok-operator/pull/475)
- Implement Target Metadata by @hjkatz in [#477](https://github.com/ngrok/ngrok-operator/pull/477)
- Bindings forwarder implementation by @jonstacks in [#476](https://github.com/ngrok/ngrok-operator/pull/476)

#### Helm values.yaml schema validation

The `ngrok-operator` helm chart now includes a `schema.json` file that can be used to validate the `values.yaml` file.

- Generate and commit schema.json file by @alex-bezek in [#472](https://github.com/ngrok/ngrok-operator/pull/472)

### Changed

#### Traffic Policy

Updates `TrafficPolicy` CRD and inline policy to support new phase-based names as well as the new `TrafficPolicy` API.

- update traffic policy for phase-based naming by @TheConcierge in [#456](https://github.com/ngrok/ngrok-operator/pull/456)

#### Values.yaml file deprecations

- Rename .Values.metaData -> .Values.ngrokMetadata by @hjkatz in [#434](https://github.com/ngrok/ngrok-operator/pull/434).
  - Deprecate `.Values.metaData` in favor of `.Values.ngrokMetadata` for clarity
  - Deprecate `.Values.ingressClass` in favor of `.Values.ingress.ingressClass` for feature set namespacing
  - Deprecate `.Values.useExperimentalGatewayApi` in favor of `.Values.gateway.enabled` for feature set namespacing
  - Deprecate `.Values.watchNamespace` in favor of `.Values.ingress.watchNamespace` for feature set namespacing
  - Deprecate `.Values.controllerName` in favor of `.Values.ingress.controllerName` for feature set namespacing

### New Contributors
- @TheConcierge made their first contribution in [#456](https://github.com/ngrok/ngrok-operator/pull/456)


## 0.15.0

### DEPRECATION ANNOUNCEMENT / ACTION REQUIRED

See Full Announcement: https://github.com/ngrok/kubernetes-ingress-controller/discussions/4

On Wednesday September 11th, 2024 this Helm Chart will be renamed to ngrok/ngrok-operator.

If you take no action, then you will not receive future updates to the ingress controller.

Please update your Helm repo with the following commands:

    $ helm repo add ngrok charts.ngrok.com --force-update
    $ helm repo update

If you need additional help, please reach out to our support team at https://ngrok.com/support

## 0.14.3
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.14.2...helm-chart-0.14.3

### Changed

- Update `icon` location in `Chart.yaml`. This is a passive change as we migrate to our helm repository to `charts.ngrok.com`.


## 0.14.2
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.14.1...helm-chart-0.14.2

### Added

- feat: Ability to specify cluster domain [#339](https://github.com/ngrok/ngrok-operator/pull/339). Thank you, @fr6nco !

### Changed

- Bump image version from `0.12.1` to `0.12.2`

## 0.14.1
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.14.0...helm-chart-0.14.1

### Changed

- Bump image version from `0.12.0` to `0.12.1`

## 0.14.0
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.13.0...helm-chart-0.14.0

### Added

- feat: Auto-provision domain for TLS Edges [#386]( https://github.com/ngrok/ngrok-operator/pull/386)
- feat: Support for Load Balancer services [#387](https://github.com/ngrok/ngrok-operator/pull/387)
- feat: Support TLS termination in modulesets for Load Balancer Services [388](https://github.com/ngrok/ngrok-operator/pull/388)

## 0.13.0

**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/helm-chart-0.12.4...helm-chart-0.13.0

**Important**: If you are upgrading from a previous version and are using `helm install` or `helm upgrade`, you will need to manually apply the changes to the CRDs. This is because the CRDs are not [updated automatically when the chart is updated](https://helm.sh/docs/chart_best_practices/custom_resource_definitions/#some-caveats-and-explanations). To do this, apply the contents of the `crds` directory in the chart to your cluster.

Ex (from the root of the repository):
```shell
kubectl apply -f ./helm/ingress-controller/templates/crds/
```

### Added

- root-cas setting [#371](https://github.com/ngrok/ngrok-operator/pull/371)
  Takes an install option for `--set rootCAs=host` and plumb the isHostCA check into the caCerts for it to just get the host certs.
- feat: Add support for mutualTLS [#373](https://github.com/ngrok/ngrok-operator/pull/373)

### Changed

- Update nix flake, go version, and Makefile dep versions [#379](https://github.com/ngrok/ngrok-operator/pull/379)

## 0.12.4

- Add the `apiURL` value.
  This sets the ngrok API endpoint used by the controller.
  It corresponds to the `--api-url` argument to the manager binary.

- Update to version 0.10.4 of the ingress controller.
  See its changes [here](../../CHANGELOG.md#0104).

## 0.12.1

- Update to version 0.10.1 of the ingress controller, which includes:
  - IPPolicy controller wasn't applying the attached rules, leaving the IP policy in its current state [#315](https://github.com/ngrok/ngrok-operator/pull/315)

## 0.12.0

- Update to version 0.10.0 of the ingress controller, this includes:
  - TLSEdge support - see the [TCP and TLS Edges Guide](https://github.com/ngrok/ngrok-operator/blob/main/docs/user-guide/tcp-tls-edges.md) for more details.
  - A fix for renegotiating TLS backends

## 0.11.0

** Important ** This version of the controller changes the ownership model for https edge and tunnel CRs. To ease out the transition to the new ownership, make sure to run `migrate-edges.sh` and `migrate-tunnels.sh` scripts before installing the new version.

### Changed
- Specify IPPolicyRule action as an enum of (allow,deny) as part of [#260](https://github.com/ngrok/ngrok-operator/pull/260)
- Handle special case for changing auth types that causes an error during state transition [#259](https://github.com/ngrok/ngrok-operator/pull/259)
- Better handling when changing pathType between 'Exact' and 'Prefix' [#262](https://github.com/ngrok/ngrok-operator/pull/262)
- Update ngrok-go to 1.4.0 [#298](https://github.com/ngrok/ngrok-operator/pull/298)
- Tunnels are now unique in their respective namespace, not across the cluster [#281](https://github.com/ngrok/ngrok-operator/pull/281)
- The CRs that ingress controller creates are uniquely marked and managed by it. Other CRs created manually are no longer deleted when the ingress controller is not using them [#267](https://github.com/ngrok/ngrok-operator/issues/267); fixed for tunnel in [#285](https://github.com/ngrok/ngrok-operator/pull/285) and for https edges in [#286](https://github.com/ngrok/ngrok-operator/pull/286)
- Better error handling and retry, specifically for the case where we try to create an https edge for a domain which is not created yet [#283](https://github.com/ngrok/ngrok-operator/issues/283); fixed in [#288](https://github.com/ngrok/ngrok-operator/pull/288)
- Watch and apply ngrok module set CR changes [#287](https://github.com/ngrok/ngrok-operator/issues/287); fixed in [#290](https://github.com/ngrok/ngrok-operator/pull/290)
- Label https edges and tunnels with service UID to make them more unique within ngrok [#291](https://github.com/ngrok/ngrok-operator/issues/291); fixed in [#293](https://github.com/ngrok/ngrok-operator/pull/293) and [#302](https://github.com/ngrok/ngrok-operator/pull/302)

### Added
- Add support for configuring pod affinities, pod disruption budget, and priorityClassName [#258](https://github.com/ngrok/ngrok-operator/pull/258)
- The controller stopping at the first resource create [#270](https://github.com/ngrok/ngrok-operator/pull/270)
- Using `make deploy` now requires `NGROK_AUTHTOKEN` and `NGROK_API_KEY` to be set [#292](https://github.com/ngrok/ngrok-operator/pull/292)

## 0.10.0

### Added
- Support HTTPS backends via service annotation [#238](https://github.com/ngrok/ngrok-operator/pull/238)

### Changed
- Normalize all ngrok `.io` TLD to `.app` TLD [#240](https://github.com/ngrok/ngrok-operator/pull/240)
- Chart Icon

### Fixed
- Add namespace to secret [#244](https://github.com/ngrok/ngrok-operator/pull/244). Thank you for the contribution, @vincetse!

## 0.9.0
### Added
- Add a 'podLabels' option to the helm chart [#212](https://github.com/ngrok/ngrok-operator/pull/212).
- Permission to `get`,`list`, and `watch` `services` [#222](https://github.com/ngrok/ngrok-operator/pull/222).

## 0.8.0
### Changed
- Log Level configuration to helm chart [#199](https://github.com/ngrok/ngrok-operator/pull/199).
- Bump default controller image to use `0.6.0` release [#204](https://github.com/ngrok/ngrok-operator/pull/204).

### Fixed
- update default-container annotation so logs work correctly [#197](https://github.com/ngrok/ngrok-operator/pull/197)

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
- Ingress Class has Default set to false [#109](https://github.com/ngrok/ngrok-operator/pull/109)

### Added
- Allow controller name to be configured to support multiple ngrok ingress classes [#159](https://github.com/ngrok/ngrok-operator/pull/159)
- Allow the controller to be configured to only watch a single namespace [#157](https://github.com/ngrok/ngrok-operator/pull/157)
- Pass key/value pairs to helm that get added as json string metadata in ngrok api resources [#156](https://github.com/ngrok/ngrok-operator/pull/156)
- Add IP Policy CRD and IP Policy Route Module [#120](https://github.com/ngrok/ngrok-operator/pull/120)
- Load certs from the directory `"/etc/ssl/certs/ngrok/"` for ngrok-go if present [#111](https://github.com/ngrok/ngrok-operator/pull/111)

## 0.5.0
### Changed
- Renamed chart from `ngrok-operator` to `kubernetes-ingress-controller`.
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
