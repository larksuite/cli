---
name: lark-workflow-pre-meeting-brief
version: 1.0.0
description: "会前情报简报工作流：在会议开始前，自动拉取即将召开的日程，围绕议题检索相关文档、过往同主题会议纪要以及与会议相关的未完成任务，汇总为一页会前简报。当用户说“下一场会我该准备什么”“帮我生成会前简报”“开会前给我一份背景资料”时使用。"
metadata:
  requires:
    bins: ["lark-cli"]
---

# 会前情报简报工作流

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**。然后阅读 [`../lark-calendar/SKILL.md`](../lark-calendar/SKILL.md)、[`../lark-doc/SKILL.md`](../lark-doc/SKILL.md)、[`../lark-vc/SKILL.md`](../lark-vc/SKILL.md)、[`../lark-task/SKILL.md`](../lark-task/SKILL.md)，了解各域操作细节。

## 适用场景

- "下一场会我该准备什么" / "帮我出一份会前简报"
- "开会前给我拉点背景资料" / "即将开始的会议准备"
- "明天上午那个会的背景" / "今天 15 点的评审会准备什么"

**本工作流只读**。不会创建或修改任何日程、文档、任务。只在用户明确要求时才调用 `docs +create` 把简报落盘。

## 前置条件

仅支持 **user 身份**。执行前确保已授权：

```bash
lark-cli auth login --domain calendar,docs,drive,vc,task
```

范围说明：
- `calendar`：读即将开始的日程（`+agenda`）
- `docs,drive`：按议题关键词搜相关文档（`docs +search`）、可选写出简报（`docs +create`）
- `vc`：检索过去的同主题会议纪要（`vc +search`、`vc +notes`）
- `task`：查与议题相关的未完成待办（`task +get-my-tasks`、`+search`）

如果用户并不希望涉及会议记录或待办，可以只授权 `calendar,docs,drive`，相应步骤跳过即可。

## 工作流

```
now ─► calendar +agenda ──► 目标日程（默认取最近即将开始的 1 场）
           │
           ├─► 议题关键词提取（事件 summary、描述、与会人、会议室）
           │
           ├─► docs +search      ──► 相关文档候选（用户确认）
           ├─► vc +search        ──► 过去同主题会议（可选 +notes 取纪要链接）
           └─► task +get-my-tasks / +search ──► 相关未完成待办
                        │
                        ▼
                  AI 汇总：背景 / 相关材料 / 待办 / 可能议题
                        │
                        ▼
                  一页会前简报（用户确认）
                        │
                        └─► 可选 docs +create 落盘
```

### Step 1: 锁定目标会议

默认锁定**最近一场即将开始的会议**；如果用户明确指定了时间或主题，按用户的描述定位。

> **日期/时间计算必须调用系统命令（如 `date`），不要心算。** 时间必须是 ISO 8601 带时区，遵守 [`../lark-calendar/SKILL.md`](../lark-calendar/SKILL.md) 的时间与日期推断规范。

```bash
# 示例：取从现在起、到未来 3 小时内的日程（时间用 `date` 命令算好再填）
lark-cli calendar +agenda \
  --start "2026-04-15T14:30:00+08:00" \
  --end   "2026-04-15T17:30:00+08:00" \
  --format json
```

- 如果返回多场会议，**必须把候选会议列表展示给用户，让用户选一场**，不要擅自挑选
- 如果窗口内无会议，先向外扩展到 24 小时内再报告，而不是静默失败
- 记录目标会议的：`summary`（主题）、`start_time`、`end_time`、`description`、`attendees`（参会人）、`location`/`resources`（会议室）

### Step 2: 提取议题关键词

从 `summary` 和 `description` 中提取 1–3 个检索关键词：

- 优先使用完整主题（如"Q2 产品评审"）；若过长，拆成短语（如"Q2"、"产品评审"）
- 去除通用词（"周会"、"同步"、"1:1"）——单独这些词检索会噪声过大
- 如果主题过于笼统（如"沟通"、"对齐"），**回到 Step 1** 把 `description` 和参会人姓名一起纳入关键词
- 关键词必须向用户展示，**若置信度低，请用户补充或调整**

### Step 3: 并行拉取三类情报

下列三个检索互不依赖，可以顺序执行也可以并行。每类结果**保留 Top N 条展示给用户**（默认 N=5，过多会噪声）。

#### 3.1 相关文档（docs +search）

```bash
lark-cli docs +search --query "<关键词>" --format json
```

- 按 `updated_time` 倒序展示，记录 `title`、`url`、`owner`、`updated_time`
- 命中为 0 时尝试单关键词 / 换同义词；仍为 0 则在输出中诚实标注"未检索到相关文档"
- 详细用法：[`../lark-doc/references/lark-doc-search.md`](../lark-doc/references/lark-doc-search.md)

#### 3.2 过去同主题会议（vc +search → vc +notes，可选）

