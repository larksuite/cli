# 知识库维护标准初始化 Workflow

Workflow id: `knowledge_base_bootstrap`

Risk / Structure: `R2` / `S2`

本文实现已注册的知识库维护标准初始化 workflow。执行前必须先读取 [`lark-drive-workflow.md`](lark-drive-workflow.md) 和 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)，并遵循共享执行协议、Artifact Contract、Workflow Loading、认证和写入确认规则。

本文定义 workflow 专属的状态机、类型分诊规则、写法选择规则和 command family 允许范围。规范文本模板和用户可见输出模板放在配套 outputs 文档，仅在进入需要它的状态时按需加载。

配套 outputs 文档只是本 workflow 的引用文件，不是独立 skill。不要把用户请求直接路由到 outputs 文档。

## 必读上下文

执行本 workflow 前，必须先读取 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)，用于处理身份、认证、权限和写操作确认规则。

按阶段渐进加载其他 skill / 引用文档：

- 读取知识库结构与节点解析：[`../../lark-wiki/SKILL.md`](../../lark-wiki/SKILL.md) 和 [`../../lark-wiki/references/lark-wiki-node-list.md`](../../lark-wiki/references/lark-wiki-node-list.md)
- 读取节点文档现有内容：[`../../lark-doc/SKILL.md`](../../lark-doc/SKILL.md) 和 [`../../lark-doc/references/lark-doc-fetch.md`](../../lark-doc/references/lark-doc-fetch.md)
- 写入节点文档正文：[`../../lark-doc/references/lark-doc-update.md`](../../lark-doc/references/lark-doc-update.md)；按 `--doc-format` 读取 [`../../lark-doc/references/lark-doc-md.md`](../../lark-doc/references/lark-doc-md.md) 或 [`../../lark-doc/references/lark-doc-xml.md`](../../lark-doc/references/lark-doc-xml.md)
- 为非 docx 节点补建规范页：[`../../lark-wiki/references/lark-wiki-node-create.md`](../../lark-wiki/references/lark-wiki-node-create.md)
- 规范文本模板和输出模板：[`lark-drive-workflow-knowledge-base-bootstrap-outputs.md`](lark-drive-workflow-knowledge-base-bootstrap-outputs.md)

## 适用范围

本 workflow 用于对一个已存在的飞书 Wiki 知识库，基于其现有节点结构和草稿内容，生成并写入标准维护要求：通用维护规范写入根节点，节点专属维护要求写入各子节点，供后续维护者按标准补充资料。

适用触发语包括：

- "给这个知识库 / 各节点写清楚维护要求 / 维护标准"
- "建立标准知识库维护方法，后续同事按标准往里补资料"
- "帮我把维护规范更新到知识库的各个节点里"
- "为知识库各节点补上收录范围、命名规范和维护职责"

目标必须是一个已存在的 Wiki 知识库链接、知识空间或知识库节点。当目标结构过简（例如只有根节点、缺少承载分类的子节点）时，本 workflow 先基于知识库主题提议一套子节点大纲，经用户确认后新建节点，再进入维护标准撰写；不在未确认前擅自新建节点树。

## 非目标

本 workflow 不处理：

- 移动、复制、删除或重命名节点；这类结构调整应使用 [`knowledge_organize`](lark-drive-workflow-knowledge-organize.md) 或 [`topic_move_collector`](lark-drive-workflow-topic-move-collector.md)。本 workflow 仅在 `OUTLINE_PROPOSE` 经用户确认后新建大纲子节点，不改动已有节点的位置、归属或标题。
- 把外部文件归档 / 上传到节点下；单文件智能归档是独立需求，不属于本 workflow。
- 知识库内容问答或检索。
- 知识空间成员、权限或密级治理；这类需求使用 [`permission_governance`](lark-drive-workflow-permission-governance.md)。
- 覆盖用户已有的实质草稿内容；除非用户在写前确认阶段明确点名要求重写该节点。
- 改写 sheet / bitable / mindnote / slides 等非 docx 节点的内部数据结构。

## Agent 执行约束

触发本 workflow 后，agent 必须：

