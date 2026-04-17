# Wiki CLI E2E Coverage

## Metrics
- Denominator: 6 leaf commands
- Covered: 6
- Coverage: 100.0%

## Summary
- TestWiki_NodeWorkflow: proves the full currently-tested wiki domain surface; key `t.Run(...)` proof points are `create node as bot`, `get created node as bot`, `get space as bot`, `list spaces as bot`, `list nodes and find created node as bot`, `copy node as bot`, and `list nodes and find copied node as bot`.
- The workflow covers both node creation/copy/listing and space lookup/listing with persisted token assertions.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | wiki nodes copy | api | wiki_workflow_test.go::TestWiki_NodeWorkflow/copy node as bot | `space_id`; `node_token` in `--params`; target/title in `--data` | |
| ✓ | wiki nodes create | api | wiki_workflow_test.go::TestWiki_NodeWorkflow/create node as bot | `space_id` in `--params`; `node_type`; `obj_type`; `title` in `--data` | |
| ✓ | wiki nodes list | api | wiki/helpers_test.go::findWikiNodeByToken; wiki_workflow_test.go::TestWiki_NodeWorkflow/list nodes and find created node as bot; wiki_workflow_test.go::TestWiki_NodeWorkflow/list nodes and find copied node as bot | `space_id`; `page_size`; optional `page_token` | |
| ✓ | wiki spaces get | api | wiki_workflow_test.go::TestWiki_NodeWorkflow/get space as bot | `space_id` in `--params` | |
| ✓ | wiki spaces get_node | api | wiki_workflow_test.go::TestWiki_NodeWorkflow/get created node as bot | `token`; `obj_type` in `--params` | |
| ✓ | wiki spaces list | api | wiki_workflow_test.go::TestWiki_NodeWorkflow/list spaces as bot | `page_size` in `--params` | |
