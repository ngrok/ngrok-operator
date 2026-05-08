## Release Preparation Summary

### Current Versions
- Container: `0.20.3` (tag: `ngrok-operator-0.20.3`)
- Helm operator chart: `0.22.2` (tag: `helm-chart-ngrok-operator-0.22.2`)
- Helm CRDs chart: `0.2.1` (tag: `helm-chart-ngrok-crds-0.2.1`)

### Proposed Versions
- Container: `0.20.4`
- Helm operator chart: `0.22.3`
- Helm CRDs chart: `0.2.2`

### Changes Since Last Release
26 PRs merged to main since the last release. Breakdown:

**Controller fixes (12 PRs):** Majority of changes are bug fixes across multiple controllers -- agent driver, bindings safety, gateway controller, manager driver, CloudEndpoint, KubernetesOperator, and general conflict retry logic. A key performance fix skips redundant ngrok API updates for CloudEndpoints.

**RBAC overhaul (1 PR, #804):** Large restructuring of Helm RBAC templates into per-component directories with namespace-scoped roles. Touches both helm templates and controller code.

**CRD changes (5 PRs):** Printer column improvements for `-o wide`, ProxyProtocolVersion enum fix, URI/URL inconsistency fix, misc annotation fixes, and CRD regeneration from RBAC overhaul. This requires a CRDs chart bump.

**CI/infra (6 PRs):** Nix setup consolidation, workflow improvements, trivy skip, copilot setup, manifest generation changes. These don't affect the release artifacts.

**Testing (3 PRs):** Flaky test fixes and a testing agent addition.

**New feature (1 PR):** Description and metadata annotations for ingress/gateway endpoints.

### CRDs Need Update
Yes -- 5 PRs touched files in `helm/ngrok-crds/templates/` or `api/`, requiring a CRDs chart version bump from `0.2.1` to `0.2.2`. The `ngrok-operator` chart's dependency on `ngrok-crds` must also be updated.

### Note on "Patch" Sizing
The RBAC overhaul (#804) is a significant structural change to the Helm chart. While the user requested a "patch release," this change is larger than typical patch-level work. The controller bug fixes are clearly patch-appropriate. You may want to consider whether the RBAC overhaul warrants a minor version bump for the Helm chart (0.23.0) instead of 0.22.3.

### Release Process
Per `scripts/release.sh` and `.github/agents/release-agent.agent.md`:
1. Create branch: `release-ngrok-operator-0.20.4-helm-chart-0.22.3`
2. Update `VERSION`, `helm/ngrok-operator/Chart.yaml`, `helm/ngrok-crds/Chart.yaml`
3. Update ngrok-crds dependency version in operator Chart.yaml
4. Run `make helm-update-snapshots helm-test`
5. Update all three changelogs
6. Commit: `Release ngrok-operator-0.20.4 helm-chart-0.22.3 ngrok-crds-0.2.2`
7. Push and create PR to main
