# Common Helm K8s Overrides

This document provides some common use cases and recommendations when using this helm chart in a production setting.

## Helm Overrides for Common Use Cases
The Helm chart offers a variety of overrides that are useful for many different use cases in Kubernetes. Below is a list of common recipes that you may find helpful. If your use case is not achievable with the existing set of values, please log an issue detailing your use case and the values you would like to see added. For a full list of values, see [here](https://github.com/ngrok/kubernetes-ingress-controller/blob/main/helm/ingress-controller/values.yaml).

## Deployment Scaling
By default, the replica count is set to 1. We recommend overriding this to 2 or more to ensure high availability during roll-outs and failures, and to spread out the load.

## Image Configuration
The default image tag is latest, which is not recommended for production deployments. Instead, you should set the image.tag value to a specific version of the controller. You can find available versions in the releases section. Additionally, you may choose to mirror the image to a private registry and set the image.repository value to the private registry, along with any required pull secrets.

## Applying Labels and Annotations
You may want to add specific labels or annotations to your resources, to help them be discovered and interact with other services like log scrapers or service meshes. You can set the commonLabels and commonAnnotations values for all resources created by the helm chart. Additionally, you can set annotations just on the pods themselves by setting the podAnnotations value.

## Adding Extra Volumes and Environment Variables
The helm chart also allows you to add extra volumes and environment variables to the controller, which is useful for mounting secrets or configmaps that contain credentials or other configuration values. This can be done by setting the extraVolumes and extraEnv values.

## Ngrok-Specific Configuration Options
In addition to the common overrides, ngrok's Kubernetes Ingress Controller offers specific configurations that allow you to expose your Kubernetes services to the Internet securely and easily.
