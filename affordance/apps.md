# apps
> skill: lark-apps

## +mcp-get
Use when connecting an existing Miaoda app to an MCP client.

### Prerequisites
- Developer or administrator access to the app.

### Tips
- This operation may create a missing runtime credential, even though the API uses GET.
- Default output is display-only and redacted. Use --include-secret only when a usable client configuration is needed.
- This does not publish the app, list tools, or complete the client's user OAuth flow.

### Examples
**Inspect a redacted connection**
```bash
lark-cli apps +mcp-get --app-id app_example --as user
```

### Skills
- lark-apps/references/lark-apps-mcp.md

## +mcp-key-create
Use when preparing an app's runtime MCP credential without requesting a client configuration.

### Tips
- Returns an existing credential when present; never rotates it.
- Plaintext output requires --include-secret. It is not a user OAuth token.

### Examples
**Prepare a credential without exposing its secret**
```bash
lark-cli apps +mcp-key-create --app-id app_example --as user
```

### Skills
- lark-apps/references/lark-apps-mcp.md
