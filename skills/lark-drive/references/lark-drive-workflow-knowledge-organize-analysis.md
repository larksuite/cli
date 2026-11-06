# 知识整理工作流：Analysis

Loaded by states: `ISSUE_ANALYSIS`, `RULE_GENERATION`.

This file owns issue analysis, classification rules, and target tree generation. It MUST NOT create execution plans, ask for execution confirmation, or perform write operations.

## Required Context

Before executing rules in this file:

1. `resource_items` MUST already exist from [`lark-drive-workflow-knowledge-organize-discovery.md`](lark-drive-workflow-knowledge-organize-discovery.md).
2. For document partial reads, follow [`../../lark-doc/SKILL.md`](../../lark-doc/SKILL.md) and [`../../lark-doc/references/lark-doc-fetch.md`](../../lark-doc/references/lark-doc-fetch.md).
3. For sheet / bitable down-drill, follow [`../../lark-sheets/SKILL.md`](../../lark-sheets/SKILL.md) or [`../../lark-base/SKILL.md`](../../lark-base/SKILL.md) only when user-required content conditions need reading.

### Analysis Progress Reporting

Applies to `ISSUE_ANALYSIS` and `RULE_GENERATION`.

Rules:

1. For `ISSUE_ANALYSIS`, if analysis runs longer than about 60 seconds, output progress about every 60 seconds with current stage, processed resource count when known, detected problem type count when known, and the next analysis step.
2. For `RULE_GENERATION`, if classification rule or target-tree generation runs longer than about 60 seconds, output progress about every 60 seconds with current stage, classified item count when known, unresolved item count when known, and target category / path count when known.
3. Progress reports MUST be factual and stage-specific. Do not output generic "still running" messages without counts or the current stage.
4. Do not ask the user to continue between internal batches unless auth, permission, API, target scope, or environment blockers occur.
5. Do not expose internal chain-of-thought, raw tokens, or intermediate rule drafts.

Examples:

```text
分析进度：正在归纳整理问题，已处理 <processed_count>/<resource_count> 项资源，已识别 <problem_type_count> 类问题。继续生成整理思路，不会执行移动或创建。
```

```text
规则生成进度：正在生成分类规则和目标目录，已归类 <classified_count> 项，待人工确认 <needs_review_count> 项。继续生成完整计划前置数据。
```

## State: ISSUE_ANALYSIS

Entry: `resource_items` is ready.

Use existing title, path, type, and search evidence. Read content only when needed to determine user-required content conditions. Mark insufficient classification evidence as `needs_review=true`; do not read every low-confidence resource or mark `partial=true` solely because optional content was not read.

MUST:

1. Identify `low_confidence_items` and detect problems from organization perspective only. Do not generate research conclusions.
2. Generate an organization approach based on inventory, available evidence, and detected problems.
3. Include how non-reused source containers will be handled after their contents are moved.
4. Apply `Analysis Progress Reporting`.
5. Output `Inventory And Organization Approach Decision`.
6. Stop and wait for the user to confirm the approach before `RULE_GENERATION`.

Problem rules:

| Problem | Detection Rule |
|---------|----------------|
| 根目录堆积 | 根目录直接资源过多，或超过总资源的明显比例 |
| 同类文件分散 | 标题 / 类型相似的资源分布在多个无关路径 |
| 命名不统一 | 同类资源日期、客户、项目命名格式明显不一致 |
| 临时内容过多 | 标题 / 路径含 `临时`、`测试`、`tmp`、`draft`、`转移`、`未整理` |
| 空目录 | 目录类节点无后代资源 |
| 重复目录 | 目录名归一化后相同或高度相似 |
| 过旧归档内容 | 旧年份资源仍散落在活跃目录 |

MUST output evidence count or example paths. Do not output only abstract judgment.

### Problem Pagination

| Output Area | Rule |
|-------------|------|
| Problem overview | Show at most 5 problem types per page |
| Problem examples | Show at most 3 example paths per problem type |
| Pagination | Affects display only; complete `issue_summary` MUST remain internal |

### Inventory And Organization Approach Decision

