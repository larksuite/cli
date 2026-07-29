# 主题资料收集工作流：资源解析与内容验证

由状态 `RESOURCE_RESOLVE`、`CONTENT_VERIFY` 加载。

本文档负责资源解析、结构化父级、移动资格、内容验证和 `ResourceItem`。不得判断相关性、生成移动计划、创建目标、移动资源或执行恢复操作。

本文档只服务 `topic_move_collector`。进入本文档时，`workflow_id` 必须是 `topic_move_collector`；不得把当前任务改路由到其他 workflow。

## 必读上下文

执行本文档规则前：

1. 按 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 处理身份、认证和权限。
2. 按 [`lark-drive-inspect.md`](lark-drive-inspect.md) 处理 URL / token 解析。
3. 使用 `drive metas batch_query` 补齐 Drive 资源 owner、标题和 URL。
4. 必要时使用 `drive permission.members auth` 读取权限信号；该接口不提供 `full_access` / 移动权限的直接判定，不能把 `manage_public` 等同为可移动。
5. 按 [`../../lark-wiki/references/lark-wiki-node-get.md`](../../lark-wiki/references/lark-wiki-node-get.md) 处理 Wiki 节点解析。
6. 按 [`../../lark-doc/references/lark-doc-fetch.md`](../../lark-doc/references/lark-doc-fetch.md) 读取文档内容。
7. 需要验证 Sheet 内容时，按 [`../../lark-sheets/SKILL.md`](../../lark-sheets/SKILL.md) 执行。

## 进入解析与验证阶段前校验

进入本文档后，如果 `resource_items` 还不存在，当前状态必须是 `RESOURCE_RESOLVE`。

禁止从 `candidate_items` 直接进入 `CONTENT_VERIFY` 或 `RELEVANCE_CLASSIFY`，也禁止从 `RESOURCE_RESOLVE` 直接进入 `RELEVANCE_CLASSIFY`。即使候选项已有标题、URL、摘要或 token，也必须依次执行 `RESOURCE_RESOLVE` 和 `CONTENT_VERIFY`；两个状态不得合并。

## 状态：`RESOURCE_RESOLVE`

进入条件：候选列表已准备。

必须：

1. 为每个 `CandidateItem` 生成稳定 `resource_id`，并转换为标准化 `ResourceItem`。
2. 解析 canonical token、资源类型、URL、结构化当前父级、Wiki 节点身份和读取权限状态。
3. 对 Wiki 资源同时保留 `wiki_node_token` 和 `wiki_obj_token`。
4. 按 `move_method` 补齐 `owner_id`、`is_owner`、`source_move_state`、`source_parent_write_state`、`target_write_state`、`move_permission_state` 和 `move_permission_basis`。
5. 基于 `target_location` 检测不支持的移动方向。
6. 未解析成功的资源仍保留在审核分组中，不得静默丢弃。
7. 即使搜索结果已经包含标题、URL 或 token，也必须经过本状态生成 `ResourceItem`；不得从召回结果直接进入相关性分级。
8. 只有确认 `move_permission_state=movable` 且 `target_write_state=confirmed` 的资源，才能进入后续默认移动链路。
9. 解析耗时超过约 60 秒时，必须输出进度提示，之后约每 60 秒提示一次。

### 解析规则

| 候选类型 | agent 必须执行 |
|----------------|---------------|
| Drive URL / token | token 或类型不确定时，使用 `drive +inspect`。 |
| Wiki URL / token | 使用 `drive +inspect` 或 `wiki +node-get`；保留节点身份和对象身份。 |
| 文件夹候选 | 标记为容器；不要当作普通文档做内容验证。 |
| 快捷方式候选 | 能解析源资源时解析源资源；同时保留快捷方式身份。 |
| 无读取权限 | 保留可见元数据，并设置 `permission_state=denied`。 |
| 无移动权限或移动权限未知 | 保留可见元数据和召回证据，并设置对应 `move_permission_state`。 |
| 无法解析当前父级 | 设置 `current_parent_kind=unknown`，保留已知路径，后续计划项设置 `rollback_supported=false` 和明确 blocker；不得编造父级 token。 |

### 资源解析进度 UI

当 `RESOURCE_RESOLVE` 持续超过约 60 秒时，输出当前进度：

```text
资源解析进度：已解析 <resolved_count>/<total_count> 项，已确认可移动 <movable_count> 项，无移动权限 <denied_count> 项，移动权限未知 <unknown_count> 项，解析失败 <failed_count> 项。
当前资源：<title>
继续解析中，不会创建或移动资源。
```

如果正在处理权限或 owner 元数据，可补充：

```text
当前步骤：解析 owner / 当前父级 / 移动资格。
```

`RESOURCE_RESOLVE` 完成后，输出摘要：

```text
资源解析完成：
- 候选总数：N 项
- 可进入内容验证：N 项
- 无移动权限：N 项
- 移动权限未知：N 项
- 解析失败或无读取权限：N 项

下一步会对可移动资源做内容验证；不会创建或移动资源。
```

