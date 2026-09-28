# Extension

Embed lark-cli into your own Agent or application — swap credential sources, audit every command, restrict the command surface — without modifying CLI source. Write a Go package against these interfaces, import it from a wrapper `main`, and build your own enhanced binary.

Main extension points:

| Package | Extension point | What it does |
| ------- | --------------- | ------------ |
| [`credential/`](./credential/) | **Credential** | Bring your own credential source: database, Vault, config center… |
| [`transport/`](./transport/) | **Transport** | Intercept every HTTP request: inject headers, rewrite targets, logging & monitoring |
| [`platform/`](./platform/) | **Restrict · Observer · Wrap · On** | Command allow/deny rules, audit hooks, onion-style middleware (approval gates, rate limiting), process lifecycle — see the [Plugin SDK README](./platform/README.md) |

📖 Full guide: [Embed lark-cli in your Agent](https://open.larksuite.com/document/mcp_open_tools/feishu-cli/embed-feishu-cli-in-agent) ([中文](https://open.larkoffice.com/document/mcp_open_tools/feishu-cli/embed-feishu-cli-in-agent))

## Citations

For read commands, set `Definition.Citation`:

```go
Citation: &command.CitationDefinition[Data]{
    SourceTypes: []citation.SourceType{citation.SourceFile},
    Build:       buildCitations, // func(Data) []citation.Citation
},
```

Use types from [citation/](./citation/). With `LARKSUITE_CLI_CITATION=1`, eligible
JSON envelopes include citations. `Build` runs after content scanning and must
only use the final data, without API calls. In `Execute`, use
`ctx.CitationsEnabled()` to prepare metadata only for eligible output.
