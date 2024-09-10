# Releases

- [Artifacts](#artifacts)
  - [Docker Image](#docker-image)
  - [Helm Chart](#helm-chart)
- [Semantic Versioning](#semantic-versioning)
- [Release Process](#release-process)
  - [Tagging](#tagging)
    - [Helm Chart](#helm-chart-1)
    - [Controller](#controller)

## Artifacts

The ngrok Ingress Controller has 2 main artifacts, a docker image and a helm chart.
While the helm chart is the recommended way to install the Ingress Controller, the
docker image can be used to run the Ingress Controller in a Kubernetes cluster without helm.

### Docker Image

The Docker image contains the ngrok Ingress Controller binary and is available on
Docker Hub [here](https://hub.docker.com/r/ngrok/ngrok-operator). We currently
support `amd64` and `arm64` architectures, with future plans to build for other architectures.

### Helm Chart

The helm chart is packaged and published to its own [helm repository](https://charts.ngrok.com/index.yaml)
and can be installed by following the instructions in the chart's [README](../helm/ingress-operator/README.md).

## Semantic Versioning

This project uses [semantic versioning](https://semver.org/) for both the the docker image
and helm chart.

From the [semver spec](https://semver.org/#spec-item-4):

> Major version zero (0.y.z) is for initial development. Anything MAY change at any time. The public API SHOULD NOT be considered stable.

That said, we will treat changes in "y" as major releases and changes in "z" as minor releases until version 1.0 is reached.

## Release Process

The Docker Image and Helm chart are released independently since a feature or bug fix in one
may not require a release in the other. Sometimes a change will require a version bump and
release in both.

### Tagging

There is a different git tag pattern for each artifact.

#### Helm Chart

Releases of the helm chart will be tagged with a prefix of `helm-chart-`. For example, version `1.2.0`
of the helm chart will have a git tag of `helm-chart-1.2.0` which contains the code used to package
and publish version `1.2.0` of the helm chart.

When changes are made to the helm chart's `Chart.yaml` file, a github workflow will trigger upon
merging the PR to the `main` branch. The workflow will package and publish the helm chart for
consumption. The workflow will also create a git tag as described above.

When changing `version` in the helm chart's `Chart.yaml` file, the version should be bumped according
to the semantic versioning spec as described above.

#### Controller

Releases of the controller will be tagged with a prefix of `ngrok-operator-`. For example,
version `1.2.0` of the docker image will have a git tag of `ngrok-oeprator-1.2.0` which
contains the code used to build the docker image `ngrok/ngrok-operator:1.2.0`.

When changes that would affect the controller's docker image are pushed to `main`, a github workflow
will trigger. The workflow will build and publish the `ngrok/ngrok-operator:latest` docker
image.

If the `VERSION` file at the root of the repo is changed, the workflow will also create a git tag
for the controller as described above and publish a tagged docker image. For instance when the
`VERSION` is changed to `1.2.0`, the workflow will create a git tag of `ngrok-operator-1.2.0`
and publish the docker image `ngrok/ngrok-operator:1.2.0`.
