# 知识整理工作流：Analysis

Loaded by states: `CONTENT_READ`, `ISSUE_ANALYSIS`, `RULE_GENERATION`.

This file owns low-confidence partial reads, issue analysis, classification rules, and target tree generation. It MUST NOT create execution plans, ask for execution confirmation, or perform write operations.

## Required Context

Before executing rules in this file:

1. `resource_items` MUST already exist from [`lark-drive-workflow-knowledge-organize-discovery.md`](lark-drive-workflow-knowledge-organize-discovery.md).
2. For document partial reads, follow [`../../lark-doc/SKILL.md`](../../lark-doc/SKILL.md) and [`../../lark-doc/references/lark-doc-fetch.md`](../../lark-doc/references/lark-doc-fetch.md).
3. For sheet / bitable down-drill, follow [`../../lark-sheets/SKILL.md`](../../lark-sheets/SKILL.md) or [`../../lark-base/SKILL.md`](../../lark-base/SKILL.md) only when title and path are insufficient.

## State: CONTENT_READ

Entry: `resource_items` exists.

MUST:

1. Build `low_confidence_items`.
2. Apply `Low-Confidence Partial Read`.
3. Read only supported docs through `lark-doc-fetch`.
4. Switch to `lark-sheets` / `lark-base` only when sheet / bitable title and path are insufficient.
5. Record read evidence for classification.
6. Continue reading low-confidence resources in internal batches until all supported low-confidence resources in the current inventory are processed or a blocker occurs.
7. Output progress / summary without asking the user to continue between batches.

Exit: low-confidence items are classified or marked `needs_review=true`.

### Low-Confidence Partial Read

Low-confidence resources include:

- 标题为空
- 标题为 `test` / `测试` / 纯数字 / 无意义短词
- 标题、路径、类型之间没有足够分类线索
- 同一标题或相似标题出现在多个候选分类中
- 用户要求按项目 / 客户 / 业务线归类，但标题和路径没有明确项目 / 客户 / 业务线名称

| Condition | Agent MUST Do | Agent MUST NOT Do |
|-----------|---------------|-------------------|
| Title / path / type clearly determine classification | Classify directly | Do not perform content read |
| Resource is low-confidence and docs-fetch-supported | Read outline via `lark-doc-fetch` | Do not skip partial read |
| Candidate project / customer / business / document-type terms exist | After outline, run keyword partial read with candidate terms | Do not use broad generic keywords |
| Partial read returns usable block id and classification is still unclear | Read the relevant section via `lark-doc-fetch` | Do not read the full document |
| Partial read still cannot classify | Set `needs_review=true`; classify to manual confirmation target | Do not invent classification |
| Read fails or permission is insufficient | Set `needs_review=true`; record failure reason | Do not retry indefinitely |

### Partial Read Limits

| Limit | Default |
|-------|---------|
| `batch_size` | 20 resources per internal batch |
| `progress_report_interval` | 50 low-confidence resources |
| `max_attempts_per_resource` | 3 partial reads: outline, keyword, section |

Batching rules:

1. Sort low-confidence resources by impact before reading: root-level loose items, duplicated titles, project/customer ambiguity, then empty or meaningless titles.
2. Read supported low-confidence resources across internal batches without asking the user to continue after each batch.
3. Process reads in internal batches of `batch_size`; do not ask the user between internal batches unless auth, permission, or API errors block progress.
4. After each internal batch, update `low_confidence_items` with read evidence or `needs_review=true`.
5. After every `progress_report_interval` processed resources, output a progress summary and continue automatically.
6. If unread low-confidence resources remain because of auth, permission, API, unsupported type, or tool budget blockers, set `partial=true`, report unread count, and default remaining unread items to `needs_review=true` with target path set to manual confirmation target.
7. Never bypass these limits by reading full documents.

### Low-Confidence Read Start Notice

When `low_confidence_total > 100`, output this notice before reading:

```text
低置信度资源较多，共 <low_confidence_total> 项。我会分批做轻量读取并定期汇报进度；不会读取全文，也不会执行移动或创建。
```

### Low-Confidence Read Summary

Use this as progress / final summary output. Do not ask the user to continue unless a blocker occurs.

```text
低置信度内容读取进度

- 低置信度资源总数：<low_confidence_total>
- 已读取：<read_done>/<low_confidence_total>
- 已补充证据并完成分类：<classified_count>
- 暂入待人工确认：<needs_review_count>
- 失败：<failed_count>

继续分析整理问题。
```

Output this summary:

- After every 50 processed low-confidence resources.
- Once after low-confidence reading finishes.

## State: ISSUE_ANALYSIS

Entry: `resource_items` and partial-read evidence are ready.

MUST:

1. Detect problems from organization perspective only. Do not generate research conclusions.
2. Generate an organization approach based on inventory, low-confidence read evidence, and detected problems.
3. Include how non-reused source containers will be handled after their contents are moved.
4. Output `Inventory And Organization Approach Decision`.
5. Stop and wait for the user to confirm the approach before `RULE_GENERATION`.

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
| 已完成低置信度读取 |  |
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
A. 基于这个思路生成目标目录和计划
B. 调整整理思路
C. 查看问题详情
D. 取消本次整理
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
9. Continue to `PLAN_GENERATION` without a separate target-tree-only confirmation.

### Classification

| Condition | Agent MUST Do |
|-----------|---------------|
| Existing structure is clear | Reuse existing directory names and hierarchy |
| Title / path / type is enough | Classify without content read |
| Item remains uncertain after mandatory partial read | Put into manual confirmation target and set `needs_review=true` |
| Item is temporary / test / draft | Prefer temporary / test target |
| Root has many loose resources | Prefer organizing root-level obvious items first |
| User asks project / customer grouping | Use project / customer names from title, path, and partial read evidence |
| Naming is inconsistent | Report the issue with examples only; do not generate rename actions |

### Adaptive Classification

The agent MUST NOT start from a fixed default category list. A fixed taxonomy can bias classification and confuse users when category names or numeric prefixes do not match their resources.

Derive categories from the current `resource_items` and partial-read evidence:

1. First group resources by clear signals from title, current path, type, and mandatory partial-read evidence.
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
