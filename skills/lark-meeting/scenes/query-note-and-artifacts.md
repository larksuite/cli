# 基于 note_id 查询智能纪要及关联产物

- 身份：`note +detail` 支持 `--as user` / `--as bot`；`note +transcript` 仅支持 `--as user`。`note_id` 若由某个身份取得（例如 `vc +detail --as bot`），`note +detail` 及后续 Doc、Drive 命令必须显式沿用同一个 `--as`。
- 如果 `note +detail --as bot` 返回 `unified`，不要静默切到 `--as user` 继续。先向用户说明该纪要逐字稿只能以用户身份读取，只有用户明确同意才切换身份重试。

## 从智能纪要 Docx 查询关联链接

当用户提供智能纪要 Docx URL/token，且只需要纪要类型或关联产物链接时，直接执行：

```bash
lark-cli docs +fetch --doc "<docx_url_or_token>" --doc-format markdown --as <source_identity>
```

从返回结构中仅提取：

- 输入文档：作为智能纪要主文档，返回用户提供的原始 URL。
- `<vc-transcribe-tab vc-node-id="...">`：可作为明确的 `note_id`。
- 标记为“文字记录”的 Docx URL。
- `/minutes/` 妙记 URL。
- `<cite type="doc" doc-id="..." file-type="...">` 中的共享文档 token。

如果没有从 `<vc-transcribe-tab>` 取得明确的 `note_id`，但存在妙记 URL，从 URL 路径最后一段提取 `minute_token`，再查询妙记基础信息：

```bash
lark-cli minutes +detail --minute-tokens "<minute_token>" --as <source_identity>
```

从对应的 `note_id` 继续 Note 查询；该字段为空或未返回时，继续按 Doc 处理。不要把 Doc token 或 `minute_token` 直接传给 Note 命令。

## 确认 note_id

Note 域只接受明确的 `note_id`：

- 用户直接提供的 `note_id`。
- `vc +detail` 从 `meeting_id` 返回的 `note_id`。
- `minutes +detail` 从 `minute_token` 返回的顶层 `note_id`。
- `docs +fetch` 返回的 `<vc-transcribe-tab vc-node-id="...">` 中的 `vc-node-id`。

不要从 Doc token、Docx URL、文档标题、正文或 backlink 反推 `note_id`。只有自然语言纪要标题或 Docx 链接时，先使用 Drive/Doc 搜索或读取文档；没有明确 `vc-node-id` 时继续按 Doc 处理，不进入 Note 查询。如果文档正文明确给出“逐字稿”或“文字记录”的 Docx 链接，将该链接继续作为 Doc 沿用当前身份读取；该链接仍不是 `note_id`。

如果当前只有 `meeting_id`、`minute_token` 或 Calendar `event_id`，先使用会议、妙记或日程场景取得 `note_id`；不要把这些标识直接传给 Note 命令。

## 查询关联产物标识

```bash
lark-cli note +detail --note-id <note_id> --as <source_identity>
```

保留以下字段，并按用户目标选择后续操作：

| 字段 | 含义 | 后续操作 |
|---|---|---|
| `note_id` | Note 唯一标识 | 后续 Note 命令继续使用该值 |
| `note_display_type` | `normal` / `unified` / `unknown` | 决定逐字稿入口 |
| `note_doc_token` | AI 智能纪要正文 | 交给 Doc 读取正文，或交给 Drive 查询名称和 URL |
| `verbatim_doc_token` | `normal` 或部分 `unknown` Note 的独立逐字稿 Doc | 仅按下方展示类型规则使用 |
| `shared_doc_tokens` | 会中共享文档列表 | 按用户目标查询元信息或正文 |

用户只需要关联产物标识时，返回上述可用 token 和 `note_display_type` 后停止，不读取文档正文。

## 读取智能纪要正文

用户需要 AI 智能纪要中的总结、待办、章节或正文时，读取 `note_doc_token`：

```bash
lark-cli docs +fetch --doc <note_doc_token> --doc-format markdown --as <source_identity>
```

读取正文后，检查返回 Markdown 中的第一个 `<whiteboard token="...">`。该画板是智能纪要封面；存在时提取 token，沿用同一身份下载到 `./notes/<note_id>/cover`，与 `note +transcript` 的逐字稿归入同一 Note 目录，并随正文一起展示：

```bash
lark-cli docs +media-download --type whiteboard --token <whiteboard_token> --output ./notes/<note_id>/cover --as <source_identity>
```

没有 `<whiteboard>` 时直接跳过，不视为失败。只有第一个 `<whiteboard>` 按封面处理；不要自动下载正文中的其他画板。

只需要文档名称或 URL 时不要读取正文，使用 Drive 元信息接口：

```bash
lark-cli drive metas batch_query --data '{"request_docs":[{"doc_type":"docx","doc_token":"<note_doc_token>"}],"with_url":true}' --as <source_identity>
```

## 读取逐字稿(文字记录)

逐字稿入口由 `note +detail` 返回的 `note_display_type` 决定，不要只根据 `verbatim_doc_token` 是否为空判断：

normal Note 逐字稿是 Doc 读取结果，unified Note 可由 `note +transcript` 保存为 Markdown 或 plain text，Minutes Transcript 则是妙记产物文本。它们都可作为原始发言记录，但序列化格式不是统一契约；应以实际返回内容为准，不要硬编码“发言人 + 相对时间戳”等固定行格式。

### note_display_type = normal 且 有 verbatim_doc_token

```bash
lark-cli docs +fetch --doc <verbatim_doc_token> --doc-format markdown --as <source_identity>
```

### note_display_type = unknown 且 有 verbatim_doc_token

```bash
lark-cli docs +fetch --doc <verbatim_doc_token> --doc-format markdown --as <source_identity>
```

### note_display_type = unknown 且 无 verbatim_doc_token

停止并说明无法确定逐字稿入口，不要反复重试或猜成 unified

### note_display_type = unified

```bash
lark-cli note +transcript --note-id <note_id> --as user
```

`note +transcript` 会自动获取完整分页并保存文件；目标文件已存在时，只有用户明确要求覆盖才添加 `--overwrite`。

## 查询会中共享文档

`shared_doc_tokens` 是该 Note 关联的会中共享文档，不是逐字稿或 `meeting_note`。按用户目标处理：

- 只要文档名称或 URL：使用 `drive metas batch_query`，每批最多查询 10 个 token。
- 需要正文：逐个使用 `docs +fetch --doc <shared_doc_token>`。
- 有多个共享文档时，先返回标题和 URL 让用户选择；用户明确要求全部读取时再逐个读取。
- 某个共享文档不存在或无权限时，保留该 token 并逐项报告，不把整组结果误报为失败。

## 基于纪要内容回答

- 用户只要现成 AI 总结、待办或章节时，读取智能纪要正文并返回对应内容。
- 用户要求提炼、重新总结、复盘、争议分析或“谁说了什么”时，按展示类型读取逐字稿原始内容并独立分析；禁止直接改写 AI 智能纪要作为独立结论。
- 用户只要链接或关联产物清单时，不读取正文或逐字稿。
- `meeting_note` 是 Calendar 日程上用户手工绑定的文档，不属于 Note 的 `shared_doc_tokens`，也不能通过 `note_id` 查询。
