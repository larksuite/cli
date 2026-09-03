# 本地资料入库 Workflow

Workflow id: `knowledge_ingest`

Risk / Structure: `R2-R3` / `S3`

本文实现已注册的本地资料入库 workflow。执行前必须先读取 [`lark-drive-workflow.md`](lark-drive-workflow.md) 和 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)，并遵循共享执行协议、Artifact Contract、Workflow Loading、认证和写入确认规则。

本文定义 workflow 专属的状态机、来源盘点规则、资料分诊与映射规则、转换写入铁律和 command family 允许范围。分析细节、转换/写入细节和输出模板拆到配套 phase / outputs 文档，仅在进入需要它的状态时按需加载。

配套 phase / outputs 文档只是本 workflow 的引用文件，不是独立 skill。不要把用户请求直接路由到这些文档。

## 必读上下文

执行本 workflow 前，必须先读取 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)，用于处理身份、认证、权限和写操作确认规则。

按阶段渐进加载其他 skill / 引用文档，见 `Progressive Load Map`。不要因为命中本 workflow 就预加载全部文档。

## 适用范围

本 workflow 用于把用户明确授权的**本地文件**，经盘点、去重、敏感初筛和内容分析后，转换成飞书 **docx 知识页**写入一个**已存在**的 Wiki 知识库，供搜索和知识问答使用。对应「把散落的本地资料整理入库」的场景。

适用触发语包括：

- "把这个文件夹 / 这些本地资料整理进知识库"
- "帮我把这批文档归类后放到知识库对应节点里"
- "盘点这些本地文件，去重后转成知识页入库"
- "这批资料入库，按知识库的收录规范归位和命名"

目标必须是一个**已存在**的 Wiki 知识库链接、知识空间或节点。来源必须是**本地文件或目录路径**。

## 核心立场：知识页而非原文件归档

本 workflow 默认交付**可检索的 docx 知识页**，不是原文件归档。原因：上传的原文件（`obj_type=file`）无法被 `docs +fetch` 读取正文，也难以进入飞书知识问答语料；只有 `obj_type=docx` 的可检索正文才是标准知识源。

因此：

- 用户说「把资料补充到知识库」而未明确说「只上传原文件 / 作为附件」时，一律按知识页处理。
- `drive +upload` 只用于**来源附件**（`source_attachment`），永远不算知识页完成；默认关闭，仅用户明确要求保留原件时才开启。
- 这两条由 `publish_gate.py` 硬门禁强制，agent 声明只能收紧、不能绕过。

## 非目标

本 workflow 不处理：

- **从零创建知识空间**：目标 Wiki 不存在时，指路 [`knowledge_base_bootstrap`](lark-drive-workflow-knowledge-base-bootstrap.md) 建库，不自行 `wiki +space-create`（见 `Situation Routing` 情况 2）。
- **撰写维护规范**：立规范是 [`knowledge_base_bootstrap`](lark-drive-workflow-knowledge-base-bootstrap.md) 的职责；本 workflow 只**读**其产物做映射依据。
- **云盘 / Wiki / 会议纪要等非本地来源**：本版仅摄取本地文件。
- **移动、复制、删除或重命名已有节点**：结构调整用 [`knowledge_organize`](lark-drive-workflow-knowledge-organize.md) 或 [`topic_move_collector`](lark-drive-workflow-topic-move-collector.md)。本 workflow 仅在 `NODE_PROPOSE` 经用户确认后新建承载节点。
- **删除、修改或移动原始资料**：原始文件只读，绝不改动。
- **知识内容问答或检索**、**AI 质量评分**、**建飞书复核任务**：均不在本版范围。
- **知识空间成员、权限或密级治理**：用 [`permission_governance`](lark-drive-workflow-permission-governance.md)。

## 与 knowledge_base_bootstrap 的关系（松耦合、只读、不互调）

两个 workflow 数据松耦合，不流程串联，不互相调用：

