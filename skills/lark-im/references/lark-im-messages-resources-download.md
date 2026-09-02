# im +messages-resources-download

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first to understand authentication, global parameters, and safety rules.

Download an image or file attached to a message. Use the `message_id` and resource key returned by a message-reading command; do not guess or combine identifiers from different messages.

> **Note:** read-only message commands render resource keys in message content, but they do not download binaries automatically. Use this command whenever you need to fetch the actual image/file bytes or save them to a specific path.

Shortcut: `lark-cli im +messages-resources-download`.

## Commands

```bash
# Download an image (save to the current directory)
lark-cli im +messages-resources-download --message-id om_xxx --file-key img_v3_xxx --type image

# Download a file
lark-cli im +messages-resources-download --message-id om_xxx --file-key file_v3_xxx --type file

# Specify the output path
lark-cli im +messages-resources-download --message-id om_xxx --file-key img_v3_xxx --type image --output ./photo.png

# Download as a bot
lark-cli im +messages-resources-download --message-id om_xxx --file-key img_v3_xxx --type image --as bot

# Preview the request without executing it
lark-cli im +messages-resources-download --message-id om_xxx --file-key img_v3_xxx --type image --dry-run
```

## Parameters

| Parameter | Required | Description |
|------|------|------|
| `--message-id <id>` | Yes | Message ID (`om_xxx` format) |
| `--file-key <key>` | Yes | Resource key (`img_xxx` or `file_xxx`) |
| `--type <type>` | Yes | Resource type: `image` or `file` |
| `--output <path>` | No | Output path, relative or absolute, that must resolve inside the built-in allowed roots (the working directory, `/tmp`, `~/files`); system and credential directories stay refused. When omitted, the command uses the attachment name when available and otherwise falls back to the resource key |
| `--as <identity>` | No | Identity type: `user` (default) or `bot` |
| `--dry-run` | No | Print the request only, do not execute it |

## Choose `--type`

Different resource markers in message content correspond to different `file_key` and `type` values:

| Message Type | Marker in Content | `file_key` Format | `--type` |
|---------|-------------|---------------|--------|
| Image | `img_xxx` | `img_xxx` | `image` |
| File | `file_xxx` | `file_xxx` | `file` |
| Audio | `file_xxx` | `file_xxx` | `file` |
| Video | `file_xxx` | `file_xxx` | `file` |

Stickers cannot be downloaded with this command.

## Output

On success, read:

| Field | Meaning |
|------|---------|
| `data.saved_path` | Saved local path |
| `data.size_bytes` | Saved byte count |

## Usage Scenario

### Scenario: Extract and download an image from a message

```bash
# Step 1: Fetch messages and find one containing an image
lark-cli im +chat-messages-list --chat-id oc_xxx
# In the response you see: { "msg_type": "image", "content": "{\"image_key\":\"img_v3_xxx\"}" }

# Step 2: Download the image
lark-cli im +messages-resources-download --message-id om_xxx --file-key img_v3_xxx --type image
```

## Common Errors and Troubleshooting

| Symptom | Root Cause | Solution |
|---------|---------|---------|
| Resource does not match the message | `file_key` and `message_id` came from different messages | Read the message again and use its matching identifiers |
| Permission denied | `im:message:readonly` is not authorized | For user identity, run `lark-cli auth login --scope "im:message:readonly"`; for bot identity, grant the scope to the app in the developer console |
| Attachment unavailable | The message or resource is deleted, hidden, restricted, or inaccessible to the caller | Do not retry unchanged; report the exact CLI error |
| Retryable network error | The transfer did not complete | Retry the same command |

## References

- [lark-im](../SKILL.md) - all message-related commands
- [lark-shared](../../lark-shared/SKILL.md) - authentication and global parameters