```bash
# 搜过去 30 天内同主题的会（end 不填则到现在；start 必须给）
lark-cli vc +search --query "<关键词>" \
  --start "2026-03-16T00:00:00+08:00" \
  --end   "2026-04-15T23:59:59+08:00" \
  --page-size 15 --format json
```

- 仅适合"重复性议题"或"长期项目"；**若目标会议是一次性主题（如"面试 XX"），跳过此步**
- 从结果中取最近 1–3 场的 `id`，再用 `vc +notes --meeting-ids <id1,id2>` 拿纪要文档 token
- `vc +search` **只能检索历史会议**（见 [`../lark-vc/references/lark-vc-search.md`](../lark-vc/references/lark-vc-search.md)），不能用来查未来会议

#### 3.3 相关未完成待办（task）

优先走 `+get-my-tasks`，再根据关键词进一步筛选：

```bash
# 当前分配给我、未完成、且在目标会议结束前需要跟进的
lark-cli task +get-my-tasks --complete=false \
  --due-end "2026-04-15T17:30:00+08:00" --format json
```

- 若用户只想看与主题强相关的任务，用关键词调用 `+search`（见 [`../lark-task/references/lark-task-search.md`](../lark-task/references/lark-task-search.md)）
- 如果 `+get-my-tasks` 不加过滤，结果可能非常多（参见 [`../lark-task/references/lark-task-get-my-tasks.md`](../lark-task/references/lark-task-get-my-tasks.md) 的警告）；**必须带上 `--complete=false` 和 `--due-end`**

### Step 4: 汇总为会前简报

按以下模板生成并展示给用户**确认**；不要省略"信息缺口"一节——如果某类情报为空，必须明确标注，避免让用户以为真的没有相关材料。

```markdown
# 会前简报：{会议主题}

> **开始时间**：{YYYY-MM-DD HH:mm (timezone)}
> **时长**：{HH:mm–HH:mm}，共 {N} 分钟
> **参会人**：{姓名列表（无 ID）}
> **会议室 / 地点**：{room or link}

## 会议目的（基于日程 description 推断）

{一段话，50 字内。description 为空则写"日程未填写目的，以下材料基于主题关键词检索"}

## 相关文档（Top {N}）

| 文档标题 | 负责人 | 最近更新 | 链接 |
|---|---|---|---|
| ... | ... | ... | ... |

## 过去同主题会议

- {YYYY-MM-DD} {会议主题}（纪要：{link 或"无纪要"}）
- ...

> 若无同主题历史会议：明确写"近 30 天未检索到同主题会议"。

## 我的相关待办

- [ ] {task_summary}（截止：{due}）—— {link}
- ...

## 建议准备重点

- {AI 基于以上材料给出的 2–4 条准备建议，每条 1 行}

## 信息缺口

- {如"描述为空，建议在会前补充议程"；或"未检索到相关文档，可能是关键词过窄"}
```

### Step 5: 落盘为飞书文档（可选，用户明确要求时）

阅读 [`../lark-doc/SKILL.md`](../lark-doc/SKILL.md) 了解云文档命令。

```bash
lark-cli docs +create \
  --title "会前简报：{会议主题} ({YYYY-MM-DD HH:mm})" \
  --markdown "<Step 4 生成的 Markdown>"
```

**写入前必须确认用户意图。**

## 权限表

| 命令 | 所需 scope |
|------|-----------|
| `calendar +agenda` | `calendar:calendar.event:read` |
| `docs +search` | 详见 [`../lark-doc/references/lark-doc-search.md`](../lark-doc/references/lark-doc-search.md) |
| `vc +search` | 详见 [`../lark-vc/references/lark-vc-search.md`](../lark-vc/references/lark-vc-search.md) |
| `vc +notes` | 详见 [`../lark-vc/SKILL.md`](../lark-vc/SKILL.md) |
| `task +get-my-tasks` | `task:task:read` |
| `docs +create` | 详见 [`../lark-doc/references/lark-doc-create.md`](../lark-doc/references/lark-doc-create.md) |

## 安全与范围

- 本工作流只读；Step 5 是唯一写操作，**执行前必须用户明确确认**
- 不要在输出里暴露参会人的 `open_id` 等内部标识，只展示姓名
- 简报属于 AI 生成内容，建议用户在会议开始前自行浏览一遍，必要时手工补充
- 所有关键词来自用户日程，不会把 description 原文中敏感内容外发到非飞书系统
- 时间和日期务必用系统命令换算，不要心算；错一天会导致简报完全不相关

## 参考

- [lark-shared](../lark-shared/SKILL.md) — 认证、权限（必读）
- [lark-calendar](../lark-calendar/SKILL.md) — `+agenda` 详细用法
- [lark-doc](../lark-doc/SKILL.md) — `+search`、`+create` 详细用法
- [lark-vc](../lark-vc/SKILL.md) — `+search`、`+notes` 详细用法
- [lark-task](../lark-task/SKILL.md) — `+get-my-tasks`、`+search` 详细用法
