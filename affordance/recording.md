# recording
> skill: lark-meeting

## +list

Use when the user asks for their recordings across sources or within a time range, without a meeting ID.

### Avoid when
- Looking up a known meeting's recording → use [[vc +recording]].
- Searching Minutes by title → use [[minutes +search]].

### Tips
- Keep the time bounds unchanged when following the next page token.
- Route Minutes artifacts by artifact_token and Note artifacts by artifact_id; recording_id is not interchangeable with either.

### Examples

**List the current user's recordings**
```bash
lark-cli recording +list --as user --page-size 20
```

### Skills
- lark-meeting/references/lark-recording-list.md

