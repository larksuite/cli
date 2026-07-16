# 主题资料收集工作流：输入与目标确认

由状态 `PARSE_INPUT`、`RESOLVE_TARGET`、`CONFIRM_CONTEXT` 加载。

本文档负责用户输入解析、目标位置解析、搜索前确认和 `TargetLocation`。不得执行搜索召回、资源分类、目标创建或资源移动。

本文档只服务 `topic_move_collector`。进入本文档后必须确认 `workflow_id=topic_move_collector`；不得把当前任务改路由到其他 workflow。

## 必读上下文

执行本文档规则前：

1. 按 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 处理身份、认证和权限。
2. 解析 Drive 目标时，遵循 [`lark-drive-inspect.md`](lark-drive-inspect.md)、[`lark-drive-create-folder.md`](lark-drive-create-folder.md) 和 [`lark-drive-search.md`](lark-drive-search.md)。
3. 解析 Wiki 目标时，遵循 [`../../lark-wiki/SKILL.md`](../../lark-wiki/SKILL.md)、[`../../lark-wiki/references/lark-wiki-node-get.md`](../../lark-wiki/references/lark-wiki-node-get.md) 和 [`../../lark-wiki/references/lark-wiki-node-create.md`](../../lark-wiki/references/lark-wiki-node-create.md)。

## 状态：`PARSE_INPUT`

进入条件：workflow 被触发。

必须：

1. 提取 `topic`、`target`、`identity`、`owner_scope` 和 `constraints`。
2. 将 `topic` 和 `target` 视为必填字段。
3. 除非用户明确要求 bot / app 视角，否则 `identity` 默认使用用户身份。
4. 默认 `allow_cross_container_move=true`，但必须在 `CONFIRM_CONTEXT` 展示。
5. 默认 `owner_scope=mine`，表示只搜索当前用户 owner / 负责的资源。
6. 只有用户明确要求“不限 owner”“包括共享给我的”“所有我能看到的文档”或“全量搜索”时，才设置 `owner_scope=all_visible`。
7. 除非用户明确提供限制，否则 `constraints` 保持为空。
8. 如果缺少 `topic` 或 `target`，只提出最小澄清问题。

### 输入字段

| 字段 | 说明 |
|-------|------|
| `topic` | 用户要查找的主题、关键词、内容线索、同义词、缩写、排除词。 |
| `target` | 归档目标，可以是已有 Drive 文件夹、已有 Wiki 节点、待创建 Drive 文件夹或待创建 Wiki 节点。 |
| `identity` | 执行身份，默认 `--as user`。 |
| `owner_scope` | 搜索 owner 范围，默认 `mine`；`all_visible` 仅在用户明确要求扩展到所有可见资源时使用。 |
| `constraints` | 用户显式给出的类型、时间、创建人、评论、标题、范围等限制。 |
| `allow_cross_container_move` | 是否允许跨 Drive / Wiki 容器移动；默认允许，但必须确认。 |

### 澄清模板

```text
我还需要补齐两个信息后才能开始：

1. 要查找的主题 / 关键词 / 内容线索是什么？
2. 找到后要移动到哪个 Drive 文件夹或 Wiki 节点？如果需要新建目标，也请说明父级位置和新名称。
```

## 状态：`RESOLVE_TARGET`

进入条件：`topic` 和 `target` 已获得。

必须：

1. 将已有目标解析为具体 token。
2. 如果目标需要创建，只解析父级位置和新目标名称。
3. 在本状态中不得创建文件夹或 Wiki 节点。
4. 分别保留 Drive 文件夹 token、Wiki 节点 token、Wiki 对象 token、space ID 和 parent token。
5. 如果目标 URL / token 存在，但当前身份无法读取或解析目标位置，设置 `target_resolve_status=permission_denied`，保持在 `RESOLVE_TARGET` 并等待用户更换目标或结束；不得进入搜索。
6. 如果已知移动方向不支持，尽早标记。

### 目标解析

| 条件 | agent 必须执行 | 设置 `target_type` |
|-----------|---------------|-------------------|
| 已有 Drive 文件夹 URL 或 token | 有 URL 时用 `drive +inspect` 解析；保留 `folder_token` | `drive_folder` |
| 已有 Wiki 节点 URL 或 token | 用 `wiki +node-get` 或 `drive +inspect` 解析；保留 `wiki_node_token` 和 `space_id` | `wiki_node` |
| 在已知父级下新建 Drive 文件夹 | 解析父文件夹；保存新文件夹名称；不创建 | `new_drive_folder` |
| 在已知父级下新建 Wiki 节点 | 解析知识空间和可选父节点；保存新节点标题；不创建 | `new_wiki_node` |
| 以 Wiki 空间根节点作为目标 | 解析 `space_id`；parent token 可以为空 | `wiki_space` |
| 目标名称有歧义 | 仅在必要时搜索或列出候选；展示候选并等待用户选择 | `unknown` |

