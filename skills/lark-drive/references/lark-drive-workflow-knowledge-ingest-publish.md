# 本地资料入库 — 发布计划、转换写入与验证

本文件是 [`lark-drive-workflow-knowledge-ingest.md`](lark-drive-workflow-knowledge-ingest.md) 的配套 phase 文档，在 workflow 进入 `PUBLISH_PLAN`、`CONVERT_WRITE` 和 `VERIFY` 状态时加载。

本文承载资料入库的**写与验证**细节：发布计划与门禁、异构资料转换铁律、执行台账、写后验证。发布计划 JSON schema 与输出模板见 [`lark-drive-workflow-knowledge-ingest-outputs.md`](lark-drive-workflow-knowledge-ingest-outputs.md)。

## 核心铁律：最终载体必须是可检索 docx 正文

本 workflow 的完成标准是「目标 Wiki 节点下存在 `obj_type=docx` 的可检索正文」，不是「上传了原文件」。据此：

- 每个知识页最终都是目标节点下的飞书 docx；原始文件只作来源附件、来源链接，不替代正文。
- `drive +upload` 只上传原文件（结果类型 `file`），不是文档转换命令，**禁止用于完成知识页**，也禁止把上传成功记为页面 `created` / `verified`。
- 用户说「补充到知识库」默认目标是知识页；只有明确说「只上传原文件 / 作为附件」时才允许只建文件节点。

这些由 `publish_gate.py` 硬门禁强制。

## PUBLISH_PLAN：发布计划与门禁

### 生成发布计划

为每份 `ready` 分析结果生成一个计划项（字段与 schema 见 outputs 文档），至少含：

- `source_id`、`title`、`target_node`；
- `publish_role`：`knowledge_page`（默认）/ `source_attachment`（仅用户明确要求保留原件时）；
- `write_via`：`import_docx` / `docs_update` / `node_create_docx` / `drive_upload`（见 entry 的 `Write Via Selection`）；
- `target_obj_type`、`target_token`；
- `sensitivity`、`sensitive_review_status`、`conflict_status`、`parse_status`；
- 知识页的 6 行治理字段 `governance`（见 outputs 文档）。

### 运行门禁

```bash
python3 "<SKILL_ROOT>/references/scripts/publish_gate.py" --plan "<发布计划 JSON 路径>"
```

门禁两级判定（详见 entry 的 `Publish Gate`）：硬拦项 `ready=false` 记入 `unsupported_checks` 不写入；`narrowed=true` 项按收紧后的页面状态写入。

### 产出计划 + 三清单

向用户展示：发布计划表（目标节点 + `write_via` + 门禁结果 + 逐份精确处置），以及三张清单——**冲突清单**（`conflict_status=suspected/confirmed`）、**敏感清单**（`prohibited` / `restricted` 未审）、**无法解析清单**（`parse_status=unsupported/failed`）。用户确认后仅 `ready=true` 项进入 `CONVERT_WRITE`。全部被拦时（情况 9）报告「本批 0 入库」+ 逐项原因，不静默结束。

## CONVERT_WRITE：异构资料转换与写入

收到确认后逐份执行。批量写入 / 导入同一位置时**串行**，避免并发冲突。

**身份一致性**：本阶段所有写命令（`drive +import`、`docs +update`、`wiki +move`、`wiki +node-create`、`drive +upload`）都用 `PARSE_SOURCES` 确定的同一身份执行——命令模板中的 `--as <runtime identity>` 是占位符，须替换为该身份（用户选 bot 路径就是 `bot`），不得硬编码 `user`。discovery、目标解析、写入、验证若用不同身份，Drive 资源和权限不同，后续 move / 验证会失败或操作到错误资源。

### Word / Markdown / TXT / HTML（write_via=import_docx，仅用于新建页）

`import_docx` 只用于 `proposed_action=add`（**新建页**）。`update` / `merge` 面向既有目标页，必须走 `docs_update` 定向更新，不得用 import_docx——否则会新增一个子页面而不是更新既有页（见「更新既有页」小节）。

用 `drive +import --type docx` 转飞书云文档，读回整理后落到目标节点；不要把原始分页、页眉页脚当知识结构。

```bash
lark-cli drive +import --as <runtime identity> --type docx --file "<本地文件>" --folder-token "<暂存/目标文件夹>" --name "<发布计划中的标题>"
```

- 导入结果必须返回 `docx` 类型和在线文档 token，否则转换失败。
- **异步续跑**：`drive +import` 内置轮询窗口内未完成时会返回 `ready=false` / `timed_out=true` 和 `ticket`，用 `drive +task_result --scenario import --ticket <TICKET>` 续查，拿到最终在线文档 token 后再继续，不把未完成当完成。
- **迁入目标节点**：一份对应一页且保真结构适用时，用 `wiki +move --obj-type docx --obj-token <导入文档 token> --target-space-id <SPACE_ID> --target-parent-token <目标父节点>` 迁入目标位置。`docs_to_wiki` 迁入可能返回 `task_id` 异步执行；返回 `ready=false` / `timed_out=true` 时，用 `drive +task_result --scenario wiki_move --task-id <TASK_ID>` 续查至完成，不把超时当失败。迁入完成后必须 fresh read 确认目标 Wiki 节点 `obj_type=docx` 且 `docs +fetch` 可读（ready-state 验证），再套 6 行治理表。
- 多份合并 / 一份拆页 / 需统一重写时，创建目标 docx 节点后 `docs +update` 写整理内容，导入件仅作暂存来源。
- PDF **不可**用 `drive +import`（不在支持扩展名内），走下节。

### PDF（write_via=docs_update）

PDF 不假设可直接导入。先解析文本层；扫描件借 agent 多模态能力 OCR，记录解析方式、页码与置信度。随后 `docs +update` 把有效内容重建为 docx 正文；原 PDF 仅在需保留证据时作附件。

