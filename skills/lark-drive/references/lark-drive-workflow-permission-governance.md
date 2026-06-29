# lark-drive 权限治理 Workflow

Workflow id: `permission_governance`

Risk / Structure: `R2` / `S2`

本文实现已注册的权限治理 workflow。执行前必须先读取 [`lark-drive-workflow.md`](lark-drive-workflow.md) 和 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)，并遵循共享执行协议、Artifact Contract、Workflow Loading、认证和写入确认规则。

## 适用范围

当用户要求检查或治理 Drive / Docs / Wiki 资产访问权限时，使用本 workflow。典型意图包括：

- 单资源公开性、外部访问、公司内链接、分享 / 复制 / 下载 / 评论设置检查。
- 多资源、Wiki space / node、Drive folder 或个人文档库的权限风险诊断和权限设置清单。
- 访问复核、低活跃高暴露、权限申请、owner 转移、密级标签调整、AI Agent / RAG 前置权限治理。
- 只读整改 dry-run，或经确认后的权限收紧 / 权限申请 / owner 转移 / 密级标签更新。

目标可以是明确 URL / token、小规模明确列表、Wiki space / Wiki node 或 Drive folder。容器范围必须先只读 `DISCOVER_TARGETS` 并产出覆盖摘要；这里的"所有文档"只表示当前身份在确认范围内可枚举到的文档。任何写入都必须再次确认。

单目标轻量路径：用户只问“是否对外公开 / 外部可访问 / 公司内链接可见”且目标是单个明确 URL / token 时，设置 `intent=public_exposure_check`、`target_scope=single_resource`，走 `PARSE_INTENT -> TARGET_INSPECT -> FACT_READ -> RISK_ASSESS -> DONE`。该路径是 `target_count=1` 的轻量输出模式，不是独立判断逻辑；不执行 `DISCOVER_TARGETS`、不生成 `risk_manifest` / `risk_id`，只输出结论、权限含义、检查边界和必要下一步。

## Target Set Evaluation

本 workflow 不按“单篇 / 多篇 / 容器”复制权限判断逻辑。所有范围先归一为 target set，再对每个可审计目标生成 `per_target_permission_assessment`，最后按目标数量和风险分组聚合输出。

| target_scope | Target Collection | Output Mode |
|--------------|-------------------|-------------|
| `single_resource` | 直接解析一个 URL / token | `target_count=1` 时轻量渲染；不生成 `risk_manifest` |
| `explicit_list` | 用户给出的多个 URL / token 逐个 inspect / normalize | 逐目标渲染摘要；需要后续治理时生成稳定 `risk_id` |
| `wiki_space` / `wiki_node` / `drive_folder` | 先只读递归发现，再归一化为 `discovered_targets` | 输出覆盖情况、风险分组、可定位待复核对象和 artifact / dry-run CTA |

特殊的是目标收集和输出聚合，不是权限语义。`link_access`、`external_sharing`、`copy_scope`、`security_scope`、`comment_scope`、`sec_label`、`check_scope` 等语义字段必须在单目标、多目标明确列表和容器发现目标之间复用。

## 非目标

本 workflow 不处理：

- 目录组织、迁移、归档或清理；这类需求应使用知识整理 workflow。
- 内容审查、过期内容判断或知识质量评分。
- backup owner 补充、部门 / 项目负责人绑定、协作者创建 / 撤销、成员列表审计；本 workflow 只支持把 owner 转移给每个目标明确指定的新 owner，不建模 backup owner 或负责人绑定关系。
- 文件夹自身公开权限审计或修复。`drive permission.public get` / `patch` 不支持 `type=folder`；必须记录到 `unsupported_checks`，然后继续读取文件夹下其他支持的文档事实。
- 当前身份无法枚举到的不可见文档的完整发现；只能处理已发现目标，或用户显式提供的 URL / token。
- 未按范围确认的批量写入。

不要声称已完成协作者列表验证：当前 CLI surface 没有 `permission.members list` shortcut。

## Progressive Load Map

本表只规定每个 state 需要加载的额外上下文；命令可用范围以 `Command Map` 为准。需要拼装具体 `lark-cli` 命令时，再按需读取 [`lark-drive-workflow-permission-governance-commands.md`](lark-drive-workflow-permission-governance-commands.md)。

