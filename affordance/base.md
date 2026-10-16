# base
> skill: lark-base

## Skills
- `lark-base/references/lark-base-field-groups.md`
- `lark-base/references/lark-base-dashboard-global-filter.md`
- `lark-base/references/lark-base-dashboard-button-rule.md`
- `lark-base/references/lark-base-dashboard-linkage.md`

## +field-group-list

### Skills
- `lark-base/references/lark-base-field-groups.md`

## +field-group-manage

### Skills
- `lark-base/references/lark-base-field-groups.md`

## +dashboard-block-create

### Skills
- `lark-base/references/lark-base-dashboard-view.md`

## +dashboard-block-update

### Skills
- `lark-base/references/lark-base-dashboard-linkage.md`
- `lark-base/references/lark-base-dashboard-view.md`

## +dashboard-block-get

### Skills
- `lark-base/references/lark-base-dashboard-linkage.md`
- `lark-base/references/lark-base-dashboard-view.md`

## +dashboard-layout
The required last step of every dashboard layout change: submit the coordinates of every block on the dashboard in one call. It writes only the blocks whose coordinates changed, then always runs the server's tidy pass, which fixes holes, overlaps and out-of-bounds blocks, and reports the final coordinates. Workflow: read the existing blocks with [[+dashboard-block-list]] (a new dashboard has none) → plan the coordinates of all blocks for the request (existing blocks may be moved or resized) → create new blocks with `--position` on [[+dashboard-block-create]] → finish with this command.

### Avoid when
- Changing only a block's name or data_config → [[+dashboard-block-update]] without `--position`; the layout is untouched and needs no finishing call.

### Prerequisites
- `dashboard_id` from [[+dashboard-list]]
- every block's `block_id` and current coordinates from [[+dashboard-block-list]], with the new blocks already created
- `--layout` covers the whole board: existing blocks you did not move are submitted at their current coordinates; a missing, unknown or repeated block_id fails and writes nothing

### Examples

**Finish a three-block board: two half-width cards over one full-width chart**
```bash
lark-cli base +dashboard-layout --base-token <base_token> --dashboard-id <dashboard_id> --layout '[{"block_id":"blk_a","x":0,"y":0,"w":24,"h":8},{"block_id":"blk_b","x":24,"y":0,"w":24,"h":8},{"block_id":"blk_c","x":0,"y":8,"w":48,"h":16}]'
```

**Commit a layout kept in a file**
```bash
lark-cli base +dashboard-layout --base-token <base_token> --dashboard-id <dashboard_id> --layout @layout.json
```

### Skills
- lark-base/references/lark-base-dashboard-visual-standards.md
