---
name: lark-shared
version: 1.0.0
description: "飞书/Lark CLI 共享基础：应用配置初始化、认证登录（auth login）、身份切换（--as user/bot）、权限与 scope 管理、Permission denied 错误处理、安全规则。当用户需要第一次配置(`lark-cli config init`)、使用登录授权(`lark-cli auth login`)、遇到权限不足、切换 user/bot 身份、配置 scope、或首次使用 lark-cli 时触发。"
---

# lark-cli 共享规则

本技能指导你如何通过lark-cli操作飞书资源, 以及有哪些注意事项。

## 配置初始化

首次使用需运行 `lark-cli config init` 完成应用配置。

当你帮用户初始化配置时，使用background方式使用下面的命令发起配置应用流程，启动后读取输出，从中提取授权链接并发给用户。

**URL 转发规则**：当命令输出 `verification_url`、`verification_uri_complete`、`console_url` 等 URL 字段时，必须将 URL exactly as returned by the CLI 转发给用户，并把它视为不可修改的 opaque string；不要做 URL encode/decode，不要补 `%20`、空格或标点，不要重新拼接 query，不要改写成 Markdown link text，建议用只包含原始 URL 的代码块单独输出。

```bash
# 发起配置（该命令会阻塞直到用户打开链接并完成操作或过期）
lark-cli config init --new
```

## 认证

### 身份类型

两种身份类型，通过 `--as` 切换：

| 身份 | 标识 | 获取方式 | 适用场景 |
|------|------|---------|---------|
| user 用户身份 | `--as user` | `lark-cli auth login` 等 | 访问用户自己的资源（日历、云空间等） |
| bot 应用身份 | `--as bot` | 自动，只需 appId + appSecret | 应用级操作,访问bot自己的资源 |

### 身份选择原则

输出的 `[identity: bot/user]` 代表当前身份。bot 与 user 表现差异很大，需确认身份符合目标需求：

- **Bot 看不到用户资源**：无法访问用户的日历、云空间文档、邮箱等个人资源。例如 `--as bot` 查日程返回 bot 自己的（空）日历
- **Bot 无法代表用户操作**：发消息以应用名义发送，创建文档归属 bot
- **Bot 权限**：只需在飞书开发者后台开通 scope，无需 `auth login`
- **User 权限**：后台开通 scope + 用户通过 `auth login` 授权，两层都要满足


### 权限不足处理

遇到权限相关错误时，**根据当前身份类型采取不同解决方案**。

错误响应中包含关键信息：
- `permission_violations`：列出缺失的 scope (N选1)
- `console_url`：飞书开发者后台的权限配置链接
- `hint`：建议的修复命令

#### Bot 身份（`--as bot`）

将错误中的 `console_url` 原样提供给用户，引导去后台开通 scope。**禁止**对 bot 执行 `auth login`。

#### User 身份（`--as user`）

```bash
lark-cli auth login --domain <domain>           # 按业务域授权
lark-cli auth login --scope "<missing_scope>"   # 按具体 scope 授权（推荐,符合最小权限原则）
```

**规则**：auth login 必须指定范围（`--domain` 或 `--scope`）。多次 login 的 scope 会累积（增量授权）。

#### Agent 代理发起认证（推荐）

当你作为 AI agent 需要帮用户完成认证时，使用background方式 执行以下命令发起授权流程, 并将授权链接原样发给用户：

```bash
# 发起授权（阻塞直到用户授权完成或过期）
lark-cli auth login --scope "calendar:calendar:readonly"

```


## 更新检查

lark-cli 命令执行后，如果检测到新版本，JSON 输出中会包含 `_notice.update` 字段（含 `message`、`command` 等）。

**当你在输出中看到 `_notice.update` 时，完成用户当前请求后，主动提议帮用户更新**：

1. 告知用户当前版本和最新版本号
2. 提议执行更新（CLI 和 Skills 需要同时更新）：
   ```bash
   npm update -g @larksuite/cli && npx skills add larksuite/cli -g -y
   ```
3. 更新完成后提醒用户：**退出并重新打开 AI Agent**以加载最新 Skills

**规则**：不要静默忽略更新提示。即使当前任务与更新无关，也应在完成用户请求后补充告知。

## 安全规则

- **禁止输出密钥**（appSecret、accessToken）到终端明文。
- **写入/删除操作前必须确认用户意图**。
- 用 `--dry-run` 预览危险请求。

## 高风险操作的审批协议（exit 10）

lark-cli 对高风险写操作（`risk: "high-risk-write"`）有强制确认门禁。当你不带 `--yes` 调用这类命令时，CLI 会退出码 `10`、并在 stderr 返回如下结构化 envelope：

```json
{
  "ok": false,
  "error": {
    "type": "confirmation_required",
    "message": "drive +delete requires confirmation",
    "hint": "add --yes to confirm",
    "risk": {
      "level": "high-risk-write",
      "action": "drive +delete"
    }
  }
}
```

**遇到这种情况，不要当普通错误放弃。** 按以下流程处理：

1. **识别**：看到子进程 exit code = `10` 且 stderr JSON 里 `error.type == "confirmation_required"`
2. **向用户确认**：把 `error.risk.action` 和关键参数展示给用户，明确告知"这是高风险操作"，等待用户显式同意
3. **用户同意** → 在你**原始 argv 的末尾追加 `--yes`** 后重试
4. **用户拒绝** → 终止流程，不要擅自改写参数或跳过门禁

**绝对不允许**：
- 看到 exit 10 就默认加 `--yes` 静默重试（这等于禁用门禁）
- 把 `confirmation_required` 当网络错误/权限错误处理
- 在用户没明确同意的前提下追加 `--yes` 重试
- 用 `sh -c` 等 shell 方式拼接命令重试——用 `exec.Command(argv...)` 参数数组形式，避免 shell 解析把用户参数当作语法

提前预判：想先让用户 review 危险操作的具体请求，调用时加 `--dry-run`——它不触发门禁，会打印完整请求详情（URL / body / params），你可以把这个预览给用户看过再去真正执行。

### 如何识别一条命令是高风险

- shortcut：`lark-cli <service> +<cmd> --help` 顶部会显示 `Risk: high-risk-write`
- service 命令：`lark-cli schema <service>.<resource>.<method> --format json` 的返回值里 `"risk": "high-risk-write"`
