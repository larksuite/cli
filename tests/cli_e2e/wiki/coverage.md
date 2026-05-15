# Wiki CLI E2E Coverage

## Metrics
- Denominator: 6 leaf commands + 3 shortcut commands
- Covered: 9
- Coverage: 100.0%

## Summary
- TestWiki_NodeWorkflow: proves the full currently-tested bare-API surface; key `t.Run(...)` proof points are `create node as bot`, `get created node as bot`, `get space as bot`, `list spaces as bot`, `list nodes and find created node as bot`, `copy node as bot`, and `list nodes and find copied node as bot`.
- TestWiki_ShortcutWorkflow: covers the shortcut layer for `wiki +space-list`, `wiki +node-list`, and `wiki +node-copy` — flag→body mapping, envelope shape (`{spaces|nodes, has_more, page_token}` + `meta.count`), `--page-all` / `--page-limit` truncation, my_library alias resolution (user positive + bot validation rejection), and copy-source-survival.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | wiki nodes copy | api | wiki_workflow_test.go::TestWiki_NodeWorkflow/copy node as bot | `space_id`; `node_token` in `--params`; target/title in `--data` | |
| ✓ | wiki nodes create | api | wiki_workflow_test.go::TestWiki_NodeWorkflow/create node as bot | `space_id` in `--params`; `node_type`; `obj_type`; `title` in `--data` | |
| ✓ | wiki nodes list | api | wiki/helpers_test.go::findWikiNodeByToken; wiki_workflow_test.go::TestWiki_NodeWorkflow/list nodes and find created node as bot; wiki_workflow_test.go::TestWiki_NodeWorkflow/list nodes and find copied node as bot | `space_id`; `page_size`; optional `page_token` | |
| ✓ | wiki spaces get | api | wiki_workflow_test.go::TestWiki_NodeWorkflow/get space as bot | `space_id` in `--params` | |
| ✓ | wiki spaces get_node | api | wiki_workflow_test.go::TestWiki_NodeWorkflow/get created node as bot | `token`; `obj_type` in `--params` | |
| ✓ | wiki spaces list | api | wiki_workflow_test.go::TestWiki_NodeWorkflow/list spaces as bot | `page_size` in `--params` | |
| ✓ | wiki +space-list | shortcut | wiki_shortcut_workflow_test.go::TestWiki_ShortcutWorkflow/+space-list: stable envelope shape | `--page-size`; `--format json`; bot identity | |
| ✓ | wiki +node-list | shortcut | wiki_shortcut_workflow_test.go::TestWiki_ShortcutWorkflow/+node-list: finds child under parent; +node-list: --page-limit caps the loop and exposes cursor; +node-list --space-id my_library --as bot: validation rejection; +node-list --space-id my_library --as user: resolves and lists | `--space-id`; `--parent-node-token`; `--page-all`; `--page-size`; `--page-limit`; my_library alias | |
| ✓ | wiki +node-copy | shortcut | wiki_shortcut_workflow_test.go::TestWiki_ShortcutWorkflow/+node-copy: copies child + verifies source survives + cleanup | `--space-id`; `--node-token`; `--target-space-id`; `--title` | |
