<p align="center">
  <a href="https://ngrok.com">
    <img src="./images/ngrok-blue-lrg.png" alt="ngrok Logo" width="500" url="https://ngrok.com" />
  </a>
  <a href="https://kubernetes.io/">
  <img src="./images/Kubernetes-icon-color.svg.png" alt="Kubernetes logo" width="250" />
  </a>
</p>

# ngrok Kubernetes Ingress Controller Documentation

This is the ngrok ingress controller. It can be deployed and operated to a cluster and operated by a team allowing others to create ingress objects to dynamically self service ingress to their apps and services using a shared ngrok account. This is a great way to get started with ngrok and Kubernetes.

The controller watches for [Ingress](http://kubernetes.io/docs/user-guide/ingress/) objects and creates the corresponding ngrok tunnels and edges. More details on how these are derived can be found [here](./user-guide/ingress-to-edge-relationship.md). Other ngrok features such as [TCP Edges](TODO) can be configured via [CRDs](TODO).

If you are looking to install the controller for the first time, see our [deployment-guide](./TODO).
If its already installed and you are looking to configure ingress for an app or service, see our [user-guide](./TODO).

# How It Works
- the path network traffic takes to get into the cluster through an established tunnel
- how k8s resources are read and converted into ngrok resources

# Contributing
 - see [developer-guide](./TODO)
