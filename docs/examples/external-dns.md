# ngrok Ingress Controller + External DNS

When creating an Ingress object with a custom domain you own, ngrok will wait until the domain ownership is verified before it will create an edge for it. While you can do this manually as seen in the [Custom Domain Guide](../user-guide/custom-domains.md), you can also use [External DNS](https://github.com/kubernetes-sigs/external-dns) to automate this process since the controller adds the required CNAME record to the Ingress status object.

<img src="../assets/images/Under-Construction-Sign.png" alt="Under Construction" width="350" />