---
name: lark-meeting
version: 1.0.0
description: "飞书视频会议：查询会议记录与会议产物(纪要/逐字稿/妙记)、妙记搜索/上传/下载/编辑、机器人参与会议；查询进行中的会议、实时会议内容(发言/聊天/共享文档)问答(会上/会里)、发送会中聊天/表情；基于 meeting_id、meeting_no、event_id、note_id、minute_token、vc-node-id 或妙记 URL 查询相关信息。预约会议、忙闲和会议室管理走 lark-calendar。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli vc --help;lark-cli minutes --help;lark-cli note --help"
---

# lark-meeting

飞书视频会议业务的统一入口，支持查询会议记录、实时会议互动、管理妙记、阅读智能纪要等操作。本技能负责领域关系、任务路由和跨命令编排。

无需预读 [`lark-shared`](../lark-shared/SKILL.md) 或预跑 `auth status --verify`，仅遇到未认证、token / 身份或 scope 错误时读取该 Skill，修复后重试。认证、身份或 scope 管理请求则直接使用该 Skill。

## 身份初始化与延续

把 `source_identity` 作为跨命令工作流的状态：

1. 上下文已有来源身份：严格沿用。用户要求切换时先说明身份连续性和权限影响，不静默切换。
2. 没有来源身份，用户明确指定身份：使用用户指定的 `--as`。
3. 没有来源身份且用户未指定：操作语义明确要求应用机器人时使用 `--as bot`，否则显式使用 `--as user`。

确定 `source_identity` 后，再检查目标命令是否支持该身份：

- 支持：显式传入并继续执行。
- 不支持：说明限制并停止；不要为了让命令成功而替换身份。
- 只有用户明确同意切换身份后，才以新身份重新开始一条工作流。

## 领域模型与概念

```text
[会议来源]

Calendar 日程 (event_id) ──预约或关联──┐
即时会议（无 event_id）────────────────┴──► 会议 (meeting_id)

Calendar 日程 ──meeting_note────────────► Doc（用户纪要，独立于 AI 智能纪要）

[会议产物]

会议 (meeting_id)
├── AI 总结 ──► Note 智能纪要 (note_id)
│               ├── 智能纪要文档 ───────────► Doc (note_doc_token)
│               ├── 逐字稿
│               │   ├── normal  ──────────► Doc (verbatim_doc_token)
│               │   └── unified ──────────► note +transcript（非独立 Doc）
│               └── 共享文档 ──────────────► Doc (shared_doc_tokens)
│
└── 录制 ──► Minutes 妙记 (minute_token)
                 ├── AI 产物：Summary / Todo / Chapter / Keyword
                 ├── Transcript（文字记录）
                 └── 原始音视频

本地音视频 ─────────────────────────────► Minutes 妙记 (minute_token)
```

| 对象 | 主标识 | 概念与关系 |
|---|---|---|
| Calendar 日程 | `event_id` | 日历上的日程，包含时间、参与人、会议室和 RSVP，可预约或关联 VC 会议；不是完整的会议记录。日程上的 `meeting_note` 是用户手工绑定的 Doc，与 AI 智能纪要无关。 |
| Meeting 会议 | `meeting_id` | 实际发生的视频会议，可以来自 Calendar，也可以是没有日程的即时会议。会议主题、时间、参会人快照和会中事件属于会议数据；Note 与 Minutes 是它可能关联的会后产物。 |
| Note 智能纪要 | `note_id` | 开启 AI 总结后形成的逻辑产物集合。`note_display_type` 决定获取逐字稿中文字记录的不同方式。 |
| Minutes 妙记 | `minute_token` | 由会议录制或本地音视频上传生成，包含总结、待办、章节、关键词、文字记录和原始音视频；可以关联 VC 会议，也可以独立存在。 |
| Doc 文档 | Doc token | 内容载体，不是会议标识。`note_doc_token`、`shared_doc_tokens` 和部分 `verbatim_doc_token` 指向 Doc；Doc token 不能当作 `note_id` 或 `meeting_id`。 |

### 核心标识

- `meeting_id`：会议 ID。长数字字符串，不是 9 位会议号。
- `meeting_no`：会议号。9 位纯数字；CLI 参数名为 `--meeting-number`。
- `minute_token`：妙记 Token。小写字母数字串，通常取自妙记 URL `/minutes/<minute_token>`。

以上标识均按字符串原样传递，不能相互替代。

### 领域不变量

- Note 与 Minutes 分别来自 AI 总结和录制两条独立链路。一场会议可能同时有两类产物、只有其中一类，也可能都没有；不能根据 `note_id` 推断必然存在 `minute_token`，反之亦然。
- Minutes 可以由本地音视频直接生成，因此不一定关联 `meeting_id` 或 Calendar `event_id`。
- Calendar `meeting_note`、Note `note_id`、Minutes `minute_token` 和各类 Doc token 标识不同对象，不能互换、代入其他域的命令或从一者反推另一者。

## 快速行动

### 查询进行中的会议内容

```bash
# 当前用户所在会议
lark-cli vc +meeting-list-active --as user

# 应用机器人可见的目标用户会议
lark-cli vc +meeting-list-active --as bot --user-id <open_id>

# 确定唯一 meeting_id 后沿用来源身份
lark-cli vc +meeting-events --as <source_identity> --meeting-id <meeting_id> --page-all --format pretty
```

同时有多场会议时，需要先选择要查询的会议；只有一场会议时，直接查询该场会议的会议事件。

应用身份只返回“目标用户正在参会、且应用机器人也在同一会议中”的会议；返回空不代表目标用户没有在开会。向用户说明结果时使用“用户身份”或“应用身份”，不要暴露 `user` / `bot` 这类内部缩写。