```text
盘点与整理思路

盘点结果：
| 指标 | 数量 |
|------|------|
| 总资源数 |  |
| 各类型资源数 |  |
| 一级目录数量 |  |
| 根目录直接资源数 |  |
| 空目录数量 |  |
| 低置信度资源数 |  |
| 待人工确认 |  |
| partial |  |

共发现 <problem_type_count> 类问题，当前展示第 <page>/<total_pages> 页。

| 问题 | 证据数量 | 样例路径 | 说明 |
|------|----------|----------|------|

整理思路：
- <approach item 1>
- <approach item 2>
- 对证据不足、读取失败或权限不足的资源放入"待人工确认"
- 如存在不再复用的来源目录，内容迁出后将目录本体收起到 `待人工确认/待清理旧目录`，避免整理后一级目录仍杂乱
- 不删除、不重命名、不修改权限

是否基于这个整理思路生成目标目录和移动 / 创建计划？

你可以选择：
1. 基于这个思路生成目标目录和计划
2. 调整整理思路
3. 查看问题详情
4. 取消本次整理
```

## State: RULE_GENERATION

Entry: user confirms the organization approach.

MUST:

1. Generate `classification_rules`.
2. Generate `target_tree`.
3. Generate `target_tree` to at least two levels; include third level when needed for project / customer / document-type grouping.
4. Reuse existing clear structure when possible.
5. Identify reused top-level containers and non-reused source containers, and set `source_container_disposition`.
6. For non-reused source containers, ensure `target_tree` includes a source-container cleanup target, defaulting to `待人工确认/待清理旧目录`, unless the user explicitly asks to keep source containers in place.
7. Ensure target tree can contain every planned `target_path`.
8. Ensure the target tree contains a manual confirmation target named `待人工确认` unless the user explicitly provides an equivalent name.
9. Apply `Analysis Progress Reporting`.
10. Continue to `PLAN_GENERATION` without a separate target-tree-only confirmation.

### Classification

| Condition | Agent MUST Do |
|-----------|---------------|
| Existing structure is clear | Reuse existing directory names and hierarchy |
| Title / path / type is enough | Classify without content read |
| Item classification is uncertain | Put into manual confirmation target and set `needs_review=true` |
| Item is temporary / test / draft | Prefer temporary / test target |
| Root has many loose resources | Prefer organizing root-level obvious items first |
| User asks project / customer grouping | Use project / customer names from title, path, and partial read evidence |
| Naming is inconsistent | Report the issue with examples only; do not generate rename actions |

### Adaptive Classification

The agent MUST NOT start from a fixed default category list. A fixed taxonomy can bias classification and confuse users when category names or numeric prefixes do not match their resources.

Derive categories from the current `resource_items` and partial-read evidence:

1. First group resources by clear signals from title, current path, type, and available evidence.
2. Prefer category names that appear in the user's own content, such as project names, customer names, business lines, document types, years, or existing folder / Wiki node names.
3. Create a category only when there is enough evidence for at least one resource.
4. Do not create generic buckets such as archive, temporary, test, meeting, dashboard, or operations unless the current resources contain matching evidence.
5. Do not add numeric prefixes to category names unless the user explicitly asks for ordered naming.
6. Always keep a manual confirmation target named `待人工确认` or an equivalent user-specified name for unresolved items.

### Target Tree

`target_tree` is generated in this state but shown together with the move / create plan in `PLAN_GENERATION`. Do not stop after displaying a target tree alone.

## Analysis Failure Handling

| Failure / Blocker | Agent MUST Do | Agent MUST NOT Do |
|-------------------|---------------|-------------------|
| Missing API scope | Follow `lark-shared` permission handling and stop | Do not retry the same command repeatedly |
| Resource access denied | Stop and follow the main workflow `Permission Request Gate` | Do not request permission automatically or in batch |
| Partial document read fails for a low-confidence item | Mark item `needs_review=true`, record reason, and route to manual confirmation target | Do not classify by guessing |
| Item remains ambiguous after partial read | Mark `needs_review=true` and route to manual confirmation target | Do not invent classification |
