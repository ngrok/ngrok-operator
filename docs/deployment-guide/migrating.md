# Migration Guide

This guide provides instructions for migrating from the ngrok Ingress Controller chart to the ngrok Operator chart.

**Note: It is recommended you test this upgrade procedure in a non-production environment before upgrading your production environment.**

Since the name of the chart has changed and helm releases are not easily able to be renamed, we will be installing the new chart in
a new namespace and then migrating the existing resources to the new helm release. When installing the new chart, we will start it with
only the agent enabled initially to keep your existing applications on-line while you migrate. Once the old resources have been migrated,
you can then scale up the API manager so that the ngrok operator is reconciling your resources instead of the old ingress controller.

## Migrating from the ngrok Ingress Controller chart to the ngrok Operator chart

1. Create a new namespace for the ngrok Operator:

   ```shell
   kubectl create namespace ngrok-operator
   ```

2. Copy the `ngrok-credentials` secret from the old namespace, usually `ngrok-ingress-controller`, to the new namespace and verify it is correct. Your secret name may be different if you have customized the installation:

   ```shell
   kubectl get secret ngrok-credentials -n ngrok-ingress-controller -o yaml | sed 's/namespace: ngrok-ingress-controller/namespace: ngrok-operator/' | kubectl apply -n ngrok-operator -f -
   ```

3. Change the helm ownership of the ngrok CRDS and existing ingress class. Your release name and ingressClass name may be different if you have customized the installation:

   ```shell
   kubectl get crds | grep "ngrok" | awk '{print $1}' | xargs -I '{}' kubectl annotate crd '{}' 'meta.helm.sh/release-name=ngrok-operator' --overwrite
   ```
   ```shell
   kubectl get crds | grep "ngrok" | awk '{print $1}' | xargs -I '{}' kubectl annotate crd '{}' 'meta.helm.sh/release-namespace=ngrok-operator' --overwrite
   ```
   ```shell
   kubectl get ingressclass | grep "ngrok" | awk '{print $1}' | xargs -I '{}' kubectl annotate ingressclass '{}' 'meta.helm.sh/release-name=ngrok-operator' --overwrite
   ```
   ```shell
   kubectl get ingressclass | grep "ngrok" | awk '{print $1}' | xargs -I '{}' kubectl annotate ingressclass '{}' 'meta.helm.sh/release-namespace=ngrok-operator' --overwrite
   ```

   If you are using the gateway feature, you will also need to change the ownership of the gatewayclasses similarly:

   ```shell
   kubectl get gatewayclasses.gateway.networking.k8s.io | grep "gateway" | awk '{print $1}' | xargs -I '{}' kubectl annotate gatewayclasses.gateway.networking.k8s.io '{}' 'meta.helm.sh/release-name=ngrok-operator' --overwrite
   ```
   ```shell
   kubectl get gatewayclasses.gateway.networking.k8s.io | grep "gateway" | awk '{print $1}' | xargs -I '{}' kubectl annotate gatewayclasses.gateway.networking.k8s.io '{}' 'meta.helm.sh/release-namespace=ngrok-operator' --overwrite
   ```

4. Install the ngrok Operator chart, with only the `agent` enabled. This will start the new agent to keep your existing applications on-line while you migrate. If you have customized the installation, you may need to adjust the `--set` flags to match your configuration:
   
   ```shell
   helm upgrade ngrok-operator \
                ngrok/ngrok-operator \
                --install \
                --atomic \
                --wait \
                --timeout 20m \
                --namespace ngrok-operator \
                --set credentials.secret.name=ngrok-credentials \
                --set replicaCount=0 \
                --set agent.replicaCount=3
   ```

5. Scale the old ngrok Ingress Controller deployment to 0 replicas:

   ```shell
   kubectl -n ngrok-ingress-controller scale deployment <deployment-name> --replicas=0
   ```

   This will stop the old Ingress Controller from reconciling resources. Verify that your applications are still accessible.

6. Assuming you have no other workloads running in the namespace, you can delete the old ngrok Ingress Controller namespace:

   ```shell
   kubectl delete namespace ngrok-ingress-controller
   ```

   If you have other workloads running in the namespace, you will need to manually delete the old resources.

7. Scale the manager replica count to 1:

    ```shell
    kubectl -n ngrok-operator scale deployment ngrok-operator-manager --replicas=1
    ```

    Don't forget to update the `replicaCount` in any future helm commands to match.
