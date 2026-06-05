# lark-cli plugin examples

Runnable fork-and-blank-import examples that demonstrate the Plugin
SDK in production-shape. Each subdirectory is a complete `main`
package: `go build .` produces a working CLI.

| Example | What it shows |
| --- | --- |
| [audit-observer](./audit-observer/) | Simplest possible plugin: one Observer matching every command, logs to stderr. |
| [readonly-policy](./readonly-policy/) | Policy plugin: `Restrict()` with `MaxRisk=read`, demonstrates the `FailClosed` + `Restricts=true` auto-pairing. |

All examples are built by CI (`make examples-build`) so they cannot
silently drift from the SDK.
