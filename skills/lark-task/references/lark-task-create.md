# task +create

> **Prerequisites:** Please read `../lark-shared/SKILL.md` to understand authentication, global parameters, and security rules.

Create a new task in Lark.

## Recommended Commands

```bash
# Create a task with all details
lark-cli task +create \
  --summary "Quarterly Sales Review" \
  --description "Review the sales performance for the last quarter." \
  --assignee "ou_xxx" \
  --due "2026-03-25" \
  --tasklist-id "https://applink.larkoffice.com/client/todo/task_list?guid=a4b00000-000-000-000-00000000036c"

# Create a task assigned to an app
lark-cli task +create \
  --summary "Nightly Sync" \
  --assignee "cli_xxx"

# Create a simple task
lark-cli task +create \
  --summary "Buy milk"

# Preview the API call without executing
lark-cli task +create --summary "Test Task" --dry-run
```

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `--summary <text>` | Yes | The title or summary of the task |
| `--description <text>` | No | Detailed description of the task |
| `--assignee <id>` | No | Assignee ID. Use user `open_id` like `ou_xxx` for people, or app ID like `cli_xxx` for apps. |
| `--follower <id>` | No | Follower ID. Use user `open_id` like `ou_xxx` for people, or app ID like `cli_xxx` for apps. |
| `--due <time>` | No | Due date. Supports ISO 8601, `YYYY-MM-DD`, relative time (e.g., `+2d`), or ms timestamp. `YYYY-MM-DD` and relative time will automatically set it as an all-day task. |
| `--tasklist-id <id>` | No | The GUID of the tasklist, or a full AppLink URL (the CLI will automatically extract the `guid` parameter from the URL). |
| `--idempotency-key <key>` | No | Client token to ensure idempotency of the request. |
| `--dry-run` | No | Preview the API call (JSON payload) without actually creating the task. |

## Workflow

1. Confirm with the user: task summary, due date, assignee, and tasklist if necessary.
   - **Crucial Rule for Assignee**: If the user explicitly or implicitly says "create a task for me" (给我创建一个任务), or "help me create a task" (帮我新建/创建一个任务), you MUST assign the task to the current logged-in user. You can get the current user's `open_id` by executing `lark-cli auth status` (it already outputs JSON by default, so do not add `--json`) or `lark-cli contact +get-user` first, extracting `.identities.user.openId` (from `auth status`) or `.data.user.open_id` (from `contact +get-user`), and then passing it to the `--assignee` parameter.
2. If the user provides both a specific tasklist and an assignee name, resolve the assignee within the tasklist context before global contact search:
   - Locate the tasklist and collect candidate user IDs from `owner`, `creator`, and `members` (ignore non-user members such as chats).
   - Batch hydrate those IDs with `lark-cli contact +search-user --user-ids "<ou_a,ou_b,...>" --as user`.
   - Prefer an exact full-name match (`localized_name` equals the requested name). If exactly one user matches, use that `open_id`.
   - If there is no exact match in the tasklist candidates, fallback to `lark-cli contact +search-user --query "<name>" --exclude-external-users --as user`.
   - If global search returns multiple exact same-name users, ask the user to confirm by email or department before creating the task; do not choose the first result.
3. Execute `lark-cli task +create --summary "..." ...`
4. Report the result: task ID and summary.

> [!CAUTION]
> This is a **Write Operation** -- You must confirm the user's intent before executing.

## References

- [lark-task](../SKILL.md) -- All task commands
- [lark-shared](../../lark-shared/SKILL.md) -- Authentication and global parameters
