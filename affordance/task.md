# task
> skill: lark-task

## +download-attachment

Use after selecting one attachment GUID from a known task. The command obtains
and consumes the short-lived Task attachment URL without exposing it.

### Avoid when
- Downloading a Drive file token → use `drive +download`
- Downloading an attachment from a Base record → use `base +record-download-attachment`

### Prerequisites
- The exact attachment GUID from `task tasks get`
- Read access to the attachment's owning task

### Tips
- Pass a relative output path; an existing directory or a path ending in `/` uses the attachment name.
- Existing files are preserved unless `--overwrite` is explicit.
- Do not log, cache, or forward the temporary URL returned by the Task API.

### Examples

**Download one attachment using its original name**
```bash
lark-cli task +download-attachment --attachment-guid <attachment_guid> --output ./downloads/
```

### Skills
- lark-task/references/lark-task-download-attachment.md
