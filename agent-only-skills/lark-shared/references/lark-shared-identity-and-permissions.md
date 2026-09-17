# Agent Employee 身份与权限

## 身份与诊断

AAT（Agent Access Token）代表 Agent Employee，是特殊 user 主体。`--as user` 有效，表示 Agent Employee 自身；不要把它解释为代当前人类用户操作。个人资源和共享资源是否可见取决于该主体的实际权限。不存在独立的 agent 身份参数，不得切换到 bot 获取空结果或绕过限制。

| 用户意图 | 首选命令 / 回答 |
|---|---|
| 检查当前主体及 token 有效性 | `lark-cli auth status --json --verify`；引用实际返回的 `identity`、`credential`、`principal`、`verified` 与身份诊断字段 |
| 快速查看当前生效身份 | `lark-cli whoami` |
| 首次配置、凭据存储修复、主体重新绑定 | 读取 [配置 Agent Employee](lark-shared-config-init.md)，使用 Agent Employee 应用凭据执行 `lark-cli config init` |
| 缺少权限或收回能力 | 由错误 hint 指定的智能体平台或租户管理员调整 Agent Employee 能力；不是本地退出登录或人类授权流程 |

机器读取 JSON 时，为减少 `_notice` 干扰，可在命令前加：

```bash
LARKSUITE_CLI_NO_UPDATE_NOTIFIER=1 LARKSUITE_CLI_NO_SKILLS_NOTIFIER=1 lark-cli auth status --json --verify
```

## 身份延续与续期

保持目标 profile 和 Agent Employee 主体一致。CLI 自动将已验证的 AAT 用于 user 调用；需要明确身份时沿用 `--as user`。不要因为输出包含 user 就另取人类用户凭据。

配置使用 App ID + App Secret 获取 AAT，并通过 Introspection 验证和存储主体绑定，无需交互授权。CLI 自动重新获取并验证 AAT；不存在 refresh token。若主体改变，续期会拒绝静默重绑定：核实平台绑定后按提示重新执行 `config init`。提前续期失败时，CLI 可能警告并继续使用尚未过期的缓存；已失效时按错误修复网络、应用凭据或平台能力，不切换身份重试。

## 能力与权限不足处理

- **能力门禁由 CLI 在调用时执行，不需要也无法预检**：注册命令和 shortcut 在发请求前自动判定 Agent Employee 能力，不可用时返回 `is not available for an Agent Employee profile` 和对应 `hint`。没有命令可以提前查到这个判定：`lark-cli schema` 的 `_meta.access_tokens` 只会显示 `bot` / `user`，不会显示 `agent`。直接调用并按返回结果处理，不要为此先查 schema。
- **被拒绝后不要换通道重试**：按 `hint` 指定的执行方申请能力，不切换身份，也不用 `lark-cli api` 裸调改写同一个已被拒绝的操作。裸调不重复这层门禁，用它绕过已知拒绝等于伪造可用性。
- `missing_scopes` 表示缺失 scope（可能 N 选 1）；只申请完成任务所需的最小能力。
- 以错误 `hint` 指定的执行方为准，将实际 `console_url` 原样交给智能体平台或租户管理员，不因 `identity: user` 改走人类授权。
- 平台调整后按实际提示重试；若本地 scope 快照已落后，重新执行 `config init` 验证并更新绑定和 scope。不要手改 token 或 scope 缓存，也不要关闭门禁绕过能力限制。