1. 按 `Execution State Machine` 的顺序执行，并维护 `Runtime State` 字段。
2. 执行某个状态前，先加载 `Progressive Load Map` 中该状态要求的引用文档；不要预加载全部文档。
3. 在 `WRITE_CONFIRM` 之前，绝不执行文档正文写入（`docs +update`）。唯一例外是 `OUTLINE_PROPOSE`：仅在用户单独确认大纲后，才可执行 `wiki +node-create` 新建大纲节点；除此之外任何状态都不得新建、修改节点。
4. 只执行 `Command Map` 允许的 command family；命令语法、scope 要求、参数规则以被引用 skill / reference 为准。
5. 用户可见说明、字段说明和表格文案使用中文；状态名、字段名、枚举值、命令名保留英文稳定标识。
6. 内部枚举值在用户可见输出中转为自然语言中文标签。
7. 不声明 CLI / API 不支持的能力；无法执行的写入必须记入 `unsupported_checks`，不得静默省略。
8. 写入后必须用 fresh read 验证内容落地，不以写入命令的返回值直接判定成功。

## Runtime State

本 workflow 在共享 `Artifact Contract` 基础上维护以下字段：

| Field | Meaning |
|-------|---------|
| `current_state` | `Execution State Machine` 中的当前状态 |
| `target_space` | 解析出的 `space_id`、`root_node`（承载通用规范的唯一根节点：token + obj_type）、`top_level_nodes`（空间顶层节点列表）和用户原始输入 URL。通用规范只写入单一 `root_node`；解析规则见 `Root Node Resolution` |
| `identity` | 执行身份，默认 `user`；节点解析与写入必须使用同一身份 |
| `node_inventory` | 全部节点的归一化列表：`node_token`、`obj_token`、`title`、`obj_type`、`node_type`、父子层级 |
| `node_class` | 每个节点的分诊结果：`writable_docx` / `non_docx_entity` / `shortcut` |
| `permissions_observed` | 各写动作实际观测到的权限：`read` / `edit_existing_docx`（`docs +update`）/ `create_node`（`wiki +node-create`），取值 `true` / `false` / `unknown`；只记录已验证或实际报错得到的结果，不由身份或读取成功推断写权限 |
| `outline_proposal` | 结构过简时提议的子节点大纲：拟建节点标题、层级、收录范围摘要；经用户确认后用于新建节点 |
| `draft_map` | 每个 docx 节点的现有内容判定：`empty_placeholder` / `has_draft` 及内容摘要 |
| `standard_plan` | 生成的维护规范：根节点通用规范 + 各子节点专属维护要求 |
| `write_mode_map` | 每个节点的写法决策：`overwrite` / `append` / `new_docx` / `skip` |
| `unsupported_checks` | 因节点类型或权限无法写入的节点及原因 |
| `write_results` | 每个节点的写入结果 |
| `verification_results` | 每个已写节点的 fresh read 校验结果 |
| `partial` | 结果是否不完整，以及不完整原因（权限、类型、API / 分页失败） |

## Execution State Machine

