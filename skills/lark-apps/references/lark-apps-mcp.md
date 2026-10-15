# 妙搭应用 MCP 连接

把已有妙搭应用接到支持 MCP 的客户端时使用。这里只处理应用运行态连接和凭证；不生成 MCP 业务代码、不发布应用、不执行工具，也不修改客户端配置文件。

## 使用流程

1. 用户明确要接入 MCP 客户端时，调用 `lark-cli apps +mcp-get --app-id <app_id> --as user --format pretty`。stdout 只有一条带 token 的 MCP URL，直接填入客户端；该 URL 即凭证，不要贴到聊天、日志或提交到仓库。客户端连上后仍须完成飞书用户 OAuth 授权。
2. 仅需准备应用凭证时，使用 `lark-cli apps +mcp-key-create --app-id <app_id> --as user`。默认只返回 `credential_id` 和 `redacted=true`；显式加 `--include-secret` 才返回 `plain_key`。

参数和输出格式以各命令 `--help` 为准。`--dry-run` 只预览请求，不调用后端，也不创建凭证。

## 凭证与身份

- 两个命令都可能创建缺失的运行态凭证，因此风险均为 `write`，包括底层使用 GET 的 `+mcp-get`。不要仅为检查 MCP 是否开通而调用它。
- 同一应用重复调用返回已有凭证，不轮换、不重置。创建凭证不要求应用已发布，但使用运行态 MCP 需要发布且具备对应用户访问权限。
- 只支持用户身份。`+mcp-key-create` 仅开发者可用；非开发者调用 `+mcp-get` 需在应用线上可见范围内，且开发者已开启 MCP，否则返回 `app MCP is not enabled`，需开发者先运行 `+mcp-get` 开启。
- `+mcp-key-create` 的 `plain_key` 是连接时的 `X-Mcp-Token`，不是飞书用户 OAuth token；默认脱敏，需显式 `--include-secret`。
- CLI 不保存密钥。
- 普通开放 API Key 与应用 MCP 凭证不同，不使用 `+openapi-key-*` 命令替代。

## 边界与失败处理

当前命令不返回工具或 Skill 清单；工具、资源和 Prompt 的发现由 MCP 客户端通过标准协议完成。没有凭证列表、指定 key 查询、删除或重置命令，也不会回退到开发态。

接口返回 404 时核对服务是否支持该 OpenAPI，不自动创建其他类型的 API Key。连接配置无效或凭证字段缺失时命令失败，不输出不完整的成功结果。权限或登录错误按 CLI 的结构化错误提示处理。
