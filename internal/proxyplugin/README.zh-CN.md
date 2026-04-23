# proxyplugin 使用说明

English version: see `README.md`.

`proxyplugin` 用于开启安全代理模式，让 CLI 的 HTTP(S) 请求固定走本地安全代理，并按需信任额外 CA 证书。

支持两种配置方式：

1. `proxy_config.json`
2. `LARKSUITE_CLI_PROXY_ENABLE`、`LARKSUITE_CLI_PROXY_ADDRESS` 和 `LARKSUITE_CLI_CA_PATH` 环境变量

## 配置文件位置

默认配置文件路径：

```text
~/.lark-cli/proxy_config.json
```

如果设置了 `LARKSUITE_CLI_CONFIG_DIR`，则配置文件路径变为：

```text
$LARKSUITE_CLI_CONFIG_DIR/proxy_config.json
```

## 方式一：使用配置文件

在 `proxy_config.json` 中写入：

```json
{
  "LARKSUITE_CLI_PROXY_ENABLE": true,
  "LARKSUITE_CLI_PROXY_ADDRESS": "http://127.0.0.1:3128",
  "LARKSUITE_CLI_CA_PATH": "/absolute/path/to/proxy-ca.pem"
}
```

字段说明：

- `LARKSUITE_CLI_PROXY_ENABLE`: 是否启用 proxyplugin，支持布尔值。
- `LARKSUITE_CLI_PROXY_ADDRESS`: 本地 HTTP 代理地址，必须是 `http://127.0.0.1:<port>`。
- `LARKSUITE_CLI_CA_PATH`: 额外信任的根证书 PEM 文件绝对路径；不需要时可留空。

## 方式二：使用环境变量

也可以不写 `proxy_config.json`，直接通过环境变量启用：

```bash
export LARKSUITE_CLI_PROXY_ENABLE=true
export LARKSUITE_CLI_PROXY_ADDRESS=http://127.0.0.1:3128
export LARKSUITE_CLI_CA_PATH=/absolute/path/to/proxy-ca.pem
```

## 配置优先级

以下环境变量存在时，会覆盖 `proxy_config.json` 中对应字段：

- `LARKSUITE_CLI_PROXY_ENABLE`
- `LARKSUITE_CLI_PROXY_ADDRESS`
- `LARKSUITE_CLI_CA_PATH`

也就是说：

- 你可以把默认值写进 `proxy_config.json`。
- 再用环境变量做临时覆盖。
- 如果没有配置文件，但设置了 代理相关环境变量，也可以正常工作。

## 参数约束

- `LARKSUITE_CLI_PROXY_ADDRESS` 只允许 `http` 协议。
- `LARKSUITE_CLI_PROXY_ADDRESS` 的 host 必须是 `127.0.0.1`。
- `LARKSUITE_CLI_PROXY_ADDRESS` 不能带路径。
- `LARKSUITE_CLI_CA_PATH` 必须是 PEM 文件的绝对路径。
- 布尔值支持 `true/false`、`1/0`、`on/off`、`yes/no`、`y/n`。

## 推荐用法

长期固定配置建议使用 `proxy_config.json`：

- 适合开发机或受控环境的稳定配置。
- 避免在 shell 中反复注入环境变量。

临时调试建议使用环境变量：

- 适合本次会话临时切换代理或证书。
- 不需要修改磁盘上的配置文件。
