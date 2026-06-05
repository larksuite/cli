# 知识整理工作流：Discovery

Loaded by states: `PARSE_SCOPE`, `INVENTORY`.

This file owns target parsing, scope clarification, resource inventory, ResourceItem normalization, dedupe, and partial inventory handling. It MUST NOT generate classification rules, execution plans, or perform write operations.

## Required Context

Before executing rules in this file:

1. Follow [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) for identity, auth, and permission handling.
2. For Wiki / personal library targets, follow [`../../lark-wiki/SKILL.md`](../../lark-wiki/SKILL.md).
3. For Drive folder inventory, follow [`lark-drive-files-list.md`](lark-drive-files-list.md).
4. For Drive search targets, follow [`lark-drive-search.md`](lark-drive-search.md).
5. For URL / token inspection, follow [`lark-drive-inspect.md`](lark-drive-inspect.md) and [`../../lark-wiki/references/lark-wiki-node-get.md`](../../lark-wiki/references/lark-wiki-node-get.md).

## State: PARSE_SCOPE

Entry: workflow triggered.

MUST:

1. Identify `target_scope`, `environment_profile`, and `identity`.
2. Apply `Scope Parsing`.
3. Output `Scope Confirmation`.
4. Stop and wait for user confirmation before `INVENTORY`.

Exit: user confirms target scope.

### Scope Parsing

| Condition | Agent MUST Do | Set `target_scope` | Next State |
|-----------|---------------|--------------------|------------|
| Input is `/wiki/<token>` URL | Resolve the Wiki node and preserve both node identity and object identity | Wiki node | `INVENTORY` after user confirms scope |
| Input is Wiki space name / `space_id` | Resolve the Wiki space; 0 matches -> stop and ask; 1 exact match -> continue; multiple matches -> show candidates and wait for user selection; do not treat `my_library` as a normal listed space | Wiki space | `INVENTORY` after user confirms scope |
| Input has Personal Library Intent | Treat as Wiki personal library / `my_library`; resolve real `space_id` before root write; do not treat it as Drive root or owned Drive document search | Personal doc library | `INVENTORY` after user confirms scope |
| Input is `/drive/folder/<token>` URL | Extract `folder_token` | Drive folder | `INVENTORY` after user confirms scope |
| Input has Drive Folder Intent but no concrete folder URL, token, or unique folder name | Ask for folder URL / token / name; if a concrete folder name exists, search folder candidates and wait for user selection when 0 or multiple matches exist | Unknown or Drive folder candidate | Stay in `PARSE_SCOPE` until scope is confirmed |
| Input has Broad Cloud Drive Intent without explicit owned-document search request | Ask the user to choose concrete scope: Drive folder URL / token, Drive root, owned Drive document search, or another explicit search filter; do not default to `drive +search --mine` | Unknown | Stay in `PARSE_SCOPE` until scope is confirmed |
| Input is single cloud resource URL | Resolve the resource type; if not folder / Wiki scope, do not expand automatically | Single resource | Ask whether scope is this resource, parent folder, owning Wiki, or related search results |
| Input is real keyword / name | Search with the real keyword according to `lark-drive-search` | Search scope | `INVENTORY` after user confirms scope |
| Input is range browsing / statistical description with no real keyword | Search by filters / empty-query browsing according to `lark-drive-search` | Search scope | `INVENTORY` after user confirms scope |
| Input is ambiguous | Ask the minimum clarification question and stop | Unknown | Stay in `PARSE_SCOPE` |

Personal Library Intent means the user is referring to the current user's own Feishu document library / personal document library / personal knowledge library, such as `个人文档库`, `飞书个人文档库`, `我的文档库`, `个人知识库`, `我的知识库`, `My Document Library`, or `my_library`.

When this intent is detected, use Wiki personal library semantics. Do not use Drive root, `drive +search --mine`, or broad owned-document search unless the user explicitly asks to search owned Drive documents.

Drive Folder Intent means the user wants to organize a specific Drive folder or Drive folder tree. A Drive folder scope requires a concrete folder URL, folder token, or user-selected folder candidate.

When this intent is detected without a concrete folder identity, stop in `PARSE_SCOPE` and ask for clarification. Do not use Drive root, `drive +search --mine`, or broad owned-document search unless the user explicitly asks for Drive root or owned-document search.

Broad Cloud Drive Intent means the user refers to a broad cloud-drive-level scope such as `我的飞书云盘`, `我的云盘`, `我的云空间`, `我的空间`, or `整理云盘`, without a concrete folder URL / token / unique folder name.

