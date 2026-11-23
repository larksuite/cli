# application

## +slash-command-list
Use this to inspect the current app's slash commands and obtain a stable `command_id` before an update or deletion.

### Tips
- The upstream API returns all commands at once (maximum 100 per app); there is no pagination.

### Examples

**List all slash commands of the currently bound app**
```bash
lark-cli application +slash-command-list
```

## +slash-command-create
Use this for a new command name. On a name collision, update the existing command only when that is the user's intent.

### Avoid when
- The command already exists and the user wants to edit it → use [[+slash-command-update]].

### Tips
- Changes may take about five minutes to appear in clients because of client-side caching; the server updates immediately.

### Examples

**Create a localized slash command**
```bash
lark-cli application +slash-command-create --command greet --description "say hi" --description-i18n zh_cn=问候
```

## +slash-command-update
Use this to change description, localized descriptions, or icon while retaining the command name and `command_id`.

### Prerequisites
- Pass `--command-id` from [[+slash-command-list]], or pass `--command` to let the CLI resolve the current id by name with a live list request.

### Tips
- PATCH is field-level partial: fields you do not pass are preserved server-side.
- The command name cannot be changed. Renaming means deleting and recreating the command, which produces a new `command_id`.

### Examples

**Update by name (the CLI resolves the current command id first)**
```bash
lark-cli application +slash-command-update --command greet --description "new text"
```

## +slash-command-delete
Use this only after the user explicitly confirms the target and the irreversible deletion.

### Prerequisites
- Explicit user confirmation is required before passing `--yes`; an agent must not self-approve the deletion.
- Pass `--command-id` from [[+slash-command-list]], or pass `--command` to let the CLI resolve the current id by name with a live list request.

### Tips
- Deleted commands may linger in clients for about five minutes because of client-side caching.
- Recreating the same command name produces a new `command_id`.

### Examples

**Delete by name after explicit user confirmation**
```bash
lark-cli application +slash-command-delete --command greet --yes
```
