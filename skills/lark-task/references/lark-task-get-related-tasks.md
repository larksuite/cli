# task +get-related-tasks

> **Prerequisites:** Please read `../../lark-shared/SKILL.md` to understand authentication, global parameters, and security rules.
>
> **⚠️ Note:** This API must be called with a user identity. **Do NOT use an app identity, otherwise the call will fail.**
>
> **Pagination / Time Cursor Rule:** `page_token` is a task `updated_at` cursor in microseconds. Derive a starting cursor from a time boundary only when the user explicitly limits tasks by **last update time**. A due, creation, or completion date is not an update-time boundary. Continue with the returned `page_token` when `has_more=true`; each invocation can fetch at most 40 pages with `--page-all`.

> **Due-date queries:** This shortcut has no due-date filter. Retrieve related tasks as JSON, then compare each `items[].due.timestamp` (milliseconds) with the requested local date range. For “today” in Asia/Shanghai, use the half-open interval from local 00:00 to the next local 00:00. An incomplete-task request also needs `--include-complete=false`. Empty matches in one batch do not prove there are no matches while `has_more=true`; resume with the returned cursor until `has_more=false`, or state that the result is incomplete.

List tasks related to the current user.

## Recommended Commands

```bash
# List all related tasks, including tasks with other assignees
lark-cli task +get-related-tasks --page-all --format json

# List incomplete related tasks before filtering today's due.timestamp in Asia/Shanghai
lark-cli task +get-related-tasks --include-complete=false --page-all --format json

# Continue a scan when the previous response still has has_more=true
lark-cli task +get-related-tasks --include-complete=false --page-all --page-token "<returned_page_token>" --format json

# Show only tasks created by me
lark-cli task +get-related-tasks --created-by-me
```

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `--include-complete=<bool>` | No | Default behavior includes completed tasks. Set to `false` to keep only incomplete tasks. |
| `--page-all` | No | Automatically paginate through all pages (max 40). |
| `--page-limit <int>` | No | Max page limit (default 20). |
| `--page-token <string>` | No | Start from the specified page token. This token is the task's last update time cursor in microseconds. |
| `--created-by-me` | No | Keep only tasks whose creator is the current user. This is a client-side filter applied after fetching related-task pages. |
| `--followed-by-me` | No | Keep only tasks followed by the current user. This is a client-side filter applied after fetching related-task pages. |

> **Page Token Note:** The returned `page_token` resumes the upstream related-task scan. Do not turn a due-date boundary into a `page_token`.
>
> **Pagination Note for Client-side Filters:** When `--created-by-me` or `--followed-by-me` is used, filtering happens locally after each upstream related-task page is fetched. The returned `has_more` and `page_token` still describe the upstream cursor, so later pages may contain more matching tasks, or may contain none.

## Workflow

1. Determine the requested relation, completion state, and time field. For an unscoped task list, use related tasks as the current-user candidate set and say so in the answer.
2. Execute `lark-cli task +get-related-tasks ... --format json`. When completeness matters, scan until `has_more=false`, resuming with the returned `page_token` after each 40-page batch.
3. For a due-date request, filter the raw `items[].due.timestamp` using the requested timezone after fetching. For assignees, read `items[].members` entries whose `role` is `assignee`.
4. Report matches and their scope. If the scan stopped with `has_more=true`, report that the result is partial and include the next `page_token`.