- 本 workflow 在 `TARGET_ALIGN` **读** `knowledge_base_bootstrap` 写入各节点的维护规范（收录范围、命名规范），据其决定「资料归到哪个节点、怎么命名」。
- 读不到规范时**降级继续**（据资料内容 + 节点标题推断映射与命名），并提示用户可先跑 `knowledge_base_bootstrap` 立规范；不代跑、不擅自立规范。
- 目标库不存在或节点不足时的处理见 `Situation Routing`。

## Agent 执行约束

触发本 workflow 后，agent 必须：

1. 按 `Execution State Machine` 的顺序执行，并维护 `Runtime State` 字段。
2. 执行某个状态前，先加载 `Progressive Load Map` 中该状态要求的引用文档；不要预加载全部文档。
3. 在 `PUBLISH_PLAN` 用户确认之前，绝不执行任何飞书写入（`docs +update` / `drive +import` 写入目标库 / `drive +upload`）。唯一例外是 `NODE_PROPOSE`：仅在用户单独确认承载节点大纲后，才可执行 `wiki +node-create` 新建节点。
4. `inventory.py` 只算元数据、不读正文；资料正文分析在 `ANALYZE_TRIAGE` 由 agent 完成。原始文件全程只读，绝不修改 / 移动 / 删除。
5. 只执行 `Command Map` 允许的 command family；命令语法、scope 要求、参数规则以被引用 skill / reference 为准。
6. 用户可见说明、字段说明和表格文案使用中文；状态名、字段名、枚举值、命令名保留英文稳定标识；内部枚举值在用户可见输出中转为自然语言中文标签。
7. 不声明 CLI / API 不支持的能力；无法执行的写入必须记入 `unsupported_checks`，不得静默省略。
8. 转换写入后必须用 fresh read 验证载体为 docx、正文落地，不以写入命令返回值直接判定成功。
9. 一份资料写入失败或被门禁拦截，只影响该资料，不连累其余相互独立的资料。

## Runtime State

本 workflow 在共享 `Artifact Contract` 基础上维护以下字段：

| Field | Meaning |
|-------|---------|
| `current_state` | `Execution State Machine` 中的当前状态 |
| `source_scope` | 用户授权的本地路径（文件或目录）和原始输入；仅本地来源 |
| `target_space` | 解析出的 `space_id`、目标节点范围和用户原始输入 URL；目标为已有 Wiki |
| `identity` | 执行身份，默认 `user`；解析、读取、写入、验证使用同一身份 |
| `inventory` | `inventory.py` 产出的资料台账：每份资料的 `source_id`(SHA-256)、类型、大小、`parse_readiness`、`risk_hint`、重复组 |
| `node_inventory` | 目标库节点的归一化列表：`node_token`、`obj_token`、`title`、`obj_type`、`node_type`、层级 |
| `standard_map` | 各节点从 `knowledge_base_bootstrap` 规范读到的收录范围 / 命名规范；无规范的节点标记缺失 |
| `alignment_mode` | `standard`（有规范可依）/ `degraded`（无规范，据内容推断映射）/ `mixed`（逐节点不同） |
| `outline_proposal` | 节点不足时据真实资料提议的承载节点：拟建标题、位置、收录范围；经用户确认后新建 |
| `material_map` | 每份资料的分析结果：判类、`target_node`、拟定标题、版本冲突状态、敏感等级、`proposed_action`（add/update/merge/reference/review/skip）、`mapping_confidence` |
| `publish_plan` | 逐资料写入计划：`publish_role`、`write_via`、目标节点、`parse_status`、6 行治理字段 |
| `gate_result` | `publish_gate.py` 门禁结果：每项 ready / blocked / narrowed 及原因 |
| `execution_ledger` | 执行台账：每份资料的 `planned` / `created` / `verified` / `failed` / `blocked` / `skipped` 状态，用于断点续跑 |
| `unsupported_checks` | 因权限、类型、解析或门禁无法写入的资料及原因 |
| `verification_results` | 每个已写页面的 fresh read 校验结果 |
| `partial` | 结果是否不完整，以及不完整原因（权限、解析、分页或 API 失败） |

