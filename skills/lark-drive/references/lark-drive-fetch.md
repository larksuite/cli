# drive +fetch

读取 Doc、Sheet、Base、Slides、File、Minutes 等飞书云空间资源的内容并返回 Markdown。传入 URL 时自动识别类型；Wiki 链接会自动解包到底层资源，且无需先执行 `drive +inspect`。

对 Docx，本 shortcut 与 `docs +fetch --doc-format markdown` 的整篇读取复用同一套 Markdown 读取链路。已知 Doc / Docx 是否使用本命令，按 [`lark-doc`](../../lark-doc/SKILL.md) 的“快速决策”选择。

Wiki URL 可直接使用本命令；首次结果不足时，根据 `data.resource.type`、`data.warnings`、`data.has_more` 和任务需要决定继续分页、整篇读取或切换实体 skill。

## 什么时候用它，什么时候用别的

| 目标 | 用什么 |
|---|---|
| `lark-doc` 快速决策选择 Drive，或需要读取其他支持类型并返回 Markdown | `drive +fetch` |
| 表格精确取单元格值、统计行数、筛选排序 | `sheets`（表格）/ `base`（多维表）原生命令 |
| Slides 需要图表精确数值 | `lark-slides` 原生命令 |
| Minutes 指定产物、逐字稿、基于逐字稿独立分析 | `minutes +detail` |
| 按词检索 Doc、读取指定章节或范围 | `docs +fetch`（`--scope` / `--keyword`） |
| 获取 block ID、原始结构或编辑前信息 | `docs +fetch --doc-format xml`，按需使用 `with-ids` / `full` |
| 获取原始文件字节并保存到本地 | `drive +download` |

## 支持的类型

| 类型 | URL 路径 | fetch 读出来是什么 |
|---|---|---|
| 文档 docx / doc | `/docx/` `/doc/` | 整篇 Markdown，标题 / 表 / 图 / 画板挂 `{#block-id}` 锚点 |
| 电子表格 sheet | `/sheets/` | 文档名 + 每张子表的 GFM 表 |
| 多维表 base | `/base/` | 文档名 + 每张子表的 GFM 表 |
| 幻灯片 slides | `/slides/` | 标题分层 + 表格转 GFM + 图片描述 |
| 网盘文件 file | `/file/` | 提取文本（PDF / Word / Excel / 附件） |
| 妙记 minutes | `/minutes/` | 摘要 + 章节 + 待办 + 关键词；`--include transcript` 内联逐字稿，`--include note-doc` 获取关联纪要文档 token；取不到时保留正文并在 warnings 说明 |
| 知识库 wiki | `/wiki/` | 先解包到底层资源，再按上表读 |

除 Minutes 外，读取默认请求分页；服务端支持分页时返回当前页和续读游标，不支持时直接返回全部内容。

## 命令

