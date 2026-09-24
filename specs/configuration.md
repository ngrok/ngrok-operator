# Configuration

How configuration reaches the ngrok-operator's components, where default values live, and why the design is shaped this way.

This document covers design philosophy and the rules that follow from it. For the actual list of Helm values, see [helm/common.md](helm/common.md) and the per-component specs beside it.

## Two Classes of Configuration

The chart configures two fundamentally different things, and conflating them is what made the pre-0.25.0 values tree hard to reason about.

| Class          | Examples                                                        | Consumed by         | Delivered as                     |
|----------------|-----------------------------------------------------------------|---------------------|----------------------------------|
| **Pod config** | `resources`, `podAnnotations`, `nodeSelector`, `tolerations`     | Kubernetes          | Fields of the Deployment         |
| **App config** | `ngrok.region`, `log.level`, `features.bindings.enabled`         | The operator binary | A mounted file, from a ConfigMap |

Pod config never reaches the operator's Go code. App config never appears in a Deployment's pod spec except as the volume that carries it. The two classes travel by different routes, but they share the same shape and the same merge rule, so a user only has to learn one thing.

## Principles

### 1. A value has exactly one default, and it lives in Go

`internal/config.Default()` is the only place a default value is written. The chart never repeats a default; it renders only the keys a user actually set.

This is what makes `go run` viable: the binary is fully functional with no flags, no env vars, and no config file. It is also the only way to guarantee the chart and the binary cannot disagree — if the chart carried its own copy of `rootCAs: trusted`, the two copies would drift the first time one of them changed.

The corollary is that a chart default and a binary default are different things. `features.ingress.enabled` genuinely has a chart-side meaning (it decides whether an IngressClass renders), so it is a real chart value. `ngrok.rootCAs` does not; it is only ever passed through, so the chart leaves it unset.

### 2. Adding a config value should touch as few places as possible

Every place a value must be declared is a place it can be forgotten. The design is chosen to minimize that count:

| Step                                      | Required?      |
|-------------------------------------------|----------------|
| Field + default in `internal/config`      | Yes            |
| Flag registration                         | Yes, once      |
| Key + `@param` comment in `values.yaml`   | Yes            |
| Entry in `values.schema.json`             | No — generated |
| Row in the chart `README.md`              | No — generated |
| Edit to any Deployment template           | No             |

The last row is the important one, and it is the main reason app config is delivered as a rendered document rather than as a hand-written table of environment variables. A template that names each variable explicitly costs three more edits per value and silently does nothing when one is missed.

"Once" is load-bearing. Flags for shared config are registered by a single helper in `internal/config` that every component calls; a component then registers only the flags it alone owns. Registering the shared set separately in each `cmd/*.go` would put three copies of every flag name, usage string, and default in the tree — which is the shape this refactor exists to remove.

### 3. One merge rule, applied everywhere

**Maps deep-merge. Arrays replace.**

That rule governs `defaults` → component for pod config and shared → component for app config. It is also how Helm's own value coalescing behaves, so it is the rule users already expect.

Arrays replacing is a deliberate choice over concatenation: concatenation makes it impossible to remove an inherited element, and silently growing a list is a worse surprise than losing one.

### 4. Minimal configuration to run

A developer should be able to point a component at a cluster and go. Every setting that can have a sensible default has one, and the required configuration is reduced to credentials, which come from the environment.

### 5. Generated files are never hand-edited

`values.schema.json` and the chart `README.md` are generated from `values.yaml` and its `@param` annotations. A hand-maintained schema drifts from the values it claims to describe, and the drift is invisible until a user hits it.

### 6. No heavy dependencies

Configuration loading uses `spf13/pflag` and `sigs.k8s.io/yaml`, both already in the dependency tree. A configuration framework would add a large dependency to solve a problem that a few dozen lines solve adequately.

## Precedence

Highest wins:

1. **CLI flag** — `--log-level=debug`
2. **Environment variable** — `NGROK_OPERATOR_LOG_LEVEL=debug`
3. **Config file** — `--config=./hack/config.yaml`, repeatable; later files override earlier ones
4. **Built-in default** — `internal/config.Default()`

