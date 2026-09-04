# Extension

Embed lark-cli into your own Agent or application — swap credential sources, audit every command, restrict the command surface — without modifying CLI source. Write a Go package against these interfaces, import it from a wrapper `main`, and build your own enhanced binary.

Main extension points:

| Package | Extension point | What it does |
| ------- | --------------- | ------------ |
| [`credential/`](./credential/) | **Credential** | Bring your own credential source: database, Vault, config center… |
| [`transport/`](./transport/) | **Transport** | Register one aggregate provider for request interception, URL rewriting, and an optional distribution manifest |
| [`platform/`](./platform/) | **Restrict · Observer · Wrap · On** | Command allow/deny rules, audit hooks, onion-style middleware (approval gates, rate limiting), process lifecycle — see the [Plugin SDK README](./platform/README.md) |

📖 Full guide: [Embed lark-cli in your Agent](https://open.larksuite.com/document/mcp_open_tools/feishu-cli/embed-feishu-cli-in-agent) ([中文](https://open.larkoffice.com/document/mcp_open_tools/feishu-cli/embed-feishu-cli-in-agent))

The transport registry has one process-wide owner. Register the aggregate
provider during `init`, before constructing or executing the CLI. URL rewriting
runs before the request interceptor and also covers CLI-owned presentation URLs
and URLs passed to child processes. `ScopedProvider` limits only the request
interceptor.

When `DistributionProvider` returns a manifest URL, that URL and the artifact
URLs inside the manifest are final download addresses. Distribution downloads
retain lark-cli's built-in proxy and custom-CA policy, but deliberately bypass
the registered URL rewriter and request interceptor.

## Distribution manifest protocol

The manifest is JSON with this fixed schema:

```json
{
  "schema": 1,
  "version": "1.2.3",
  "artifacts": {
    "darwin-arm64": { "url": "https://dist.example/lark-cli-darwin-arm64.tar.gz", "checksum": "sha256:<64 lowercase hex characters>" },
    "skills": { "url": "https://dist.example/skills.tar.gz", "checksum": "sha256:<64 lowercase hex characters>" }
  }
}
```

`schema`, `version`, and `artifacts` are required; unknown fields are ignored.
`version` is an exact opaque target; the staged binary is verified by running
`<binary> --version`, whose output must be exactly `lark-cli version <version>`.
`artifacts` must contain `skills` and a key named
`<GOOS>-<GOARCH>` for every published platform. URLs must be absolute HTTP or
HTTPS URLs. Checksums cover the downloaded archive bytes.

Archives may be zip or gzip-compressed tar files. A binary archive contains
`lark-cli` at its root (`lark-cli.exe` on Windows). A Skills archive contains
one directory per Skill at its root, for example `lark-doc/SKILL.md`.
