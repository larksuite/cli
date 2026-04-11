# lark-wiki +node-move

Move a wiki node (and its subtree) to a target space or under a target parent node. Unlike copy, the node is removed from its original location.

## Usage

```bash
lark-cli wiki +node-move \
  --space-id <source_space_id> \
  --node-token <node_token> \
  (--target-space-id <target_space_id> | --target-parent-node-token <token>) \
  [--as user|bot]
```

## Flags

| Flag | Required | Description |
|------|----------|-------------|
| `--space-id` | **Yes** | Source wiki space ID |
| `--node-token` | **Yes** | Token of the node to move |
| `--target-space-id` | Conditional | Target space ID. Required if `--target-parent-node-token` is not set |
| `--target-parent-node-token` | Conditional | Target parent node token. Required if `--target-space-id` is not set |
| `--as` | No | Identity: `user` or `bot` (default: `user`) |

> At least one of `--target-space-id` or `--target-parent-node-token` must be provided.

## Output

```json
{
  "space_id": "target_space_id",
  "node_token": "wikcn_EXAMPLE_TOKEN",
  "obj_token": "doccn_EXAMPLE_TOKEN",
  "obj_type": "docx",
  "node_type": "origin",
  "title": "Getting Started",
  "parent_node_token": "wikcn_EXAMPLE_TOKEN",
  "has_child": false
}
```

## Difference from +node-copy

| | `+node-move` | `+node-copy` |
|--|--|--|
| Original node | Removed | Kept |
| `--title` flag | Not supported | Optional |
| Use case | Reorganize structure | Duplicate / migrate |

## Notes

- Moving is recursive — the entire subtree is moved together.
- Requires editing permission on the node, its original parent, and the target parent.
- Rate limit: 100 calls/minute.
- Single move operation (including child nodes) cannot exceed 2,000 nodes.

## Required Scope

`wiki:node:move`
