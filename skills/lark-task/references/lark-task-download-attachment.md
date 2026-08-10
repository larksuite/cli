# task +download-attachment

Download one Task attachment by attachment GUID. The command gets a fresh
short-lived URL from the Task attachment API, downloads it without forwarding
the Lark Authorization header, and atomically saves the file through the CLI
FileIO provider.

## Usage

```bash
# Save with the attachment's server-provided name
lark-cli task +download-attachment \
  --attachment-guid <attachment_guid> \
  --output ./downloads/

# Save to an explicit relative file path
lark-cli task +download-attachment \
  --attachment-guid <attachment_guid> \
  --output ./downloads/report.pdf

# Explicitly replace an existing file
lark-cli task +download-attachment \
  --attachment-guid <attachment_guid> \
  --output ./downloads/report.pdf \
  --overwrite
```

## Flags

| Flag | Required | Description |
|------|----------|-------------|
| `--attachment-guid <guid>` | Yes | Attachment GUID from the task's `attachments` or `attachment_deliveries` data. |
| `--output <path>` | Yes | Relative save path. An existing directory or a path ending in `/` uses the attachment name. |
| `--overwrite` | No | Replace an existing output file. Without this flag the command stops before consuming the temporary URL. |
| `--user-id-type <type>` | No | User ID type passed to the attachment API. Defaults to `open_id`. |

## Workflow

1. If the task GUID is known but the attachment GUID is not, run:

   ```bash
   lark-cli task tasks get \
     --params '{"task_guid":"<task_guid>"}' \
     --as user
   ```

2. Select the intended item from `data.task.attachments` or
   `data.task.attachment_deliveries` and use its `guid`. Do not substitute the
   task GUID, display number (`t123`), or `file_token`.
3. Run `+download-attachment` with a relative `--output` path.
4. Report `saved_path`, `size_bytes`, and `attachment_guid`; never expose the
   temporary download URL.

## Behavior and recovery

- Scope: `task:attachment:read` (or the broader
  `task:attachment:write`). The caller must also be able to read the attachment's
  owning task.
- Task download URLs are short-lived. The command uses one full response rather
  than multipart ranges. If a URL is rejected with HTTP 401/403, it fetches
  fresh metadata and retries once.
- The URL is accepted only when it is HTTPS and contains no user information.
  It is sent through the external HTTP policy path with no Lark Authorization
  header.
- Output paths must be relative and remain inside the current workspace.
  Writes are atomic; existing files require `--overwrite`.
- Do not use `drive +download` for Task attachments. A Task attachment
  `file_token` does not replace the Task attachment authorization flow.