### 目标解析状态

| 条件 | `target_resolve_status` |
|------|--------------------------|
| 目标已解析，或待创建目标的父级位置已解析 | `resolved` |
| 目标名称有歧义、候选不唯一，或 `target_type=unknown` 需要用户选择 | `ambiguous` |
| 已知目标方向或目标类型不支持本 workflow | `unsupported` |
| 目标 URL / token 存在，但当前身份无权读取、解析或确认目标位置 | `permission_denied` |

### 目标解析出口门禁

| `target_resolve_status` | 下一状态 | agent 必须执行 |
|-------------------------|----------|----------------|
| `resolved` | `CONFIRM_CONTEXT` | 展示已解析目标并进入搜索前确认。 |
| `ambiguous` | 保持 `RESOLVE_TARGET` | 展示候选并等待用户选择；不得进入 `CONFIRM_CONTEXT`。 |
| `unsupported` | 保持 `RESOLVE_TARGET` | 展示不支持原因，等待用户更换目标或结束；不得搜索。 |
| `permission_denied` | 保持 `RESOLVE_TARGET` | 展示权限 blocker，等待用户更换目标或结束；不得搜索。 |

用户提供新目标后，重新执行 `RESOLVE_TARGET`。只有新的解析结果为 `resolved`，才能进入 `CONFIRM_CONTEXT`；用户选择结束时进入 `DONE`。

### 跨容器规则

| 来源 -> 目标 | 默认规则 |
|------------------|---------|
| Drive 资源 -> Drive 文件夹 | 支持，使用 `drive +move`。 |
| Drive 文档类资源 -> Wiki 节点 / 空间 | 资源类型支持时，使用 `wiki +move`。 |
| Wiki 节点 -> Wiki 节点 / 空间 | 支持，使用 `wiki +move --node-token`。 |
| Wiki 节点 -> Drive 文件夹 | `wiki +move-to-drive`。 |

## 状态：`CONFIRM_CONTEXT`

进入条件：`target_resolve_status=resolved`。

必须：

1. 展示主题、目标、身份、搜索 owner 范围、限制和目标解析字段。
2. 说明下一步只进行搜索 / 读取。
3. 说明是否计划创建目标，但尚未执行。
4. 展示是否允许跨容器移动。
5. 在进入 `SEARCH_RECALL` 前停止并等待用户确认。
6. 如果 `owner_scope=all_visible`，明确提示候选数量可能较多，且可能包含无法移动的资源。

### 确认 UI

```text
我先确认本次收集任务。

查找主题：
目标位置：
目标解析：
执行身份：
搜索范围：
可选限制：
跨容器移动：
下一步操作：只进行搜索和读取验证，不创建目标，不移动资源。

请确认是否按以上信息开始搜索？
```

默认搜索范围文案：

```text
搜索范围：当前用户 owner / 负责的资源
```

扩展搜索范围文案：

```text
搜索范围：所有当前身份可见资源
风险提示：候选数量可能较多，且部分资源可能无法移动；后续仍会经过资源解析和内容验证。
```

如果用户修改任一字段，更新 `topic`、`target_location`、`owner_scope` 或 `constraints`，然后只重新执行受影响的 setup 状态，再次展示确认信息。

## TargetLocation

```json
{
  "target_type": "drive_folder|wiki_node|wiki_space|new_drive_folder|new_wiki_node|unknown",
  "target_token": "已有目标的 folder_token 或 wiki_node_token",
  "parent_token": "待创建目标的父级 folder_token 或 wiki_node_token",
  "space_id": "知识库空间 ID",
  "target_name": "待创建目标名称",
  "create_required": false,
  "allow_cross_container_move": true,
  "target_resolve_status": "resolved|ambiguous|unsupported|permission_denied"
}
```

| 字段 | 说明 |
|-------|------|
| `target_type` | 目标位置类型，用于决定后续创建和移动命令。 |
| `target_token` | 已有目标的可执行 token。 |
| `parent_token` | 待创建目标的父级位置 token。 |
| `space_id` | Wiki 目标所属知识空间 ID。 |
| `target_name` | 待创建目标的名称。 |
| `create_required` | 是否需要在 `EXECUTE` 阶段创建目标。 |
| `allow_cross_container_move` | 是否允许 Drive / Wiki 之间移动。 |
| `target_resolve_status` | 目标位置解析状态；不要和 `ResourceItem.item_resolve_status` 混用。 |