```bash
# 传 URL（推荐）：自动识别类型，wiki 自动解包
lark-cli drive +fetch --url "https://xxx.feishu.cn/docx/doxcnxxx"

# 裸 token 必须显式 --type
lark-cli drive +fetch --token doxcnxxx --type docx

# wiki 里只读某张子表：?table= / ?sheet= 选择器会保留
lark-cli drive +fetch --url "https://xxx.feishu.cn/wiki/wikcnxxx?table=tblXXX"
```

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--url` | 二选一 | 文档 URL（推荐） |
| `--token` + `--type` | 二选一 | 裸 token 需 `--type`（docx / sheet / bitable / slides / file / minutes / wiki；也接受别名 doc / sheets / base） |
| `--embed-max-rows` | 否 | 物化表格每表最多 N 行（默认 50，0 = 不限），超了截断并提示 |
| `--full` | 否 | 除 Minutes 外：关闭自动分页，一次返回全部内容 |
| `--page-token` | 否 | 除 Minutes 外：传入上次返回的 `next_page_token` 续读；不能与 `--full` 同用 |
| `--page-size` | 否 | 除 Minutes 外：每页大小提示（默认 0 = 服务端默认）；不能与 `--full` 同用 |
| `--include` | 否 | 仅 minutes：`transcript` 内联逐字稿 / `note-doc` 取纪要文档 token |

## 输出

默认输出遵循 CLI JSON envelope：`{ok, identity, data: {...}, ...}`。正文按交付方式出现在 `content` 或 `content_file`；以下字段均位于 `data`：

- `data.content`：内联 Markdown 内容；超大正文自动落盘时可能不返回
- `data.content_file` / `data.content_preview`：`--full` 的超大正文自动落盘时，完整内容位于 `data.content_file.path`，`content_preview` 仅用于确认内容
- `data.content_delivery_hint` / `data.content_inline`：自动落盘不支持或写入失败时正文保持内联，`content_delivery_hint` 给出后续恢复方式
- `data.resource`：`{type, title, url, token, selector, update_time, create_time, source, note_id, note_doc_token, verbatim_doc_token}`
  - `selector`：URL 里的 `?sheet=` / `?table=` / `?view=` 透传过来
  - `source`：仅 wiki 输入出现，记录解包前的 wiki 节点
  - `create_time`：仅 minutes（minutes 没有 `update_time`）
  - `note_id` / `note_doc_token` / `verbatim_doc_token`：仅 minutes 且指定 `--include note-doc`；取不到时省略并在 `data.warnings` 说明
- `data.has_more` / `data.next_page_token`：服务端分页时标记是否还有内容并给出续读游标
- `data.warnings`：提示信息（如妙记逐字稿取不到）

## 内容读取的边界（拿不全时怎么办）

- **表格被截断**：GFM 表超过 `--embed-max-rows`（默认 50 行）会截断，尾部写「还有 X 行」。要全量有两种方式——调大 `--embed-max-rows`（设 `0` 拿不截断的 Markdown，适合通读全表）；或改用 `sheets +cells-get` / `base +record-list`（适合精确取数、统计、筛选）。
- **返回内容分页**：除 Minutes 外，先读默认页；当前内容足够即停止，已命中但需要连续后文时，将 `data.next_page_token` 传给 `--page-token` 续读少量页面。整篇或跨章节覆盖、答案位置未知、需要多轮检索时只执行一次 `--full`；禁止对同一资源重复 `--full`，`--full` 失败或超时再回退分页。若 `data.has_more=true` 但 `data.next_page_token` 为空，视为结果不完整并说明；服务端不分页时首次读取即返回全部内容。
- **完整内容交付**：`--full` 返回 `data.content_file` 时，后续直接对 `path` 本地 read / search，`content_preview` 不能替代完整正文，也不要再次 fetch 同一资源。若出现 `data.content_delivery_hint`，正文保持内联；当前内容足够时直接使用，不足时按 hint 优先在本地重定向，只有无法使用 shell 重定向时才用 `--page-token` 分页。
- **File 读取回退**：`drive +fetch` 返回的正文足够回答时直接使用；正文不足且需要原始文件字节时用 `drive +download`，需要核对 PDF / HTML / 图片等预览版式时用 `drive +preview`。
- **提纲 / 清单 / 跨章节总结**：先从目录或同级标题列出覆盖清单；最终答案必须让每个清单项都有明确对应，交付前逐项核对。可以合并表述，但不得静默省略；确实没有相关内容时明确说明。
- **docx 内嵌的电子表格**：默认就展开成 GFM 表（受 `--embed-max-rows` 截断，截断行为同正文表）。
- **docx 内嵌的多维表格**：默认展成 GFM 表（受 `--embed-max-rows` 截断）；要全量或精确取数，拿 `[多维表格](token=xxx)` 里的 token 去 base 技能（`base +record-list`）。

## 正文里的两种标记

- `{#block-id}`（标题 / 表 / 图 / 画板后）：定位**文档里的这块内容**，要编辑它先用 `docs +fetch --doc-format xml --detail with-ids` 拿到可编辑结构。
- `[名称](token=xxx)`（画板、内嵌表后）：`xxx` 是该**资源**本身的标识，和 block-id 不是一回事。画板 token 可用 `docs +media-download --type whiteboard` 取素材；内嵌多维表格 token 走 base 技能。
