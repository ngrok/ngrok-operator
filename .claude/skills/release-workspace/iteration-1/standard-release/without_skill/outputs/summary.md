## Release Summary

### What changed since the last release (0.20.3 / helm 0.22.2 / crds 0.2.1)

27 commits on `origin/main` since `ngrok-operator-0.20.3`, spanning ~18 merged PRs. The dominant theme is a large batch of bug fixes (many from the K8SOP-240 audit) and an RBAC overhaul.

### Key changes

**RBAC Overhaul (PR #804)** — The biggest change. All RBAC is now hand-managed in Helm templates instead of generated from kubebuilder markers. Templates reorganized into per-component directories. Namespace-scoping support added (Role vs ClusterRole when watchNamespace is set). This is a breaking change for anyone who had customized the generated RBAC resources.

**Bug fix sweep (PRs #797-803, #806)** — Systematic audit uncovered and fixed ~20 bugs across all controllers: cache corruption, nil panics, data races, swallowed errors, nondeterministic status ordering, inverted validation logic, and more.

**New features** — `k8s.ngrok.com/metadata` and `k8s.ngrok.com/description` annotations for Ingress/Gateway (#788), Ready condition reason/message in `-o wide` (#772).

**CRD changes** — URI-to-URL rename in BoundEndpoint (#779), ProxyProtocolVersion enum fix (#792), additional printer columns for all CRDs (#772).

### Version rationale

- **Container: 0.20.3 -> 0.21.0** — Minor version bump. The RBAC overhaul and large bug fix batch represent significant behavioral changes, but no breaking API changes to CRDs. Per the project's semver convention (pre-1.0: Y = major, Z = minor), bumping Y is appropriate for this scope of change.
- **Helm operator: 0.22.2 -> 0.23.0** — Minor version bump. The RBAC template restructuring and namespace-scoping support are significant Helm chart changes.
- **Helm CRDs: 0.2.1 -> 0.3.0** — Minor version bump. The URI-to-URL rename and enum fix are CRD schema changes.

### PRs excluded from changelogs (CI/test/docs only)

- #795 — CI: consolidate nix setup
- #807 — CI: merge main, adopt nix-setup action
- #808 — CI: manual workflow_dispatch for branch builds
- #786 — chore: skip trivy job
- #787 — feat: Add copilot setup steps
- #791 — Add `go fix ./...` to Makefile and CI
- #793 — feat: Add testing agent for testing
- #794 — test: Fix flakey agent endpoint controller test
- #789 — Fix flaky GatewayClass finalizer test
- #784 — Fix test flake in service controller
- #782 — Change full install manifests to main

### Release process (from docs/developer-guide/releasing.md)

1. Create branch `release-ngrok-operator-0.21.0`
2. Bump `VERSION` to `0.21.0`
3. Bump `helm/ngrok-crds/Chart.yaml` version + appVersion to `0.3.0`
4. Bump `helm/ngrok-operator/Chart.yaml` version to `0.23.0`, appVersion to `0.21.0`, ngrok-crds dependency to `0.3.0`
5. Add changelog entries (the files in this directory)
6. Run `make helm-update-snapshots helm-test`
7. Submit PR to `main`, merge triggers CI to publish