- 文字、表格、结论标注原 PDF 页码；
- OCR 低置信度、表格错位或关键页不可解析时标 `parse_status=partial/unsupported`，不猜测补齐（门禁会拦 unsupported、收紧 partial 的已完成状态）；
- 不复制封面、页眉、水印和纯装饰图。

### 图片（write_via=docs_update）

结合上下文判断媒体作用再决定是否入页：只保留能解释规则 / 步骤 / 入口 / 证据的图片，放在其解释的段落附近，加图注（说明 + 来源 + 必要时间），并把图中关键文字转成可检索正文——不让答案只存在于截图里。

图片类 `docs +update` 绑定本地资源时，必须用目标页的 **docx 对象 token（`doxcn_*`，即计划里的 `target_obj_token`）或规范 Wiki URL** 定位，裸 Wiki node token（`wikcn_*`）不触发资源解析。计划须同时保留 `target_token`（Wiki 操作用）和 `target_obj_token`（写正文 / 绑图用）。

### 更新既有页（write_via=docs_update，proposed_action=update/merge）

资料映射到一个**既有目标页**（`proposed_action=update` 或 `merge`）时，不论原始资料是不是 Word，都走 `docs_update` 对既有页定向更新，**不走 import_docx**（import 会新增子页面，而非更新目标页）：

- 先用稳定 token（`target_token`）定位既有页并读取现状，不按标题匹配。落笔前 `docs +fetch` 重读并记录 `revision`，携带该 `revision` 再写；若确认后、写入前内容已变（协作者改动），停下重新确认，不用默认 `revision-id=-1` 静默覆盖最新版。
- 优先定向替换或 block 级编辑受影响部分；仅整页失效且用户确认整页重建时才 overwrite。
- Word/PDF 等来源仍先解析为整理内容，再写入既有页；导入件（如用到）仅作暂存来源，不作为最终页。
- 更新后保持 6 行治理表结构，刷新版本、生效 / 更新时间、更新原因与复核策略。

### 6 行治理表

每个新建或更新的知识页顶部套统一的 6 行治理表（字段与顺序见 outputs 文档），再写正文。字段未知填「待确认」并保持 `page_status=进行中`；只有 6 行齐备、无「待确认」、完整解析的页面才可标「已完成」。

### 原文件附件（write_via=drive_upload，默认关闭）

仅当用户明确要求保留原件时，才把原文件挂为 `source_attachment`。附件分两类，执行路径不同：

**A. 知识页的伴随附件**（同一份资料既转知识页、又保留原件作证据）：上传必须**在其对应知识页 fresh-read 验证通过之后**，即只消费 `execution_ledger` 中状态为 `verified` 的知识页条目：

- `CONVERT_WRITE` 阶段只写知识页，不上传伴随附件；
- 对应知识页经 `VERIFY` 通过、记为 `verified` 后，才上传其 `source_attachment`（`drive +upload --wiki-token`）；
- 对应知识页验证失败（`failed` / `blocked`）的资料**不上传**其原件，避免留下没有可检索知识页的孤儿附件。

**B. 纯附件**（用户明确说“只上传原文件 / 只作附件、不要知识页”，该资料没有对应知识页）：这类项 `publish_role=source_attachment` 且没有伴随的 `knowledge_page` 台账条目，独立执行——在 `CONVERT_WRITE` 直接 `drive +upload --wiki-token` 挂到用户确认的目标节点，`VERIFY` 校验附件本身已挂载成功。它不依赖任何知识页 `verified`（否则永远执行不了）。

两类附件上传成功都只记为附件结果，绝不推进知识页状态。绝不删除、修改或移动原始文件。

### 执行台账

`execution_ledger` 是唯一进度源，逐份记状态：`planned` → `created`（写入完成待验证）→ `verified`（读验通过）/ `failed`（失败记因，有界重试不无限重试）/ `blocked`（等权限或业务确认）/ `skipped`（用户决定不处理）。重跑时从 `created` 做验证、从 `failed` 做有界重试，不重建 `verified` 项。

## VERIFY：写后验证

对每个已写页面 fresh read，逐项确认：

- Wiki 节点 `obj_type=docx`，不是 `file` / `sheet` / 其他类型；
- `docs +fetch` 能读取目标 `obj_token`；
- 6 行治理表完整；
- 正文可检索、非空、不依赖附件才能理解；
- 图片位置 / 图注正确，来源页码 / 时间码可追溯。

出现以下任一情况判**转换失败**，记 `failed`，不得标 `verified`：只得到原文件下载卡片、节点类型为 `file`、正文为空、正文只有附件链接、无法 `docs +fetch` 读取。

用户可见输出：验证表 + 最终汇总（已入库页数、跳过 / 被拦资料及原因、失败项、台账位置、知识库链接）。样式见 [`lark-drive-workflow-knowledge-ingest-outputs.md`](lark-drive-workflow-knowledge-ingest-outputs.md)。

## References

- [entry：knowledge_ingest 主文档](lark-drive-workflow-knowledge-ingest.md)
- [analyze：盘点、对齐、分诊与映射](lark-drive-workflow-knowledge-ingest-analyze.md)
- [outputs：模板](lark-drive-workflow-knowledge-ingest-outputs.md)
- [lark-drive-import](lark-drive-import.md)、[lark-drive-upload](lark-drive-upload.md)
- [lark-doc-update](../../lark-doc/references/lark-doc-update.md)、[lark-doc-fetch](../../lark-doc/references/lark-doc-fetch.md)
- [lark-wiki-node-create](../../lark-wiki/references/lark-wiki-node-create.md)、[lark-wiki-move](../../lark-wiki/references/lark-wiki-move.md)
