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
3. 在 `WRITE_CONFIRM` 之前，绝不执行任何节点写入。
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
| `target_space` | 解析出的 `space_id`、根节点 token 和用户原始输入 URL |
| `identity` | 执行身份，默认 `user`；节点解析与写入必须使用同一身份 |
| `node_inventory` | 全部节点的归一化列表：`node_token`、`obj_token`、`title`、`obj_type`、`node_type`、父子层级 |
| `node_class` | 每个节点的分诊结果：`writable_docx` / `non_docx_entity` / `shortcut` |
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
| `PARSE_TARGET` | `route` / `scope` | Workflow 触发 | 加载 wiki skill；解析知识库链接为 `space_id` + 根节点；确认目标就是该知识库 | 目标知识库确认或澄清问题 | `true` | `READ_STRUCTURE` |
| `READ_STRUCTURE` | `read` | 目标已确认 | 递归读取节点树填充 `node_inventory`；对 docx 节点读取现有内容填充 `draft_map`；判定结构是否过简（无子节点或子节点不足以承载分类） | 结构概览：节点数、层级、草稿 / 占位分布 | 除非读取被阻断，否则为 `false` | `OUTLINE_PROPOSE` or `TYPE_TRIAGE` |
| `OUTLINE_PROPOSE` | `assess` / `plan` / `confirm` | 结构过简 | 加载 outputs 文档；基于知识库主题、根节点标题和已有草稿提议子节点大纲填充 `outline_proposal`；请用户确认后用 `wiki +node-create --obj-type docx` 新建拟定子节点，并回读并入 `node_inventory` | 大纲提议表 + 新建确认请求；确认后报告新建结果 | `true` | `TYPE_TRIAGE` |
| `TYPE_TRIAGE` | `assess` / `plan` | 结构已读（含新建节点） | 按 `obj_type` / `node_type` 将每个节点分诊为 `writable_docx` / `non_docx_entity` / `shortcut`，填充 `node_class` | 分诊表：可写正文节点、需特殊处理节点及原因 | 存在 `non_docx_entity` / `shortcut` 时为 `true`，否则为 `false` | `GEN_STANDARD` |
| `GEN_STANDARD` | `assess` / `plan` | 分诊完成 | 加载 outputs 文档；为可写范围生成 `standard_plan`：根节点通用规范 + 各子节点专属维护要求；子节点收录范围优先从草稿归纳，缺失再据业务常识补全 | 维护规范草案预览 | 除非用户直接进入确认，否则为 `false` | `WRITE_CONFIRM` |
| `WRITE_CONFIRM` | `confirm` | 规范草案就绪 | 生成逐节点写入计划：写哪些节点、写入内容、`overwrite` / `append` / `new_docx` / `skip`，并说明非 docx / shortcut 的处置 | 写入计划表 + 覆盖 / 追加说明 + 跳过清单及原因 | `true` | `WRITE` or `DONE` |
| `WRITE` | `execute` | 用户已确认写入范围 | 加载 doc-update reference；按 `write_mode_map` 逐节点写入（根节点与子节点可并行）；非 docx 节点仅在用户选择 `new_docx` 时新建 docx 规范页 | 写入进度报告 | 除非被阻断，否则为 `false` | `VERIFY` |
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
| `WRITE_CONFIRM` | [`lark-drive-workflow-knowledge-base-bootstrap-outputs.md`](lark-drive-workflow-knowledge-base-bootstrap-outputs.md) |
| `WRITE` | [`../../lark-doc/references/lark-doc-update.md`](../../lark-doc/references/lark-doc-update.md)；`new_docx` 时读取 [`../../lark-wiki/references/lark-wiki-node-create.md`](../../lark-wiki/references/lark-wiki-node-create.md) |
| `VERIFY` | 复用 `READ_STRUCTURE` 阶段的读取上下文 |

## Node Type Triage

`TYPE_TRIAGE` 依据 `wiki +node-list` 返回的 `obj_type` 和 `node_type` 对每个节点分诊。禁止假设所有节点都是 docx。

| 分类 | 判定条件 | 处理方式 |
|------|----------|----------|
| `writable_docx` | `obj_type=docx` 且 `node_type=origin` | 主路径：用 `docs +update` 写入维护规范正文 |
| `non_docx_entity` | `obj_type ∈ {sheet, bitable, mindnote, slides}` 且 `node_type=origin` | 默认 `skip` 并记入 `unsupported_checks`。仅当用户在 `WRITE_CONFIRM` 明确要求时，走 `new_docx`：在该节点同级或子级新建 docx 节点承载其维护规范，原对象不改动 |
| `shortcut` | `node_type=shortcut`（任意 `obj_type`） | 一律 `skip` 并记入 `unsupported_checks`；快捷方式无自有正文，写入无意义 |

