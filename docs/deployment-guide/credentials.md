In order to use the ngrok Kubernetes Ingress Controller, you will need a paid ngrok account. We are working on bringing the full experience to the free tier, and we will update this documentation when that is released. Once you have an account, you will need to log into the [ngrok dashboard](https://dashboard.ngrok.com) to gather the necessary credentials.

You will need two things from the dashboard:
- Your auth token, which can be found [here](https://dashboard.ngrok.com/auth/your-authtoken)
- An API key, which can be found [here](https://dashboard.ngrok.com/api)

These credentials will be created as a Kubernetes secret, which the controller will have access to. The auth token is used to create tunnels, and the API key is used to manage edges and other resources via the ngrok API.

### Setup

While the quickstart guide shows you can pass the credential values directly via helm values, we do not recommend this for production scenarios. Instead, we recommend creating a Kubernetes secret and passing the secret name to the helm chart. This allows for easier infrastructure as code in a more secure manner.

#### Creating the Secret

To create the secret, follow these steps:
- Make sure the secret is in the same namespace as the ingress controller
- Use a well-formed name that can be passed to the helm chart
- Add two keys to the secret: `API_KEY` and `AUTHTOKEN`

Here is an example secret manifest:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-custom-ngrok-ingress-controller-credentials
  namespace: ngrok-ingress-controller
data:
  API_KEY: "YOUR-API-KEY-BASE64"
  AUTHTOKEN: "YOUR-AUTHTOKEN-BASE64"