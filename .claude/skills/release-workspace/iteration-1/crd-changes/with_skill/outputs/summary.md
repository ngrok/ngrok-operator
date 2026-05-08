## Release Preparation Summary

### Data Source
Ran `gather-release-data.sh` which found 22 PRs merged since the last release tags:
- Container: `ngrok-operator-0.20.3`
- Helm operator: `helm-chart-ngrok-operator-0.22.2`
- CRDs: `helm-chart-ngrok-crds-0.2.1`

### PR Breakdown by Component (new_for)
- **Container**: 18 PRs (features, fixes, CI improvements, dependency updates)
- **Helm operator**: 1 PR (RBAC patch permissions for finalizers)
- **CRDs**: 4 PRs (breaking rename, new columns, enum fix, scaffolding cleanup)

### Version Suggestions
| Component | Current | Suggested | Reason |
|-----------|---------|-----------|--------|
| Container | 0.20.3 | **0.21.0** | New features: `-o wide` columns (#772), metadata/description annotations (#788) |
| Helm operator | 0.22.2 | **0.22.3** | Patch: RBAC fix only, no breaking helm changes |
| CRDs | 0.2.1 | **0.3.0** | Breaking: BoundEndpoint field rename `endpointURI` -> `endpointURL` (#779) |

### Breaking Changes
- **PR #779**: Renames `BoundEndpoint.spec.endpointURI` to `spec.endpointURL`. The old field is kept as deprecated for backward compatibility but the CRD schema changed, warranting a minor version bump on the CRDs chart.

### Key CRD Changes (4 PRs)
1. **#772** - Added printer columns for Ready condition reason/message on 5 CRDs (AgentEndpoint, CloudEndpoint, Domain, IPPolicy, BoundEndpoint)
2. **#779** - BREAKING: Renamed BoundEndpoint `endpointURI` -> `endpointURL`
3. **#792** - Fixed AgentEndpoint CRD: ProxyProtocolVersion enum changed from integer to string values
4. **#803** - Cleaned up scaffolding comments from IPPolicy/CloudEndpoint/NgrokTrafficPolicy types, regenerated CRD templates

### Notable Container Changes
- Go upgraded from 1.25.7 to 1.26.1
- New `k8s.ngrok.com/metadata` and `k8s.ngrok.com/description` annotations for Ingress/Gateway
- Multiple bug fixes: driver sync logging, object-modified errors, CloudEndpoint status handling, data race in health checker, API pagination error propagation
- CI improvements: consolidated nix setup, added `go fix` gate, copilot setup steps

### Files That Would Be Modified
- `VERSION` (0.20.3 -> 0.21.0)
- `CHANGELOG.md` (new 0.21.0 entry)
- `helm/ngrok-operator/Chart.yaml` (version 0.22.3, appVersion 0.21.0, CRDs dependency 0.3.0)
- `helm/ngrok-operator/CHANGELOG.md` (new 0.22.3 entry)
- `helm/ngrok-crds/Chart.yaml` (version 0.3.0, appVersion 0.3.0)
- `helm/ngrok-crds/CHANGELOG.md` (new 0.3.0 entry)