默认策略：`non_docx_entity` 默认 `skip` + 报告；`new_docx` 是用户显式选择后的可选动作。任何 `skip` 都必须在最终汇总列出节点类型和原因。

## Write Mode Selection

对 `writable_docx` 节点，依据 `draft_map` 决定写法，避免误删用户已有内容。

| docx 节点现状 | 写法 | 理由 |
|---------------|------|------|
| 飞书默认模板占位 / 无实质内容（`empty_placeholder`） | `overwrite` | 占位内容可整篇重写 |
| 已有用户实质草稿（`has_draft`） | `append`（默认） | 追加规范，保留用户原文 |
| 用户在 `WRITE_CONFIRM` 明确点名要求重写的有草稿节点 | `overwrite` | 需用户对该节点显式确认 |

## Command Map

只能使用当前状态允许的 command family。命令详细语法属于被引用 skill / reference。

| State | Allowed Command Families | Purpose |
|-------|--------------------------|---------|
| `PARSE_TARGET` | `wiki +node-get`、`wiki +space-list` | 解析链接为 `space_id` 和根节点 |
| `READ_STRUCTURE` | `wiki +node-list`、`docs +fetch` | 递归读取节点树和读取节点草稿内容 |
| `OUTLINE_PROPOSE` | `wiki +node-create --obj-type docx`（仅用户确认大纲后）、`wiki +node-list` | 新建确认后的大纲子节点并回读 |
| `TYPE_TRIAGE` | 无写命令 | 仅对已读结构做分类 |
| `GEN_STANDARD` | 无写命令 | 模型生成维护规范 |
| `WRITE_CONFIRM` | 无写命令 | 生成写入计划并请用户确认 |
| `WRITE` | `docs +update`（`overwrite` / `append` / `block_*`）；仅 `new_docx` 时 `wiki +node-create --obj-type docx` | 执行已确认的受控写入 |
| `VERIFY` | `docs +fetch`、`wiki +node-list` | fresh read 校验写入结果 |

## Transition Rules

1. `PARSE_TARGET` 无法解析出唯一知识库时，只问目标澄清问题并停止。
2. 认证或 API scope 缺失时，按 `lark-shared` 权限处理并停止。
3. 节点访问权限缺失时，记入 `unsupported_checks` 并在结构概览中报告，不擅自申请权限。
4. `READ_STRUCTURE` 判定结构过简（无子节点或子节点不足以承载分类）时进入 `OUTLINE_PROPOSE`；结构已足够时直接进入 `TYPE_TRIAGE`。
5. `OUTLINE_PROPOSE` 中用户拒绝新建大纲、或只想为现有根节点写规范时，跳过新建，直接以现有节点进入 `TYPE_TRIAGE`；不擅自新建任何节点。
6. `TYPE_TRIAGE` 发现全部节点均为 `non_docx_entity` / `shortcut` 时，说明本知识库没有可写入正文的 docx 节点，询问是否对相关节点走 `new_docx`，不静默结束。
7. 用户在 `WRITE_CONFIRM` 拒绝写入时，输出已保存的规范草案并转入 `DONE`。
8. `WRITE` 中单个节点写入失败时，记录失败并继续其余相互独立的节点；写入结束后在 `VERIFY` 统一报告失败项。
9. 身份在 `PARSE_TARGET` 确定后保持不变；节点解析、读取、写入、验证使用同一身份。

## References

- [lark-drive workflow 总框架](lark-drive-workflow.md)
- [Outputs：规范模板与输出模板](lark-drive-workflow-knowledge-base-bootstrap-outputs.md)
- [lark-shared](../../lark-shared/SKILL.md)
- [lark-wiki](../../lark-wiki/SKILL.md)
- [lark-wiki-node-list](../../lark-wiki/references/lark-wiki-node-list.md)
- [lark-wiki-node-create](../../lark-wiki/references/lark-wiki-node-create.md)
- [lark-doc](../../lark-doc/SKILL.md)
- [lark-doc-fetch](../../lark-doc/references/lark-doc-fetch.md)
- [lark-doc-update](../../lark-doc/references/lark-doc-update.md)
- [knowledge_organize](lark-drive-workflow-knowledge-organize.md)
- [topic_move_collector](lark-drive-workflow-topic-move-collector.md)
