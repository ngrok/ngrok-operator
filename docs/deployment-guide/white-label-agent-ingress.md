# White Label Agent Ingress

If you are trying to run ngrok and/or the ingress controller behind a corporate firewall, you may encounter errors due to connectivity issues from the agent or controller to the ngrok network edge. You can run the ngrok cli command documented [here](https://ngrok.com/docs/guides/running-behind-firewalls) to diagnose these problems.

If opening that connectivity is not a viable option, instead you can setup a custom ingress domain [on your Dashboard](https://dashboard.ngrok.com/tunnels/ingress). Once created, configure the ingress controller to use that domain instead of the default ngrok.io domain.

```bash
helm install my-ingress-controller ngrok/ingress-controller \
  --set serverAddr="ngrok.mydomain.com:443"
```