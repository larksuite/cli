# lark-wiki +node-list

List wiki nodes in a space or under a specific parent node. Automatically paginates through all pages.

## Usage

```bash
lark-cli wiki +node-list --space-id <space_id> [--parent-node-token <token>] [--as user|bot]
```

## Flags

| Flag | Required | Description |
|------|----------|-------------|
| `--space-id` | **Yes** | Wiki space ID. Use `my_library` for personal document library |
| `--parent-node-token` | No | Parent node token. Omit to list root-level nodes of the space |
| `--as` | No | Identity: `user` or `bot` (default: `user`) |

## Output

```json
{
  "nodes": [
    {
      "space_id": "6946843325487912356",
      "node_token": "wikcn_EXAMPLE_TOKEN",
      "obj_token": "doccn_EXAMPLE_TOKEN",
      "obj_type": "docx",
      "parent_node_token": "",
      "node_type": "origin",
      "title": "Getting Started",
      "has_child": true
    }
  ],
  "total": 1
}
```

## Traverse the wiki tree

To list all content recursively, call `+node-list` again with each node's `node_token` as `--parent-node-token` when `has_child` is `true`.

```bash
# Step 1: list root nodes
lark-cli wiki +node-list --space-id 6946843325487912356

# Step 2: drill into a node that has children
lark-cli wiki +node-list --space-id 6946843325487912356 --parent-node-token wikcn_EXAMPLE_TOKEN
```

## Required Scope

`wiki:node:retrieve`