This matches what operators and platform tools in this ecosystem converge on, Argo CD included, and it is the order users assume without being told.

Credentials are excluded from this chain. `NGROK_API_KEY` and `NGROK_AUTHTOKEN` are read only from the environment, sourced from a Secret. They are never accepted from a config file or a flag, so they cannot end up in a ConfigMap, a shell history, or a process listing.

### Environment variables

A flag's name determines its variable: `NGROK_OPERATOR_` + the flag name uppercased with `-` replaced by `_`. `--log-level` becomes `NGROK_OPERATOR_LOG_LEVEL`.

The mapping is derived by walking the registered flag set, not written down per flag. A new flag gets env support for free, and the two can never disagree about a name.

Map- and list-valued flags use the encoding `pflag` already implements — `key=value,key2=value2` — so no encoding scheme of our own is needed.

The chart does not use this layer; it delivers config as a mounted file (see [Delivery](#delivery)). Environment variables exist for the person, not the chart: overriding one setting from a `.envrc` while running a component locally, or setting one on a pod to reproduce something without editing the ConfigMap.

## Delivery

One ConfigMap, `{fullname}-config`, with one key per component. Each key holds that component's **fully resolved** configuration as a YAML document. The chart performs the shared → component merge at render time; every component mounts the ConfigMap and reads its own key.

```yaml
# ConfigMap {fullname}-config
data:
  agent.yaml: |
    log:
      level: debug        # agent's own override won
    ngrok:
      region: eu          # inherited from the shared section
  apiManager.yaml: |
    log:
      level: info
    ngrok:
      region: eu
```

Adding a config value means adding it to `values.yaml`. No template changes, because each key is produced by rendering a values subtree rather than by naming fields one at a time.

This is also why the Go side has no merge logic: `internal/config.Load` decodes files onto the defaults in order, and that is all it does. There is no notion of a component inside the config package, no section to select, and no overlay step. Merging happens once, in the layer that already owns merging.

Rollouts stay scoped. Each Deployment's `checksum/config` annotation is computed from its own key, so changing the agent's log level rolls the agent and nothing else.

### On following Argo CD

Argo CD keeps a single `argocd-cmd-params-cm` with dot-delimited keys (`server.log.level`, `reposerver.parallelism.limit`) alongside unprefixed globals (`log.format.timestamp`). Each Deployment injects both the component-prefixed key and the global one as `configMapKeyRef` entries with `optional: true`, and the binary picks the winner at runtime.

We take the part of that which is worth taking — one ConfigMap holding everything, so there is a single place to look — and not the part that exists for a constraint we do not have. Argo ships raw manifests and has no render step, so its component-over-global fallback *must* happen at runtime, which forces a hand-maintained table of two env references per setting in every Deployment. Helm gives us a render step. Resolving there costs nothing and keeps the per-value edit count at zero.

## Representing "unset"

A key present in `values.yaml` with a `null` value means *not set — use the binary's default*. Helm strips null keys during value coalescing, so a null key is simply absent from `.Values` and never reaches the rendered config document.

Booleans and free-form maps are the exception and carry real values. Helm cannot tell `false` from unset, so a boolean has no way to express "unset" in the first place; and a map whose default is empty cannot drift, because `{}` in `values.yaml` and the binary's default are the same value. See [Schema generation](#schema-generation) for what this costs on the schema side.

The alternative is to comment the key out entirely, which reads better and puts the default in front of the reader. It was rejected because it cannot coexist with typo detection. The schema is generated from the keys that exist in `values.yaml`, so a commented-out key is absent from it — and `additionalProperties: false` would then reject the very key it documents. Drop `additionalProperties: false` and `features.gateway.enabld: false` installs cleanly and silently does nothing, which is a bad failure for a value whose whole job is to turn something off. Real keys with null values keep the generated schema complete and make that mistake fail at `helm install`.

> **Two-way door.** This convention is worth revisiting. It costs a line of explanation in `values.yaml`, and `null` as a sentinel can read as a mistake. If the schema generator gains the ability to describe commented-out keys of every type, or if we adopt a post-processing step rich enough to synthesize them, we should compare the two again. Nothing depends on which one we picked.

## Schema generation

`values.schema.json` is produced by `readme-generator --schema`, then corrected by a small deterministic post-pass. It is never edited by hand.

The post-pass applies four rules, all mechanical:

1. **Mirror `defaults` into each component.** Every key under `defaults` is also settable as `apiManager.<key>`, `agent.<key>` and `bindingsForwarder.<key>`. The generator only describes keys that are written out, so declaring those three times in `values.yaml` would put every pod setting in four places — the opposite of principle 2. Instead the post-pass copies the generated `defaults` properties into each component, leaving any key the component declares itself untouched.
2. **Seal every object.** The generator never emits `additionalProperties: false`, so an unrecognized value would be accepted silently. It is injected into every object that has a `properties` key. A nesting node gets `properties`; a free-form map (`ngrok.metadata`) gets `description` and `default` instead, so this rule closes the tree without also sealing the maps users are meant to fill in.
3. **Admit the subchart passthroughs.** `global`, `common` and `ngrok-crds` coalesce into the root of `.Values` from the chart's dependencies and are declared nowhere in `values.yaml`, so sealing the root without them would reject a legitimate install. They are added as free-form objects.
4. **Spell `nullable` portably.** The generator emits the OpenAPI extension `"nullable": true`, which a JSON Schema validator is free to ignore — and the helm-unittest plugin's validator does. It is rewritten to `"type": ["string", "null"]`, which Helm and the plugin both honour.

Rules 2 and 4 are order-dependent: sealing matches `"type": "object"` and must run before a type becomes a list.

Keeping the post-pass that small constrains how values are written, and those constraints are worth stating outright:

- **Every settable key is a real key with `@param`.** `@skip` and `@extra` both remove a key from the schema, so a key documented only that way is a key `additionalProperties: false` will reject. `@skip` is still usable alongside `@param` to keep a row out of the README.
- **A null key must carry a type modifier.** `@param` on a null key with no modifier fails generation with `Invalid type 'nil'`. This is a forcing function, not an obstacle — it fails at generation time, loudly.
- **Null strings and arrays use `[string,nullable]` and `[array,nullable]`.** Both produce a correct, complete schema entry.
- **Booleans and free-form maps are never null.** They carry real values. There is no `[boolean]` modifier, and `[object,nullable]` drops the key from the schema entirely, which `additionalProperties: false` would then turn into a rejection of the very key it documents. A boolean is no loss: Helm cannot distinguish `false` from unset, so a boolean has to render unconditionally regardless. A map is no loss either: its default is empty, so `{}` in `values.yaml` and the binary's default are the same value and cannot drift.

What we must not do is edit the output, because then the generator can no longer be run without losing the edits — which is exactly the state this design replaces.

## Local development

```bash
NGROK_API_KEY=... POD_NAMESPACE=ngrok-operator go run . api-manager
```

That is the baseline and it must keep working. A component starts with two environment variables and nothing else — no config file, no flags — because every other setting has a default. Anything that would require a mounted file, a wall of flags, or an image rebuild to get a component running has broken principle 4.

`POD_NAMESPACE` is the one setting with no sensible default. It names the namespace the operator manages its own resources in, and guessing it wrong is worse than refusing to start, so every component fails fast when it is missing. It predates this design and is deliberately outside the precedence chain, like the credentials.

Config files are a convenience on top, not a prerequisite:

```bash
NGROK_API_KEY=... go run . api-manager --config=./hack/config.yaml
```

`hack/config.yaml` only adjusts what a human at a terminal wants differently from a log pipeline. The same mechanism replaces long `--set` chains in the `make deploy` targets and chainsaw fixtures, where a checked-in values file is easier to read and to diff than the flags it stands in for.

The goal all of this serves is replacing a build-push-rollout cycle with a `go run` pointed at a cluster.

## Adding a new configuration value

1. Add the field to the relevant struct in `internal/config`, and its default to `Default()` if it has one.
2. Register a flag for it — in the shared helper if every component reads it, or in the one component that owns it. The env var and its precedence follow automatically.
3. Add the key to `values.yaml` — `null` if the binary's default should win — with an `@param` comment.
4. Regenerate the README and schema.

If any of this turns out to require editing a template or a generated file, that is a sign the design has eroded, and it is worth fixing rather than working around.
