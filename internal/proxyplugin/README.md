# proxyplugin Usage Guide

Chinese version: see `README.zh-CN.md`.

`proxyplugin` enables a secure proxy mode for the CLI. It forces outbound HTTP(S)
requests to go through a local security proxy and can optionally trust an
additional CA certificate bundle.

It supports two configuration methods:

1. `proxy_config.json`
2. `LARKSUITE_CLI_PROXY_ENABLE`, `LARKSUITE_CLI_PROXY_ADDRESS`, and `LARKSUITE_CLI_CA_PATH` environment variables

## Config File Location

Default config file path:

```text
~/.lark-cli/proxy_config.json
```

If `LARKSUITE_CLI_CONFIG_DIR` is set, the path becomes:

```text
$LARKSUITE_CLI_CONFIG_DIR/proxy_config.json
```

## Option 1: Config File

Put the following content into `proxy_config.json`:

```json
{
  "LARKSUITE_CLI_PROXY_ENABLE": true,
  "LARKSUITE_CLI_PROXY_ADDRESS": "http://127.0.0.1:3128",
  "LARKSUITE_CLI_CA_PATH": "/absolute/path/to/proxy-ca.pem"
}
```

Field descriptions:

- `LARKSUITE_CLI_PROXY_ENABLE`: Enables proxyplugin. Boolean values are supported.
- `LARKSUITE_CLI_PROXY_ADDRESS`: Local HTTP proxy address. It must be `http://127.0.0.1:<port>`.
- `LARKSUITE_CLI_CA_PATH`: Absolute path to an extra trusted root CA PEM file. Leave empty if not needed.

## Option 2: Environment Variables

You can also enable proxyplugin directly with environment variables without
creating `proxy_config.json`:

```bash
export LARKSUITE_CLI_PROXY_ENABLE=true
export LARKSUITE_CLI_PROXY_ADDRESS=http://127.0.0.1:3128
export LARKSUITE_CLI_CA_PATH=/absolute/path/to/proxy-ca.pem
```

## Precedence

The following environment variables override the corresponding fields in
`proxy_config.json` when they are present:

- `LARKSUITE_CLI_PROXY_ENABLE`
- `LARKSUITE_CLI_PROXY_ADDRESS`
- `LARKSUITE_CLI_CA_PATH`

This means:

- Put stable defaults in `proxy_config.json`.
- Use environment variables for temporary overrides.
- proxy-related environment variables can work even without a config file.

## Constraints

- `LARKSUITE_CLI_PROXY_ADDRESS` must use the `http` scheme only.
- The host of `LARKSUITE_CLI_PROXY_ADDRESS` must be `127.0.0.1`.
- `LARKSUITE_CLI_PROXY_ADDRESS` must not contain a path.
- `LARKSUITE_CLI_CA_PATH` must be an absolute path to a PEM file.
- Boolean values support `true/false`, `1/0`, `on/off`, `yes/no`, and `y/n`.

## Recommendations

For long-term stable setup, prefer `proxy_config.json`:

- Good for developer machines or controlled environments.
- Avoids repeatedly injecting environment variables into the shell.

For temporary debugging, prefer environment variables:

- Good for switching proxy or CA for just one session.
- No need to modify files on disk.
