# Configuration

How operator configuration reaches each component, and where its defaults live.

## One default, in the flag

Every setting is a flag on the component that reads it, defined in `cmd/`. The flag's default is the only place a default is written. Settings the api-manager and agent share are registered once, by `addNgrokFlags` and `addFeatureFlags` in `cmd/flags.go`.

A flag is named after its setting's path in the chart's values, in kebab case: `ngrok.rootCAs` is `--ngrok-root-cas`, and `apiManager.config.oneClickDemoMode` is `--api-manager-one-click-demo-mode`.

## Precedence

Highest wins:

1. **Flag**: `--ngrok-region=eu`
2. **Environment variable**: `NGROK_OPERATOR_NGROK_REGION=eu`
3. **Flag default**

There is no configuration file.

## Environment variables

Every flag reads the variable named `NGROK_OPERATOR_` and the flag name in upper snake case: `--ngrok-root-cas` reads `NGROK_OPERATOR_NGROK_ROOT_CAS`. The name is derived from the flag, so it cannot drift from it.

| Type    | Example value                          |
|---------|----------------------------------------|
| String  | `eu`                                   |
| Boolean | `true`                                 |
| List    | `["a == 'x,y'", "true"]` (YAML or JSON) |
| Map     | `{env: dev}` (YAML or JSON)            |

Lists do not split on commas, so a CEL expression can contain one. A flag takes the same encoding as its variable.

An empty variable counts as unset. A `NGROK_OPERATOR_*` variable that names no flag of any component is an error, so a misspelled setting fails at startup instead of being ignored. A variable that only another component reads is allowed, because the chart gives every component the shared settings.

Logging keeps controller-runtime's `--zap-*` flags. The `--log-level`, `--log-format` and `--log-stacktrace-level` flags set them, and a `--zap-*` flag passed on the command line wins.

Credentials (`NGROK_ACCESS_TOKEN`) and `POD_NAMESPACE` are read separately and are not flags.

## In the chart

The chart renders one variable per setting into each component's Deployment, named from its values path by `ngrok-operator.envName`, the same rule as the operator's. It passes no configuration flags; the args it passes (`--release-name`, `--manager-name` and the bind addresses) are runtime plumbing, not settings. See [helm/common.md](helm/common.md#operator-configuration) for the values and their merge rule.

Values are written into the pod spec, not a ConfigMap. Each ReplicaSet keeps the configuration it was rolled out with, so an old pod that restarts during a rollout does not start with the new release's settings. A change to any setting changes the pod spec, which rolls the Deployment.

The chart writes only values the user set. An empty value is left out, so the flag default applies. Map settings are listed in `ngrok-operator.settingsEnv`, so the chart writes them as one JSON variable instead of walking into them.

Two tests in `cmd/chart_test.go` keep the chart and the flags in step:

- `TestChartValuesMatchFlags` walks `values.yaml` and fails if a setting is not a flag, if it holds anything other than empty or the flag default, or if a configuration flag has no values key.
- `TestChartEnvMatchesFlags` renders the chart with every list and map set and reads each Deployment's variables into that component's flags the way the operator does.

A per-component `extraEnv` entry is listed after the chart's variables, so a variable there overrides the chart for that component.

## Local development

```bash
NGROK_ACCESS_TOKEN=... POD_NAMESPACE=ngrok-operator go run . api-manager
```

Nothing else is required. Override individual settings with flags, or with variables, for example in `.envrc-user`:

```bash
export NGROK_OPERATOR_LOG_LEVEL=debug
export NGROK_OPERATOR_LOG_FORMAT=console
export NGROK_OPERATOR_FEATURES_GATEWAY_ENABLED=false
```

## Adding a configuration value

1. Add the flag in `cmd/`, named after its values path, with its default. Use `addNgrokFlags` or `addFeatureFlags` if the api-manager and agent both read it. A list or map takes `newStringsValue` or `newMapValue`.
2. Add the key to `values.yaml` under `ngrok`, `log` or `features` (or `<component>.config` if only that component has the setting), empty unless it is a boolean, with an `@param` line that states the default. A map setting also goes in the map list in `ngrok-operator.settingsEnv`.
3. Run `make update-readme` in `helm/ngrok-operator` to regenerate the chart README and schema.

The environment variable and the chart rendering follow from the name. The tests fail if a step is missed.
