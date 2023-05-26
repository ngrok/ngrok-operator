# White Label Agent Ingress

If you're running an ngrok and/or ingress controller behind a corporate firewall, you may encounter connectivity issues due to the agent or controller not being able to communicate with the ngrok network edge. If you're facing such issues, you can run the ngrok cli command, which is documented [here](https://ngrok.com/docs/guides/running-behind-firewalls) to diagnose the problem.

However, if opening connectivity is not an option, you can set up a custom ingress domain on your dashboard. This way, you can configure the ingress controller to use the custom domain instead of the default ngrok.app domain.

To get started with this, go to your dashboard and create a custom ingress domain. Once created, you can configure the ingress controller by using the following command:

```bash
helm install my-ingress-controller ngrok/kubernetes-ingress-controller \
  --set serverAddr="ngrok.mydomain.com:443"
  ```