## Execution State Machine

| State | Protocol Step | Entry Condition | Agent MUST Do | User-Facing Output | wait_for_user | Next State |
|-------|---------------|-----------------|---------------|--------------------|---------------|------------|
| `PARSE_SOURCES` | `route` / `scope` | Workflow 触发 | 解析本地授权路径为 `source_scope`；解析目标 Wiki 为 `space_id` 和节点范围。按 `Situation Routing` 判定：目标库不存在 → 停下指路 `knowledge_base_bootstrap`（情况 2）；目标不唯一 → 停下请用户选定（情况 7）。确认来源路径与目标库 | 来源路径 + 目标知识库确认；或无库指路 / 多目标选定请求 | `true` | `INVENTORY` |
| `INVENTORY` | `read` | 来源与目标已确认 | 运行 `inventory.py` 盘点本地资料填充 `inventory`（SHA-256 去重、敏感初筛、可解析性判断）。若存在上次运行的旧台账，做 SHA-256 diff，未变资料标记跳过（增量）。仅本地读，零写入、零修改原件 | 资料盘点概览：文件数、重复组、可能敏感数、无法解析数；增量时标出跳过数 | 除非报错否则 `false` | `TARGET_ALIGN` |
| `TARGET_ALIGN` | `read` / `assess` | 盘点完成 | 递归读取目标库节点树填充 `node_inventory`（`wiki +node-list --page-all`，对 `has_child=true` 逐层下钻）；逐节点探测 `knowledge_base_bootstrap` 维护规范填充 `standard_map`，据覆盖情况设 `alignment_mode`（`standard`/`degraded`/`mixed`）。判定节点是否足以承载本批资料 | 目标结构概览 + 规范探测结果（哪些节点有规范）+ 对齐模式；无规范时明确降级提示 | 除非读取被阻断否则 `false` | `NODE_PROPOSE` or `ANALYZE_TRIAGE` |
| `NODE_PROPOSE` | `assess` / `plan` / `confirm` | 节点不足以承载资料（情况 1 / 5） | 加载 analyze phase；据已盘点的真实资料内容提议承载节点填充 `outline_proposal`；请用户确认后用 `wiki +node-create --obj-type docx` 新建，回读并入 `node_inventory` | 承载节点提议表 + 新建确认请求；确认后报告新建结果 | `true` | `ANALYZE_TRIAGE` |
| `ANALYZE_TRIAGE` | `assess` / `plan` | 结构就位（含新建节点） | 加载 analyze phase；逐份读取资料正文，判类、识别版本冲突、映射到目标节点（据 `standard_map`；无规范则降级据内容推断）、按规范或主题生成拟定标题、给 `proposed_action`；对目标节点做 Node Type Triage（非 docx / shortcut 处理）填充 `material_map` | 资料分析表：判类、目标节点、拟定名、冲突/敏感标记、映射置信度 | 除非用户直接进入确认否则 `false` | `PUBLISH_PLAN` |
| `PUBLISH_PLAN` | `plan` / `confirm` | 分析完成 | 加载 publish phase + outputs；生成逐资料 `publish_plan`（含 `publish_role`、`write_via`、目标节点、`parse_status`、6 行治理字段）；运行 `publish_gate.py` 门禁；产出发布计划 + 冲突/敏感/无法解析三清单。仅 `ready=true` 的资料进入 `CONVERT_WRITE`，被拦记入 `unsupported_checks`，`narrowed` 项按收紧后状态写入 | 发布计划表（含目标节点 + write_via + 门禁结果）+ 逐资料精确处置 + 三清单 + 被拦/收紧/跳过原因 | `true` | `CONVERT_WRITE` or `DONE` |
| `CONVERT_WRITE` | `execute` | 用户已确认发布计划 | 加载 publish phase；按计划逐份转换写入知识页：Word/.md/.txt/.html 走 `drive +import --type docx`，PDF/图片解析后 `docs +update`，每页套 6 行治理表；`new_docx` 目标先 `wiki +node-create`。**本状态只写知识页，不上传任何附件**。逐项更新 `execution_ledger` | 转换写入进度报告 | 除非被阻断否则 `false` | `VERIFY` |
| `VERIFY` | `verify` | 写入完成 | 对每个已写页面 fresh read：确认 `obj_type=docx`、`docs +fetch` 可读、6 行治理表存在、正文非空且不依赖附件；任一不满足记 `failed`。**仅对 `verified` 的知识页，且用户开启附件时，才 `drive +upload` 挂其 `source_attachment`（页面验证失败的不上传原件，避免孤儿附件）**。汇总 `unsupported_checks` | 验证表 + 最终汇总 | `false` | `DONE` |
| `DONE` | `done` | 无更多动作 | 停止 | 最终回复：已入库页数、跳过 / 被拦资料及原因、失败项、台账位置、知识库链接 | `false` | End |

