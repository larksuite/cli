---
name: lark-task
version: 1.0.0
description: "飞书任务：管理任务。创建待办任务、按任务 GUID 查看和更新任务状态、添加评论、完成或重新打开任务、上传任务附件。当用户需要创建待办事项、更新任务状态、为任务添加评论或上传附件文件时使用。不负责任务清单组织、任务搜索、成员分配与任务智能体注册。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli task --help"
---

# task (v2)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

## 命令选择与渐进式发现（必读）

执行任何 Task 命令前，必须先确认能力真实存在，禁止根据用户意图自行拼接或猜测 `+<verb>`：

1. 先将用户意图与下方 Shortcut 表精确匹配。只有表中明确列出的 shortcut 才可直接选择；参数不确定时读取对应 reference 或运行该 shortcut 的 `--help`。
2. 没有精确匹配、或无法确认当前版本是否支持时，先运行 `lark-cli task --help`，以当前 CLI 输出的命令列表为准。
3. help 中存在匹配 shortcut 时，使用 help 列出的完整 shortcut token（例如 `+create`）运行 `lark-cli task <shortcut> --help`，再按真实 flag 执行。
4. help 中没有匹配 shortcut 时，不得尝试相似的 `+<verb>`；从 help 中选择原生 resource，运行 `lark-cli task <resource> --help` 确认 method，再运行 `lark-cli schema task.<resource>.<method>` 获取参数结构，最后调用 `lark-cli task <resource> <method> ...`。
5. 遇到 `unknown_subcommand` 时必须停止猜测或尝试变体，回到第 2 步重新发现能力。

shortcut 名称只能来自本 Skill 的 Shortcut 表或 `lark-cli task --help`；原生 resource/method 以逐级 help 为准，参数名、类型和嵌套结构以 method schema 为准。

> **术语理解 — 待办 disambiguation（必读）**：
> - 用户提到「待办 / todo / 任务」时，**先判断归属**，不要默认走本 skill。
> - **走 [lark-meeting](../lark-meeting/SKILL.md) 的 `minutes +todo`**（禁止本 skill）：上下文含 **妙记 / 会议纪要 / minute_token / 妙记 URL**（`/minutes/`）；或「在某某妙记里新建/修改待办」「妙记 AI 待办」「会议录制里的待办」。
> - **走本 skill（lark-task）**：项目待办、截止日期/提醒；或 applink 含 `client/todo/task?guid=`；或明确说「飞书任务」「任务中心」。
> - **禁止**：用户要在妙记里加待办时，**不要**调用 `task +create` 或任何 task 命令去「找清单再放任务」。
> **「我」指谁**：用户说「给我创建」「分配给我」时，「我」是提出请求的人类用户，不是 Agent Employee 自己。根据已知姓名/邮箱用 [lark-contact](../lark-contact/SKILL.md) 解析成 `open_id` 再传入，必要时确认候选；不要把 Agent 自身当作默认负责人。
> **友好输出**：在输出任务（或清单）的执行结果给用户时，建议同时提取并输出命令返回结果中的 `url` 字段（任务链接），以便用户可以直接点击跳转查看详情。

> **创建/更新注意**：
> 1. 只有在设置了 `due`（截止时间）的情况下，才能设置 `repeat_rule`（重复规则）和 `reminder`（提醒时间）。
> 2. 若同时设置了 `start`（开始时间）和 `due`（截止时间），开始时间必须小于或等于截止时间。

> **查询注意**：
> 1. 在输出任务详情时，如果需要渲染负责人、创建人等人员字段，除了展示 `id` (例如 open_id) 外，还必须通过其他方式（例如调用通讯录技能）尝试获取并展示这个人的真实名字，以便用户更容易识别。
> 2. 在输出任务详情时，如果需要渲染创建时间、截止时间等字段，需要使用本地时区来渲染（格式为2006-01-02 15:04:05）。

> **Task GUID 定义**：
> Task OpenAPI 中用于更新/操作任务的 `guid` 是任务的全局唯一标识（GUID），不是客户端展示的任务编号（例如 `t104121` / `suite_entity_num`）。
> 对于 Feishu 的任务 applink（例如 `.../client/todo/task?guid=...`），必须使用 URL query 里的 `guid` 参数作为 task guid。

> **改完后的回读约定**：
> `+update` 返回 `updated_fields` 和每个任务的服务端 `confirmed` 字段；`+complete` 返回 `status`、`completed_at`、`already_completed`。这些字段已确认目标状态时，不要例行追加 `tasks get`；仅在服务端未返回所需字段或用户明确要求完整复核时再查询详情。

| Shortcut | 说明 |
|----------|------|
| [`+create`](references/lark-task-create.md) | create a task |
| [`+update`](references/lark-task-update.md) | update task attributes |
| [`+comment`](references/lark-task-comment.md) | add a comment to a task |
| [`+complete`](references/lark-task-complete.md) | mark a task as complete |
| [`+reopen`](references/lark-task-reopen.md) | reopen a completed task |
| [`+upload-attachment`](references/lark-task-upload-attachment.md) | upload a local file as an attachment to a task |

## API Resources

```bash
lark-cli schema task.<resource>.<method>   # 调用 API 前必须先查看参数结构
lark-cli task <resource> <method> [flags] # 调用 API
```

> **重要**：使用原生 API 时，必须先运行 `schema` 查看 `--data` / `--params` 参数结构，不要猜测字段格式。

### tasks

  - `create` — 创建任务
  - `get` — 获取任务详情
  - `patch` — 更新任务

## 权限表

| 方法 | 所需 scope |
|------|-----------|
| `tasks.create` | `task:task:write` |
| `tasks.get` | `task:task:read` |
| `tasks.patch` | `task:task:write` |
