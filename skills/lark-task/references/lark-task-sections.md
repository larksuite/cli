# task sections

> **Prerequisites:** Please read `../../lark-shared/SKILL.md` to understand authentication, global parameters, and security rules.

## Scope and Operation Guide

This reference organizes Task `sections` workflows by Catalog method. It currently covers locating sections and listing their tasks. See the [main Skill](../SKILL.md) for group terminology and ownership routing. The current CLI Catalog/schema is authoritative for parameters, permissions, and response fields.

| User goal | Read |
|----------|----------|
| List sections in a resource or locate a section by name | [sections.list](#sectionslist) |
| List tasks in a selected section | [sections.tasks](#sectionstasks) |

## Shared Rules

1. Follow the main Skill's discovery flow: run `lark-cli task --help` and `lark-cli task sections --help`, confirm the method, and read its schema before executing a native command.
2. Use `--as user` when querying the current user's personal "My Tasks". For other operations, confirm identity and permissions through the shared rules and the method's help/schema.
3. For queries that support pagination, use `--page-all --page-limit 0` when a complete listing is required. This avoids the default page limit. If the query is interrupted or partially fails, report that results are incomplete; do not conclude that a section or task does not exist.

## sections.list

List sections in "My Tasks" or a specific tasklist, or resolve a section name to its GUID.

### Recommended Commands

**List sections in "My Tasks"**

```bash
lark-cli schema task.sections.list
lark-cli task sections list --resource-type my_tasks --as user --page-all --page-limit 0
```

**List sections in a specific tasklist**

```bash
lark-cli schema task.sections.list
lark-cli task sections list --resource-type tasklist --resource-id "<tasklist_guid>" --as user --page-all --page-limit 0
```

### Parameters

Read `lark-cli schema task.sections.list` for the full parameter structure. Choose the owning resource as follows:

- "My Tasks": use `my_tasks`; no tasklist GUID or `resource_id` is needed.
- A specific tasklist: use `tasklist` and pass its GUID through `--resource-id`.

### Workflow

1. Determine ownership from context; ask for the owning resource if it remains unclear. Use a known tasklist GUID directly. If only the tasklist name is available, follow the main Skill's tasklist lookup flow first. Do not use the section name as a tasklist search term.
2. Execute the command for the owning resource.
3. Match `name` in the returned `items`, and use the selected item's `guid` as `section_guid`. If names are duplicated, show candidate GUIDs and their known ownership so the user can select the target.
4. Report the sections or pass the selected GUID to `sections.tasks` when the user requests their tasks. If a complete listing has no match, report that the section was not found in that resource; do not automatically scan other tasklists.

## sections.tasks

List tasks in a selected section.

### Recommended Commands

```bash
lark-cli schema task.sections.tasks
lark-cli task sections tasks --section-guid "<section_guid>" --as user --page-all --page-limit 0
```

### Parameters

Read `lark-cli schema task.sections.tasks` for the full parameter structure. Pass the target section's GUID through `--section-guid`.

### Workflow

1. Use a known section GUID directly. If only the section name is available, locate it with [sections.list](#sectionslist) first. A known section GUID does not require another section or tasklist lookup.
2. Execute the command to list the section's tasks.
3. Present the returned tasks as requested. Report a complete result only after pagination finishes; otherwise, state that the results are incomplete.

## Adding Operation Guidance

For each added operation, create a `## sections.<method>` chapter using its Catalog method identifier and update the operation guide. Start with a short description, then use the Task reference convention: **Recommended Commands → Parameters → Workflow**. Keep shared rules at the start of this reference and operation-specific differences in the relevant chapter. Commands must show schema discovery before execution. Use Parameters for operation-specific choices and a schema pointer; leave complete parameter, permission, and response definitions to the schema.