| State | Required Reference |
|-------|--------------------|
| `PARSE_INTENT` | 本文件、[`lark-drive-workflow.md`](lark-drive-workflow.md)、[`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) |
| `TARGET_INSPECT` | [`lark-drive-inspect.md`](lark-drive-inspect.md) |
| `DISCOVER_TARGETS` | 容器范围时读取 [`../../lark-wiki/references/lark-wiki-node-list.md`](../../lark-wiki/references/lark-wiki-node-list.md) 或 [`lark-drive-files-list.md`](lark-drive-files-list.md) |
| `FACT_READ` | `lark-cli schema drive.metas.batch_query`；涉及公开权限时再读取 `lark-cli schema drive.permission.public.get`；涉及活跃度、访问复核或生命周期判断时再读取 `lark-cli schema drive.file.statistics.get` 和 `lark-cli schema drive.file.view_records.list` |
| `RISK_ASSESS` | 本文件的 `Risk Classification` |
| `EXEC_CONFIRM` | 只为用户选择的动作读取 [`lark-drive-apply-permission.md`](lark-drive-apply-permission.md)、[`lark-drive-secure-label.md`](lark-drive-secure-label.md)，或 `lark-cli schema drive.permission.public.patch` / `lark-cli schema drive.permission.members.transfer_owner`；需要确认模板时读取 [`lark-drive-workflow-permission-governance-outputs.md`](lark-drive-workflow-permission-governance-outputs.md) |
| `EXECUTE` | 复用 `EXEC_CONFIRM` 已加载且已确认的写命令上下文 |
| `VERIFY` | 复用 `FACT_READ` 阶段使用的 read schemas |

## Runtime State Extension

本 workflow 在共享 `Artifact Contract` 基础上扩展以下字段组：

| Group | Fields | Meaning |
|-------|--------|---------|
| Scope | `intent`, `target_scope`, `targets`, `discovered_targets`, `coverage_summary`, `discovery_blockers` | 记录用户意图、确认范围、直接目标、容器发现目标和未覆盖范围 |
| Facts | `metadata_facts`, `public_permission_facts`, `activity_facts`, `manage_public_auth` | 记录 metadata、公共访问与协作权限、访问证据，以及写前 `manage_public` 校验 |
| Assessment | `per_target_permission_assessments`, `risk_findings`, `unsupported_checks` | 记录逐目标语义判断、带 `risk_id` / URL / owner / sec_label / evidence / action 的风险发现，以及无法执行的检查 |
| Governance | `risk_manifest`, `selected_risk_items`, `access_review_items`, `permission_request_candidates`, `owner_transfer_candidates` | 支持用户按 `risk_id`、风险分组、owner、路径、URL 或 artifact `selected=true` 选择治理范围，并记录 owner 转移候选 |
| Execution | `remediation_plan`, `owner_transfer_plan`, `public_permission_snapshots` | 记录 dry-run / 已确认整改计划、owner 转移计划、字段 diff、验证方式和 public-permission 有限回滚快照 |

## Execution State Machine

| State | Protocol Step | Agent MUST Do | User-Facing Output | wait_for_user | Next State |
|-------|---------------|---------------|--------------------|---------------|------------|
| `PARSE_INTENT` | `route` / `scope` | 解析 intent、target scope、desired policy，以及只读审计、单目标公开性判断、权限申请、owner 转移还是修复模式；单目标公开性判断设置 `intent=public_exposure_check`、`target_scope=single_resource` | 范围确认；如果缺少目标、新 owner 或期望动作，只问一个澄清问题 | 缺少 target / new owner / action，或容器范围需要用户确认时为 `true` | `TARGET_INSPECT` |
| `TARGET_INSPECT` | `scope` | 解析单资源、明确列表、Wiki space / node、Drive folder；保留原始 URL、scope type、canonical token/type | 目标范围表，包含 scope、title/type/token status | 除非解析失败，否则为 `false` | `DISCOVER_TARGETS` or `FACT_READ` |
| `DISCOVER_TARGETS` | `scope` / `read` | 对 Wiki space / node 或 Drive folder 递归只读枚举，归一化为 `discovered_targets`；记录 `discovery_blockers` | 发现进度和覆盖摘要；不展示内部 cursor/token，除非用户要求 | 除非发现范围无法确认或全部被阻断，否则为 `false` | `FACT_READ` |
| `FACT_READ` | `read` | 对直接目标或 `discovered_targets` 执行 `drive metas batch_query`；对支持的非 folder 目标执行 `drive permission.public get`；当 `intent=public_exposure_check` 且 `target_scope=single_resource` 时，可复用 `drive +inspect` 返回的 title / URL / type，只补读文档公共访问和协作权限设置；在用户要求活跃度 / 访问复核 / 生命周期判断时读取访问统计和访问记录 | 权限事实摘要、coverage summary、activity facts 和 unsupported checks | 除非所有目标都被 auth 阻断，否则为 `false` | `RISK_ASSESS` |
| `RISK_ASSESS` | `assess/plan` | 对每个可审计目标生成 `per_target_permission_assessment` 并分类证据；如用户提供 policy，则对照 policy；`public_exposure_check + single_resource` 只渲染单目标结论，不生成 `risk_id`；owner 转移路径生成 `owner_transfer_candidates` / `owner_transfer_plan`；治理路径构建可定位风险清单、访问复核清单、dry-run 整改计划或候选修复计划，完整清单必须生成稳定 `risk_id` | 带 priority、URL、risk_id、owner、sec_label 的 findings、confidence、review items、建议动作和下一步 CTA；单目标公开性判断只输出结论和关键字段 | 治理路径为 `true`，单目标公开性判断为 `false` | `EXEC_CONFIRM` or `DONE` |
| `EXEC_CONFIRM` | `confirm` | 展示准确写入范围、command family、target count、risk、verification method | 确认请求 | `true` | `EXECUTE` or `DONE` |
| `EXECUTE` | `execute` | 只执行 `Command Map` 中已确认的写入 | 进度 / 结果摘要 | 除非被阻断，否则为 `false` | `VERIFY` |
| `VERIFY` | `verify` | 重新执行支持的读取，并与目标状态对比 | 验证表和剩余缺口 | `false` | `DONE` |
| `DONE` | `done` | 停止 | 最终回复，包含完成事项、验证结果和剩余风险 | `false` | End |

## Command Map

本 workflow 只能使用以下 command families：

| State | Allowed Command Families | Purpose |
|-------|--------------------------|---------|
| `TARGET_INSPECT` | `drive +inspect` | 解析 URL、type、canonical token、title 和 wiki unwrap data |
| `DISCOVER_TARGETS` | `wiki +node-list` | 递归发现 Wiki space / node 下当前身份可见的节点 |
| `DISCOVER_TARGETS` | `drive files list` | 递归发现 Drive folder 下当前身份可见的文件和子文件夹 |
| `FACT_READ` | `drive metas batch_query` | 读取 title、URL、owner 和 secure-label metadata |
| `FACT_READ` | `drive permission.public get` | 读取支持类型的文档公共访问和协作权限设置，包括链接分享、对外分享、协作者管理、复制内容、创建副本、打印、下载和评论 |
| `FACT_READ` | `drive file.statistics get` | 在用户要求活跃度、闲置暴露、生命周期或访问复核时读取文件访问统计 |
| `FACT_READ` | `drive file.view_records list` | 在用户要求最近访问人、访问复核或低活跃证据时读取访问记录 |
| `EXEC_CONFIRM` | `drive +secure-label-list` | 提议 label update 前解析可用 secure-label IDs |
| `EXEC_CONFIRM` | `drive permission.members auth` | 文档公共访问和协作权限设置修改前检查 `action=manage_public` |
| `EXEC_CONFIRM` | `lark-cli schema drive.permission.members.transfer_owner` | owner 转移前读取当前字段、支持类型和高风险写入门禁 |
| `EXECUTE` | `drive +apply-permission` | 向 owner 提交 view/edit access request；只允许单目标、小列表或已明确确认的候选列表逐个执行 |
| `EXECUTE` | `drive permission.public patch` | 修改已确认的 public/link settings；必须传 `--yes` |
| `EXECUTE` | `drive permission.members transfer_owner` | 转移已确认目标的 owner；必须传 `--yes` |
| `EXECUTE` | `drive +secure-label-update` | 设置已确认的 secure-label ID |
| `VERIFY` | `drive metas batch_query`, `drive permission.public get` | 验证支持的 metadata，包括 owner、secure-label 和文档公共访问与协作权限设置变更；权限申请只能表述为已发起 |

## Command Patterns

本入口不内联命令样例。需要拼装具体 `lark-cli` 命令时，按当前 state 读取 [`lark-drive-workflow-permission-governance-commands.md`](lark-drive-workflow-permission-governance-commands.md)。命令是否允许执行仍以 `Command Map` 和写入规则为准。

## Discovery Rules

容器范围只能先做只读发现和覆盖摘要，不能在发现阶段执行权限申请、权限 patch 或密级更新。

通用规则：

1. "所有文档"只表示当前身份在确认范围内可枚举到的文档。不可见、无权限、API 不返回或工具预算不足的部分必须进入 `discovery_blockers` 或 `unsupported_checks`。
2. 发现阶段必须生成稳定 `path`。不要只保存 title；同名文档必须能通过 path 或 token 区分。
3. 只把 `drive.permission.public.get` 当前 schema 支持的类型加入公开权限可审计目标。已知支持包括 `doc`、`sheet`、`file`、`wiki`、`bitable`、`docx`、`mindnote`、`minutes`、`slides`；未来新增类型以运行时 schema 为准。
4. `minutes` 只能作为 `partial_public_permission` 目标：可读取 / 修改公开权限和 owner 转移能力以运行时 schema 为准，但 `drive metas batch_query` 当前不支持 `minutes`，URL、owner、密级等 metadata 可能进入 `unsupported_checks`。
5. `folder` 只作为递归容器，不执行 `permission.public get` / `patch`。如果用户明确要求 owner 转移且 schema 支持 `folder`，必须按 owner-transfer 写入规则单独确认。`shortcut`、`catalog` 或缺少 stable token/type 的条目必须记录为 unsupported，除非后续 API 明确解析出支持目标。
6. 对大范围目标输出进度时，只展示已扫描容器数、已发现目标数、已审计目标数、剩余队列或 blocker；不要默认展示内部 page token / cursor。

Wiki space / node 发现：

1. `/wiki/space/<space_id>` 直接解析为 `target_scope=wiki_space`。不要因为 `drive +inspect` 对该 URL 返回 not found 就停止。
2. 用 `wiki +node-list --space-id <space_id>` 读取根节点；当节点 `has_child=true` 时，用该节点的 `node_token` 继续递归读取子节点。
3. Wiki 节点必须同时保留 `node_token`、`obj_token` 和 `obj_type`。权限读取优先用 `type=wiki` + `node_token` 表达 Wiki 节点权限；元数据补充可使用 `obj_type` + `obj_token`。
4. 如果节点只有 `obj_token` / `obj_type`，但无法确认 Wiki 节点权限 token，保留该目标为 partial，并在 `unsupported_checks` 中说明只能读取底层对象或无法完整判断 Wiki 节点权限。

Drive folder 发现：

1. `/drive/folder/<folder_token>` 解析为 `target_scope=drive_folder`。文件夹自身公开权限不支持；继续枚举其子文档。
2. 按 [`lark-drive-files-list.md`](lark-drive-files-list.md) 递归处理 `data.files`、`has_more` 和 `next_page_token`。不要把第一页数量当作完整范围。
3. 只对返回项中的 `folder` 继续递归；对子文档按 `type + token` 归一化为 `discovered_targets`。
4. 如果某个目录分页失败、无 continuation token、权限不足或 API 报错，只阻断该目录分支，并在 `discovery_blockers` 中记录；继续处理其他可枚举分支。

## Fact Read Rules

1. `drive metas batch_query` 单次最多 200 个 `request_docs`；当 `targets` 或 `discovered_targets` 超过 200 个时，必须分批读取并合并结果。
2. `drive permission.public get` 没有批量读取接口；对支持目标逐个读取。单个目标失败时记录 `unsupported_checks` 或 `partial`，不要阻断其他目标。
3. 对 Wiki 发现目标，公开权限读取优先使用 `type=wiki` + `node_token`；metadata 可使用 `obj_type` + `obj_token` 补充 title、owner、URL 和 `sec_label_name`。
4. 当 intent 是 `list_permission_settings` 时，只输出权限设置清单和覆盖限制，不主动生成修复计划。
5. 单目标、多目标明确列表和容器发现目标都必须复用同一套逐目标事实读取与语义归一逻辑；差异只体现在目标来源、coverage summary 和输出聚合。
6. `permission_public` 用户可见含义是“文档公共访问和协作权限设置”，语义以官方 OpenAPI 字段说明为准，同时兼容当前 CLI schema 返回的字段：优先使用 `external_access_entity`，缺失时才用 `external_access` boolean 映射为 `open` / `closed`；`manage_collaborator_entity`、`copy_entity`、`lock_switch` 等字段缺失时标记为 unknown，不要伪造；未识别字段保留在 raw evidence / partial note 中。
7. `drive file.statistics get` 和 `drive file.view_records list` 只在用户要求最近访问、活跃度、闲置暴露、访问复核，或用户提供的 policy 明确依赖活跃度时执行；不要为普通权限审计默认读取访问记录。
8. 访问统计 / 访问记录当前只对 `doc`、`docx`、`sheet`、`bitable`、`mindnote`、`wiki`、`file` 作为支持类型处理。其他类型必须进入 `unsupported_checks`，不能推断活跃度。
9. `view_records` 是访问证据，不是权限列表。没有返回访问记录只能表述为“未获得最近访问证据”或“低活跃候选”，不能表述为“无人有权限”。

## Risk Classification

风险标签只能作为 evidence labels。除非用户提供明确 policy，否则不要表述为绝对违规、已泄露或已外部访问。

默认优先级面向用户决策，而不是制造告警感：

- `P0`：`link_share_entity=anyone_readable/anyone_editable`，互联网公开链接候选风险。
- `P1`：`external_access_entity=open` / `external_access=true`、关联组织访问、公司内链接可编辑，或外部分享且缺少 / 低于 policy 密级标签。
- `P2`：公司内知道链接可读、协作者管理范围较宽。
- `PolicyReview`：复制、创建副本、打印、下载、评论等依赖 policy 的设置；没有明确 policy 时不要称为高风险。
- `Unknown`：读取失败、已删除、无权限、API 不支持、协作者名单 / 继承链 / DLP / AI 索引 / 审计日志未覆盖。

每个可审计目标都必须先归一化为 `per_target_permission_assessment`，再按 [`lark-drive-workflow-permission-governance-outputs.md`](lark-drive-workflow-permission-governance-outputs.md) 的 `Semantic Rendering` 渲染。`public_exposure_check` 只是 `target_count=1` 的轻量渲染模式；它和多目标、容器诊断复用同一套语义字段与风险分类。该判断只覆盖当前文档公共访问和协作权限设置，不审计协作者名单、历史权限变更、完整继承链或审计日志。

`AI 检索暴露候选风险` 只是基于权限和标签的代理标签。除非另有工具明确返回索引状态，否则不要声称某个文档已经被 Agent、Copilot 或 RAG 索引。

## 写入规则

- 文档公共访问和协作权限设置修改（`drive permission.public patch`）属于高风险写入。请求确认前，必须展示 target title、token、current setting、desired setting 和准确 field changes。
- 如果 `manage_public_auth.auth_result=false`，禁止 patch。告诉用户需要具备 manage-public 权限的用户，或由 owner 操作。
- `drive permission.public get` 只用于 `drive +inspect` 或 `DISCOVER_TARGETS` 可解析且运行时 schema 支持的目标类型；类型集合不要硬编码，执行时以 `lark-cli schema drive.permission.public.get` 为准。
- 不要 patch 已解析类型不支持的字段。对于 wiki 目标，必须省略 schema 明确标注为 wiki 不支持的字段。
- 不要在同一个写入确认中合并密级标签更新和文档公共访问与协作权限设置修改；必须分别确认。
- `drive +apply-permission` 默认不批量执行；每次调用都会向 owner 发送通知。
- `permission_request_candidates` 可以来自用户直接提供的目标、明确列表或容器发现目标；只要能构造 token、type、权限类型和申请理由，就可以进入候选。不要因为目标不在 `discovered_targets` 中而拒绝单目标 / 小列表权限申请。
- 容器范围内的"统一申请权限"必须先产出 `permission_request_candidates`。未展示候选目标、数量、权限类型和 owner 通知影响前，禁止调用 `drive +apply-permission`。
- 用户显式确认批量权限申请后，也必须逐个目标顺序调用 `drive +apply-permission`，并在结果中区分已发起申请、失败、无法构造申请请求和未发现目标。
- `drive permission.members transfer_owner` 属于 owner 转移高风险写入。必须先确认目标、当前 owner、新 owner 的 `member_id` / `member_type`、`need_notification`、`remove_old_owner`、`old_owner_perm`、`stay_put`、执行顺序和验证方式；不能只凭姓名猜测新 owner。
- owner 转移没有 `permission.members auth` 的等价 precheck。执行前只能用 schema 和当前 metadata 做计划，执行后必须用 `drive metas batch_query` fresh read 验证 owner；metadata 不支持的类型必须把验证标记为 partial。
- 批量 owner 转移必须逐个顺序执行；失败项进入结果清单，不要重复执行已成功目标。`remove_old_owner=true` 或 `old_owner_perm` 降权必须单独在确认中高亮。
- 用户要求“生成整改方案 / dry-run / 先看看会改什么”时，只生成 `remediation_plan`，不执行任何写命令。dry-run 必须包含 target count、field changes、跳过原因、验证方式和有限回滚范围。
- 用户基于完整风险清单选择对象时，必须先解析 `risk_id`、风险分组、URL 或 artifact 中 `selected=true` 的行，生成 `selected_risk_items`。无法匹配到当前 `risk_manifest` 的选择必须要求用户重新确认或重新读取清单。
- 针对 `selected_risk_items` 生成 dry-run 前，必须重新读取所选目标的 `drive permission.public get`；如果当前设置和清单快照不同，标记为 `changed_since_report` 并跳过或要求用户确认更新后的计划。
- 执行 `drive permission.public patch` 前，必须把当前 `public_permission_facts` 中会被改动的字段保存为 `public_permission_snapshots`。该快照只用于文档公共访问和协作权限设置字段的有限回滚说明，不覆盖协作者、owner、继承权限或密级标签。
- 如果用户要求批量收紧权限，必须按风险分层和目标顺序逐个执行；失败项进入结果清单，不要因为单个失败而重复执行已成功目标。
- 遇到 secure-label downgrade error `1063013` 时，停止重试，并告诉用户需要在文档 UI 中完成审批。

## 未来扩展边界

以下能力已有部分 CLI surface 或用户价值，但不要在当前 workflow 中作为可执行分支直接调用：

- `drive permission.members create` 可创建协作者权限，但当前 workflow 不做协作者 grant / update / revoke；未来需要单独定义授权对象解析、最小权限、确认模板和验证方式。
- backup owner、部门 / 项目负责人绑定没有当前 workflow 可执行写入面；如用户要落地为 owner 转移，必须先给出明确目标和新 owner，并走本 workflow 的 owner-transfer 确认。
- `wiki +member-list` 可作为 Wiki space 成员治理的读侧事实来源；当前 workflow 只治理文档 / 节点 / 文件夹下可发现文档的权限，不做 space member governance。
- 当前 CLI 没有 `permission.members list`、完整继承链、DLP 扫描、AI 索引状态、审计日志和跨平台权限事实。遇到这些需求必须记录为 `unsupported_checks` 或建议新增独立 workflow。

## 输出策略

- 默认 summary-first：单目标输出简短审计摘要；多目标明确列表输出逐目标摘要；容器目标输出安全诊断报告摘要，不堆叠字段计数。
- 单目标 `public_exposure_check` 按 outputs 的 `Semantic Rendering` 渲染 `per_target_permission_assessment`，输出用户语言结论和检查边界；默认不展示底层字段名、风险清单或整改 CTA。
- 容器安全诊断必须包含一句话结论、覆盖情况、风险分级、可定位待复核对象、建议下一步和剩余限制。
- 待复核对象必须包含稳定 `risk_id`、path/title、URL、type、owner、sec_label、风险原因、证据和建议动作；缺少 URL 时展示 token / node_token 和原因。
- 容器摘要按规模渐进披露，不能固定 Top N；未完全展开时必须说明完整清单总数，并给出生成 artifact / dry-run / owner 复核清单等 CTA。
- 面向用户优先使用业务语言和“候选风险 / 待复核 / 待策略确认”；底层字段只作为证据。完整模板按需读取 [`lark-drive-workflow-permission-governance-outputs.md`](lark-drive-workflow-permission-governance-outputs.md)。
- 不要默认创建文件、飞书文档或长表格；最终回复必须包含已完成事项、验证结果和剩余限制。异步权限申请审批只能表述为“已发起申请”。
