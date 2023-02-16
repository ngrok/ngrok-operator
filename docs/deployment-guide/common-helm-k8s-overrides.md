# Common Helm K8s Overrides

Along with ngrok specific types of configuration values, this helm chart also supports a number of common overrides that are useful for many different use cases in kubernetes. The following is a non-exhaustive list of common recipes you may find useful. If your use case is not achievable with the existing set of values, please log an issue detailing your use case and the values you would like to see added. A full list of values can be found [here](https://github.com/ngrok/kubernetes-ingress-controller/blob/main/helm/ingress-controller/README.md#parameters).


## Deployment basics

By default, the replica count is set to 1 and typically would be overridden to 2 or more. This is to ensure that the controller is highly available during roll-outs and failures, and can handle the load of the cluster.

For the image itself, the default tag is `latest` which is not recommended for production deployments. Instead, you should set the `image.tag` value to a specific version of the controller. This can be found in the [releases](https://github.com/ngrok/kubernetes-ingress-controller/releases). Additionally, you may choose to mirror the image to a private registry and set the `image.repository` value to the private registry, along with any required pull secrets.

## Apply labels and annotations

There are many situations where you may want the pod or all the resources created by the helm chart to have a specific label or annotation that allow them to be be discovered and interact with other services like log scrapers or service meshes. This can be done by setting the `podLabels` and `podAnnotations` values. Additionally, you can set annotations just on the pods themselves by setting the `podAnnotations` value.

## Extra Volumes and Environment Variables

The helm chart offers the ability to add extra volumes and environment variables to the controller. This is useful for mounting secrets or configmaps that contain credentials or other configuration values. This can be done by setting the `extraVolumes` and `extraEnv` values.