## Release Preparation Summary

### Data Source
Ran `gather-release-data.sh` which found 22 PRs merged since the last release tags.

### Last Release Tags
| Component | Tag | Version |
|-----------|-----|---------|
| Container | `ngrok-operator-0.20.3` | 0.20.3 |
| Helm operator | `helm-chart-ngrok-operator-0.22.2` | 0.22.2 |
| Helm CRDs | `helm-chart-ngrok-crds-0.2.1` | 0.2.1 |

### Suggested Next Versions
| Component | Current | Next | Reason |
|-----------|---------|------|--------|
| Container | 0.20.3 | **0.21.0** | New features (metadata/description annotations, wide output columns) + breaking change (endpointURI -> endpointURL) |
| Helm operator | 0.22.2 | **0.23.0** | Accompanies minor container version bump; includes RBAC fix and CRDs dependency bump |
| Helm CRDs | 0.2.1 | **0.3.0** | Breaking change: BoundEndpoint endpointURI renamed to endpointURL |

### PR Breakdown by Component (new_for)
- **Container**: 18 PRs (2 features, 7 changes/chores, 8 fixes, 1 breaking)
- **Helm operator**: 1 PR (RBAC patch permissions fix)
- **Helm CRDs**: 4 PRs (1 breaking rename, 1 feature, 2 fixes)

### Breaking Changes
- PR #779: BoundEndpoint `spec.endpointURI` renamed to `spec.endpointURL` (deprecated field preserved for backwards compat)

### Notable Changes
- New `k8s.ngrok.com/metadata` and `k8s.ngrok.com/description` annotations for Ingress/Gateway resources
- Go updated from 1.25.7 to 1.26.1
- Multiple "object has been modified" error fixes via patch + RetryOnConflict
- Data race fix in ChannelHealthChecker
- Swallowed API pagination error fix in domain lookup
- ProxyProtocolVersion enum type mismatch fix
- Several CI improvements (go fix gate, nix consolidation, copilot agent setup)
- Three flaky test fixes

### Skipped PRs (already released / not new_for any component)
- PR #777 (SECURITY.md) - docs only, not new for any component
- PR #778 (remove useExperimentalGatewayApi) - already released in 0.22.2
- PR #781 (gateway/ingress status fixes) - already released in 0.20.3
- PR #783 (release PR itself) - release infrastructure only
