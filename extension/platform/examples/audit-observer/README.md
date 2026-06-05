# Example: audit observer

The simplest possible lark-cli plugin: one After observer that logs
every dispatched command to stderr (success or failure).

## Build & run

```sh
cd extension/platform/examples/audit-observer
go build -o audit-cli .
./audit-cli config plugins show
# {"plugins":[{"name":"audit", ...}], "total":1}

./audit-cli api GET /open-apis/contact/v3/users/me
# [audit] api ok            (on stderr)
```

## Key points

- `platform.NewPlugin(...).MustBuild()` from `init()`. The blank
  import of this package in `main.go` triggers `init()`.
- `Observer(platform.After, ...)` runs **after** the command's RunE,
  even on failure (Observers cannot prevent execution).
- `FailOpen()` means: if Install ever fails, the binary logs a
  warning and continues without this plugin. Right default for
  audit-only plugins.
