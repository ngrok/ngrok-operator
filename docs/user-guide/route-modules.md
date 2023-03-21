# Modules


ngrok's Cloud Edge [Modules](https://ngrok.com/docs/cloud-edge/modules/) allow you to configure features like compression, IP Restrictions, OAuth, adding/removing headers, and more.

<!-- TOC depthfrom:2 -->

- [Design](#design)
    - [Reusable](#reusable)
    - [Composable](#composable)
- [Supported Modules](#supported-modules)
    - [Circuit Breaker](#circuit-breaker)
    - [Compression](#compression)
        - [Enabled](#enabled)
        - [Disabled](#disabled)
    - [Headers](#headers)
        - [Request](#request)
        - [Response](#response)
    - [IP Restrictions](#ip-restrictions)
    - [OAuth](#oauth)
        - [Ngrok Managed OAuth Application](#ngrok-managed-oauth-application)
            - [Google](#google)
        - [User Managed OAuth Application](#user-managed-oauth-application)
            - [Google](#google)
    - [OpenID Connect OIDC](#openid-connect-oidc)
    - [SAML](#saml)
    - [TLS Termination](#tls-termination)
    - [Webhook Verification](#webhook-verification)

<!-- /TOC -->

## Design

### Reusable

`NgrokModuleSet`s are designed to be reusable. This allows you to define a set of modules and their configuration once and apply it to multiple Ingresses. Ex:

```yaml
---
kind: NgrokModuleSet
apiVersion: ingress.k8s.ngrok.com/v1alpha1
metadata:
  name: module-set-1
modules:
  compression:
    enabled: true
  tlsTermination:
    minVersion: "1.2"
  headers:
    request:
      add:
        a-request-header: "my-custom-value"
        another-request-header: "my-other-custom-value"
      remove:
      - "x-remove-at-edge"
    response:
      add:
        a-response-header: "a-response-value"
---
kind: Ingress
apiVersion: networking.k8s.io/v1
metadata:
  name: example-ingress
  annotations:
    k8s.ngrok.com/modules: module-set-1
---
kind: Ingress
apiVersion: networking.k8s.io/v1
metadata:
  name: example-ingress-2
  annotations:
    k8s.ngrok.com/modules: module-set-1
```

In this example, the `compression`, `tlsTermination`, and `headers` modules are applied to both Ingresses and the same configuration is used for both. If you change the configuration of the `NgrokModuleSet`, the change will be applied to all Ingresses that use it.


### Composable

`NgrokModuleSet`s are designed to be composable. If multiple `NgrokModuleSet`s are applied to an Ingress and a module is configured in more than one, the last one wins. Ex:

```yaml
---
kind: NgrokModuleSet
apiVersion: ingress.k8s.ngrok.com/v1alpha1
metadata:
  name: module-set-1
modules:
  compression:
    enabled: false
---
kind: NgrokModuleSet
apiVersion: ingress.k8s.ngrok.com/v1alpha1
metadata:
  name: module-set-2
modules:
  compression:
    enabled: true
  tlsTermination:
    minVersion: "1.2"
---
kind: Ingress
apiVersion: networking.k8s.io/v1
metadata:
  name: example-ingress
  annotations:
    k8s.ngrok.com/modules: module-set-1,module-set-2
```

In this example, the result is the `compression` module is enabled since `module-set-2` was supplied last.

## Supported Modules

### Circuit Breaker

[Circuit breakers](https://ngrok.com/docs/cloud-edge/modules/circuit-breaker/) are used to protect upstream servers by rejecting traffic to them when they become overwhelmed.

```yaml
kind: NgrokModuleSet
apiVersion: ingress.k8s.ngrok.com/v1alpha1
metadata:
  name: circuit-breaker
modules:
  circuitBreaker:
    trippedDuration: 10s
    rollingWindow: 10s
    numBuckets: 10
    volumeThreshold: 10
    errorThresholdPercentage: "0.50"
```

### Compression

If an HTTP request includes an Accept-Encoding header, HTTP responses will be automatically compressed and a Content-Encoding response header will be added.

#### Enabled

```yaml
kind: NgrokModuleSet
apiVersion: ingress.k8s.ngrok.com/v1alpha1
metadata:
  name: compression-enabled
modules:
  compression:
    enabled: true
```

#### Disabled

```yaml
kind: NgrokModuleSet
apiVersion: ingress.k8s.ngrok.com/v1alpha1
metadata:
  name: compression-disabled
modules:
  compression:
    enabled: false
```


### Headers

#### Request

The [Request Headers](https://ngrok.com/docs/cloud-edge/modules/request-headers/) module allows you to add and remove headers from HTTP requests before they are sent to your upstream server.

```yaml
kind: NgrokModuleSet
apiVersion: ingress.k8s.ngrok.com/v1alpha1
metadata:
  name: request-headers
modules:
  headers:
    request:
      add:
        a-request-header: "my-custom-value"
        another-request-header: "my-other-custom-value"
      remove:
      - "x-remove-before-upstream"
```

#### Response

The [Response Headers module](https://ngrok.com/docs/cloud-edge/modules/response-headers/) allows you to add and remove headers from HTTP responses before they are returned to the client.

```yaml
kind: NgrokModuleSet
apiVersion: ingress.k8s.ngrok.com/v1alpha1
metadata:
  name: response-headers
modules:
  headers:
    response:
      add:
        a-response-header: "a-response-value"
        another-response-header: "another-response-value"
      remove:
      - "x-remove-from-resp-to-client"
```

### IP Restrictions

[IP Restrictions](https://ngrok.com/docs/cloud-edge/modules/ip-restrictions/) allow you to attach one or more IP policies to the route.

Policies may be specified by either their `ID` in the ngrok API or by the name of an `ippolicy.ingress.k8s.ngrok.com` Custom Resource if managed by the ingress controller.

```yaml
kind: IPPolicy
apiVersion: ingress.k8s.ngrok.com/v1alpha1
metadata:
  name: policy-1
spec:
  description: "My Trusted IPs"
  rules:
  - action: "allow"
    cidr: 1.2.3.4/32
    description: "My Home IP"
  - action: "allow"
    cidr: 1.2.3.5/32
    description: "My Work IP"
---
kind: NgrokModuleSet
apiVersion: ingress.k8s.ngrok.com/v1alpha1
metadata:
  name: ip-restrictions
modules:
  ipRestriction:
    policies:
    - "policy-1" # Reference to the `ippolicy.ingress.k8s.ngrok.com` Custom Resource above
    - "ipp_1234567890" # Reference to an IP Policy by its ngrok API ID
```


### OAuth

The [OAuth module](https://ngrok.com/docs/cloud-edge/modules/oauth/) enforces an OAuth authentication flow in front of any route it is enabled on.

#### Ngrok Managed OAuth Application

##### Google
```yaml
kind: NgrokModuleSet
apiVersion: ingress.k8s.ngrok.com/v1alpha1
metadata:
  name: ngrok-managed-google-oauth
modules:
  oauth:
    google:
      optionsPassthrough: true
      inactivityTimeout: 10m
      maximumDuration: 24h
      authCheckInterval: 20m
      emailAddresses:
      - my-email@my-domain.com
      # Or specify a list of domains instead of individual email addresses
      # emailDomains:
      # - my-domain.com
```

#### User Managed OAuth Application

##### Google
```yaml
---
kind: Secret
apiVersion: v1
metadata:
  name: google-oauth-secret
type: Opaque
data:
  CLIENT_SECRET: "<base64-encoded-client-secret>"
---
kind: NgrokModuleSet
apiVersion: ingress.k8s.ngrok.com/v1alpha1
metadata:
  name: user-managed-google-oauth
modules:
  oauth:
    google:
      optionsPassthrough: true
      inactivityTimeout: 10m
      maximumDuration: 24h
      authCheckInterval: 20m
      clientId: "<client-id>.apps.googleusercontent.com"
      clientSecret: 
        name: google-oauth-secret # The name of the k8s secret
        key: CLIENT_SECRET        # The key in the k8s secret containing the client secret
      scopes:
      - openid
      - email
```

### OpenID Connect (OIDC)

The [OIDC module](https://ngrok.com/docs/cloud-edge/modules/openid-connect/) restricts endpoint access to only users authorized by a OpenID Identity Provider.

```yaml
---
kind: Secret
apiVersion: v1
metadata:
  name: oidc-secret
type: Opaque
data:
  CLIENT_SECRET: "<base64-encoded-client-secret>"
---
kind: NgrokModuleSet
apiVersion: ingress.k8s.ngrok.com/v1alpha1
metadata:
  name: oidc
modules:
  oidc:
    clientId: "<client-id>.apps.googleusercontent.com"
    clientSecret:
      name: oidc-secret
      key: CLIENT_SECRET
    maximumDuration: 24h
    inactivityTimeout: 3h
    issuer: https://accounts.google.com
    optionsPassthrough: true
    scopes:
    - openid
    - email
```

### SAML

The [SAML module](https://ngrok.com/docs/cloud-edge/modules/saml/) restricts endpoint access to only users authorized by a SAML IdP.

### TLS Termination

Allows you to configure whether ngrok terminates TLS traffic at its edge or forwards the TLS traffic through unterminated.

```yaml
kind: NgrokModuleSet
apiVersion: ingress.k8s.ngrok.com/v1alpha1
metadata:
  name: tls
modules:
  tlsTermination:
    minVersion: "1.3"
```

### Webhook Verification

The webhook verification module allows ngrok to assert requests to your endpoint originate from a supported webhook provider like Slack or Github.

```yaml
---
apiVersion: v1
kind: Secret
metadata:
  name: github-webhook-token
type: Opaque
data:
  SECRET_TOKEN: "<base64-encoded-webhook-secret>"

---
kind: NgrokModuleSet
apiVersion: ingress.k8s.ngrok.com/v1alpha1
metadata:
  name: webhook-verification
modules:
  webhookVerification:
    provider: github
    secret:
      name: github-webhook-token
      key: SECRET_TOKEN
```