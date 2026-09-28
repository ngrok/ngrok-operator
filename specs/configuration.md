# Configuration

How operator configuration reaches each component, and where its defaults live.

## One default, in Go

`internal/config.Default()` is the only place a default is written. Every component loads the same `config.Config` struct and ignores the settings it does not use.

The struct's JSON tags name each setting. The same name is the key in the chart's values and, through `config.EnvName`, the environment variable. Settings that belong to one component alone live in that component's section, such as `apiManager.oneClickDemoMode`.

## Environment variables

Configuration is read only from `NGROK_OPERATOR_*` environment variables. There are no configuration flags and no configuration file.

A variable's name is `NGROK_OPERATOR_` and the setting's path in upper snake case: `ngrok.rootCAs` is `NGROK_OPERATOR_NGROK_ROOT_CAS`. The name is derived from the struct, so it cannot drift from the setting it names.

Two forms are read, and the more specific one wins:

| Form    | Example                                              | Value |
|---------|------------------------------------------------------|-------|
| Section | `NGROK_OPERATOR_FEATURES`                            | The section as JSON. Keys it leaves out keep their defaults; unknown keys are an error |
| Setting | `NGROK_OPERATOR_FEATURES_BINDINGS_ENDPOINT_SELECTORS` | Strings as-is; booleans, lists and maps as YAML or JSON |

An empty variable counts as unset.

Logging keeps controller-runtime's `--zap-*` flags. The `log` settings seed them, and a `--zap-*` flag passed on the command line wins.

Credentials (`NGROK_ACCESS_TOKEN`) and `POD_NAMESPACE` are read separately and are not part of `config.Config`.

## In the chart

The chart renders one section variable per top-level section (`NGROK_OPERATOR_NGROK`, `_LOG`, `_FEATURES`, `_API_MANAGER`) into each component's Deployment. See [helm/common.md](helm/common.md#operator-configuration) for the values and their merge rule.

Values are written into the pod spec, not a ConfigMap. Each ReplicaSet keeps the configuration it was rolled out with, so an old pod that restarts during a rollout does not start with the new release's settings. A change to any setting changes the pod spec, which rolls the Deployment.

The chart writes only values the user set. An empty value is left out, so `Default()` applies. `TestChartValuesMatchDefault` walks `Default()` and fails if a setting is missing from `values.yaml` or holds anything other than empty or the Go default. `TestChartEnvLoads` renders the chart and loads each Deployment's variables the way the operator does.

A per-component `extraEnv` entry is listed after the chart's variables, so a setting variable there overrides the chart for that component.

## Local development

```bash
NGROK_ACCESS_TOKEN=... POD_NAMESPACE=ngrok-operator go run . api-manager
```

Nothing else is required. Override individual settings with setting variables, for example in `.envrc-user`:

```bash
export NGROK_OPERATOR_LOG_LEVEL=debug
export NGROK_OPERATOR_LOG_FORMAT=console
export NGROK_OPERATOR_FEATURES_GATEWAY_ENABLED=false
```

## Adding a configuration value

1. Add the field to `internal/config.Config`, and its default to `Default()`.
2. Add the key to `values.yaml` under `ngrok`, `log` or `features` (or `<component>.config` if only that component has the setting), empty unless it is a boolean, with an `@param` line that states the default.
3. Run `make update-readme` in `helm/ngrok-operator` to regenerate the chart README and schema.

The environment variable, the chart rendering and the tests follow automatically. No template changes are needed.
