# Extension

Embed lark-cli into your own Agent or application — swap credential sources, audit every command, restrict the command surface — without modifying CLI source. Write a Go package against these interfaces, import it from a wrapper `main`, and build your own enhanced binary.

Main extension points:

| Package | Extension point | What it does |
| ------- | --------------- | ------------ |
| [`credential/`](./credential/) | **Credential** | Bring your own credential source: database, Vault, config center… |
| [`transport/`](./transport/) | **Transport** | Intercept every HTTP request: inject headers, rewrite targets, logging & monitoring |
| [`platform/`](./platform/) | **Restrict · Observer · Wrap · On** | Command allow/deny rules, audit hooks, onion-style middleware (approval gates, rate limiting), process lifecycle — see the [Plugin SDK README](./platform/README.md) |

📖 Full guide: [Embed lark-cli in your Agent](https://open.larksuite.com/document/mcp_open_tools/feishu-cli/embed-feishu-cli-in-agent) ([中文](https://open.larkoffice.com/document/mcp_open_tools/feishu-cli/embed-feishu-cli-in-agent))
