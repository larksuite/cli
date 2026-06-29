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

Note 域只接受显式 `note_id`：用户直接提供，或 `docs +fetch --api-version v2` 返回的 `<vc-transcribe-tab vc-node-id="...">` 中的 `vc-node-id`。不要从 `doc_token`、标题、正文或 backlink 反推 `note_id`。

## 命令路由

| 用户表达 / 上下文 | 路由 |
|---------|------|
| 已知 `note_id`，查纪要类型 / 文档 token | `note +detail --note-id NOTE_ID` |
| `docs +fetch --api-version v2` 返回 `<vc-transcribe-tab vc-node-id="...">` | 取 `vc-node-id` 作为 `NOTE_ID`，先 `note +detail --note-id NOTE_ID` |
| 已知 `note_id`，读纪要正文 | `note +detail` → `docs +fetch --api-version v2 --doc <note_doc_token>` |
| 已知 `note_id`，查 unified 原始记录 / 逐字稿 | `note +transcript --note-id NOTE_ID` |
| 只有自然语言纪要标题，用户要逐字稿 / 原始记录 / 谁说了什么 | 不进本 skill；先走文档搜索与 `docs +fetch`，拿到 `vc-node-id` 后再回来 |

## `note_display_type` 路由

| `note +detail` 结果 | 用户要逐字稿 / 原始记录时 |
|------|---------------|
| `normal` + `verbatim_doc_token` 非空 | `docs +fetch --api-version v2 --doc <verbatim_doc_token>` |
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

- 通过 `meeting_id` / `calendar_event_id` / `minute_token` 定位纪要 → [lark-vc](../lark-vc/SKILL.md)。
- 自然语言纪要标题搜索 → [lark-drive](../lark-drive/SKILL.md) / [lark-doc](../lark-doc/SKILL.md)。
- Docx 正文读取 → [lark-doc](../lark-doc/SKILL.md)。
- 妙记基础信息与媒体文件 → [lark-minutes](../lark-minutes/SKILL.md)。

## Shortcuts

| Shortcut | 何时读 reference |
|----------|------|
| [`+detail`](references/lark-note-detail.md) | 需要解释输出字段或根据展示类型继续路由 |
| [`+transcript`](references/lark-note-transcript.md) | 需要拉取 unified 原始记录或处理本地输出文件 |