| State | Protocol Step | Entry Condition | Agent MUST Do | User-Facing Output | wait_for_user | Next State |
|-------|---------------|-----------------|---------------|--------------------|---------------|------------|
| `PARSE_TARGET` | `route` / `scope` | Workflow 触发 | 加载 wiki skill；把目标解析为 `space_id`：给定 wiki 节点 / 文档 URL 时用 `wiki +node-get` 取 `space_id` 且该节点即候选根，给定普通知识空间时用 `wiki +space-list` 取 `space_id` 并用 `wiki +node-list --page-all`（省略 parent）列出顶层节点；**目标是个人知识库（`my_library` 或个人库 URL）时，`wiki +space-list` 不返回个人库，须改用 `wiki spaces get --params '{"space_id":"my_library"}'` 解析出真实 `space_id`，再列顶层节点**；按 `Root Node Resolution` 确定唯一 `root_node`，多个顶层节点无法自动定根时停下请用户选定；确认目标就是该知识库 | 目标知识库与根节点确认，或（多顶层时）请用户选定根节点 | `true` | `READ_STRUCTURE` |
| `READ_STRUCTURE` | `read` | 目标已确认 | 递归读取整棵节点树填充 `node_inventory`：每次 `wiki +node-list` 必须用 `--page-all`（或按 `page_token` 翻页到 `has_more=false`），并对 `has_child=true` 的节点逐层下钻，不得只取首页；对 docx 节点读取现有内容填充 `draft_map`；判定结构是否过简（无子节点或子节点不足以承载分类）。注意 `--page-all` 仍有默认翻页上限（`--page-limit` 默认 10 页、每页至多 50 节点）：某层超过该上限或子节点读取失败时，置 `partial` 并记原因。**`partial` 时 fail closed：不进入 `OUTLINE_PROPOSE` / `TYPE_TRIAGE` / `WRITE`**，因为截断的树可能被误判为"结构过简"而重复建大纲、或写规范时漏掉未读到的节点；须先读全（提高 `--page-limit` / 续 `page_token` / 缩小目标）或由用户缩小范围后再继续 | 结构概览：节点数、层级、草稿 / 占位分布；不完整时明确标注并停下 | 读取不全时为 `true`（停下待用户），否则为 `false` | `OUTLINE_PROPOSE` or `TYPE_TRIAGE` |
| `OUTLINE_PROPOSE` | `assess` / `plan` / `confirm` | 结构过简 | 加载 outputs 文档；基于知识库主题、根节点标题和已有草稿提议子节点大纲填充 `outline_proposal`；请用户确认后用 `wiki +node-create --obj-type docx` 新建拟定子节点，并回读并入 `node_inventory` | 大纲提议表 + 新建确认请求；确认后报告新建结果 | `true` | `TYPE_TRIAGE` |
| `TYPE_TRIAGE` | `assess` / `plan` | 结构已读（含新建节点） | 按 `obj_type` / `node_type` 将每个节点分诊为 `writable_docx` / `non_docx_entity` / `shortcut`，填充 `node_class` | 分诊表：可写正文节点、需特殊处理节点及原因 | 存在 `non_docx_entity` / `shortcut` 时为 `true`，否则为 `false` | `GEN_STANDARD` |
| `GEN_STANDARD` | `assess` / `plan` | 分诊完成 | 加载 outputs 文档；为可写范围生成 `standard_plan`：根节点通用规范 + 各子节点专属维护要求；子节点收录范围优先从草稿归纳，缺失再据业务常识补全 | 维护规范草案预览 | 除非用户直接进入确认，否则为 `false` | `WRITE_CONFIRM` |
| `WRITE_CONFIRM` | `confirm` | 规范草案就绪 | 生成逐节点写入计划（含 `node_token`、`obj_type`、`write_mode`、6 行治理字段）；运行 `kb_gate.py` 门禁校验，仅 `ready=true` 的节点可进入 `WRITE`，被拦节点记入 `unsupported_checks`，门禁将 `narrowed` 节点的页面状态收紧为进行中；向用户展示计划、门禁结果、精确正文 / diff 与跳过原因 | 写入计划表（含 node_token + 命令族 + 门禁结果）+ 逐节点精确内容 / diff + 被拦 / 收紧 / 跳过清单及原因 | `true` | `WRITE` or `DONE` |
| `WRITE` | `execute` | 用户已确认写入范围 | 加载 doc-update reference；按 `write_mode_map` 逐节点写入（根节点与子节点可并行）；**每个 `overwrite` 节点落笔前先 `docs +fetch` 重读并按 `Write Mode Selection` 的基线校验（占位覆盖须仍为 `empty_placeholder`；已确认草稿覆盖须与确认时基线一致），携带读到的 `revision` 再写，与基线不符则停下重新确认**；非 docx 节点仅在用户选择 `new_docx` 时新建 docx 规范页 | 写入进度报告 | 除非被阻断，否则为 `false` | `VERIFY` |
| `VERIFY` | `verify` | 写入完成 | 对每个已写节点执行 fresh read 校验内容落地；汇总 `unsupported_checks` | 验证表和最终汇总 | `false` | `DONE` |
| `DONE` | `done` | 无更多动作 | 停止 | 最终回复：已更新节点数、跳过节点及原因、知识库链接 | `false` | End |

## Progressive Load Map

Agent 必须在执行某状态前，读取该状态要求的引用文档。

