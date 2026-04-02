---
name: lark-openclaw
version: 1.0.0
description: "OpenClaw Agent 中使用 lark-cli 的集成指南：沙箱安装、凭据隔离、工具选择决策。当 AI agent 运行在 OpenClaw 框架中、需要判断用 openclaw-lark 原生工具还是 lark-cli、需要在 Docker 沙箱中配置 lark-cli、或需要为多 Bot 架构隔离飞书凭据时使用。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli --help"
---

# OpenClaw Integration

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理和安全规则。**

## 双工具格局

OpenClaw 环境中飞书能力由两套工具覆盖，选择原则：**原生工具存在时不用 lark-cli 重复调用**。

| 工具 | 特点 | 覆盖域 |
|------|------|--------|
| **openclaw-lark 原生工具** | 结构化 Schema、自动 OAuth、scope 校验 | IM、日历、云文档、多维表格、电子表格、任务、通讯录 |
| **lark-cli** | 命令行、`--format csv`、`--page-all` 自动分页 | 邮件、视频会议、审批、OKR + 所有原生工具未覆盖的 API |

### 决策树

```
需要飞书功能？
├─ openclaw-lark 有原生工具？ → 用原生工具
└─ 没有 → lark-cli
    ├─ 需要批量导出？ → lark-cli --format csv --page-all
    └─ API 不确定？ → lark-cli schema <service.resource.method>
```

## 安装

### 场景 1：宿主机（ECS）

```bash
npm install -g @larksuite/cli
lark-cli config init          # 首次使用，按提示配置 appId/appSecret
```

### 场景 2：Docker 沙箱

在 Agent 的 Dockerfile 中添加：

```dockerfile
# 推荐：使用官方 Node.js 18+ LTS 基础镜像，避免 apt 默认安装旧版 Node
FROM node:18-slim AS base
RUN npm install -g @larksuite/cli

# 如果需要在现有镜像中添加 Node.js，使用 NodeSource 显式指定版本：
# RUN apt-get update -qq && \
#     apt-get install -y -qq --no-install-recommends curl && \
#     curl -fsSL https://deb.nodesource.com/setup_18.x | bash - && \
#     apt-get install -y -qq --no-install-recommends nodejs && \
#     npm install -g @larksuite/cli && \
#     apt-get clean && rm -rf /var/lib/apt/lists/*
```

### 场景 3：多 Bot 凭据隔离

不同 Agent（门店 Bot vs 投资人 Bot）使用不同飞书应用。通过 OpenClaw 的 per-agent 环境变量注入：

```bash
# openclaw.json 中为每个 agent 配置独立的环境变量
# store agent sandbox:
FEISHU_APP_ID=cli_store_xxx      # 门店 Bot App
FEISHU_APP_AUTH=<store_secret>

# investor agent sandbox:
FEISHU_APP_ID=cli_investor_xxx   # 投资人 Bot App
FEISHU_APP_AUTH=<investor_secret>
```

沙箱启动时初始化：

```bash
echo "$FEISHU_APP_AUTH" | lark-cli config init \
  --app-id "$FEISHU_APP_ID" --app-secret-stdin --brand feishu
```

## Shortcuts（推荐优先使用）

以下命令适用于 openclaw-lark **未覆盖**的功能域。操作前先用 `-h` 查看完整参数。

| Shortcut | 说明 | 身份 |
|----------|------|------|
| `mail +triage` | 收件箱摘要 | user |
| `mail +send` | 起草邮件（默认存草稿，`--confirm-send` 发送） | user |
| `mail +reply` | 回复邮件 | user |
| `mail +thread` | 读整个邮件会话 | user |
| `mail +triage --query ...` | 搜索邮件 | user |
| `vc +search` | 列出已结束的会议记录 | user |
| `vc +notes` | 获取会议纪要（摘要、待办、章节） | user |
| `contact user list` | 批量导出用户列表 | bot |
| `contact department list` | 列出部门树 | bot |

> 详细用法参考对应域的 Skill：[`lark-mail`](../lark-mail/SKILL.md)、[`lark-vc`](../lark-vc/SKILL.md)、[`lark-contact`](../lark-contact/SKILL.md)

### 通用 API 调用

当没有 Shortcut 且没有原生工具时，用通用 API：

```bash
# 1. 查参数结构（必须先查，不要猜）
lark-cli schema approval.instance.list --format pretty

# 2. 调用 API
lark-cli api GET /open-apis/approval/v4/instances --params '{"page_size":"20"}'
lark-cli api POST /open-apis/approval/v4/instances --data '{"approval_code":"xxx"}'

# 3. 批量导出（适合大量分页数据）
lark-cli api GET /open-apis/okr/v1/users/me/okrs --format csv --page-all
```

> 更多 API 探索方法参考 [`lark-openapi-explorer`](../lark-openapi-explorer/SKILL.md)

## 典型工作流

### OpenClaw Agent 读取飞书表格数据

```bash
# 场景：Agent 需要读取店长填写的采购反馈表
# Step 1: openclaw-lark 有 feishu_sheet 原生工具 → 优先用原生工具
# 如果原生工具不可用，fallback 到 lark-cli:
lark-cli sheets +read --url "https://xxx.feishu.cn/sheets/SHTCNxxx" --format csv
```

### Agent 发送未读邮件摘要

```bash
# 场景：每日定时检查未读邮件并推送摘要（openclaw-lark 无邮件工具）
# Step 1: 读取未读邮件
lark-cli mail +triage --as user
# Step 2: 基于结果生成摘要，通过 openclaw-lark 原生 IM 工具推送给用户
```

## 安全规则

1. **禁止输出密钥**：appSecret、accessToken 不得打印到终端或日志
2. **写入前预览**：发消息、发邮件、删除等操作先用 `--dry-run` 预览
3. **DELETE 需确认**：删除操作前必须向用户明确确认意图
4. **优先原生工具**：openclaw-lark 有原生工具时，不用 lark-cli 重复实现
5. **凭据隔离**：不同 Agent 使用各自环境变量凭据，不共享 appSecret

## 权限

### 查看已授权 scope

```bash
lark-cli auth scopes
```

### 发起授权（user 身份）

```bash
lark-cli auth login --scope "mail:user_mailbox.message:readonly"   # 按 scope（推荐）
lark-cli auth login --domain mail                           # 按业务域
```

### 常用 scope 参考

| 功能域 | scope | 身份 |
|--------|-------|------|
| 邮件读取 | `mail:user_mailbox.message:readonly` | user |
| 邮件发送 | `mail:user_mailbox.message:send` | user |
| 视频会议 | `vc:meeting:readonly` | user |
| 通讯录 | `contact:user.base:readonly` | bot/user |
| 审批 | `approval:approval:readonly` | bot/user |
| OKR | `okr:okr:readonly` | user |

> 遇到 Permission denied（错误码 99991672）→ `lark-cli auth scopes` 确认当前授权 → 按缺失 scope 增量授权
