# Workflow quick

Use this as the first stop for Base workflow tasks. It decides the short path first; keep the full guide and schema as the source of truth for new step fields and read only the relevant sections when this quick path is not enough.

## Route

| Task | First commands | When to read more |
|---|---|---|
| Find a workflow by name | `+workflow-list --base-token <base> --as user --format json --jq '<filter>'` | Use `+workflow-get` only for the uniquely matched workflow you will inspect or update. |
| Disable or enable | `+workflow-list` -> `+workflow-disable` / `+workflow-enable` | Do not read the steps schema; state changes do not modify steps. |
| Inspect title/status/steps | `+workflow-get --workflow-id <wkf>` | Do not read schema just to report title, status, IDs, or that steps are empty. |
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

## Stay In The Short Path

Do not open the full guide/schema for these tasks:

- list workflows, find a unique workflow ID, enable, disable, or report current status;
- rename a workflow by changing `title` while preserving returned `status` and `steps`; use `+workflow-enable` or `+workflow-disable` for state changes;
- update an existing workflow by editing a small part of a `+workflow-get` response and leaving existing step `type`, `data`, `next`, and `children` shapes intact;
- create an empty named workflow block when the user did not ask to configure trigger/action steps.

Open the full guide/schema only when you must construct a new step data shape, add a step type that is not already present, change branch/loop links, write ref paths, or recover a platform schema error. When reading them locally, search for the heading first and read only that narrow section.

## Unsupported Trigger Boundaries

- There is no native record-deleted trigger in the current workflow step schema. Do not search for or invent `DeleteRecordTrigger`, `DeletedRecordTrigger`, or `RecordDeletedTrigger`.
- If the user asks for "record deleted" / "删除记录" to trigger a workflow, say the exact delete event is not supported by Base workflow steps. Offer a modelable alternative: set a status such as "离职/已删除" before deletion, clear a marker field, or update a boolean flag, then use `SetRecordTrigger` / `ChangeRecordTrigger`.
- If the task must be triggered after a physical row is already deleted, stop and report the capability boundary instead of creating a misleading workflow.

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
| Record deletion trigger | Do not read more for `DeleteRecordTrigger`; it is not supported. Model with status/field change before deletion, or report unsupported if exact deletion is required. |
| Common simple flow: record/timer trigger -> message, add/update/find record, delay, or AI text action | Search headings first; read only the exact guide example or schema step data section that is missing |
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
