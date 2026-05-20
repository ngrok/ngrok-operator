# ngrok-operator

- This repo is the ngrok Kubernetes Operator (`github.com/ngrok/ngrok-operator`), built with kubebuilder and organized by API group under `internal/controller/`.
- Prefer the Nix environment for local work: `nix develop`, then `make preflight` if you need to confirm the toolchain.
- Core validation commands:
  - `make test`
  - `make helm-update-snapshots helm-test`
  - `make manifest-bundle`
- Release prep uses the shared skill at `.agents/skills/release/`.
  - Claude discovers it through the `.claude/skills/release` symlink.
- Release artifact versions live in:
  - `VERSION`
  - `helm/ngrok-operator/Chart.yaml`
  - `helm/ngrok-crds/Chart.yaml`
