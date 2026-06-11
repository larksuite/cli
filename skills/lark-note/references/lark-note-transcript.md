
# note +transcript

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

通过已知 `note_id` 查询 **unified 纪要的原始逐字记录**，CLI 内部全量翻页后保存到本地文件。只读操作，仅支持 `user` 身份。

本 skill 对应 shortcut：`lark-cli note +transcript`。

> **何时用这个命令？** 当 `note +detail` 或 `vc +notes` 返回 `note_display_type=unified`，且用户想要逐字稿 / 原始记录 / 谁说了什么时。普通纪要（`normal`）的逐字稿是独立文档，应改用 `docs +fetch --api-version v2 --doc <verbatim_doc_token>`。
>
> **`note_id` 来源：** 如果当前只有纪要文档 token / URL，先 `docs +fetch --api-version v2 --doc <doc_token>`；返回中的 `<vc-transcribe-tab vc-node-id="..."></vc-transcribe-tab>` 表示 VC 原始记录 block，其中 `vc-node-id` 属性值就是 `--note-id`。如果没有该 block，但正文里有“文字记录/逐字稿”等明确 Docx 链接，那是普通纪要的独立逐字稿文档，应改用 `docs +fetch --api-version v2 --doc <verbatim_doc_token>`，不要调用本命令。

## 命令

```bash
# 默认 markdown，保存到 ./notes/{note_id}/unified_transcript.md
lark-cli note +transcript --note-id NOTE_ID

# 纯文本输出，保存到 ./notes/{note_id}/unified_transcript.txt
lark-cli note +transcript --note-id NOTE_ID --transcript-format plain_text

# 指定输出文件
lark-cli note +transcript --note-id NOTE_ID --output ./transcript.md --overwrite

# 预览 API 调用
lark-cli note +transcript --note-id NOTE_ID --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--note-id <id>` | 是 | Note ID；如果来自 `docs +fetch --api-version v2`，取 `<vc-transcribe-tab>` 的 `vc-node-id` 属性值 |
| `--transcript-format <fmt>` | 否 | 逐字稿内容格式：`markdown`（默认）/ `plain_text` |
| `--locale <locale>` | 否 | 系统文案语言；默认跟随 profile language，未配置时 Feishu 为 `zh_cn`、Lark 为 `en_us`，也支持 `ja_jp` 等 |
| `--output <path>` | 否 | 输出文件路径；不传时默认落到 `./notes/{note_id}/unified_transcript.{md,txt}` |
| `--overwrite` | 否 | 覆盖已存在的输出文件 |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 输出结果

| 字段 | 说明 |
|------|------|
| `note_id` | 输入的 Note ID |
| `transcript_format` | 逐字稿内容格式：`markdown` / `plain_text` |
| `transcript_file` | 本地 transcript 文件路径 |
| `size_bytes` | 写入文件大小 |

输出示例：

```json
{
  "note_id": "note_xxxx",
  "transcript_format": "markdown",
  "transcript_file": "notes/note_xxxx/unified_transcript.md",
  "size_bytes": 123456
}
```

## 执行说明

- 该 API 分页返回，CLI 内部自动翻页（`cursor_id`）并把全部内容拼接保存，**不暴露分页参数**。
- 任一页失败会整体报错，不保存半截 transcript。
- 首期不支持 `structured` 输出格式。
- 默认 `markdown`，作为 AI Friendly 输出；`plain_text` 为轻量纯文本。

## 常见错误与排查

| 错误现象 | 根本原因 | 解决方案 |
|---------|---------|---------|
| `--note-id is required` | 未传入 note_id | 补全 `--note-id` |
| `output file already exists` | 目标文件已存在 | 加 `--overwrite` 覆盖，或换 `--output` 路径 |
| `no read permission for this note` | 调用者无该纪要阅读权限 | 向纪要所有者申请权限 |
| `missing required scope(s)` | 缺少 `vc:note:read` | 按提示运行 `auth login --scope vc:note:read` |

## 参考

- [lark-note](../SKILL.md) — Note 域总入口
- [lark-note-detail](lark-note-detail.md) — 纪要详情与展示类型查询
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
