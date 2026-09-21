# lark-cli business command sets

`extension/command` is the build-time contract for adding business commands to
a lark-cli distribution. A wrapper `main` passes command sets to
`cmd.WithCommandSets`; the host compiles them into the same typed shortcut
machinery as the built-in `+` commands, so each command gets tag-driven flags,
`--dry-run`, `--format`, typed errors, scope pre-flight, policy rules and
plugin hooks without per-command wiring.

```go
os.Exit(cmd.ExecuteWithOptions(
    cmd.WithCommandSets(
        command.Set{Domain: command.ExtendDomain(command.DomainIm), Commands: []command.Command{chatBrief}},
        command.Set{
            Domain:   command.NewDomain(command.DomainSpec{Name: "acme-hr", Description: "Acme HR employee records"}),
            Commands: []command.Command{employeeGet},
        },
    ),
))
```

Declare a command with `Definition` and `Define`; drive its hooks in unit tests
with [`commandtest`](./commandtest/). [examples/chat-brief](./examples/chat-brief/)
is a runnable distribution.

## Choose the domain

A set mounts under exactly one domain.

| Declaration | Use when | What the host does |
| --- | --- | --- |
| `ExtendDomain(command.DomainIm)` | the commands belong to a domain lark-cli ships (the `Domain*` constants) | mounts them next to the built-in commands; help, `auth login --domain` and policy already know the domain |
| `NewDomain(DomainSpec{Name, Description})` | the commands form a domain lark-cli does not ship | mounts a root command with that description under "Lark domains:", accepts `auth login --domain <name>` with the set's declared scopes, tags the commands for policy rule globs and `ByDomain` selectors, and reads the `affordance/<name>.md` and skill the distribution ships |

Every command's `Metadata.Service` is the set's domain name.

A new domain is validated when the CLI starts; a violation installs the same
fail-closed guard as any invalid contribution (`validation/failed_precondition`
on every command):

- `Name` is lowercase kebab-case starting with a letter (`acme-hr`) and is not
  a built-in domain or one of its aliases (use `ExtendDomain`), a lark-cli root
  command (`auth`, `config`, ...), or `all`.
- One set declares a domain, and `ExtendDomain` cannot target it. Concatenate
  the command slices instead of splitting a domain across sets.
- `Description` is one trimmed line.

Prefix the name with the distribution (`acme-hr`, not `hr`): a later lark-cli
release that ships a built-in domain of the same name refuses to start the
build instead of silently merging into it.

`commandtest.ValidateSets(sets...)` runs the same compiler in the
distribution's unit tests, so every violation but a collision with a root
command such as `auth` fails there before the binary refuses to start.

## Ship the domain's guidance

lark-cli renders help from two content trees the wrapper wires (see
[Ship skills and command guidance](../platform/README.md#ship-skills-and-command-guidance)):

- `affordance/<name>.md`: `> skill: <skill>` and one `## +command` section per
  command with tips and examples. Without it, `lark-cli <name> --help` points
  at a `lark-<name>` skill only when one exists.
- `skills/<skill>/SKILL.md`: through `cmd.SetEmbeddedSkillContent` or a plugin
  `SkillsOverlay.Overlay`.

[testdata/wrapper](./testdata/wrapper/) declares `acme-hr` with both, and
`wrapper_e2e_test.go` asserts the resulting help, completion and login surface.

## Outside this contract

Typed API (catalog) commands and event keys stay repository-owned; a new domain
carries shortcut-style commands only. Brand restrictions, domain aliases and
auth-domain folding apply to built-in domains only.
