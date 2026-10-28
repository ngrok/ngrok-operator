# Configuration

How operator configuration reaches each component, and where its defaults live.

## One definition per setting

Every setting is defined once in `internal/flags/flags.go`, on one line naming its flag, its environment variable, its default and its help. `String`, `Bool`, `List` and `Map` each return a function that binds the flag:

```go
Region = String("region", "NGROK_OPERATOR__REGION", "", "The region to use for ngrok tunnels")
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
| `ngrok.features.gateway.enabled` | `NGROK_OPERATOR__FEATURES__GATEWAY__ENABLED` | `--features-gateway-enabled` |
| `ngrok.log.stacktraceLevel` | `NGROK_OPERATOR__LOG__STACKTRACE_LEVEL` | `--log-stacktrace-level` |
| `ngrok.region` | `NGROK_OPERATOR__REGION` | `--region` |

The variable is `NGROK_OPERATOR__` and the path under `ngrok`, with `__` between levels and `_` between words. The flag is the variable without the prefix, in kebab case. Names are written out in full, not generated; `TestSettingNames` and `TestChartEnvNames` check they follow the rule.

## Precedence

Highest wins:

1. **Flag**: `--region=eu`
2. **Environment variable**: `NGROK_OPERATOR__REGION=eu`
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

Each flag carries its variable as an annotation, which startup checks:

- A variable its setting cannot parse, such as `NGROK_OPERATOR__FEATURES__GATEWAY__ENABLED=nope`, stops the operator with an error naming it (`flags.Validate`). This holds even when a flag for the same setting is passed, and for a setting only another component reads.
- A set `NGROK_OPERATOR__*` variable that no command reads is ignored, with an "ignoring environment variable" log line naming it (`flags.Unknown`), so a misspelled setting is visible without failing the pod. A variable only another component reads is not reported, because the chart gives every component the shared settings. Variables with a single `_` after `NGROK_OPERATOR`, such as the ones Kubernetes injects for a Service named `ngrok-operator-*` (`NGROK_OPERATOR_METRICS_SERVICE_HOST`) or `NGROK_OPERATOR_RESTART_ON_CERT_CHANGE`, which is read outside the settings, are not settings and are not checked.

The log settings are ordinary settings. `flags.Log` binds them, and the command builds its logger with `LogOptions.Logger`, so no command depends on the logging library.

Credentials (`NGROK_ACCESS_TOKEN`) and `POD_NAMESPACE` are read separately and are not settings.

## In the chart

`helm/ngrok-operator/files/operator-env.yaml` lists each setting's variable by its path under `ngrok` (`region: NGROK_OPERATOR__REGION`). For each Deployment, `ngrok-operator.componentConfig` merges the shared values with the component's log overrides, and `ngrok-operator.componentEnv` writes a variable for each path in the list that holds a value. See [helm/common.md](helm/common.md#operator-configuration) for the values and their merge rule.

Values are written into the pod spec, not a ConfigMap. Each ReplicaSet keeps the configuration it was rolled out with, so an old pod that restarts during a rollout does not start with the new release's settings. A change to any setting rolls the Deployment.

The chart writes only values the user set. An empty setting is left out, so the built-in default applies; a map or list setting is written whole, so an empty value inside it, such as a label `example: ""`, is kept. An `extraEnv` entry with the same name as one of the chart's variables replaces it for that component, rather than adding a second entry with the same name, which server-side apply rejects.

Tests in `cmd/chart_test.go` read each setting's variable and default from the flags of a fresh command tree, and keep the chart in step with them:

- `TestChartEnvNames`: each variable in the chart's list follows the naming rule for its path.
- `TestChartEnvMatchesFlags`: the chart's list and the commands' flags name the same variables.
- `TestChartValuesMatchDefaults`: every listed setting has a `values.yaml` key holding empty or its default, and `values.yaml` has no setting the list leaves out.
- `TestChartEnvParses`: the chart, rendered with every setting set, gives the api-manager every variable, and each reads back as the value set.

## Local development

```bash
NGROK_ACCESS_TOKEN=... POD_NAMESPACE=ngrok-operator go run . api-manager
```

Leader election is off unless `--election-id` is set, so a local run needs nothing else.

Override settings with flags, or with variables, for example in `.envrc-user`:

```bash
export NGROK_OPERATOR__LOG__LEVEL=debug
export NGROK_OPERATOR__LOG__FORMAT=console
export NGROK_OPERATOR__FEATURES__GATEWAY__ENABLED=false
```

## Adding a setting

1. Define it in `internal/flags/flags.go`, named after its values path, and bind it in each command that reads it.
2. Add its key to `values.yaml` under `ngrok` (a feature goes in `ngrok.features`, as an object), empty unless it is a boolean, with an `@param` line that states the default.
3. Add its path and variable to `helm/ngrok-operator/files/operator-env.yaml`.
4. Run `make update-readme` in `helm/ngrok-operator` to regenerate the chart README and schema.

The tests fail if steps 1–3 disagree.
