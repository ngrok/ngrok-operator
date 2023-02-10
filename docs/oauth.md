# Google Oauth Configuration Guide

<img src="./images/Under-Construction-Sign.png" alt="Kubernetes logo" width="350" />

We are currently refactoring the annotations system and this annotation is not present in the current build but will be back soon!


Follow the guide here https://ngrok.com/docs/cloud-edge#oauth-providers-google
Once completed, get the Client ID and the Client Secret and create a kubernetes secret with the following command:

```bash
kubectl create secret generic ngrok-corp-ingress-oauth-credentials \
  --from-literal=ClientID=$GOOGLE_CLIENT_ID \
  --from-literal=ClientSecret=$GOOGLE_CLIENT_SECRET
```

Next add these annotations to your ingress resource:

```yaml
  annotations:
    k8s.ngrok.com/https-oauth.secret-name: ngrok-corp-ingress-oauth-credentials
    k8s.ngrok.com/https-oauth.provider: google
    k8s.ngrok.com/https-oauth.scopes: https://www.googleapis.com/auth/userinfo.email,https://www.googleapis.com/auth/userinfo.profile
    k8s.ngrok.com/https-oauth.allow-domains: ngrok.com,your-application-domain.com
```

Navigate to your url and see that your Google Oauth protected login is working!