## 场景手册

当任务目标与场景匹配时，阅读对应的场景手册，按流程执行任务。

- [查询会议及其产物](scenes/query-meeting-and-artifacts.md)：按主题、时间、参会人或 `meeting_id` / `meeting_no` / `event_id` 定位历史会议；查询参会人、录像和会议关联的智能纪要或妙记；基于会议记录总结或复盘。
- [查询妙记及其产物](scenes/query-minutes-and-artifacts.md)：已有妙记 URL / `minute_token`，或按标题、所有者、参与者搜索妙记；读取总结、待办、章节、关键词、逐字稿，下载原始音视频，或查询关联智能纪要。
- [生成和修改妙记、管理妙记权限](scenes/create-and-edit-minutes.md)：将本地音视频生成妙记、逐字稿、总结、待办或章节；修改妙记标题、总结、待办、关键词或说话人；申请妙记权限，或查看、分配妙记协作者权限。
- [查询智能纪要及关联产物](scenes/query-note-and-artifacts.md)：已有 `note_id`、智能纪要 Docx URL/token，或需要查询纪要正文、逐字稿、妙记和共享文档等关联产物。
- [应用机器人参会与会中互动](scenes/live-meeting-attend.md)：完整编排应用机器人的活跃会议发现、真实入会、事件拉取、文本/表情互动和明确授权后的离会。
- [会中事件与会中互动](scenes/live-meeting-interact.md)：在不触发新的入会/离会操作时，使用用户身份或已在会中的应用身份查询活跃会议、查看发言/聊天/共享内容，或发送文本和表情。

## 命令参考

| 命令 | 用途 | 参考方式 |
|---|---|---|
| `vc +search` | 搜索历史会议 | [lark-vc-search](references/lark-vc-search.md) |
| `vc +detail` | 查询会议信息及关联的 Note、Minutes 标识 | [lark-vc-detail](references/lark-vc-detail.md) |
| `vc meeting get` | 查询会议基础信息和参会人快照 | `lark-cli vc meeting get --help` |
| `vc +recording` | 从会议定位录制及妙记 | [lark-vc-recording](references/lark-vc-recording.md) |
| `vc +meeting-list-active` | 发现当前可见的进行中会议 | [lark-vc-meeting-list-active](references/lark-vc-meeting-list-active.md) |
| `vc +meeting-events` | 读取会中事件和共享内容 | [lark-vc-meeting-events](references/lark-vc-meeting-events.md) |
| `vc +meeting-message-send` | 发送会中文本消息或表情 | [lark-vc-meeting-message-send](references/lark-vc-meeting-message-send.md) |
| `vc +meeting-join` | 让应用机器人加入会议 | [lark-vc-agent-meeting-join](references/lark-vc-agent-meeting-join.md) |
| `vc +meeting-leave` | 让应用机器人离开会议 | [lark-vc-agent-meeting-leave](references/lark-vc-agent-meeting-leave.md) |
| `minutes +search` | 搜索妙记 | [lark-minutes-search](references/lark-minutes-search.md) |
| `minutes minutes get` | 查询妙记基础信息 | `lark-cli minutes minutes get --help` |
| `minutes +detail` | 读取妙记信息和指定产物 | [lark-minutes-detail](references/lark-minutes-detail.md) |
| `minutes +download` | 下载妙记原始音视频 | [lark-minutes-download](references/lark-minutes-download.md) |
| `minutes +upload` | 从云空间音视频生成妙记 | [lark-minutes-upload](references/lark-minutes-upload.md) |
| `minutes +update` | 修改妙记标题 | [lark-minutes-update](references/lark-minutes-update.md) |
| `minutes +speaker-replace` | 替换妙记逐字稿说话人 | [lark-minutes-speaker-replace](references/lark-minutes-speaker-replace.md) |
| `minutes +summary` | 替换妙记 AI 总结 | [lark-minutes-summary](references/lark-minutes-summary.md) |
| `minutes +todo` | 增删改妙记 AI 待办 | [lark-minutes-todo](references/lark-minutes-todo.md) |
| `minutes +apply-permission` | 申请妙记查看或编辑权限 | [lark-minutes-apply-permission](references/lark-minutes-apply-permission.md) |
| `drive +member-list` | 查看妙记协作者及其权限 | [lark-drive-member-list](../lark-drive/references/lark-drive-member-list.md) |
| `drive +member-add` | 给指定成员分配妙记查看或编辑权限 | [lark-drive-member-add](../lark-drive/references/lark-drive-member-add.md) |
| `minutes +word-replace` | 批量替换妙记逐字稿关键词 | `lark-cli minutes +word-replace --help` |
| `note +detail` | 查询智能纪要及关联文档标识 | [lark-note-detail](references/lark-note-detail.md) |
| `note +transcript` | 获取 unified 智能纪要逐字稿 | [lark-note-transcript](references/lark-note-transcript.md) |

## 渐进加载规则

按“快速行动 → 场景手册 → 命令参考”渐进加载：

1. 用户目标符合“快速行动”的进入条件时，直接执行对应 CLI；不要预读场景手册、命令参考、`--help` 或 schema。
2. 不符合快速行动条件，或缺少关键标识、需要消歧、涉及写操作时，读取与目标匹配的一个主场景手册；主场景明确转交到下游场景时，只继续读取被引用的场景或章节，并按其中流程执行 CLI。
3. 仅当缺少具体参数、返回字段、特殊约束或异常处理方式时：有参考手册的命令读取对应文件；没有参考手册的命令运行表中列出的精确 `lark-cli ... --help`。场景或 reference 已给出精确命令时，不再调用 `--help`；仅在参数缺失、命令不识别或文档与运行结果冲突时调用。
