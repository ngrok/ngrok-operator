# Releases

<!-- TOC depthfrom:2 -->

- [Artifacts](#artifacts)
    - [Docker Image](#docker-image)
    - [Helm Chart](#helm-chart)
- [Semantic Versioning](#semantic-versioning)
- [Release Process](#release-process)
    - [Tagging](#tagging)

<!-- /TOC -->

## Artifacts

The ngrok Ingress Controller has 2 main artifacts, a docker image and a helm chart.
While the helm chart is the recommended way to install the Ingress Controller, the 
docker image can be used to run the Ingress Controller in a Kubernetes cluster without helm.

### Docker Image

The Docker image contains the ngrok Ingress Controller binary and is available on 
Docker Hub [here](https://hub.docker.com/r/ngrok/ngrok-ingress-controller). We currently
support `amd64` and `arm64` architectures, with future plans to build for other architectures.

### Helm Chart

The helm chart is packaged and published to its own [helm repository](https://ngrok.github.io/ngrok-ingress-controller/index.yaml)
and can be installed by following the instructions in the chart's [README](../helm/ingress-controller/README.md).

## Semantic Versioning

This project uses [semantic versioning](https://semver.org/) for both the the docker image 
and helm chart. Please note that this project is still under development(pre 1.0.0) and considered `alpha` status at this time.

From the [semver spec](https://semver.org/#spec-item-4):

> Major version zero (0.y.z) is for initial development. Anything MAY change at any time. The public API SHOULD NOT be considered stable.


## Release Process

The Docker Image and Helm chart are released independently since a feature or bug fix in one
may not require a release in the other. Sometimes a change will require a version bump and
release in both.

### Tagging

There is a different git tag pattern for each artifact. 

Releases of the controller will be tagged with a prefix of `ngrok-ingress-controller-`. For example,
version `1.2.0` of the docker image will have a git tag of `ngrok-ingress-controller-1.2.0` which
contains the code used to build the docker image `ngrok/ngrok-ingress-controller:1.2.0`.

#### Helm Chart

Releases of the helm chart will tagged with a prefix of `helm-chart-`. For example, version `1.2.0`
of the helm chart will have a git tag of `helm-chart-1.2.0` which contains the code used to package
and publish version `1.2.0` of the helm chart.

When changes are made to the helm chart's `Chart.yaml` file, a github workflow will trigger upon
merging the PR to the `main` branch. The workflow will package and publish the helm chart for
consumption. The workflow will also create a git tag as described above.

When changing `version` in the helm chart's `Chart.yaml` file, the version should be bumped according
to the semantic versioning spec as described above.
