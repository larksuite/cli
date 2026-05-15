# lark-wiki +node-copy

Copy a wiki node (including its content) to a target space or under a target parent node. Used for cross-space migration.

> ⚠️ **High-risk write** — the upstream API is flagged `danger: true`, so this shortcut requires explicit `--yes` confirmation before issuing the request. Forgetting `--yes` returns a `confirmation_required` error and the copy is **not** performed.

## Usage

```bash
lark-cli wiki +node-copy \
  --space-id <source_space_id> \
  --node-token <source_node_token> \
  (--target-space-id <target_space_id> | --target-parent-node-token <token>) \
  [--title <new_title>] \
  --yes \
  [--as user|bot]
```

## Flags

| Flag | Required | Description |
|------|----------|-------------|
| `--space-id` | **Yes** | Source wiki space ID |
| `--node-token` | **Yes** | Source node token to copy |
| `--target-space-id` | Conditional | Target space ID. Required if `--target-parent-node-token` is not set |
| `--target-parent-node-token` | Conditional | Target parent node token. Required if `--target-space-id` is not set |
| `--title` | No | New title for the copied node. Omit to keep the original title |
| `--yes` | **Yes** | Confirm the high-risk operation. Without this flag the shortcut refuses to send the API request |
| `--format` | No | Output format: `json` (default) / `pretty` / `table` / `csv` / `ndjson` |
| `--as` | No | Identity `user`/`bot` (default `auto`); wiki is user-centric → pass `--as user` |

> At least one of `--target-space-id` or `--target-parent-node-token` must be provided.

## Output

```json
{
  "space_id": "target_space_id",
  "node_token": "wikcn_EXAMPLE_TOKEN",
  "obj_token": "doccn_EXAMPLE_TOKEN",
  "obj_type": "docx",
  "node_type": "origin",
  "title": "Getting Started (Copy)",
  "parent_node_token": "",
  "has_child": false
}
```

## Migration workflow

To migrate a subtree from one space to another:

```bash
# 1. List nodes in the source space
lark-cli wiki +node-list --space-id source_space_id

# 2. Copy each node to the target space
lark-cli wiki +node-copy \
  --space-id <source_space_id> \
  --node-token wikcn_EXAMPLE_TOKEN \
  --target-space-id <target_space_id> \
  --yes
```

## Notes

- Copying is recursive — the subtree under the node is also copied.
- There is no native move API; migration = copy to target + (manually delete source if needed).

## Required Scope

`wiki:node:copy`
