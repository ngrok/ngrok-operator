# Configuration

How app configuration reaches the operator's components and where its defaults live.

## One default, in Go

`internal/config.Default()` is the only place an app config default is written. Every component decodes the same `config.Config` struct and ignores the fields it does not use.

Each component registers flags only for the settings it reads (`config.RegisterAPIManagerFlags`, `RegisterAgentFlags`, `RegisterBindingsForwarderFlags`), so its `--help` is accurate and a setting it would ignore is rejected on the command line. Settings that belong to one component alone live in that component's section of the struct, such as `apiManager.oneClickDemoMode`; in the chart they come from `<component>.config`.

Logging is the exception: the `log` section feeds controller-runtime's own `--zap-*` flags, so its defaults are zap's.

## Precedence

Highest wins:

1. **CLI flag**: `--region=eu`
2. **Environment variable**: `NGROK_OPERATOR_REGION=eu`
3. **Config file**: `--config=path.yaml`, repeatable; later files override earlier ones
4. **Built-in default**: `config.Default()`

A flag's environment variable is `NGROK_OPERATOR_` followed by the flag name in upper case with `-` replaced by `_`. The mapping is derived from the registered flags, so every flag gets one. Lists and maps use pflag's own encodings (`a,b` and `k=v,k2=v2`).

A config file uses the struct's YAML shape:

```yaml
log:
  level: debug        # --zap-log-level
  format: console     # --zap-encoder
  stacktraceLevel: panic
ngrok:
  region: eu
features:
  bindings:
    enabled: true
```

Credentials (`NGROK_ACCESS_TOKEN`) and `POD_NAMESPACE` are read from the environment only and are outside this chain.

## In the chart

The chart renders each component's settings into a config file, one key per component in the `{fullname}-config` ConfigMap, and passes `--config` to point at it. It passes no app config flags. See [helm/common.md](helm/common.md#operator-configuration) for the values and their merge rule.

The chart writes only values the user set. An empty value is left out, so `Default()` applies. `TestChartValuesMatchDefault` walks `Default()` and fails if a field is missing from `values.yaml` or holds anything other than empty or the Go default.

## Local development

```bash
NGROK_ACCESS_TOKEN=... POD_NAMESPACE=ngrok-operator go run . api-manager
```

No flags or files are required. Override individual settings with `NGROK_OPERATOR_*` variables in `.envrc-user`, or point `--config` at a file.

## Adding a configuration value

1. Add the field to `internal/config.Config`, and its default to `Default()`.
2. Register its flag in the `Register*Flags` function of every component that reads it. The environment variable follows automatically.
3. Add the key to `values.yaml` under `ngrok`, `log` or `features` (or `<component>.config` if only that component has the setting), empty unless it is a boolean, with an `@param` line that states the default.
4. Run `make update-readme` in `helm/ngrok-operator` to regenerate the chart README and schema.

No template changes are needed.
