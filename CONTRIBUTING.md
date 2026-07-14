# Contributing

Thanks for contributing. Run `make test` before you open a PR. Keep changes focused on one area and small enough to review in one sitting.

`make test` already runs the code gen targets:
- `make generate` generates DeepCopy methods (`zz_generated.deepcopy.go`) for the types in `api/v1alpha1`, and license headers.
- `make manifests` writes YAML manifests into `config/` including the CRDs and the controller ClusterRole.

## Chart changes

App PRs must not touch `chart/**`. The chart is released separately from the app.

Any change under `chart/` triggers a CI check that fails unless the chart version was bumped.

## Releasing (maintainers)

The app release is done first, and the chart follows in a separate PR. The chart cannot reference an unreleased app version.

Process:
- Merge the app PRs, tag the release (`make tag TAG=vx.y.z`).
- Open a separate PR for the chart.
- Bump `version` (and `appVersion`) in `chart/Chart.yaml`
  - update the version table in `chart/README.md`.
- Run `make helm-sync`.
  - copies the CRDs
  - runs helm lint
  - checks for RBAC drift and missing changes

The ClusterRole in the Helm chart must be updated manually: `chart/templates/rbac.yaml`.
- `make helm-sync` will fail and prints the missing rules. Re-run to confirm.
