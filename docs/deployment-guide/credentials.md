# Credentials

Currently, the ngrok Kubernetes Ingress Controller requires a paid ngrok account. We are working to bring the full experience to the free tier and will update this when that has been released.

Once you have an account, log into the [dashboard](https://dashboard.ngrok.com) and gather:
- Your auth token from [here]](https://dashboard.ngrok.com/auth/your-authtoken)
- An api key from [here](https://dashboard.ngrok.com/api)

These will be created as a kubernetes secret which the controller gets access to. It uses the auth token to create tunnels, and the api key to manage edges and other resources via the ngrok API.

## Setup

While the quickstart guide shows you can pass the credential values directly via helm values, in a production scenario, this is not recommended as its difficult do infrastructure as code in a secure manner. Instead, we recommend creating a kubernetes secret and passing the secret name to the helm chart. How you create the secret is up to you, whether its manually, or via various secrets tools like external secrets, sealed secrets, etc.

### Create the secret

The secret should:
- be in the same namespace as the ingress controller
- have a well formed name that can be passed to the helm chart
- have two keys, `API_KEY` and `AUTHTOKEN`

Example:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-custom-ngrok-ingress-controller-credentials
  namespace: ngrok-ingress-controller
data:
  API_KEY: "YOUR-API-KEY-BASE64"
  AUTHTOKEN: "YOUR-AUTHTOKEN-BASE64"
```

### Using the Secret

Once you have the secret created, you can pass the secret name to the helm chart via the `credentials.secretName` value.

Example:

```bash
helm install my-ingress-controller ngrok/ingress-controller \
  --set credentials.secret.name=my-custom-ngrok-ingress-controller-credentials
```

