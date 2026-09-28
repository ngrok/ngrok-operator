# Configuration

How app configuration reaches the operator's components and where its defaults live.

## One default, in Go

`internal/config.Default()` is the only place an app config default is written. Every component decodes the same `config.Config` struct and ignores the fields it does not use.

Each component registers flags only for the settings it reads (`config.RegisterAPIManagerFlags`, `RegisterAgentFlags`, `RegisterBindingsForwarderFlags`), so its `--help` is accurate and a setting it would ignore is rejected on the command line. Settings that belong to one component alone live in that component's section of the struct, such as `apiManager.oneClickDemoMode`.

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

## Local development

```bash
NGROK_ACCESS_TOKEN=... POD_NAMESPACE=ngrok-operator go run . api-manager
```

No flags or files are required. Override individual settings with `NGROK_OPERATOR_*` variables in `.envrc-user`, or point `--config` at a file.

## Adding a configuration value

1. Add the field to `internal/config.Config`, and its default to `Default()`.
2. Register its flag in the `Register*Flags` function of every component that reads it. The environment variable follows automatically.
