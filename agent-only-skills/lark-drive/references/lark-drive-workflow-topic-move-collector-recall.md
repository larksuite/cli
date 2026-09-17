# 主题资料收集工作流：召回

由状态 `SEARCH_RECALL`、`RECALL_ENHANCE` 加载。

本文档负责基础搜索召回、覆盖增强、query 证据、去重和 `CandidateItem`。不得解析目标移动 token、读取完整文档内容、判断相关性或执行写操作。

本文档只服务 `topic_move_collector`。进入本文档时，`workflow_id` 必须是 `topic_move_collector`；不得把当前任务改路由到其他 workflow。

## 必读上下文

执行本文档规则前：

1. 按 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 处理身份、认证和权限。
2. 按 [`lark-drive-search.md`](lark-drive-search.md) 处理 `drive +search` 语法、过滤条件、单批最多 5 页和身份语义；本 workflow 的全量续批规则见下文。

## 搜索原则

1. 默认使用 `drive +search --mine` 召回当前用户 owner / 负责的 Workspace 资源。
2. 除非用户本来就要求限定范围，否则不要要求用户指定文件夹或 Wiki 范围。
3. `SEARCH_RECALL` 和 `RECALL_ENHANCE` 必须保持为独立状态。
4. `SEARCH_RECALL` 使用用户原始关键词、`owner_scope` 和显式限制。
5. `RECALL_ENHANCE` 可以基于基础召回证据增加扩展 query，且必须继承同一个 `owner_scope`。
6. 每个候选项必须保留 query 证据，方便后续解释来源。
7. 单页或单个最多 5 页的 query 批次不代表完整覆盖；`has_more=true` 时必须保存 `next_page_token` 并自动开始下一批，直到 `has_more=false` 或出现阻塞。
8. 召回和增强召回可能耗时较长，执行超过 60 秒时必须输出进度提示，之后约每 60 秒提示一次。
9. 只有用户在 `CONFIRM_CONTEXT` 明确确认 `owner_scope=all_visible` 时，才允许移除 `--mine`。

### 分页优先级与完成语义

1. 用户确认进入 `topic_move_collector` 即表示同意为本次收集任务执行完整召回；无需再要求用户额外说“全部 / 全量 / 继续翻”。本规则覆盖 `lark-drive-search.md` 的默认首屏交互规则。
2. 仍遵守 `lark-drive-search.md` 的单轮最多 5 页限制。每读取最多 5 页形成一个批次；批次结束且 `has_more=true` 时，保存 checkpoint，并使用原 query、原过滤条件和返回的 `next_page_token` 自动开始下一批。
3. 自动续批不改变 workflow 状态，也不触发用户确认。执行超过约 60 秒时只输出进度。
4. 一个 query 只有在 `has_more=false` 时才是 `complete`。单批结束、达到 5 页或已有部分候选都不代表完成。
5. 当前状态的全部 query 都为 `complete` 后，才能进入下一状态。认证、权限、无效分页 token、连续重试失败或工具预算不足属于 blocker；必须保留 checkpoint、报告部分召回并停在当前状态，不得把部分结果当成完整召回继续分类。

### QueryRecallState

每个基础 / 增强 query 必须维护：

```json
{
  "query_id": "稳定 query ID",
  "query": "完整 query",
  "recall_stage": "search_recall|recall_enhance",
  "page_count": 0,
  "batch_count": 0,
  "next_page_token": "下一批起点",
  "has_more": true,
  "status": "pending|running|complete|blocked",
  "blocker": "阻塞原因"
}
```

## 状态：`SEARCH_RECALL`

进入条件：用户已确认 `CONFIRM_CONTEXT`。

必须：

1. 基于已确认的 `topic` 构造基础 query。
2. 应用默认 `owner_scope=mine` 和 `constraints` 中的显式限制。
3. 不隐式添加 `--folder-tokens` 或 `--space-ids`。
4. 当 `owner_scope=mine` 时，所有基础 query 必须带 `--mine`。
5. 当 `owner_scope=all_visible` 时，不带 `--mine`，并记录扩展召回风险。
6. 除非命令限制要求更低值，否则使用 `--page-size 20`。
7. 每个基础 query 按每批最多 5 页执行；批次结束仍有更多结果时自动续批，并合并所有页面。
8. 记录基础统计：query、搜索范围、页数、批次数、收集数量、重复数量、阻塞项。
9. 只有全部基础 query 的 `status=complete` 且 `has_more=false` 时，才进入 `RECALL_ENHANCE`；出现阻塞时保持在 `SEARCH_RECALL`。

### 召回进度 UI

当 `SEARCH_RECALL` 或 `RECALL_ENHANCE` 持续超过约 60 秒时，输出当前进度：

```text
搜索进度：当前阶段 <SEARCH_RECALL|RECALL_ENHANCE>，已执行 <query_count> 个 query，已读取 <page_count> 页，收集候选 <raw_count> 项，去重后 <unique_count> 项。继续搜索，不会创建或移动资源。
```

如果正在执行具体 query，可补充：

```text
当前 query：<query>
```

### 基础 Query 规则

