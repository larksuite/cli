---
name: lark-note
version: 1.0.0
description: "飞书会议纪要（Note）直查：已知 note_id 时查询纪要详情、展示类型（普通纪要 / unified 纪要）、关联文档 token，以及 unified 纪要的原始逐字记录（unified transcript）。用户已经持有 note_id 并想查纪要元信息、纪要类型、纪要/逐字稿文档 token 时使用本技能；unified 纪要的逐字稿不是独立文档，必须用 note +transcript 按 note_id 拉取。本技能只接受 note_id 入口。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli note --help"
---

# note (v1)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理。**

Note 域负责**已知 `note_id`** 时的纪要直查。它不反查会议、日程、妙记或文档标题，也不读取 Docx 正文——那些分别属于 `lark-vc`、`lark-minutes`、`lark-doc`。

> **`note_id` 来源：** 如果入口是文档，先用 `docs +fetch --api-version v2 --doc <doc_token>` 读取文档元信息；返回里的 `<vc-transcribe-tab vc-node-id="..."></vc-transcribe-tab>` 表示 VC 原始记录 block，其中 `vc-node-id` 属性值就是 Note 域使用的 `note_id`。这是显式属性映射，不要从 `doc_token`、标题、正文或 backlink 反推 `note_id`。
>
> **只有纪要标题时：** 用户说“查询 xx 纪要的逐字稿 / 原始记录 / 谁说了什么”，且没有 `note_id`、`meeting_id`、`calendar_event_id`、`minute_token`、会议号或妙记 URL 时，先搜索纪要文档并 fetch 正文。只有正文里的 `<vc-transcribe-tab vc-node-id="...">` 可以进入本 skill；否则只读取正文中明确给出的“文字记录/逐字稿” Docx 链接，不要强行进入 Note 域。

## 核心概念

- **Note（会议纪要）**：会议结束后生成的纪要实体，通过 `note_id` 标识。
- **展示类型（`note_display_type`）**：区分纪要形态，取值 `unknown` / `normal` / `unified`。
  - `normal`（普通纪要）：纪要正文和逐字稿是两份独立的飞书文档，分别对应 `note_doc_token`、`verbatim_doc_token`。
  - `unified`：纪要正文、AI 产物、逐字记录合并呈现；**逐字稿不再是独立文档**，要用 `note +transcript` 按 `note_id` 拉取原始记录。
- **文档 token**：`note_doc_token`（AI 智能纪要主文档）、`verbatim_doc_token`（普通纪要逐字稿文档）、`shared_doc_tokens`（会中共享文档）。拿到 token 后读正文交给 [lark-doc](../lark-doc/SKILL.md)。

## 触发规则

| 用户表达 | 命令 / 路由 |
|---------|------|
| 已知 `note_id`，查纪要详情 / 纪要类型 / 关联文档 token | `note +detail --note-id NOTE_ID` |
| 只有自然语言纪要标题，用户要逐字稿 / 原始记录 / 谁说了什么 | 不进本 skill；先路由到 [lark-drive](../lark-drive/SKILL.md) / [lark-doc](../lark-doc/SKILL.md)，拿到 `vc-node-id` 后再回来 |
| `docs +fetch --api-version v2` 返回了 `<vc-transcribe-tab vc-node-id="...">`，要进入 Note 域 | 把 `vc-node-id` 属性值作为 `NOTE_ID`：`note +detail --note-id <vc-node-id>` |
| 已知 `note_id`，查 unified 原始记录 / 逐字稿 | `note +transcript --note-id NOTE_ID` |
| 已知 `note_id`，读纪要正文 | 先 `note +detail` 拿 `note_doc_token`，再调 `docs +fetch --api-version v2 --doc <note_doc_token>` |

## 路由规则（拿到 detail 后按 `note_display_type` 决策）

| 条件 | Agent 后续动作 |
|------|---------------|
| 用户要纪要正文 / 总结 / 待办 / 章节 | `docs +fetch --api-version v2 --doc <note_doc_token>` |
| `note_display_type=normal`，用户要逐字稿 / 谁说了什么 | `docs +fetch --api-version v2 --doc <verbatim_doc_token>` |
| `note_display_type=unknown`，且 `verbatim_doc_token` 非空，用户要逐字稿 / 谁说了什么 | `docs +fetch --api-version v2 --doc <verbatim_doc_token>`；不要猜成 unified |
| `note_display_type=unknown`，且无可用逐字稿 token | 如果当前结果来自 `vc +notes`，可补一次 `note +detail --note-id <note_id>` 复核；如果 `note +detail` 后仍是 `unknown` 且没有逐字稿 token，停止重试并告知用户无法确定逐字稿入口 |
| `note_display_type=unified`，用户要逐字稿 / 原始记录 / 谁说了什么 | `note +transcript --note-id <note_id>` |

> **判别键是 `note_display_type`，不是 `verbatim_doc_token` 是否为空。** unified 纪要的 `verbatim_doc_token` 也可能有值，但 unified 的逐字稿应统一走 `note +transcript`（输出更结构化）。

## 禁止规则

- 不处理 `meeting_id` —— 那是 [lark-vc](../lark-vc/SKILL.md) 的入口。
- 不处理 `calendar_event_id` —— 那是 [lark-vc](../lark-vc/SKILL.md) 的入口。
- 不处理 `minute_token` —— 那是 [lark-vc](../lark-vc/SKILL.md)（纪要产物索引）/ [lark-minutes](../lark-minutes/SKILL.md)（妙记基础信息与媒体）的入口。
- 不处理自然语言纪要标题搜索 —— 先搜索纪要文档并 fetch 正文；只有 fetch 结果里的 `vc-node-id` 可以作为 `note_id`，普通纪要里的“文字记录/逐字稿” Docx 链接仍由 [lark-doc](../lark-doc/SKILL.md) 读取。
- 不读取 Docx 正文 —— 拿到文档 token 后交给 [lark-doc](../lark-doc/SKILL.md)。
- 不从纪要正文或 `doc_token` 反推 `note_id`；只有 `docs +fetch --api-version v2` 结果中 `<vc-transcribe-tab>` 的显式 `vc-node-id` 属性可以作为 `note_id`。

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli note +<verb> [flags]`）。

| Shortcut | 说明 |
|----------|------|
| [`+detail`](references/lark-note-detail.md) | Get note detail (display type, document tokens) by note_id |
| [`+transcript`](references/lark-note-transcript.md) | Fetch the unified note transcript and save it to a file |

- 使用 `+detail` 命令时，必须阅读 [references/lark-note-detail.md](references/lark-note-detail.md)。
- 使用 `+transcript` 命令时，必须阅读 [references/lark-note-transcript.md](references/lark-note-transcript.md)。

## 权限表

| 方法 | 所需 scope |
|------|-----------|
| `+detail` | `vc:note:read` |
| `+transcript` | `vc:note:read` |

## 参考

- [lark-vc](../lark-vc/SKILL.md) — 从 meeting_id / calendar_event_id / minute_token 定位 note_id
- [lark-doc](../lark-doc/SKILL.md) — 读取纪要正文 / 普通逐字稿文档正文
- [lark-minutes](../lark-minutes/SKILL.md) — 妙记基础信息与媒体下载
- [lark-shared](../lark-shared/SKILL.md) — 认证和全局参数
