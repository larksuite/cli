# secplugin 使用说明

English version: see `README.md`.

`secplugin` 用于开启安全代理模式，让 CLI 的 HTTP(S) 请求固定走本地安全代理，并按需信任额外 CA 证书。

支持两种配置方式：

1. `sec_config.json`
2. `LARKSUITE_CLI_SEC_*` 环境变量

## 配置文件位置

默认配置文件路径：

```text
~/.lark-cli/sec_config.json
```

如果设置了 `LARKSUITE_CLI_CONFIG_DIR`，则配置文件路径变为：

```text
$LARKSUITE_CLI_CONFIG_DIR/sec_config.json
```

## 方式一：使用配置文件

在 `sec_config.json` 中写入：

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

字段说明：

- `LARKSUITE_CLI_SEC_ENABLE`: 是否启用 secplugin，支持布尔值。
- `LARKSUITE_CLI_SEC_PROXY`: 本地 HTTP 代理地址，必须是 `http://127.0.0.1:<port>`。
- `LARKSUITE_CLI_SEC_CA`: 额外信任的根证书 PEM 文件绝对路径；不需要时可留空。
- `LARKSUITE_CLI_SEC_AUTH`: 是否启用代理注入 token 模式。
- `LARKSUITE_CLI_APP_ID`: 可选，`SEC_AUTH` 模式下使用的应用 ID。
- `LARKSUITE_CLI_BRAND`: 可选，取值为 `feishu` 或 `lark`。
- `LARKSUITE_CLI_DEFAULT_AS`: 可选，取值为 `user`、`bot` 或 `auto`。
- `LARKSUITE_CLI_STRICT_MODE`: 可选，取值为 `user`、`bot` 或 `off`。

## 方式二：使用环境变量

也可以不写 `sec_config.json`，直接通过环境变量启用：

```bash
export LARKSUITE_CLI_SEC_ENABLE=true
export LARKSUITE_CLI_SEC_PROXY=http://127.0.0.1:3128
export LARKSUITE_CLI_SEC_CA=/absolute/path/to/proxy-ca.pem
export LARKSUITE_CLI_SEC_AUTH=true
```

如果你在 `SEC_AUTH` 模式下希望同时提供应用信息，也可以继续设置：

```bash
export LARKSUITE_CLI_APP_ID=cli_xxx
export LARKSUITE_CLI_BRAND=feishu
export LARKSUITE_CLI_DEFAULT_AS=bot
export LARKSUITE_CLI_STRICT_MODE=bot
```

## 配置优先级

以下环境变量存在时，会覆盖 `sec_config.json` 中对应字段：

- `LARKSUITE_CLI_SEC_ENABLE`
- `LARKSUITE_CLI_SEC_PROXY`
- `LARKSUITE_CLI_SEC_CA`
- `LARKSUITE_CLI_SEC_AUTH`
- `LARKSUITE_CLI_APP_ID`
- `LARKSUITE_CLI_BRAND`
- `LARKSUITE_CLI_DEFAULT_AS`
- `LARKSUITE_CLI_STRICT_MODE`

也就是说：

- 你可以把默认值写进 `sec_config.json`。
- 再用环境变量做临时覆盖。
- 如果没有配置文件，但设置了 SEC 相关环境变量，也可以正常工作。

## SEC_AUTH 模式说明

当同时满足以下条件时，CLI 会进入 `SEC_AUTH` 模式：

```text
LARKSUITE_CLI_SEC_ENABLE=true
LARKSUITE_CLI_SEC_AUTH=true
```

此时 CLI 不直接读取真实 token，而是返回占位 token，由代理替换成真实凭证。

应用信息来源优先级如下：

1. 环境变量中的 `LARKSUITE_CLI_APP_ID` 和 `LARKSUITE_CLI_BRAND`
2. `sec_config.json` 中的同名字段
3. 常规 CLI 配置文件 `config.json` 的当前 profile

如果以上来源都拿不到可用应用信息，命令会报错。

## 参数约束

- `LARKSUITE_CLI_SEC_PROXY` 只允许 `http` 协议。
- `LARKSUITE_CLI_SEC_PROXY` 的 host 必须是 `127.0.0.1`。
- `LARKSUITE_CLI_SEC_PROXY` 不能带路径。
- `LARKSUITE_CLI_SEC_CA` 必须是 PEM 文件的绝对路径。
- 布尔值支持 `true/false`、`1/0`、`on/off`、`yes/no`、`y/n`。

## 推荐用法

长期固定配置建议使用 `sec_config.json`：

- 适合开发机或受控环境的稳定配置。
- 避免在 shell 中反复注入环境变量。

临时调试建议使用环境变量：

- 适合本次会话临时切换代理或证书。
- 不需要修改磁盘上的配置文件。