## Progressive Load Map

Agent 必须在执行某状态前，读取该状态要求的引用文档。

| State | Required Reference |
|-------|---------------------|
| `PARSE_SOURCES` | 本文件、[`lark-drive-workflow.md`](lark-drive-workflow.md)、[`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)、[`../../lark-wiki/SKILL.md`](../../lark-wiki/SKILL.md) |
| `INVENTORY` | 本文件的 `Command Map`；脚本 `scripts/inventory.py` |
| `TARGET_ALIGN` | [`../../lark-wiki/references/lark-wiki-node-list.md`](../../lark-wiki/references/lark-wiki-node-list.md)、[`../../lark-doc/references/lark-doc-fetch.md`](../../lark-doc/references/lark-doc-fetch.md) |
| `NODE_PROPOSE` | [`lark-drive-workflow-knowledge-ingest-analyze.md`](lark-drive-workflow-knowledge-ingest-analyze.md)、[`../../lark-wiki/references/lark-wiki-node-create.md`](../../lark-wiki/references/lark-wiki-node-create.md) |
| `ANALYZE_TRIAGE` | [`lark-drive-workflow-knowledge-ingest-analyze.md`](lark-drive-workflow-knowledge-ingest-analyze.md) |
| `PUBLISH_PLAN` | [`lark-drive-workflow-knowledge-ingest-publish.md`](lark-drive-workflow-knowledge-ingest-publish.md)、[`lark-drive-workflow-knowledge-ingest-outputs.md`](lark-drive-workflow-knowledge-ingest-outputs.md)；门禁脚本 `scripts/publish_gate.py` |
| `CONVERT_WRITE` | [`lark-drive-workflow-knowledge-ingest-publish.md`](lark-drive-workflow-knowledge-ingest-publish.md)、[`../../lark-drive/references/lark-drive-import.md`](lark-drive-import.md)、[`../../lark-doc/references/lark-doc-update.md`](../../lark-doc/references/lark-doc-update.md)；`import_docx` 迁入时 [`../../lark-wiki/references/lark-wiki-move.md`](../../lark-wiki/references/lark-wiki-move.md) 与异步续跑 [`lark-drive-task-result.md`](lark-drive-task-result.md)；`new_docx` 时 [`../../lark-wiki/references/lark-wiki-node-create.md`](../../lark-wiki/references/lark-wiki-node-create.md) |
| `VERIFY` | 复用 `TARGET_ALIGN` 阶段的读取上下文；用户开启附件时 [`lark-drive-upload.md`](lark-drive-upload.md)（仅对 `verified` 页面） |

## Situation Routing

`PARSE_SOURCES` 与 `TARGET_ALIGN` 按目标库现状分流。九种情况的完整处理：

| 情况 | 判定 | 处理 |
|------|------|------|
| 1. 有库、节点不足以承载资料 | `TARGET_ALIGN` | 进入 `NODE_PROPOSE`：据真实资料提议承载节点，用户确认后新建（写入需确认），回主流程 |
| 2. 目标库不存在 | `PARSE_SOURCES` | 停下，主动衔接式指路 `knowledge_base_bootstrap` 建库；**不**自行建知识空间 |
| 3. 有库、无维护规范 | `TARGET_ALIGN` | `alignment_mode=degraded`，据资料内容 + 节点标题推断映射与命名；提示可先跑 `knowledge_base_bootstrap`；不强制、不代跑 |
| 4. 有库、有规范 | `TARGET_ALIGN` | 正常主路径：按规范收录范围与命名做映射 |
| 5. 有库有规范、但无节点可承载这批资料 | `ANALYZE_TRIAGE` | 退化为情况 1，进入 `NODE_PROPOSE`；或用户选择归入最近节点 |
| 6. 部分节点有规范、部分没有 | `TARGET_ALIGN` 逐节点 | `alignment_mode=mixed`：映射到有规范节点走情况 4，映射到无规范节点走情况 3 降级 |
| 7. 目标不唯一（解析出多个库） | `PARSE_SOURCES` | 停下列候选，请用户选定唯一目标 |
| 8. 目标节点非 docx（sheet/bitable/mindnote/shortcut） | `ANALYZE_TRIAGE`（Node Type Triage） | non_docx_entity 默认跳过并记录，用户要才 `new_docx` 另建 docx 承载；shortcut 一律跳过 |
| 9. 资料全被门禁拦下（全敏感/全冲突/全不可解析） | `PUBLISH_PLAN` | 不静默结束：报告「本批 0 入库」+ 逐份原因 + 补救建议 |

## Node Type Triage

`ANALYZE_TRIAGE` 对每个**映射目标节点**依据 `obj_type` / `node_type` 分诊。禁止假设目标节点都是 docx。

| 分类 | 判定条件 | 处理方式 |
|------|----------|----------|
| `writable_docx` | `obj_type=docx` 且 `node_type=origin` | 主路径：`docs +update` 或 `import_docx` 写入知识页 |
| `non_docx_entity` | `node_type=origin` 且 `obj_type != docx` | 默认 `skip` 并记入 `unsupported_checks`。仅用户在 `PUBLISH_PLAN` 明确要求时走 `new_docx`：在其同级 / 子级新建 docx 节点承载，原对象不改动 |
| `shortcut` | `node_type=shortcut`（任意 `obj_type`） | 一律 `skip` 并记入 `unsupported_checks`；快捷方式无自有正文 |

完备性要求：每个映射目标节点都必须落入三类之一，按 `shortcut` → `writable_docx` → `non_docx_entity` 顺序穷尽划分；未识别的新 `obj_type` 归入 `non_docx_entity`，不得漏过。

## Write Via Selection

对 `writable_docx` 目标，依据 `proposed_action` 与资料类型选择 `write_via`（详见 publish phase）。**先看 `proposed_action`**：`update` / `merge` 面向既有页，一律走 `docs_update`；`add`（新建页）才按资料类型选 import 或 docs_update。

| proposed_action / 资料类型 | write_via | 说明 |
|----------------------------|-----------|------|
| update / merge（既有页，任意来源） | `docs_update` | 定向更新既有页；不用 import_docx（会新增子页面而非更新目标页） |
| add + Word / .doc / .md / .txt / .html | `import_docx` | `drive +import --type docx` 转飞书文档后整理，`wiki +move` 迁入目标节点 |
| add + PDF / 图片 / 需重写的内容 | `docs_update` | 解析 / OCR 后 `docs +update` 重建可检索正文 |
| add + 目标节点缺失、需新建承载页 | `node_create_docx` | 先 `wiki +node-create --obj-type docx` 再写正文 |
| 原始文件（仅用户开启附件时） | `drive_upload` | 只作 `source_attachment`，永不算知识页完成 |

## Publish Gate

`PUBLISH_PLAN` 生成发布计划后，必须先经 `scripts/publish_gate.py` 门禁校验，再进入 `CONVERT_WRITE`。门禁是确定性代码校验，agent 判断只能收紧、不能绕过。

先从当前 `SKILL.md` 位置解析 skill 根目录绝对路径 `<SKILL_ROOT>`，再运行；不要假设当前工作目录，也不要用相对路径直接执行。

```bash
python3 "<SKILL_ROOT>/references/scripts/publish_gate.py" --plan "<发布计划 JSON 路径>"
# 或经 stdin：cat plan.json | python3 "<...>/publish_gate.py" --plan -
```

发布计划 JSON 每项字段与门禁判定规则见 [`lark-drive-workflow-knowledge-ingest-publish.md`](lark-drive-workflow-knowledge-ingest-publish.md) 与 [`lark-drive-workflow-knowledge-ingest-outputs.md`](lark-drive-workflow-knowledge-ingest-outputs.md)。门禁判定分两级：

- **硬拦（`ready=false`，不得写入）**：知识页载体非 docx、`write_via=drive_upload` 冒充知识页、缺 6 行治理表、必填字段（来源、适用可见范围）空、页面状态非法、敏感项（prohibited 或 restricted 未审）进生产、未裁决冲突（suspected/confirmed）进生产、资料不可解析（unsupported/failed）、缺目标 token、未知 write_via / publish_role、附件缺显式确认。
- **一致性收紧（`narrowed=true`，可写但降级）**：治理字段含「待确认」却标「已完成」，或资料仅部分解析（partial）却标「已完成」→ 强制收紧为「进行中」。

`ready=false` 的资料记入 `unsupported_checks` 并在计划中标出原因，不进入 `CONVERT_WRITE`；`narrowed=true` 按收紧后状态写入。

## Command Map

只能使用当前状态允许的 command family。命令详细语法属于被引用 skill / reference。

| State | Allowed Command Families | Purpose |
|-------|--------------------------|---------|
| `PARSE_SOURCES` | `wiki +node-get`、`wiki +space-list`、`wiki +node-list --page-all`、`drive +search`（定位/消歧目标库） | 解析本地路径与目标库，判定 `Situation Routing` |
| `INVENTORY` | `python3 <SKILL_ROOT>/references/scripts/inventory.py`（本地只读盘点） | 盘点本地资料、去重、敏感初筛 |
| `TARGET_ALIGN` | `wiki +node-list --page-all`（逐层下钻）、`docs +fetch` | 读节点树与各节点维护规范 |
| `NODE_PROPOSE` | `wiki +node-create --obj-type docx`（仅用户确认后）、`wiki +node-list --page-all` | 新建确认后的承载节点并分页回读（`--page-all`，避免漏掉新建节点） |
| `ANALYZE_TRIAGE` | 无飞书写命令（agent 读本地资料正文分析） | 判类、冲突识别、映射、命名、分诊 |
| `PUBLISH_PLAN` | 无飞书写命令；`python3 <SKILL_ROOT>/references/scripts/publish_gate.py`（本地只读门禁） | 生成发布计划、门禁校验、请用户确认 |
| `CONVERT_WRITE` | `docs +fetch`（update/merge 落笔前重读 revision）、`drive +import --type docx`、`drive +task_result --scenario import`（import 异步续跑）、`docs +update`、`wiki +move --obj-type docx`（import 件迁入目标节点）、`drive +task_result --scenario wiki_move`（迁入异步续跑）、`wiki +node-create --obj-type docx`（new_docx） | 执行已确认的受控转换写入（不含附件上传） |
| `VERIFY` | `docs +fetch`、`wiki +node-list`；`drive +upload`（仅对 `verified` 页面上传 source_attachment） | fresh read 校验写入结果，并对已验证页面按需挂原件附件 |

## Transition Rules

1. `PARSE_SOURCES` 无法解析出本地来源路径或唯一目标库时，只问澄清问题并停止。
2. `PARSE_SOURCES` 检测目标库不存在（情况 2）时，停下指路 `knowledge_base_bootstrap`，不自行建知识空间；目标不唯一（情况 7）时列候选请用户选定。
3. 认证或 API scope 缺失时，按 `lark-shared` 权限处理并停止。
4. 权限按动作分别判断，一个动作受阻不连累其余：读权限缺失 → 停止（无法盘点结构）；`docs +update` 可用而 `wiki +node-create` 不可用 → 照常写可编辑 docx 节点，`NODE_PROPOSE` / `new_docx` 需新建的列入「待创建节点」并记 `unsupported_checks`；仅可读 → 只输出发布计划不写入；某动作实际返回 `permission_denied` → 只停该动作、记入 `unsupported_checks`，不同参重试、不静默切 bot、不自动申请权限。
5. 权限硬规则：读取成功不等于具备写权限；写权限只以实际写入返回为准。
6. `TARGET_ALIGN` 判定节点足以承载资料时直接进入 `ANALYZE_TRIAGE`；不足时进入 `NODE_PROPOSE`。用户拒绝新建节点时，只把资料映射到现有节点或列为待人工归位，不擅自新建。节点树读取不全（`partial`）时 fail closed：不基于残缺清单做映射或新建节点，停下报告并先补齐读取。
7. 无维护规范时（情况 3/6）降级继续并提示，不强制路由到 `knowledge_base_bootstrap`。
8. `ANALYZE_TRIAGE` 发现版本冲突且无法自动裁决时，该资料标 `conflict_status=suspected/confirmed`，不覆盖现有生产页，列入「需业务确认」；相关资料由门禁阻塞发布。
9. `PUBLISH_PLAN` 门禁拦截的资料不进入写入；用户拒绝发布时输出已保存的发布计划并转入 `DONE`。全部资料被拦时（情况 9）报告 0 入库及逐项原因，不静默结束。
10. `CONVERT_WRITE` 中单份资料失败或被拦，记录并继续其余相互独立的资料；`drive +upload` 成功只记为附件结果，绝不推进页面状态。原始文件全程只读。
11. 身份在 `PARSE_SOURCES` 确定后保持不变；盘点、读取、写入、验证使用同一身份。

## References

- [lark-drive workflow 总框架](lark-drive-workflow.md)
- [Analyze：盘点、对齐、分诊与映射](lark-drive-workflow-knowledge-ingest-analyze.md)
- [Publish：发布计划、转换写入与验证](lark-drive-workflow-knowledge-ingest-publish.md)
- [Outputs：发布计划模板与输出模板](lark-drive-workflow-knowledge-ingest-outputs.md)
- 脚本：`scripts/inventory.py`（及测试 `scripts/inventory_test.py`）、`scripts/publish_gate.py`（及测试 `scripts/publish_gate_test.py`）
- [lark-shared](../../lark-shared/SKILL.md)
- [lark-wiki](../../lark-wiki/SKILL.md)、[lark-wiki-node-list](../../lark-wiki/references/lark-wiki-node-list.md)、[lark-wiki-node-create](../../lark-wiki/references/lark-wiki-node-create.md)
- [lark-doc](../../lark-doc/SKILL.md)、[lark-doc-fetch](../../lark-doc/references/lark-doc-fetch.md)、[lark-doc-update](../../lark-doc/references/lark-doc-update.md)
- [lark-drive-import](lark-drive-import.md)、[lark-drive-upload](lark-drive-upload.md)
- [knowledge_base_bootstrap](lark-drive-workflow-knowledge-base-bootstrap.md)
- [knowledge_organize](lark-drive-workflow-knowledge-organize.md)
- [topic_move_collector](lark-drive-workflow-topic-move-collector.md)