This intent is broader than Drive Folder Intent and MUST NOT be silently converted to owned-document search. Ask the user to choose one of:

1. A specific Drive folder URL / token.
2. Drive root, only when the user explicitly accepts root-level scope.
3. Owned Drive document search, only when the user explicitly asks to organize documents owned / managed by the current user.
4. Another explicit search filter, such as keyword, type, time range, or folder token.

### Stop Conditions

Stop and ask for clarification when:

1. 用户只说"整理文件夹"、"整理目录"、"整理资料"、"整理文档"、"我的文档"，且没有 URL、token、知识库名称、Personal Library Intent、concrete Drive folder identity 或明确搜索范围。
2. 用户说"我的文件夹"、"我的目录"、"我的空间"、"我的云盘"、"我的飞书云盘"、"我的云空间"，但无法唯一判断是具体 Drive 文件夹、Drive 根目录、owned Drive document search、个人文档库还是某个 Wiki 节点。
3. 用户给的是单个资源 URL，但要求"整理一批文档"或"整理相关资料"。
4. 用户目标环境不明确，且上下文中同时存在线上、BOE、PRE 或多个 profile。

Clarification template:

```text
请提供要整理的 Drive 文件夹链接、Wiki 节点 / 知识库链接，或明确说明要整理"我的文档库"；如果只想按关键词搜索整理，也请给出关键词或范围。
```

### Scope Confirmation

```text
我先确认本次整理范围。

目标：
范围：
环境 / profile：
身份：
预计操作：先盘点并生成整理方案，不执行移动或创建。

请确认是否按这个范围继续？
```

Scope confirmation is user-facing. It MUST confirm only the business scope, environment / profile, identity, and whether write operations will run.

Do not display internal batching controls in scope confirmation, including `max_depth`, `max_items`, `page_size`, page tokens, retry counts, or `partial=true`. For example, when the user confirms Drive root, say the scope is the Drive root tree; do not append "recursive depth at most 3" or "at most 500 resources".

## State: INVENTORY

Entry: `target_scope` confirmed.

MUST:

1. Recursively list resources according to target type.
2. Generate `path` during traversal.
3. Normalize all results to `ResourceItem`.
4. Track pagination, depth, item limits, and continuation checkpoints.
5. Treat pagination, depth, item, and per-folder page limits as batching checkpoints; continue inventory in the confirmed scope unless blocked.
6. Set `partial=true` only when inventory cannot continue because of auth, permission, API / pagination failure after retries, API coverage limitations, tool budget, target scope, or environment blockers.
7. Apply `Inventory Progress Reporting`.
8. Output `Inventory Summary`.
9. Do not leave `INVENTORY` while `inventory_continuation_state` has queued folders, nodes, pages, or slices that can still be fetched.
10. Continue to `CONTENT_READ` without asking the user only after the confirmed scope is exhausted or blocked.

### Inventory Batch Checkpoints

| Scope | Internal Batch Checkpoint | Required Continuation |
|-------|---------------------------|-----------------------|
| Wiki recursion | `max_depth=3`, `max_items=500`; follow `lark-wiki-node-list` pagination | Record queued nodes / paths in `inventory_continuation_state` and immediately continue the next internal batch within the confirmed scope unless blocked |
| Drive folder tree | `max_depth=3`, `max_items=500`, max 10 pages per folder, `page_size=200` | Record queued folders / pages in `inventory_continuation_state` and immediately continue the next internal batch within the confirmed scope unless blocked |
| Search discovery | `page_size=20`, `max_items=500`; continue pages until `has_more=false` | Record remaining pages / slices in `inventory_continuation_state` and immediately continue the next internal batch within the confirmed scope unless blocked |

These checkpoints are pacing controls, not coverage limits. If the confirmed scope still has queued work after a checkpoint, continue with the next internal batch instead of presenting the current `resource_items` as final inventory or moving to content analysis.

When a depth checkpoint is reached, enqueue the child folders / nodes that would exceed the current batch depth; the next batch starts from those queued children with their original paths preserved. When an item checkpoint is reached, persist the current folder / node / page cursor plus the remaining queue, visited page keys, and resource dedupe keys, then continue from that checkpoint before analysis or planning.

If tool budget would be exceeded for a very large confirmed scope, stop only at that blocker, report that the inventory is incomplete, and suggest batching by first-level directory, Wiki space, or time window. Do not stop merely because a depth or item checkpoint was reached.

