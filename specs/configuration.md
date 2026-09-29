# Configuration

How operator configuration reaches each component, and where its defaults live.

## One definition per setting

Every setting is defined once in `internal/flags/flags.go`, on one line naming its flag, its environment variable, its default and its help. `String`, `Bool`, `List` and `Map` each return a function that binds the flag:

```go
Region = String("region", "NGROK_OPERATOR_REGION", "", "The region to use for ngrok tunnels")
```

A command binds the settings it reads onto its own options:

```go
flags.Region(c.Flags(), &opts.region)
```

Settings several commands read are bound in each, from the same definition, so their name, variable and default cannot differ. The manager flags every command has (`--release-name`, `--manager-name`, the bind addresses) are bound by `flags.Manager`. They are runtime plumbing the chart passes as args, so they have no variable.

## Names

A setting is named after its path in the chart's values:

| Values path | Variable | Flag |
|-------------|----------|------|
| `features.gateway.enabled` | `NGROK_OPERATOR_FEATURES__GATEWAY__ENABLED` | `--features-gateway-enabled` |
| `log.stacktraceLevel` | `NGROK_OPERATOR_LOG__STACKTRACE_LEVEL` | `--log-stacktrace-level` |
| `ngrok.region` | `NGROK_OPERATOR_REGION` | `--region` |

The variable is `NGROK_OPERATOR_` and the path, with `__` between levels and `_` between words. The shared `ngrok` settings sit at the top level, so the name does not say ngrok twice. The flag is the variable without the prefix, in kebab case. Names are written out in full, not generated; `TestSettingNames` and `TestChartEnvNames` check they follow the rule.

## Precedence

Highest wins:

1. **Flag**: `--region=eu`
2. **Environment variable**: `NGROK_OPERATOR_REGION=eu`
3. **Built-in default**

Binding makes the variable, when set, the flag's default, so `--help` shows the value the command will use. There is no configuration file.

## Values

| Type    | Example value                          |
|---------|----------------------------------------|
| String  | `eu`                                   |
| Boolean | `true`                                 |
| List    | `["a == 'x,y'", "true"]` (YAML or JSON) |
| Map     | `{env: dev}` (YAML or JSON)            |

Lists do not split on commas, so a CEL expression can contain one. A flag takes the same encoding as its variable. An empty variable counts as unset.

Each flag carries its variable as an annotation. At startup, `flags.Validate` walks every command's flags and fails on a variable its setting could not parse, and on a `NGROK_OPERATOR_*` variable that is not a setting, so a typo is not ignored. A variable that only another component reads is allowed, because the chart gives every component the shared settings. `NGROK_OPERATOR_RESTART_ON_CERT_CHANGE` is read outside the settings and also allowed.

The log settings are ordinary settings. `flags.Log` binds them, and the command builds its logger with `LogOptions.Logger`, so no command depends on the logging library.

Credentials (`NGROK_ACCESS_TOKEN`) and `POD_NAMESPACE` are read separately and are not settings.

## In the chart

`helm/ngrok-operator/files/operator-env.yaml` lists each setting's variable by its path in the component's configuration (`ngrok.region: NGROK_OPERATOR_REGION`). For each Deployment, `ngrok-operator.componentConfig` merges the shared values with the component's log overrides, and `ngrok-operator.componentEnv` writes a variable for each path in the list that holds a value. See [helm/common.md](helm/common.md#operator-configuration) for the values and their merge rule.

Values are written into the pod spec, not a ConfigMap. Each ReplicaSet keeps the configuration it was rolled out with, so an old pod that restarts during a rollout does not start with the new release's settings. A change to any setting rolls the Deployment.

The chart writes only values the user set. An empty value is left out, so the built-in default applies. A per-component `extraEnv` entry is listed after the chart's variables, so it overrides the chart for that component.

Tests in `cmd/chart_test.go` read each setting's variable and default from the flags of a fresh command tree, and keep the chart in step with them:

- `TestChartEnvNames`: each variable in the chart's list follows the naming rule for its path.
- `TestChartEnvMatchesFlags`: the chart's list and the commands' flags name the same variables.
- `TestChartValuesMatchDefaults`: every listed setting has a `values.yaml` key holding empty or its default, and `values.yaml` has no setting the list leaves out.
- `TestChartEnvParses`: the chart, rendered with every setting set, gives the api-manager every variable, and each reads back as the value set.

## Local development

```bash
NGROK_ACCESS_TOKEN=... POD_NAMESPACE=ngrok-operator go run . api-manager
```

Override settings with flags, or with variables, for example in `.envrc-user`:

```bash
export NGROK_OPERATOR_LOG__LEVEL=debug
export NGROK_OPERATOR_LOG__FORMAT=console
export NGROK_OPERATOR_FEATURES__GATEWAY__ENABLED=false
```

## Adding a setting

1. Define it in `internal/flags/flags.go`, named after its values path, and bind it in each command that reads it.
2. Add its key to `values.yaml` under `ngrok`, `log` or `features` (every feature an object), empty unless it is a boolean, with an `@param` line that states the default.
3. Add its path and variable to `helm/ngrok-operator/files/operator-env.yaml`.
4. Run `make update-readme` in `helm/ngrok-operator` to regenerate the chart README and schema.

The tests fail if steps 1–3 disagree.
