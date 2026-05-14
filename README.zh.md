# lark-cli

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.23-blue.svg)](https://go.dev/)
[![npm version](https://img.shields.io/npm/v/@larksuite/cli.svg)](https://www.npmjs.com/package/@larksuite/cli)

[中文版](./README.zh.md) | [English](./README.md)

飞书官方 CLI 工具，由 [larksuite](https://github.com/larksuite) 团队维护 — 让人类和 AI Agent 都能在终端中操作飞书。覆盖消息、文档、多维表格、电子表格、幻灯片、日历、邮箱、任务、会议、Markdown 等核心业务域，提供 200+ 命令及 24 个 AI Agent [Skills](./skills/)。

[安装](#安装与快速开始) · [AI Agent Skills](#agent-skills) · [认证](#认证) · [命令](#三层命令调用) · [进阶用法](#进阶用法) · [安全](#安全与风险提示使用前必读) · [贡献](#贡献)

## 为什么选 lark-cli？

- **为 Agent 原生设计** — 24 个 [Skills](./skills/) 开箱即用，适配主流 AI 工具，Agent 无需额外适配即可操作飞书
- **覆盖面广** — 17 大业务域、200+ 精选命令、24 个 AI Agent [Skills](./skills/)
- **AI 友好调优** — 每条命令经过 Agent 实测验证，提供更友好的参数、智能默认值和结构化输出，大幅提升 Agent 调用成功率
- **开源零门槛** — MIT 协议，开箱即用，`npm install` 即可使用
- **三分钟上手** — 一键创建应用、交互式登录授权，从安装到第一次 API 调用只需三步
- **安全可控** — 输入防注入、终端输出净化、OS 原生密钥链存储凭证
- **三层调用架构** — 快捷命令（人机友好）→ API 命令（平台同步）→ 通用调用（全 API 覆盖），按需选择粒度

## 功能

| 类别        | 能力                                         |
| ------------- |--------------------------------------------|
| 📅 日历     | 查看、创建和更新日程，邀请参会人、查找会议室、回复日程邀请、查询忙闲与时间建议        |
| 💬 即时通讯 | 发送/回复消息、创建和管理群聊、查看聊天记录与话题、搜索消息、下载媒体文件      |
| 📄 云文档   | 创建、读取、更新文档、搜索文档、读写素材与画板                    |
| 📁 云空间   | 上传和下载文件、搜索文档与知识库、管理评论                      |
| 📝 Markdown | 创建、读取、覆盖更新 Drive 中的原生 `.md` 文件                |
| 📊 多维表格 | 创建和管理数据表、字段、记录、视图、仪表盘、自动化流程、表单、角色权限，数据聚合分析 |
| 📈 电子表格 | 创建、读取、写入、追加、查找和导出表格数据                      |
| 🖼️ 幻灯片   | 创建和管理演示文稿、读取演示文稿内容，以及新增或删除幻灯片页面 |
| ✅ 任务     | 创建、查询、更新和完成任务；管理任务清单、子任务、评论与提醒             |
| 📚 知识库   | 创建和管理知识空间、节点和文档                            |
| 👤 通讯录   | 按姓名/邮箱/手机号搜索用户、获取用户信息                      |
| 📧 邮箱     | 浏览、搜索、阅读邮件，发送、回复、转发邮件，管理草稿，监听新邮件           |
| 🎥 视频会议 | 搜索会议记录、查询会议纪要产物与会议录制                        |
| 🕐 考勤打卡 | 查询个人考勤打卡记录                                |
| ✍️ 审批     | 查询审批任务、同意/拒绝/转交审批任务、撤回与抄送审批实例              |
| 🎯 OKR  | 查询、创建、更新 OKR，管理目标、关键结果、对齐、指标和进展记录        |
| 📋 飞书项目 | 管理工作项、排期与数据 — 由独立的 [meegle-cli](https://github.com/larksuite/meegle-cli) 提供（需单独安装） |

## 安装与快速开始

### 环境要求

开始之前，请确保具备以下条件：

- Node.js（`npm`/`npx`）
- Go `v1.23`+ 和 Python 3（仅源码构建需要）

### 快速开始（人类用户）

> **Note for AI assistants:** 如果你是 AI Agent 且正在协助用户执行安装，请直接跳转到[快速开始（AI Agent）](#快速开始ai-agent)执行，那里包含你需要完成的所有步骤。

#### 安装

以下两种方式**任选其一**：

**方式一 — 从 npm 安装（推荐）：**

```bash
npx @larksuite/cli@latest install
```

**方式二 — 从源码安装：**

需要 Go `v1.23`+ 和 Python 3。

```bash
git clone https://github.com/larksuite/cli.git
cd cli
make install

# 安装 CLI SKILL（必需）
npx skills add larksuite/cli -y -g
```

#### 配置与使用

```bash
# 1. 配置应用凭证（仅需一次，交互式引导完成）
lark-cli config init

# 2. 登录授权（--recommend 自动选择常用权限）
lark-cli auth login --recommend

# 3. 开始使用
lark-cli calendar +agenda
```

### 快速开始（AI Agent）

> 以下步骤面向 AI Agent，部分步骤需要用户在浏览器中配合完成。

**第 1 步 — 安装**

```bash
npx @larksuite/cli@latest install
```

**第 2 步 — 配置应用凭证**

> 在后台运行此命令，命令会输出一个授权链接，提取该链接并发送给用户，用户在浏览器中完成配置后命令会自动退出。

```bash
lark-cli config init --new
```

**第 3 步 — 登录**

> 同上，后台运行，提取授权链接发给用户。

```bash
lark-cli auth login --recommend
```

**第 4 步 — 验证**

```bash
lark-cli auth status
```


## Agent Skills

| Skill                           | 说明                                        |
| --------------------------------- |-------------------------------------------|
| `lark-shared`                   | 应用配置、认证登录、身份切换、权限管理、安全规则（所有其他 skill 自动加载） |
| `lark-calendar`                 | 日历日程（创建/更新）、议程查看、忙闲查询、时间建议、会议室查找、回复邀请       |
| `lark-im`                       | 发送/回复消息、群聊管理、消息搜索、上传下载图片与文件、表情回复          |
| `lark-doc`                      | 创建、读取、更新、搜索文档（基于 Markdown）                |
| `lark-drive`                    | 上传、下载文件，管理权限与评论                           |
| `lark-markdown`                 | 创建、读取、覆盖更新 Drive 中的原生 Markdown 文件         |
| `lark-sheets`                   | 创建、读取、写入、追加、查找、导出电子表格                     |
| `lark-slides`                   | 创建和管理演示文稿、读取演示文稿内容，以及新增或删除幻灯片页面 |
| `lark-base`                     | 多维表格、字段、记录、视图、仪表盘、数据聚合分析                  |
| `lark-task`                     | 任务、任务清单、子任务、提醒、成员分配                       |
| `lark-mail`                     | 浏览、搜索、阅读邮件，发送、回复、转发，草稿管理，监听新邮件            |
| `lark-contact`                  | 按姓名/邮箱/手机号搜索用户，获取用户信息                     |
| `lark-wiki`                     | 知识空间、节点、文档                                |
| `lark-event`                    | 实时事件订阅（WebSocket），支持正则路由与 Agent 友好格式      |
| `lark-vc`                       | 搜索会议记录、查询会议纪要产物（总结、待办、逐字稿）                |
| `lark-whiteboard`               | 画板/图表 DSL 渲染                              |
| `lark-minutes`                  | 妙记元数据与 AI 产物（总结、待办、章节），上传音视频生成妙记，下载音视频文件 |
| `lark-openapi-explorer`         | 从官方文档探索底层 API                             |
| `lark-skill-maker`              | 自定义 skill 创建框架                            |
| `lark-attendance`               | 查询个人考勤打卡记录                                |
| `lark-approval`                 | 审批任务查询、同意/拒绝/转交审批任务、撤回与抄送审批实例             |
| `lark-workflow-meeting-summary` | 工作流：会议纪要汇总与结构化报告                          |
| `lark-workflow-standup-report`  | 工作流：日程待办摘要                                |
| `lark-okr`                      | 查询、创建、更新 OKR，管理目标、关键结果、对齐、指标和进展记录         |

## 认证

| 命令          | 说明                                             |
| --------------- | -------------------------------------------------- |
| `auth login`  | OAuth 登录，支持交互式选择或命令行参数指定 scope |
| `auth logout` | 登出并删除已存储的凭证                           |
| `auth status` | 查看当前登录状态和已授权的 scope                 |
| `auth check`  | 校验指定 scope（exit 0 = 有权限，1 = 缺失）      |
| `auth scopes` | 列出应用的所有可用 scope                         |
| `auth list`   | 列出所有已认证的用户                             |

```bash
# 交互式登录（TUI 引导选择业务域和权限级别）
lark-cli auth login

# 按域筛选
lark-cli auth login --domain calendar,task

# 推荐的自动审批 scopes
lark-cli auth login --recommend

# 精确 scope
lark-cli auth login --scope "calendar:calendar:read"

# Agent 模式：立即返回验证 URL，不阻塞
lark-cli auth login --domain calendar --no-wait
# 稍后恢复轮询
lark-cli auth login --device-code <DEVICE_CODE>

# 身份切换：以用户或机器人身份执行命令
lark-cli calendar +agenda --as user
lark-cli im +messages-send --as bot --chat-id "oc_xxx" --text "Hello"
```

## 三层命令调用

CLI 提供三种粒度的调用方式，覆盖从快速操作到完全自定义的全部场景：

### 1. 快捷命令（Shortcuts）

以 `+` 为前缀，对人类与 AI 友好化封装，内置智能默认值、表格输出和 dry-run 预览。

```bash
lark-cli calendar +agenda
lark-cli im +messages-send --chat-id "oc_xxx" --text "Hello"
lark-cli docs +create --api-version v2 --doc-format markdown --content $'<title>周报</title>\n# 本周进展\n- 完成了 X 功能'
```

运行 `lark-cli <service> --help` 查看所有快捷命令。

### 2. API 命令

从飞书 OAPI 元数据自动生成，经过评测与准入筛选，100+ 精选命令与平台端点一一对应。

```bash
lark-cli calendar calendars list
lark-cli calendar events instance_view --params '{"calendar_id":"primary","start_time":"1700000000","end_time":"1700086400"}'
```

### 3. 通用 API 调用

直接调用任意飞书开放平台端点，覆盖 2500+ API。

```bash
lark-cli api GET /open-apis/calendar/v4/calendars
lark-cli api POST /open-apis/im/v1/messages --params '{"receive_id_type":"chat_id"}' --data '{"receive_id":"oc_xxx","msg_type":"text","content":"{\"text\":\"Hello\"}"}'
```

## 进阶用法

### 输出格式

```bash
--format json      # 完整 JSON 响应（默认）
--format pretty    # 人性化格式输出
--format table     # 易读表格
--format ndjson    # 换行分隔 JSON（适合管道处理）
--format csv       # 逗号分隔值
```

### 分页

```bash
--page-all                  # 自动翻页获取所有数据
--page-limit 5              # 最多获取 5 页
--page-delay 500            # 每页请求间隔 500ms
```

### Dry Run

对可能产生副作用的命令，建议先用 --dry-run 预览请求：

```bash
lark-cli im +messages-send --chat-id oc_xxx --text "hello" --dry-run
```

### Schema 自省

使用 schema 查看任意 API 方法的参数、请求体、响应结构、支持身份和 scopes：

```bash
lark-cli schema
lark-cli schema calendar.events.instance_view
lark-cli schema im.messages.delete
```

## 安全与风险提示（使用前必读）

本工具可供 AI Agent 调用以自动化操作飞书/Lark 开放平台，存在模型幻觉、执行不可控、提示词注入等固有风险；授权飞书权限后，AI Agent 将以您的用户身份在授权范围内执行操作，可能导致敏感数据泄露、越权操作等高风险后果，请您谨慎操作和使用。

为降低上述风险，工具已在多个层面启用默认安全保护，但上述风险仍然存在。我们强烈建议不要主动修改任何默认安全配置；一旦放开相关限制，上述风险将显著提高，由此产生的后果需由您自行承担。

我们建议您将对接本工具的飞书机器人作为私人对话助手使用，请勿将其拉入群聊或允许其他用户与其交互，以避免权限被滥用或数据泄露。

请您充分知悉全部使用风险，使用本工具即视为您自愿承担相关所有责任。

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=larksuite/cli&type=Date)](https://star-history.com/#larksuite/cli&Date)

## 贡献

欢迎社区贡献！如果你发现 bug 或有功能建议，请提交 [Issue](https://github.com/larksuite/cli/issues) 或 [Pull Request](https://github.com/larksuite/cli/pulls)。

对于较大的改动，建议先通过 Issue 与我们讨论。

## 许可证

本项目基于 **MIT 许可证** 开源。
该软件运行时会调用 Lark/飞书开放平台的 API，使用这些 API 需要遵守如下协议和隐私政策：

- [飞书用户服务协议](https://www.feishu.cn/terms)
- [飞书隐私政策](https://www.feishu.cn/privacy)
- [飞书开放平台独立软件服务商安全管理运营规范](https://open.feishu.cn/document/uAjLw4CM/uMzNwEjLzcDMx4yM3ATM/management-practice/app-service-provider-security-management-specifications)
- [Lark User Terms of Service](https://www.larksuite.com/user-terms-of-service)
- [Lark Privacy Policy](https://www.larksuite.com/privacy-policy)