| 用户输入 | 基础 Query |
|------------|----------------|
| 单个关键词 | 直接作为 `--query`。 |
| 多个关键词组成一个短语 | 优先按用户输入的短语执行。 |
| 明确精确短语 | 保留引号。 |
| 明确排除词 | 保留负向词。 |
| 没有真实关键词，只有过滤条件 | 使用 `--query ""` 搭配过滤条件。 |

在 `SEARCH_RECALL` 中不得添加同义词、仅标题搜索、仅评论搜索或 OR 扩展。

### 基础召回输出

```text
基础召回完成：
- 使用 query：
- 搜索范围：
- 应用限制：
- 收集候选：
- 去重后候选：
- 阻塞项：

下一步：继续执行覆盖增强，不需要你操作；不会创建或移动资源。
```

## 状态：`RECALL_ENHANCE`

进入条件：基础召回完成。

必须：

1. 基于已确认主题和基础召回证据生成增强 query。
2. 确保增强 query 可解释且不引入明显污染。
3. 每个增强 query 都必须继承 `owner_scope`；`owner_scope=mine` 时必须带 `--mine`。
4. 每个 query 都必须按每批最多 5 页处理分页，并自动续批直到 `has_more=false`。
5. 有稳定去重键时，按稳定去重键合并候选项。
6. 为每个候选项保留 `source_queries` 和命中证据。
7. 当 query 不再产生新候选，或出现工具预算 / API 阻塞时，停止增强。

### 召回阶段退出门禁

`RECALL_ENHANCE` 完成后，必须：

1. 确认全部基础和增强 query 的 `status=complete` 且 `has_more=false`，再固化完整 `candidate_items`，包含去重结果、`source_queries`、`match_channels`、`snippets` 和 `dedupe_status`。
2. 将 `current_state` 设置为 `RESOURCE_RESOLVE`。
3. 加载 [`lark-drive-workflow-topic-move-collector-resolve-verify.md`](lark-drive-workflow-topic-move-collector-resolve-verify.md)。
4. 把完整 `candidate_items` 交给 `RESOURCE_RESOLVE`。
5. 不得直接进入 `RELEVANCE_CLASSIFY`、`PLAN_MOVE` 或展示相关性结果。
6. 不得用搜索标题、摘要或 query 命中直接生成高 / 中 / 低相关分组。

### 增强策略

| 策略 | 说明 |
|----------|------|
| 精确短语 | 对明确短语使用 `"..."` 提高精确命中。 |
| `intitle:` | 对项目名、客户名、制度名、报表名等标题特征强的主题执行标题召回。 |
| `--only-title` | 当标题命中更可信时使用。 |
| `--only-comment` | 当主题可能只出现在评论讨论中时使用。 |
| 类型拆分 | 对 `docx`、`sheet`、`bitable`、`slides`、`file` 等分类型搜索，减少服务端排序偏差。 |
| 同义词 / 别名 | 使用业务上明确的同义词、简称、英文名、中文名。 |
| OR 扩展 | 对同一实体的别名做 OR 扩展。 |
| 负向词 | 对明显噪声使用 `-term`，但不能排除可能相关的主题词。 |

### Query 证据

每个候选项都要记录：

| 字段 | 说明 |
|-------|------|
| `source_queries` | 命中过该资源的 query 列表。 |
| `match_channels` | 命中位置，如 title、body、comment、metadata。 |
| `snippets` | 搜索返回的摘要或片段。 |
| `query_rank` | 资源在各 query 中的相对位置。 |
| `recall_stage` | `search_recall` 或 `recall_enhance`。 |

## 去重规则

必须：

1. 搜索响应提供 canonical token 时，优先使用 canonical token。
2. 对 Wiki 结果，不得只按 object token 去重；同一对象可能出现在多个 Wiki 节点中。
3. token 缺失时，使用 URL 作为 fallback。
4. 合并重复项时保留所有 query 证据。
5. 如果无法确定去重是否稳定，保留该项并设置 `dedupe_status=uncertain`。

## CandidateItem

```json
{
  "title": "资源标题",
  "url": "资源链接",
  "raw_type": "搜索返回类型",
  "source_queries": ["query"],
  "match_channels": ["title|body|comment|metadata"],
  "snippets": ["命中片段"],
  "page_rank": 1,
  "dedupe_key": "候选去重键",
  "dedupe_status": "stable|fallback|uncertain",
  "recall_stage": "search_recall|recall_enhance"
}
```

| 字段 | 说明 |
|-------|------|
| `title` | 搜索结果标题。 |
| `url` | 资源访问链接。 |
| `raw_type` | 搜索返回的原始类型。 |
| `source_queries` | 命中过该资源的搜索 query。 |
| `match_channels` | 命中位置。 |
| `snippets` | 摘要或命中片段。 |
| `page_rank` | 当前 query 下的排序位置。 |
| `dedupe_key` | 候选去重键。 |
| `dedupe_status` | 去重可信度。 |
| `recall_stage` | 资源首次进入候选集的召回阶段。 |

## 阻塞项

缺少认证 / scope、`drive +search` 返回权限或策略阻塞、分页 token 无效、分页重试后仍无法继续，或工具预算不足以完成全部页面时，必须把对应 `QueryRecallState.status` 设置为 `blocked`，保留累计候选、页数和 `next_page_token`，停止并报告。阻塞解除后从 checkpoint 续跑；在全部 query 完成前不得进入资源解析或分类阶段。