### Inventory Continuation Rules

1. Pagination, depth, item, and per-folder page limits are internal batching checkpoints.
2. When a checkpoint is reached, record `inventory_continuation_state` with `scope`, `queue`, `current_cursor`, `visited_page_keys`, `dedupe_keys`, and `blockers`; Drive queue entries MUST contain `folder_token`, `path`, `depth`, and `page_token`; Wiki queue entries MUST contain `space_id` / `node_token`, `path`, `depth`, and pagination cursor; search entries MUST contain query / filters and pagination cursor.
3. A depth checkpoint MUST enqueue deeper folders / nodes; it MUST NOT discard them or treat the current depth as final coverage.
4. An item-count checkpoint MUST persist the current cursor and queue; it MUST NOT transition to `CONTENT_READ`, `ISSUE_ANALYSIS`, or `PLAN_GENERATION` while fetchable work remains.
5. If `inventory_continuation_state` is missing, corrupt, or lacks required fields for the current scope, set `partial=true`, record the checkpoint blocker, and do not claim full coverage.
6. Do not set `partial=true` solely because a valid batching checkpoint was reached.
7. Set `partial=true` only when continuation is blocked by auth, permission, API / pagination failure after retries, API coverage limitations, tool budget, target scope, or environment blockers.
8. Do not claim full coverage until the continuation queue for the confirmed scope is exhausted or blocked.

### Inventory Progress Reporting

Inventory can be long-running when a Drive root, large folder tree, Wiki space, or broad search scope is confirmed.

Rules:

1. When inventory starts, output one concise stage notice with the confirmed scope type and the fact that no write operation will be executed.
2. If inventory runs longer than about 60 seconds, output progress about every 60 seconds.
3. Progress reports SHOULD include only fields that are currently known: scanned folders / nodes, collected resources, current depth, queued folders / nodes, current search page / slice, and current blocker if any.
4. When a batching checkpoint is reached and continuation will proceed automatically, report it as continuing inventory, not as a user action request.
5. Do not output filler such as "still running" without current counts or current stage.
6. Do not expose raw folder tokens, page tokens, retry logs, or `partial=true` unless the user explicitly asks to view inventory coverage details.

Example:

```text
盘点进度：已扫描 <scanned_container_count> 个目录 / 节点，收集 <resource_count> 项资源，队列剩余 <queued_container_count> 个目录 / 节点。继续盘点，不会执行移动或创建。
```

### Wiki Inventory Rules

1. Follow [`../../lark-wiki/references/lark-wiki-node-list.md`](../../lark-wiki/references/lark-wiki-node-list.md) traversal semantics.
2. Generate stable paths from parent-child traversal.
3. Preserve Wiki node identity fields needed by `ResourceItem`.
4. Treat `my_library` as Wiki personal library, not Drive root.

### Drive Inventory Rules

1. Use `drive files list` according to [`lark-drive-files-list.md`](lark-drive-files-list.md); its schema path is `drive.files.list`.
2. Use the same Drive folder-tree traversal for Drive root and ordinary folders after the first request. Drive root differs only for the first-level request: it uses omitted or empty `folder_token`, does not support pagination, and does not return root-level shortcuts according to schema; returned child folders MUST still be listed by their own folder tokens like ordinary folders, and those ordinary folder lists may return `type=shortcut` entries. For a Drive root target, record this root-level shortcut coverage caveat, set `partial=true` only if the user requested full root-level shortcut coverage or root pagination cannot continue, and do not claim root-level shortcut coverage as complete.
3. Recurse only into `folder` items within the confirmed scope.
4. For each directory, continue pages manually by feeding the returned `next_page_token` into request param `page_token`. Do not rely on `--page-all` for inventory.
5. If a page returns `has_more=true` but no usable `next_page_token`, retry the same page request up to 3 times. If retries still cannot produce a continuation token, set `partial=true` for that directory and record the pagination blocker.
6. Use `drive metas batch_query` when URL, owner, created time, or updated time is needed.
7. Pagination blocker details such as `partial=true`, folder token, page token, and retry logs are internal by default. Do not show them to the user unless the user explicitly asks to view inventory coverage details.

### Search Inventory Rules

