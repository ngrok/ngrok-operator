# Release Preparation Summary

## Current Versions
- Container (VERSION): `0.20.3`
- Helm operator chart: `0.22.2`
- Helm CRDs chart: `0.2.1`

## Suggested Next Versions
- Container: `0.21.0` (minor bump -- significant bug fix sweep + RBAC overhaul + new features)
- Helm operator chart: `0.23.0` (minor bump -- RBAC overhaul restructures all RBAC templates)
- Helm CRDs chart: `0.3.0` (minor bump -- CRD changes across 5 CRD templates, includes a field rename)

## CRD Changes Confirmed
CRDs have changed since `helm-chart-ngrok-crds-0.2.1`. Five CRD template files were modified:
- `bindings.k8s.ngrok.com_boundendpoints.yaml` (URI vs URL rename, wide output columns)
- `ingress.k8s.ngrok.com_domains.yaml` (wide output columns)
- `ingress.k8s.ngrok.com_ippolicies.yaml` (type annotations, wide output columns)
- `ngrok.k8s.ngrok.com_agentendpoints.yaml` (ProxyProtocolVersion enum fix, wide output columns)
- `ngrok.k8s.ngrok.com_cloudendpoints.yaml` (type annotations, wide output columns)

Key CRD PRs: #772, #779, #792, #803, #804

## Key Changes Since Last Release

### Big items
1. **RBAC Overhaul (#804)** -- Restructured RBAC across all three deployments with per-component roles, namespace-scoped permissions, and dedicated CRD editor/viewer ClusterRoles. Touches both Helm chart templates and CRDs.
2. **Bug Fix Sweep (#797-#803)** -- Series of stability fixes addressing data races, nil panics, error swallowing, informer cache corruption, and reconcile churn across nearly every controller.

### PRs included (26 total since last release)
User-facing changes: #804, #803, #802, #801, #800, #799, #798, #797, #806, #788, #773, #792, #772, #779, #785, #768
CI/test/docs only: #808, #807, #795, #794, #793, #791, #789, #787, #786, #784, #782

## Release Steps (from docs/developer-guide/releasing.md)
1. Create branch `release-ngrok-operator-0.21.0-helm-chart-0.23.0`
2. Update `helm/ngrok-crds/Chart.yaml` version to `0.3.0`
3. Update `VERSION` to `0.21.0`
4. Update `helm/ngrok-operator/Chart.yaml`: version `0.23.0`, appVersion `0.21.0`, ngrok-crds dep `0.3.0`
5. Apply changelog entries from the files generated here
6. Run `make helm-update-snapshots helm-test`
7. Commit, push, open PR to `main`

## Note on BoundEndpoint URI->URL rename (#779)
PR #779 renamed a field in the BoundEndpoint CRD from URI to URL. This could be a breaking change for users referencing the old field name. Consider whether this warrants a note in the changelog about backwards compatibility.