### 资源解析出口门禁

`RESOURCE_RESOLVE` 完成后必须：

1. 将 `content_verify_completed` 重置为 `false`。
2. 将下一状态设置为 `CONTENT_VERIFY`，不得设置为 `RELEVANCE_CLASSIFY` 或 `PLAN_MOVE`。
3. 不得在本状态生成 `relevance`、`relevance_groups` 或移动计划。
4. 即使可读取正文的资源数量为 0，也必须进入 `CONTENT_VERIFY`，为每项记录跳过验证原因并输出验证摘要。

### 移动资格判定

`owner` 只能作为部分权限证据，不得单独把资源判为 `movable`。`RESOURCE_RESOLVE` 必须先按 `move_method` 记录以下独立状态：

| 字段 | 说明 |
|------|------|
| `source_move_state` | 当前身份是否确认可以对源资源执行对应移动；Drive owner 只可作为 Drive 源资源可管理的证据，Wiki 底层资源 owner 不能证明 Wiki 节点可移动。 |
| `source_parent_write_state` | 当前身份是否确认可编辑源位置；仅 `drive_move` 必须确认，其他移动方式为 `not_required`。 |
| `target_write_state` | 当前身份是否确认可写目标位置；待创建目标以父级位置的创建 / 写入权限为准。 |

#### 按移动方式的权限矩阵

| `move_method` | `source_move_state=confirmed` 的证据 | `source_parent_write_state` | `target_write_state` |
|---------------|--------------------------------------|-----------------------------|----------------------|
| `drive_move` | 当前用户是可靠解析出的 Drive 资源 owner，或有明确资源可管理证据 | 必须为 `confirmed` | 必须为 `confirmed` |
| `wiki_move_docs_to_wiki` | 有明确的 Drive 文档直接迁入权限；仅 owner 元数据不足以证明可直接迁入 | `not_required` | 必须确认目标 Wiki 节点 / 空间可写 |
| `wiki_move_node` | 有明确的 Wiki 节点 / 源空间移动权限；不得从底层资源 owner 推导 | `not_required` | 必须确认目标 Wiki 节点 / 空间可写 |
| `wiki_move_to_drive` | 有明确的 Wiki 节点移出权限；不得从底层资源 owner 推导 | `not_required` | 必须确认目标 Drive 文件夹可写 |

#### 聚合顺序

1. 目标方向或资源类型不支持时，设置 `move_permission_state=denied`、`move_permission_basis=["unsupported_direction"]`。
2. 任一必需状态为 `denied` 时，设置 `move_permission_state=denied`，并在 `move_permission_basis` 记录 `source_denied`、`source_parent_denied` 或 `target_denied`。
3. 任一必需状态为 `unknown` 时，设置 `move_permission_state=unknown`，并记录对应的 `source_unknown`、`source_parent_unknown` 或 `target_unknown`。
4. 只有权限矩阵中的全部必需状态都为 `confirmed` 时，才能设置 `move_permission_state=movable`、`move_permission_basis=["permission_matrix_confirmed"]`。

注意：

1. `drive permission.members auth` 不提供 `full_access` 或 `move` action；不能用 `view`、`edit`、`share` 或 `manage_public` 结果推断源位置或目标位置可写。
2. `target_write_state=unknown|denied` 的资源不得进入高 / 中相关可执行分组或移动计划。
3. `move_permission_state=unknown` 的资源默认不进入内容验证、相关性高 / 中分组或移动计划。
4. 当 `owner_scope=mine` 但解析出的 owner 不是当前用户时，将该资源视为异常候选，设置 `source_move_state=unknown` 和 `move_permission_state=unknown`，不得加入移动计划。

## 状态：`CONTENT_VERIFY`

进入条件：资源列表已准备。

必须：

1. 本状态不可跳过，也不得与 `RESOURCE_RESOLVE` 或 `RELEVANCE_CLASSIFY` 合并；没有可读取正文的资源时仍须执行。
2. 只在资源解析后读取支持的内容。
3. 按数量、大小和类型能力限制读取范围。
4. 结合搜索证据和内容证据；除非标题精确且足够强，否则不要仅凭标题判为高相关。
5. 将不可读取资源标记为 `unverifiable` 或 `permission_denied`。
6. 不得自动申请权限。
7. 为每个资源写入验证状态：已读取内容证据、仅可使用搜索证据、无权限、无移动权限、移动权限未知、无法验证或不支持内容验证。
8. 对 `move_permission_state=denied|unknown` 的资源，不再读取正文内容，写入跳过验证原因并保留召回证据；写入跳过原因属于执行本状态，不等于跳过本状态。
9. 所有资源都有验证状态或跳过原因后，将 `content_verify_completed` 设置为 `true` 并输出验证摘要。
10. `content_verify_completed=true` 前不得进入 `RELEVANCE_CLASSIFY`。

### 验证方式

