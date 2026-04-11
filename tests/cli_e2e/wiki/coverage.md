# Wiki CLI E2E Coverage

## Metrics
- Denominator: 6 leaf commands
- Covered: 6
- Coverage: 100%

## Summary
- TestWiki_NodeWorkflow: proves the full public wiki command surface currently exposed by the CLI for `spaces` and `nodes`.
- Key `t.Run(...)` proof points: `create node`, `get created node`, `get space`, `list spaces`, `list nodes and find created node`, `copy node`, and `list nodes and find copied node`.
- The workflow proves both node creation paths (`create` and `copy`) and both read paths (`spaces get_node` and `nodes list`) against the same created resources.
- Current scope note: the CLI currently exposes only `wiki nodes {create,copy,list}` and `wiki spaces {get,get_node,list}`; no delete command is available to prove cleanup as a primary workflow.

## Command Table
| Status | Cmd | Type | Testcase | Key Parameter Shapes | Notes / Uncovered Reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | wiki nodes copy | api | wiki_workflow_test.go::TestWiki_NodeWorkflow/copy node | `space_id` and `node_token` in `--params`; `target_space_id` and `title` in `--data` | |
| ✓ | wiki nodes create | api | wiki_workflow_test.go::TestWiki_NodeWorkflow/create node | `space_id` in `--params`; `node_type`, `obj_type`, and `title` in `--data` | |
| ✓ | wiki nodes list | api | wiki_workflow_test.go::TestWiki_NodeWorkflow/list nodes and find created node; wiki_workflow_test.go::TestWiki_NodeWorkflow/list nodes and find copied node | `space_id` and `page_size` in `--params` | |
| ✓ | wiki spaces get | api | wiki_workflow_test.go::TestWiki_NodeWorkflow/get space | `space_id` in `--params` | |
| ✓ | wiki spaces get_node | api | wiki_workflow_test.go::TestWiki_NodeWorkflow/get created node | `token` and `obj_type` in `--params` | |
| ✓ | wiki spaces list | api | wiki_workflow_test.go::TestWiki_NodeWorkflow/list spaces | `page_size` in `--params` | |
