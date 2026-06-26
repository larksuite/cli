---
name: lark-note
version: 1.0.0
description: "飞书会议纪要（Note）直查：已知 note_id 时查询纪要详情、展示类型、关联文档 token，并读取 unified 原始逐字记录。当用户已持有 note_id，或从文档显式 vc-node-id 获得 note_id 时使用。不负责会议/日程/妙记定位、文档标题搜索或 Docx 正文读取。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli note --help"
---

# note (v1)

身份：仅使用 `--as user`。使用前阅读 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)。

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-vc/references/vc-domain-boundaries.md`](../lark-vc/references/vc-domain-boundaries.md)**，不读将导致命令使用、会议产物决策、领域边界职责判断错误：
> 1. 了解日历 & VC、会议产物 & 文档的关联关系和职责划分
> 2. 了解会议产物（妙记和纪要）之间的关联关系，例如：**妙记和纪要产生条件相互独立**
> 3. 了解不同会议产物的组成部分，以便根据需求决策使用哪种产物的数据

Note 域只接受显式 `note_id`：用户直接提供，或 `docs +fetch` 返回的 `<vc-transcribe-tab vc-node-id="...">` 中的 `vc-node-id`。不要从 `doc_token`、标题、正文或 backlink 反推 `note_id`。

## 命令路由

| 用户表达 / 上下文 | 路由 |
|---------|------|
| 已知 `note_id`，查纪要类型 / 文档 token | `note +detail --note-id NOTE_ID` |
| `docs +fetch` 返回 `<vc-transcribe-tab vc-node-id="...">` | 取 `vc-node-id` 作为 `NOTE_ID`，先 `note +detail --note-id NOTE_ID` |
| 只持有 `meeting_id` | 先 `vc +detail --meeting-ids <id>` 拿 `note_id`，再 `note +detail --note-id NOTE_ID` |
| 只持有 `minute_token`（妙记 URL） | 先 `minutes +detail --minute-tokens <token>` 顶层取 `note_id`，再 `note +detail --note-id NOTE_ID`（不要把 `minute_token` 当 `note_id`） |
| 只持有日程 `event_id` | 先 `calendar +meeting --event-ids <id>` 拿 `meeting_id`，再按上一行继续 |
| 已知 `note_id`，读纪要正文 | `note +detail` → `docs +fetch --doc <note_doc_token>` |
| 已知 `note_id`，查 unified 原始记录 / 逐字稿 | `note +transcript --note-id NOTE_ID` |
| 只有自然语言纪要标题，用户要逐字稿 / 原始记录 / 谁说了什么 | 不进本 skill；先走文档搜索与 `docs +fetch`，拿到 `vc-node-id` 后再回来 |

## `note_display_type` 路由

| `note +detail` 结果 | 用户要逐字稿 / 原始记录时 |
|------|---------------|
| `normal` + `verbatim_doc_token` 非空 | `docs +fetch --doc <verbatim_doc_token>` |
| `unknown` + `verbatim_doc_token` 非空 | 先按独立文档处理；不要猜成 unified |
| `unknown` + 无逐字稿 token | 停止重试并说明无法确定逐字稿入口 |
| `unified` | `note +transcript --note-id <note_id>` |

判别键是 `note_display_type`，不是 `verbatim_doc_token` 是否为空：unified 纪要也可能返回非空 `verbatim_doc_token`。

## 关键字段

- `note_id`：Note 域唯一入口。
- `note_display_type`：`unknown` / `normal` / `unified`。
- `note_doc_token`：纪要正文文档，正文读取交给 [lark-doc](../lark-doc/SKILL.md)。
- `verbatim_doc_token`：普通纪要逐字稿文档；unified 逐字稿不按这个 token 路由。

## 不在本 Skill 范围

- 通过 `meeting_id` 定位纪要（`note_id`）→ [lark-vc](../lark-vc/SKILL.md)（`vc +detail`）。
- 通过 `minute_token` 定位纪要（`note_id`）→ [lark-minutes](../lark-minutes/SKILL.md)（`minutes +detail` 顶层返回 `note_id`）。
- 通过日程 `event_id` 定位会议(`meeting_id`) / 用户绑定纪要(`meeting_note`) → [lark-calendar](../lark-calendar/SKILL.md)（`calendar +meeting`）。
- 自然语言纪要标题搜索 → [lark-drive](../lark-drive/SKILL.md) / [lark-doc](../lark-doc/SKILL.md)。
- Docx 正文读取 → [lark-doc](../lark-doc/SKILL.md)。
- 妙记基础信息与媒体文件 → [lark-minutes](../lark-minutes/SKILL.md)。

## Shortcuts

| Shortcut | 何时读 reference |
|----------|------|
| [`+detail`](references/lark-note-detail.md) | 需要解释输出字段或根据展示类型继续路由 |
| [`+transcript`](references/lark-note-transcript.md) | 需要拉取 unified 原始记录或处理本地输出文件 |

## 核心概念

- **会议纪要（Note）**：视频会议结束后生成的结构化文档，通过 `note_id` 标识。一个 Note 包含 AI 智能纪要文档、逐字稿文档和会中共享文档。
- **note_id**：纪要的唯一标识符，可通过 `vc +detail --meeting-ids` 获取。
- **AI 智能纪要（MainDoc）**：AI 生成的会议总结与待办，对应 `note_doc_token`。
- **逐字稿（VerbatimDoc）**：会议的逐句发言记录，含说话人和时间戳，对应 `verbatim_doc_token`。
- **共享文档（SharedDoc）**：会中投屏共享的文档，对应 `shared_doc_tokens`。

## 核心场景

### 1. 通过 note_id 获取纪要文档 Token

1. 当用户已有 `note_id`，需要获取对应的 `note_doc_token`、`verbatim_doc_token` 或 `shared_doc_tokens` 时，使用 `note +detail`。
2. `note_id` 通常来自 `vc +detail` 的返回结果。
3. 获取到文档 Token 后，可使用 `docs +fetch` 读取文档内容，或使用 `drive metas batch_query` 获取文档元信息。

```bash
# 1. 从会议获取 note_id
lark-cli vc +detail --meeting-ids <meeting_id>

# 2. 用 note_id 拿文档 Token
lark-cli note +detail --note-id <note_id>

# 3. 读取纪要文档内容
lark-cli docs +fetch --doc <note_doc_token> --doc-format markdown
```
