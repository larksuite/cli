# Workflow quick

Use this as the first stop for Base workflow create/update tasks. Keep the full guide and schema as the source of truth for step fields; read only the relevant sections when this quick path is not enough.

## Route

| Task | First commands | When to read more |
|---|---|---|
| Find a workflow by name | `+workflow-list --base-token <base> --as user --format json --jq '<filter>'` | Use `+workflow-get` only for the uniquely matched workflow you will inspect or update. |
| Disable or enable | `+workflow-list` -> `+workflow-disable` / `+workflow-enable` | Do not read the steps schema; state changes do not modify steps. |
| Create a workflow | `+base-block-create --type workflow --name "<title>"` -> `+workflow-update --workflow-id <wkf>` | Read schema sections only for the step types you need. |
| Update workflow steps | `+workflow-get` -> edit returned `title/status/steps` -> `+workflow-update` | Full replacement semantics: keep fields you do not intend to change. |

## Create Path

Prefer the stable two-step create path:

```bash
lark-cli base +base-block-create --base-token <base_token> --as user --type workflow --name "<workflow title>"
lark-cli base +workflow-update --base-token <base_token> --as user --workflow-id <wkf_id> --json @workflow.json
```

`+workflow-create` calls the direct workflow create API and may be blocked by platform limits in some tenants. If it returns a limited-method API error, do not retry it; create the workflow block and then update that workflow's definition.

New or newly configured workflows are not automatically enabled by an update. Call `+workflow-enable` only when the user asked for an active workflow or the task clearly requires activation.

## Required Checks

- Extract `base_token` from the `/base/<token>` URL before running Base commands.
- Use returned names and IDs. Do not guess table names, field names, workflow IDs, group IDs, user IDs, or receiver values.
- Before constructing `steps`, read the real Base structure you reference: tables via `+table-list`, fields via `+field-list`, existing workflows via `+workflow-list/get`.
- For workflow updates, `+workflow-update` replaces the full workflow definition. Start from `+workflow-get` for existing workflows and preserve unchanged `title`, `status`, and `steps`.
- For write operations, keep using `--as user` unless the user explicitly asks for bot identity or permission recovery requires the shared auth flow.

## Step Schema Pointers

Only read the full schema or guide when this quick file does not contain the needed step detail.

| Need | Read |
|---|---|
| Basic step shape, `next`, `children.links` | `lark-base-workflow-schema.md` -> WorkflowStep / StepChildren |
| Trigger type selection | `lark-base-workflow-schema.md` -> Trigger types |
| Message receiver/content | `lark-base-workflow-schema.md` -> LarkMessageAction |
| AI text generation | `lark-base-workflow-schema.md` -> GenerateAiTextAction |
| Find records, add records, update records | `lark-base-workflow-schema.md` -> Action data |
| Timer, workday, reminder timing | `lark-base-workflow-schema.md` -> TimerTrigger / ReminderTrigger |
| If/else, switch, loop | `lark-base-workflow-guide.md` examples plus schema branch/system sections |

## Minimal Body Shape

```json
{
  "title": "workflow title",
  "status": "disabled",
  "steps": [
    {
      "id": "step_trigger",
      "type": "AddRecordTrigger",
      "title": "trigger title",
      "next": "step_action",
      "data": {}
    },
    {
      "id": "step_action",
      "type": "LarkMessageAction",
      "title": "action title",
      "next": null,
      "data": {}
    }
  ]
}
```

This is only the outer shape. Fill each `data` object from the relevant schema section or an existing `+workflow-get` response; do not invent unsupported fields from natural language.
