# ngrok Custom Resource Definitions

This Helm chart installs the ngrok Custom Resource Definitions (CRDs) for Kubernetes.

## Usage

### Prerequisites

[Helm](https://helm.sh) must be installed to use the charts.
Please refer to Helm's [documentation](https://helm.sh/docs) to get started.

### Installation

Once Helm has been set up correctly, add the repo as follows:

`helm repo add ngrok https://charts.ngrok.com`

If you had already added this repo earlier, run `helm repo update` to retrieve the latest versions of the packages.
You can then run `helm search repo ngrok` to see the charts.

To install the ngrok-crds chart:

`helm install ngrok-crds ngrok/ngrok-crds`

To uninstall the chart:

`helm delete ngrok-crds`

### Using with ngrok-operator

The `ngrok-operator` chart can install these CRDs automatically via the `installCRDs` value (enabled by default).
You only need to install this chart separately if you want to manage CRD lifecycle independently from the operator.

## More Information

For more information about the ngrok Kubernetes Operator, see https://github.com/ngrok/ngrok-operator