| 资源类型 | 验证方式 |
|---------------|---------------------|
| `docx` / `doc` | 允许时使用 `docs +fetch --api-version v2`。 |
| `sheet` | 使用 `sheets +find` 查关键词证据，或用 `sheets +read` 读取有界范围。 |
| `bitable` | 只有必要且已加载 Base 能力时验证。 |
| `slides` | 除非具备幻灯片读取能力，否则使用元数据 / 预览 / 标题证据。 |
| `file` | 仅在支持时使用标题、元数据、预览或导出文本。 |
| `wiki` 节点 | 按 `obj_type` 验证底层对象；节点本身不是内容 token。 |
| `folder` | 除非用户明确要移动容器，否则通常不作为主题证据移动。 |

### 内容验证完成 UI

完成 `CONTENT_VERIFY` 后必须输出：

```text
内容验证完成：
- 已读取内容证据：N 项
- 仅复用搜索证据：N 项
- 因无权限或移动资格跳过：N 项
- 无法验证或不支持验证：N 项

下一步会基于以上证据进行相关性分组；不会创建或移动资源。
```

如果没有任何资源可以读取正文，仍须输出该摘要，并明确说明所有资源采用的搜索证据或跳过原因。

### 内容验证出口门禁

`CONTENT_VERIFY` 完成后必须：

1. 确认 `content_verify_completed=true`，且每个 `ResourceItem` 都已有验证状态或跳过原因。
2. 将下一状态设置为 `RELEVANCE_CLASSIFY`。
3. 加载 [`lark-drive-workflow-topic-move-collector-review-plan.md`](lark-drive-workflow-topic-move-collector-review-plan.md)。
4. 不得直接进入 `PLAN_MOVE`。

## ResourceItem

```json
{
  "resource_id": "稳定资源 ID",
  "title": "资源标题",
  "resource_type": "doc|docx|sheet|bitable|file|folder|wiki|slides|shortcut",
  "url": "资源链接",
  "canonical_token": "标准资源 token",
  "wiki_node_token": "Wiki 节点 token",
  "wiki_obj_token": "Wiki 底层对象 token",
  "wiki_obj_type": "Wiki 底层对象类型",
  "space_id": "知识空间 ID",
  "current_parent_kind": "drive_folder|drive_root|wiki_node|wiki_space_root|unknown",
  "current_parent_token": "当前父级 token",
  "current_parent_space_id": "当前父级 Wiki space_id",
  "current_path": "用于展示的当前位置",
  "owner_id": "资源 owner open_id",
  "is_owner": "true|false|unknown",
  "permission_state": "readable|denied|unknown",
  "source_move_state": "confirmed|unknown|denied",
  "source_parent_write_state": "confirmed|unknown|denied|not_required",
  "move_permission_state": "movable|denied|unknown",
  "move_permission_basis": ["权限矩阵证据或阻塞原因"],
  "target_write_state": "confirmed|unknown|denied",
  "item_resolve_status": "resolved|partial|failed",
  "content_verify_state": "verified|search_evidence_only|skipped_by_move_permission|permission_denied|unverifiable|unsupported",
  "content_evidence": ["证据"],
  "relevance": "high|medium|low|permission_denied|no_move_permission|move_permission_unknown|unverifiable|unsupported_move_target"
}
```

| 字段 | 说明 |
|-------|------|
| `canonical_token` | 内容读取、Drive 对象操作或底层对象操作使用的标准 token；Wiki 节点移动不得使用该字段。 |
| `resource_id` | 资源解析时生成的稳定 ID，用于连接 `ResourceItem` 和 `MovePlanItem`。 |
| `wiki_node_token` | Wiki 节点身份，用于 Wiki 节点移动。 |
| `wiki_obj_token` | Wiki 节点背后的真实文档 token。 |
| `current_parent_kind` / `current_parent_token` / `current_parent_space_id` | 结构化执行前父级，用于 `already_at_target` 判断和恢复；未知值不得猜测。 |
| `current_path` | 仅用于用户展示的当前位置，不得代替父级 token。 |
| `owner_id` | 资源 owner；Drive 资源优先来自 `drive metas batch_query`，Wiki 节点优先来自 `wiki +node-get`。 |
| `is_owner` | 当前用户是否为资源 owner。 |
| `permission_state` | 当前身份下的读取权限状态。 |
| `source_move_state` | 当前身份是否确认能对源资源执行所选 `move_method`；必须按权限矩阵判断。 |
| `source_parent_write_state` | Drive 内移动所需的源位置编辑状态；非 `drive_move` 为 `not_required`。 |
| `move_permission_state` | 权限矩阵聚合结果；只有 `movable` 且目标写入状态为 `confirmed` 才可进入默认移动链路。 |
| `move_permission_basis` | 移动资格判断依据，用于解释为什么纳入或排除。 |
| `target_write_state` | 目标位置是否确认可写。 |
| `item_resolve_status` | 资源项解析状态；不要和 `TargetLocation.target_resolve_status` 混用。 |
| `content_verify_state` | 内容验证状态或跳过验证原因。 |
| `content_evidence` | 支撑相关性判断的命中证据。 |
| `relevance` | 相关性和可执行性分组。 |