| State | Required Reference |
|-------|---------------------|
| `PARSE_TARGET` | 本文件、[`lark-drive-workflow.md`](lark-drive-workflow.md)、[`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)、[`../../lark-wiki/SKILL.md`](../../lark-wiki/SKILL.md) |
| `READ_STRUCTURE` | [`../../lark-wiki/references/lark-wiki-node-list.md`](../../lark-wiki/references/lark-wiki-node-list.md)、[`../../lark-doc/references/lark-doc-fetch.md`](../../lark-doc/references/lark-doc-fetch.md) |
| `OUTLINE_PROPOSE` | [`lark-drive-workflow-knowledge-base-bootstrap-outputs.md`](lark-drive-workflow-knowledge-base-bootstrap-outputs.md)、[`../../lark-wiki/references/lark-wiki-node-create.md`](../../lark-wiki/references/lark-wiki-node-create.md) |
| `TYPE_TRIAGE` | 本文件的 `Node Type Triage` |
| `GEN_STANDARD` | [`lark-drive-workflow-knowledge-base-bootstrap-outputs.md`](lark-drive-workflow-knowledge-base-bootstrap-outputs.md) |
| `WRITE_CONFIRM` | [`lark-drive-workflow-knowledge-base-bootstrap-outputs.md`](lark-drive-workflow-knowledge-base-bootstrap-outputs.md)；门禁脚本 `scripts/kb_gate.py` |
| `WRITE` | [`../../lark-doc/references/lark-doc-update.md`](../../lark-doc/references/lark-doc-update.md)、[`../../lark-doc/references/lark-doc-fetch.md`](../../lark-doc/references/lark-doc-fetch.md)（overwrite 前重读）；`new_docx` 时读取 [`../../lark-wiki/references/lark-wiki-node-create.md`](../../lark-wiki/references/lark-wiki-node-create.md) |
| `VERIFY` | 复用 `READ_STRUCTURE` 阶段的读取上下文 |

## Root Node Resolution

通用维护规范只写入**唯一**根节点 `root_node`，不得写入多个顶层节点。按以下顺序确定：

1. 目标是 wiki 节点 / 文档 URL（`wiki +node-get` 可解析出该节点）→ 该节点即 `root_node`，其子树为处理范围。
2. 目标是知识空间、且顶层只有 1 个节点 → 该顶层节点即 `root_node`。
3. 目标是知识空间、顶层有多个节点 → 不自动选根，停在 `PARSE_TARGET`，列出候选顶层节点（标题 + token）请用户选定一个作为 `root_node`；用户也可指定“为每个顶层节点分别建规范”，但仍需逐个确认，不默认批量。
4. 选定的 `root_node` 若不是 docx（`obj_type != docx`）→ 不向其 `docs +update` 写通用规范；按 `Node Type Triage` 处理：经用户确认走 `new_docx` 在其下新建 docx 规范页，或跳过并记入 `unsupported_checks`。

`root_node` 确定前不生成写入计划，也不进入 `WRITE`。

## Node Type Triage

`TYPE_TRIAGE` 依据 `wiki +node-list` 返回的 `obj_type` 和 `node_type` 对每个节点分诊。禁止假设所有节点都是 docx。

| 分类 | 判定条件 | 处理方式 |
|------|----------|----------|
| `writable_docx` | `obj_type=docx` 且 `node_type=origin` | 主路径：用 `docs +update` 写入维护规范正文 |
| `non_docx_entity` | `node_type=origin` 且 `obj_type != docx`（含 `sheet`、`bitable`、`mindnote`、`slides`、`file` 及任何其他非 docx 类型） | 默认 `skip` 并记入 `unsupported_checks`。仅当用户在 `WRITE_CONFIRM` 明确要求时，走 `new_docx`：在该节点同级或子级新建 docx 节点承载其维护规范，原对象不改动 |
| `shortcut` | `node_type=shortcut`（任意 `obj_type`） | 一律 `skip` 并记入 `unsupported_checks`；快捷方式无自有正文，写入无意义 |

默认策略：`non_docx_entity` 默认 `skip` + 报告；`new_docx` 是用户显式选择后的可选动作。任何 `skip` 都必须在最终汇总列出节点类型和原因。

完备性要求：`node_inventory` 中每个节点都必须落入以上三类之一。三类判定按 `shortcut`（`node_type=shortcut`）→ `writable_docx`（`node_type=origin` 且 `obj_type=docx`）→ `non_docx_entity`（其余 `node_type=origin`）的顺序穷尽划分；未识别的新 `obj_type` 归入 `non_docx_entity`，不得让任何节点无分类地漏过。

## Write Mode Selection

对 `writable_docx` 节点，依据 `draft_map` 决定写法，避免误删用户已有内容。

| docx 节点现状 | 写法 | 理由 |
|---------------|------|------|
| 飞书默认模板占位 / 无实质内容（`empty_placeholder`） | `overwrite` | 占位内容可整篇重写 |
| 已有用户实质草稿（`has_draft`） | `append`（默认） | 追加规范，保留用户原文 |
| 用户在 `WRITE_CONFIRM` 明确点名要求重写的有草稿节点 | `overwrite` | 需用户对该节点显式确认 |

`draft_state` 必须来自对目标节点的**新鲜读取**（`docs +fetch`），不得复用 `READ_STRUCTURE` 早期缓存或过期计划里的 `draft_state`；门禁只能校验传入 JSON、无法验证其新鲜度。

**关键：新鲜读取的时机在用户确认之后、每次 `overwrite` 落笔之前**，而不仅是 `WRITE_CONFIRM` 生成计划时。因为 `WRITE_CONFIRM` 会停下等用户确认，等待期间协作者可能改动目标节点；若沿用确认前的判定、且 `docs +update` 用默认 `revision-id=-1` 写最新版，`overwrite` 会覆盖确认后新增的内容。落笔前的校验按写法基线区分：

- **占位覆盖（确认时 `draft_state=empty_placeholder`）**：fresh read 必须仍为 `empty_placeholder`（内容未变）才写；读回已变成 `has_draft` 说明等待期间被补入草稿，停下重新确认或改 `append`。
- **已确认的草稿覆盖（确认时 `draft_state=has_draft` 且 `overwrite_confirmed=true`）**：记录确认时展示给用户的 `revision` / 内容基线，fresh read 必须与该基线**一致**（草稿未再变动）才写；一致即按确认执行，不因读回仍是 `has_draft` 而反复追问；若与基线不一致（草稿在确认后又被改），停下重新确认。

两种情况都携带读到的 `revision` 写入；有疑问时优先 `append`，不静默覆盖。

## Write Gate

`WRITE_CONFIRM` 生成写入计划后，必须先经 `scripts/kb_gate.py` 门禁校验，再进入 `WRITE`。门禁是确定性代码校验，agent 的判断只能收紧结果、不能绕过门禁。

先从当前 `SKILL.md` 位置解析 skill 根目录绝对路径 `<SKILL_ROOT>`，再运行；不要假设当前工作目录，也不要用相对路径直接执行。

```bash
python3 "<SKILL_ROOT>/references/scripts/kb_gate.py" --plan "<写入计划 JSON 路径>"
# 或经 stdin：cat plan.json | python3 "<...>/kb_gate.py" --plan -
```

写入计划 JSON 每个节点提供：`node_token`、`title`、`obj_type`、`write_mode`（`overwrite`/`append`/`new_docx`/`skip`）、`draft_state`（`empty_placeholder`/`has_draft`）、`overwrite_confirmed`，`new_docx` 另需 `parent_node_token` 或 `space_id`（确认的建节点位置），以及 6 行治理字段 `governance`（`source`、`owner`、`version_status`、`scope_visibility`、`effective_update`、`review_policy`、`page_status`）。

门禁判定分两级：

- **硬拦（`ready=false`，不得写入）**：载体不是 docx 且写法不是 `new_docx`、缺少 6 行治理表、必填字段（来源、适用与可见范围）为空、页面状态非法、覆盖有草稿节点但缺 `overwrite_confirmed`、覆盖写入但 `draft_state` 缺失或未知（fail-closed，避免误清空草稿）、`new_docx` 缺确认的建节点位置（`parent_node_token` / `space_id` 都为空，避免 user 身份回退个人库 my_library）、未知写法。
- **一致性收紧（`narrowed=true`，可写但降级）**：治理字段含“待确认”却把 `page_status` 标为“已完成”时，强制收紧为“进行中”，以保留“先建框架、后续补全”的场景，同时不让残缺内容冒充已完成。

`ready=false` 的节点记入 `unsupported_checks` 并在写入计划中标出原因，不进入 `WRITE`；`narrowed=true` 的节点按收紧后的状态写入。

## Command Map

只能使用当前状态允许的 command family。命令详细语法属于被引用 skill / reference。

| State | Allowed Command Families | Purpose |
|-------|--------------------------|---------|
| `PARSE_TARGET` | `wiki +node-get`、`wiki +space-list`、`wiki spaces get`（解析个人库 my_library）、`wiki +node-list --page-all` | 把 URL / 空间解析为 `space_id` 并确认根层节点 |
| `READ_STRUCTURE` | `wiki +node-list --page-all`（对 `has_child=true` 逐层下钻）、`docs +fetch` | 完整递归读取节点树和节点草稿内容 |
| `OUTLINE_PROPOSE` | `wiki +node-create --obj-type docx`（仅用户确认大纲后）、`wiki +node-list --page-all` | 新建确认后的大纲子节点并分页回读（`--page-all`，避免漏掉新建节点） |
| `TYPE_TRIAGE` | 无写命令 | 仅对已读结构做分类 |
| `GEN_STANDARD` | 无写命令 | 模型生成维护规范 |
| `WRITE_CONFIRM` | 无飞书写命令；`python3 <SKILL_ROOT>/references/scripts/kb_gate.py`（本地只读门禁校验） | 生成写入计划、门禁校验并请用户确认 |
| `WRITE` | `docs +fetch`（overwrite 前重读 draft_state 与 revision）、`docs +update`（`overwrite` / `append` / `block_*`）；仅 `new_docx` 时 `wiki +node-create --obj-type docx` | 执行已确认的受控写入 |
| `VERIFY` | `docs +fetch`、`wiki +node-list` | fresh read 校验写入结果 |

## Transition Rules

1. `PARSE_TARGET` 无法解析出唯一知识库时，只问目标澄清问题并停止。
2. `PARSE_TARGET` 按 `Root Node Resolution` 确定唯一 `root_node`；知识空间存在多个顶层节点且无法自动定根时，停下列出候选请用户选定，不擅自选一个或默认全建。
3. 认证或 API scope 缺失时，按 `lark-shared` 权限处理并停止。
4. 权限按动作分别判断，填充 `permissions_observed`，一个动作受阻不连累其余动作：
   - 读权限缺失 → 停止，无法盘点结构（前提不满足）；
   - `docs +update` 可用、`wiki +node-create` 不可用 → 照常为可编辑的 docx 节点写规范；`OUTLINE_PROPOSE` 或 `new_docx` 需新建的节点列入“待创建节点（缺创建权限）”并记入 `unsupported_checks`，不阻塞其余写入；
   - 两者都可用 → 正常执行建节点与写规范；
   - 仅可读 → 只输出规范草案与写入计划，不执行任何写入；
   - 某动作实际返回 `permission_denied` → 只停该动作、记入 `unsupported_checks`，不用同参数重试、不静默切 bot、不自动申请权限。
5. 权限硬规则：读取成功不等于具备写权限；写权限只以实际写入返回为准，不由身份、角色或读取成功推断。遇权限不足不自动申请，但必须精确告知受阻的动作和具体节点，不静默跳过。
6. `READ_STRUCTURE` 判定结构过简（无子节点或子节点不足以承载分类）时进入 `OUTLINE_PROPOSE`；结构已足够时直接进入 `TYPE_TRIAGE`。节点树读取不全（`partial`，含 `--page-all` 触及默认页数上限）时 fail closed：不进入 `OUTLINE_PROPOSE` / `TYPE_TRIAGE` / `WRITE`，先读全或由用户缩小范围，避免把截断的树误判为过简而重复建大纲或漏写节点。
7. `OUTLINE_PROPOSE` 中用户拒绝新建大纲、或只想为现有根节点写规范时，跳过新建，直接以现有节点进入 `TYPE_TRIAGE`；不擅自新建任何节点。
8. `TYPE_TRIAGE` 发现全部节点均为 `non_docx_entity` / `shortcut` 时，说明本知识库没有可写入正文的 docx 节点，询问是否对相关节点走 `new_docx`，不静默结束。
9. 用户在 `WRITE_CONFIRM` 拒绝写入时，输出已保存的规范草案并转入 `DONE`。
10. `WRITE` 中单个节点写入失败时，记录失败并继续其余相互独立的节点；写入结束后在 `VERIFY` 统一报告失败项。
11. 身份在 `PARSE_TARGET` 确定后保持不变；节点解析、读取、写入、验证使用同一身份。

## References

- [lark-drive workflow 总框架](lark-drive-workflow.md)
- [Outputs：规范模板与输出模板](lark-drive-workflow-knowledge-base-bootstrap-outputs.md)
- 门禁脚本：`scripts/kb_gate.py`（及测试 `scripts/kb_gate_test.py`）
- [lark-shared](../../lark-shared/SKILL.md)
- [lark-wiki](../../lark-wiki/SKILL.md)
- [lark-wiki-node-list](../../lark-wiki/references/lark-wiki-node-list.md)
- [lark-wiki-node-create](../../lark-wiki/references/lark-wiki-node-create.md)
- [lark-doc](../../lark-doc/SKILL.md)
- [lark-doc-fetch](../../lark-doc/references/lark-doc-fetch.md)
- [lark-doc-update](../../lark-doc/references/lark-doc-update.md)
- [knowledge_organize](lark-drive-workflow-knowledge-organize.md)
- [topic_move_collector](lark-drive-workflow-topic-move-collector.md)
