# apps
> skill: lark-apps

## +mcp-get
Use when connecting an existing Miaoda app to an MCP client.

### Prerequisites
- Developer access to the app, or membership in the app's online visible scope once a developer has enabled MCP.

### Tips
- For a developer this may create a missing runtime credential and enable MCP, even though the API uses GET; any other user only reads the existing connection.
- If it fails with "app MCP is not enabled" (code 40906), ask a developer to enable MCP first by running this command or +mcp-key-create.
- Pass --format pretty so stdout is only the URL; the default JSON wraps it as data.url.
- The URL is returned exactly as the server sent it and usually carries the app credential; do not share it or commit it.
- This does not publish the app, list tools, or complete the client's user OAuth flow.

### Examples
**Get the bare MCP URL for a client (a developer's call may create a credential)**
```bash
lark-cli apps +mcp-get --app-id <app_id> --as user --format pretty
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
lark-cli apps +mcp-key-create --app-id <app_id> --as user
```

### Skills
- lark-apps/references/lark-apps-mcp.md
