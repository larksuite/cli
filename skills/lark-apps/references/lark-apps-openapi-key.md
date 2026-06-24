# apps openapi-key 命令族 SOP

管理妙搭应用对外暴露的 HTTP API Key（`/openapi/**` 鉴权凭证）。全部操作需 `--as user`（AuthType: user）。`--help` 是参数细节的完整来源；本文件只记录 Agent 不看就会做错的领域规则。

## 命令路由

| 命令 | 用途 | 关键 flag | 原始密钥 |
|---|---|---|---|
| `+openapi-key-list` | 列出应用所有 API Key（脱敏） | `--app-id`, `--limit`, `--offset` | 不返回 |
| `+openapi-key-get` | 查看单个 Key 详情（脱敏） | `--app-id`, `--key-id` | 不返回 |
| `+openapi-key-create` | 创建新 Key，**原始密钥一次性可见** | `--app-id`, `--name` (必填), `--scope`, `--allow-preview` | 顶层 `data.api_key` |
| `+openapi-key-update` | 改名或改 config（不改 status） | `--app-id`, `--key-id`, `--name`/`--scope`/`--allow-preview` 至少一个 | 不返回 |
| `+openapi-key-enable` | 启用 Key（status→1） | `--app-id`, `--key-id` | 不返回 |
| `+openapi-key-disable` | 停用 Key（status→0），**泄露/疑似泄露优先用这个而非 delete** | `--app-id`, `--key-id` | 不返回 |
| `+openapi-key-delete` | 永久删除 Key（不可逆） | `--app-id`, `--key-id`, `--yes` | — |
| `+openapi-key-reset` | 轮换密钥（刷新原始 Key），**一次性可见** | `--app-id`, `--key-id`, `--yes` | 顶层 `data.api_key` |

## 脱敏口径（安全关键）

- `list` / `get` / `update` / `enable` / `disable`：返回结构里 **无** `api_key` 字段，只有 `key_preview`（格式：`****` + 原始密钥末 4 位，如 `****5f4a`）。
- `create` / `reset`：**仅** 在 `data.api_key`（顶层）返回原始密钥一次；同时在 stderr 打印一次性提示：
  ```
  warning: this api_key is shown only once and is NOT stored by lark-cli — copy it now and store it in your own secret manager.
  ```
- 原始密钥绝不写入 cache / config / recent / debug log / 错误信息。

## 一次性密钥语义

CLI 不保存原始密钥。密钥在 `create` / `reset` 时仅随响应返回一次。**密钥丢失不能用 `get` 找回**——唯一恢复方式是 `+openapi-key-reset` 重新生成新密钥（旧密钥同时失效）。

## scope 透传

`--scope` 接受 raw JSON 字符串，透传进后端 `config.request_scope`，CLI 只校验是合法 JSON、不解释其内部结构。错误的 JSON 结构会被后端以 400 拒绝，CLI 透传该错误。调用方需按后端真实 shape 自行提供 JSON。

## 高风险操作

`delete` 和 `reset` 是高风险（`high-risk-write`），有以下约束：

- 需显式传 `--yes`（框架 `cmdutil.RequireConfirmation`）；缺少时退出码 10，**不要自动补 `--yes`**（遵循 lark-shared 安全红线）。
- 支持 `--dry-run` 查看将要执行的 HTTP 请求（不含密钥）；不确定时先 dry-run。
- **泄露场景**：应优先 `+openapi-key-disable` 立即停用，而非 `+openapi-key-delete`——停用可随时 enable 恢复，delete 不可逆。

## 典型决策场景

| 用户意图 | 正确操作 |
|---|---|
| "key 泄露了，先停掉" | `+openapi-key-disable`（不是 delete） |
| "key 丢了/忘了，再给我一个" | `+openapi-key-reset`（不是 create 新 key；reset 轮换密钥、保留原 key 配置） |
| "我的 key 密钥是什么" | 解释：list/get 不回显原始密钥，只能用 `+openapi-key-reset` 轮换 |
| "给应用创建一个有权限限制的 key" | `+openapi-key-create --name ... --scope '<json>'`（JSON 结构按后端文档） |

## 不在本 skill 范围

- OpenAPI spec 全量导出、实时日志 tail、Webhook 消费、多鉴权方式：本期不支持。
- 身份选择、权限不足处理（`permission_violations`→`console_url`）、exit-10 审批、通用"禁输出密钥"红线、高风险操作通用框架：见 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，不在此重复。
