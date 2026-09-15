# Upgrade from Helm chart 0.24 to 0.25

This guide covers upgrading an existing ngrok Kubernetes Operator installation
from Helm chart `0.24.x` to `0.25.x`.

The project publishes the Helm chart, operator image, and CRDs with independent
version numbers; the operator image and `ngrok/ngrok-crds` versions for this
release are filled in when the release is cut. In the rest of this guide,
"0.25" means the `ngrok/ngrok-operator` Helm chart release.

> This guide is assembled as 0.25 work lands. Sections are added per change;
> the CRD API-version migration to `ngrok.com/v1` is documented separately.

## Before upgrading

### Back up the release configuration

Set these to match your installation:

```bash
RELEASE=ngrok-operator
NAMESPACE=ngrok-operator

helm get values "$RELEASE" --namespace "$NAMESPACE" --all \
  > ngrok-operator-0.24-values.yaml
helm get manifest "$RELEASE" --namespace "$NAMESPACE" \
  > ngrok-operator-0.24-manifest.yaml
```

### Convert `spec.metadata` to a map

0.24 made `spec.metadata` accept either a map or the older JSON-string form:

```yaml
# map form — canonical
spec:
  metadata:
    owned-by: ngrok-operator
    team: platform

# string form — deprecated
spec:
  metadata: '{"owned-by":"ngrok-operator","team":"platform"}'
```

In 0.25 the operator writes the map form only, and the canonical
`ngrok.com/v1` CRDs accept **only** the map form. The deprecated
`*.k8s.ngrok.com/v1alpha1` CRDs still accept both, so nothing breaks at this
upgrade — but an object still holding a string cannot be migrated to
`ngrok.com/v1`, and it stops working when the `v1alpha1` CRDs are removed in a
later release. Convert during the 0.25 window.

This applies to `Domain`, `IPPolicy` and its rules, `KubernetesOperator`,
`CloudEndpoint`, and `AgentEndpoint`.

**Objects the operator generates from your Ingress, Gateway API, and
LoadBalancer Service resources are converted for you** on the first reconcile
after the upgrade. Only manifests you author yourself need action.

#### 1. Audit

Most string values convert mechanically. Some cannot be expressed as a map of
strings at all and need a decision from you. This lists both:

```bash
for resource in \
  domains.ingress.k8s.ngrok.com \
  ippolicies.ingress.k8s.ngrok.com \
  kubernetesoperators.ngrok.k8s.ngrok.com \
  cloudendpoints.ngrok.k8s.ngrok.com \
  agentendpoints.ngrok.k8s.ngrok.com; do
  kubectl get "$resource" -A -o json |
    jq -r --arg resource "$resource" '
      def flatmap: type == "object" and all(.[]; type == "string");
      .items[]
      | . as $o
      | $o.spec.metadata as $m
      | if $m == null then empty
        elif ($m | type) == "object" then
          (if ($m | flatmap) then empty
           else "NEEDS EDIT (object with non-string values): \($resource) \($o.metadata.namespace)/\($o.metadata.name)" end)
        elif ($m | type) == "string" then
          (($m | try fromjson catch null) as $p
           | if $p == null then "NEEDS EDIT (not JSON): \($resource) \($o.metadata.namespace)/\($o.metadata.name)"
             elif ($p | flatmap) then "CONVERTS CLEANLY: \($resource) \($o.metadata.namespace)/\($o.metadata.name)"
             else "NEEDS EDIT (JSON, but not a map of strings): \($resource) \($o.metadata.namespace)/\($o.metadata.name)" end)
        else empty end
    '
done
```

IPPolicy rules carry their own metadata. Audit them too:

```bash
kubectl get ippolicies.ingress.k8s.ngrok.com -A -o json |
  jq -r '
    def flatmap: type == "object" and all(.[]; type == "string");
    .items[] as $item
    | $item.spec.rules[]?
    | select(.metadata != null and (.metadata | type) == "string")
    | . as $rule
    | (($rule.metadata | try fromjson catch null) as $p
       | if $p == null or ($p | flatmap | not)
         then "NEEDS EDIT: \($item.metadata.namespace)/\($item.metadata.name) rule \($rule.cidr)"
         else "CONVERTS CLEANLY: \($item.metadata.namespace)/\($item.metadata.name) rule \($rule.cidr)" end)
  '
```

#### 2. Convert the clean ones

Update the manifests in your source of truth and re-apply them. If you need to
convert a live object directly, this rewrites the field in place and leaves
objects that already use the map form — or that set no metadata — untouched:

```bash
kubectl get domains.ingress.k8s.ngrok.com my-domain -n my-namespace -o json |
  jq '
    def conv: if type == "string" then fromjson else . end;
    (if (.spec | has("metadata")) then .spec.metadata |= conv else . end)
    | (if (.spec | has("rules"))
       then .spec.rules |= map(if has("metadata") then .metadata |= conv else . end)
       else . end)
  ' |
  kubectl apply -f -
```

The same command works for every affected kind; the `rules` clause applies to
IPPolicy and is a no-op elsewhere.

#### 3. Fix the rest by hand

Three shapes the 0.24 CRDs accept cannot be represented as a map of strings.
`jq` will not rescue them — for a non-JSON value it fails outright and applies
nothing:

| What you have | What to do |
| --- | --- |
| Free-form text — `metadata: 'team=platform'` | Re-express as key/value pairs: `metadata: {team: platform}` |
| Non-string values — `metadata: '{"count":3}'` | Quote them: `metadata: {count: "3"}` |
| Nested objects or arrays — `metadata: '{"team":{"name":"infra"}}'` | Flatten: `metadata: {team-name: infra}` |

Metadata is sent to the ngrok API as a single string, so flattening changes
what you see in the ngrok dashboard. Pick key names you are happy to keep.

A `null` value inside a map (`metadata: {team: null}`) is accepted and silently
stored as no key at all. Set an empty string if you meant to keep the key.

#### 4. Verify

Re-run the audit. Anything still listed is unconverted.

## Rollback

Rolling back to 0.24 is supported: the 0.24 operator reads both the map and
string forms.

Rolling back to **0.23 or earlier is not supported after this upgrade**. The
operator now writes the map form, and 0.23 only understands the string form.
