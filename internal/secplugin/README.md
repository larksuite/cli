# secplugin Usage Guide

Chinese version: see `README.zh-CN.md`.

`secplugin` enables a secure proxy mode for the CLI. It forces outbound HTTP(S)
requests to go through a local security proxy and can optionally trust an
additional CA certificate bundle.

It supports two configuration methods:

1. `sec_config.json`
2. `LARKSUITE_CLI_SEC_*` environment variables

## Config File Location

Default config file path:

```text
~/.lark-cli/sec_config.json
```

If `LARKSUITE_CLI_CONFIG_DIR` is set, the path becomes:

```text
$LARKSUITE_CLI_CONFIG_DIR/sec_config.json
```

## Option 1: Config File

Put the following content into `sec_config.json`:

```json
{
  "LARKSUITE_CLI_SEC_ENABLE": true,
  "LARKSUITE_CLI_SEC_PROXY": "http://127.0.0.1:3128",
  "LARKSUITE_CLI_SEC_CA": "/absolute/path/to/proxy-ca.pem",
  "LARKSUITE_CLI_SEC_AUTH": true,
  "LARKSUITE_CLI_APP_ID": "cli_xxx",
  "LARKSUITE_CLI_BRAND": "feishu",
  "LARKSUITE_CLI_DEFAULT_AS": "bot",
  "LARKSUITE_CLI_STRICT_MODE": "bot"
}
```

Field descriptions:

- `LARKSUITE_CLI_SEC_ENABLE`: Enables secplugin. Boolean values are supported.
- `LARKSUITE_CLI_SEC_PROXY`: Local HTTP proxy address. It must be `http://127.0.0.1:<port>`.
- `LARKSUITE_CLI_SEC_CA`: Absolute path to an extra trusted root CA PEM file. Leave empty if not needed.
- `LARKSUITE_CLI_SEC_AUTH`: Enables proxy-injected token mode.
- `LARKSUITE_CLI_APP_ID`: Optional app ID used in `SEC_AUTH` mode.
- `LARKSUITE_CLI_BRAND`: Optional, must be `feishu` or `lark`.
- `LARKSUITE_CLI_DEFAULT_AS`: Optional, must be `user`, `bot`, or `auto`.
- `LARKSUITE_CLI_STRICT_MODE`: Optional, must be `user`, `bot`, or `off`.

## Option 2: Environment Variables

You can also enable secplugin directly with environment variables without
creating `sec_config.json`:

```bash
export LARKSUITE_CLI_SEC_ENABLE=true
export LARKSUITE_CLI_SEC_PROXY=http://127.0.0.1:3128
export LARKSUITE_CLI_SEC_CA=/absolute/path/to/proxy-ca.pem
export LARKSUITE_CLI_SEC_AUTH=true
```

If you want to provide app metadata in `SEC_AUTH` mode, set these as well:

```bash
export LARKSUITE_CLI_APP_ID=cli_xxx
export LARKSUITE_CLI_BRAND=feishu
export LARKSUITE_CLI_DEFAULT_AS=bot
export LARKSUITE_CLI_STRICT_MODE=bot
```

## Precedence

The following environment variables override the corresponding fields in
`sec_config.json` when they are present:

- `LARKSUITE_CLI_SEC_ENABLE`
- `LARKSUITE_CLI_SEC_PROXY`
- `LARKSUITE_CLI_SEC_CA`
- `LARKSUITE_CLI_SEC_AUTH`
- `LARKSUITE_CLI_APP_ID`
- `LARKSUITE_CLI_BRAND`
- `LARKSUITE_CLI_DEFAULT_AS`
- `LARKSUITE_CLI_STRICT_MODE`

This means:

- Put stable defaults in `sec_config.json`.
- Use environment variables for temporary overrides.
- SEC-related environment variables can work even without a config file.

## SEC_AUTH Mode

The CLI enters `SEC_AUTH` mode when both of the following are true:

```text
LARKSUITE_CLI_SEC_ENABLE=true
LARKSUITE_CLI_SEC_AUTH=true
```

In this mode, the CLI does not read real tokens directly. Instead, it returns
placeholder tokens and expects the proxy to replace them with real credentials.

App information is resolved in this order:

1. `LARKSUITE_CLI_APP_ID` and `LARKSUITE_CLI_BRAND` from environment variables
2. The same fields in `sec_config.json`
3. The active profile in the regular CLI `config.json`

If no valid app information can be resolved from any source, the command fails.

## Constraints

- `LARKSUITE_CLI_SEC_PROXY` must use the `http` scheme only.
- The host of `LARKSUITE_CLI_SEC_PROXY` must be `127.0.0.1`.
- `LARKSUITE_CLI_SEC_PROXY` must not contain a path.
- `LARKSUITE_CLI_SEC_CA` must be an absolute path to a PEM file.
- Boolean values support `true/false`, `1/0`, `on/off`, `yes/no`, and `y/n`.

## Recommendations

For long-term stable setup, prefer `sec_config.json`:

- Good for developer machines or controlled environments.
- Avoids repeatedly injecting environment variables into the shell.

For temporary debugging, prefer environment variables:

- Good for switching proxy or CA for just one session.
- No need to modify files on disk.