1. Search results may be normalized directly only when they include stable identity fields required by `ResourceItem`.
2. If a search result is a Wiki item and lacks `node_token`, resolve it with `drive +inspect` or `wiki +node-get` before dedupe.
3. If Wiki identity still cannot be resolved, keep the item, set `needs_review=true`, and record `needs_review_reason`.
4. For search scope, use `page_size=20` unless a lower value is required by the command.
5. Continue fetching pages until `has_more=false`.
6. If `max_items=500` is reached in one batch, record the current search cursor in `inventory_continuation_state` and continue the next internal batch without asking the user.
7. Do not stop at an arbitrary sample size such as first 5 pages unless the user explicitly asks for sampling or auth, permission, API, environment, or tool-budget blockers occur.
8. If `service_total` / result total is greater than collected items, treat it as continuation evidence: continue fetching when a cursor / page is available; set `partial=true` only if continuation is blocked.
9. Do not present a partial search sample as complete inventory. Before generating a full organization plan from partial search results, continue fetching available pages unless the user explicitly asked for sampling or a blocker prevents continuation.

## ResourceItem

Agent MUST normalize Wiki, Drive, and search results into `ResourceItem`. Later statistics, classification, and planning MUST use this model rather than raw API responses.

```json
{
  "source": "wiki|drive|search",
  "title": "资源标题",
  "type": "doc|docx|sheet|bitable|mindnote|file|wiki|folder|slides|shortcut|catalog",
  "path": "当前路径/资源标题",
  "depth": 2,
  "url": "https://...",
  "token": "canonical_token",
  "node_token": "wiki_node_token_or_empty",
  "obj_token": "wiki_obj_token_or_drive_file_token",
  "node_type": "origin|shortcut|empty",
  "origin_node_token": "wiki_origin_node_token_or_empty",
  "space_id": "wiki_space_id_or_empty",
  "parent_token": "parent_node_or_folder_token",
  "has_child": false,
  "dedupe_key": "wiki:<space_id>:<node_token>|drive:<type>:<token>|search:<type>:<token>",
  "created_at": "optional",
  "updated_at": "optional",
  "needs_review": false,
  "needs_review_reason": ""
}
```

ResourceItem rules:

1. `path` MUST be generated by recursion. Do not use title alone as path.
2. Wiki URL token may not be the underlying document token. Preserve both `node_token` and `obj_token`.
3. `type` MUST come from API fields such as `obj_type` / `doc_type`.
4. Wiki organization is by node instance. Prefer `wiki:<space_id>:<node_token>` as `dedupe_key`.
5. MUST NOT dedupe Wiki nodes only by `obj_token`; one document can appear under different Wiki paths or shortcuts.
6. If `node_type=shortcut` or dedupe is uncertain, use `wiki +node-get` to supplement `origin_node_token`; if unavailable, leave empty and set `needs_review=true`.
7. Drive folder tree dedupes by `drive:<type>:<token>`.
8. Search results may merge with recursive results only by exact identity: Wiki by same `node_token`, Drive by same `type + token`.

## Inventory Summary

```text
已完成当前可覆盖范围盘点。

<仅当适用：覆盖说明：Drive 根目录第一层清单不返回快捷方式；本次盘点不包含根目录第一层快捷方式。根目录下子文件夹会按普通文件夹继续盘点，普通文件夹内返回的 `type=shortcut` 条目仍会被纳入资源清单。>

| 指标 | 数量 |
|------|------|
| 总资源数 |  |
| 各类型资源数 |  |
| 一级目录数量 |  |
| 根目录直接资源数 |  |
| 空目录数量 |  |
| 疑似临时 / 测试 / 未整理资源数 |  |
| 低置信度待确认资源数 |  |

下一步将自动读取低置信度资源并分析整理问题；不会执行移动或创建。
```

## Discovery Failure Handling

| Failure / Blocker | Agent MUST Do | Agent MUST NOT Do |
|-------------------|---------------|-------------------|
| Target scope is ambiguous | Ask the minimum scope clarification question and stop | Do not choose a whole cloud drive / personal library by default |
| Environment / profile is ambiguous | Ask user to confirm prod / BOE / PRE and profile | Do not cross environment boundaries |
| Missing API scope | Follow `lark-shared` permission handling and stop | Do not retry the same command repeatedly |
| Resource access denied | Stop and follow the main workflow `Permission Request Gate` | Do not request permission automatically or in batch |
| Pagination / depth / item checkpoint reached | Record `inventory_continuation_state` and continue inventory in the confirmed scope | Do not set `partial=true` solely because a batching checkpoint was reached |
| Pagination cursor missing after retries / API pagination failure | Set `partial=true`; record the affected directory and blocker | Do not loop indefinitely or claim full coverage |
