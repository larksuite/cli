# Merge Request Preview Package

The tarball a reviewer or QA installs to run one merge request's CLI without
building it. Every pipeline run produces one, so nobody has to know this
exists to receive a package. The public counterpart is
`.github/workflows/pkg-pr-new.yml`, which uploads to pkg.pr.new; that service
is public and unreachable from here, so the tarball is a pipeline artifact
instead.

## Pieces

| Piece | Role |
| --- | --- |
| `../pipelines/ci.yaml` → `preview_package` | Merge-gate job. Builds, uploads the artifact, prints the install command. Gates on `fast_gate`, holds no credentials. |
| `config.env` | Package name and the platforms a preview carries. The only place these are spelled out. |
| `build.sh` | Cross-compiles one binary per platform and calls the packer. Runs the same by hand as in the job. |
| `build-preview-package.js` | Derives the preview `package.json`, stages the tree through the release packer's `stageTree`, packs it. |
| `pipeline.test.sh` | Contract of the job: automatic, gated on `fast_gate`, no credentials, nothing outside `.codebase/`, root-package build. |
| `scripts.test.sh` | `build.sh` end to end against a fixture repository and a stub `go`. |
| `build-preview-package.test.js` | Version rules, package derivation, binary collection, staging. |

## What a preview is, and is not

The layout is the bundled one staged by `stageTree` in
`../release/build-registry-package.js`: binaries under `bin/`, the same
launcher and postinstall. The released package no longer ships this way — it
splits the binaries into one platform package per `<goos>-<goarch>` that the
launcher resolves as an optional dependency — but a preview is installed from
a tarball and cannot depend on published packages, so it keeps every binary
inside and prunes after install. Four more things differ.

**The binaries come from `go build`, not GoReleaser.** A preview needs two
platforms and neither a changelog nor checksums. Add platforms in
`config.env`; each costs about 14 MB in the tarball and a cross-compile in the
job.

**The build is `go build .`, not `go build ./main.go`.** The root package
carries the `//go:embed` of `skills/` and `affordance/`, so `lark-cli skills`
serves the branch's content. The public preview build uses `./main.go` to drop
that content and stay under pkg.pr.new's size limit. Nothing here has that
limit, and a preview whose skills are empty is wrong exactly where previews are
used most — checking how an agent behaves against this branch.

**The version is a prerelease naming the merge request and the commit**, so
`lark-cli --version` identifies the build under test. The commit carries a `g`
prefix because an all-digit short hash would become a numeric semver
identifier, which npm rejects when it has a leading zero.

**It can never be published or updated.** The package is `private`, which
`libnpmpublish` refuses unconditionally before it sends anything — note that
`npm publish --dry-run` returns before reaching that check, so a dry run is
not a way to verify this. It also declares no `publishConfig`, which the
launcher reads as "no channel to move along": `lark-cli update` on a preview
build says so and points at the released package rather than resolving
`latest` of something else.

`skills/` travels inside the package. The binary embeds its own copy, but an
agent needs the files on disk and the intranet has no git host that
`npx skills add` can clone from.

## Producing a preview

Push to a merge request. The `Preview Package` job runs on every pipeline,
uploads the tarball as the `preview-package` artifact and prints the install
command in its log. The version names the branch and the commit, so
`lark-cli --version` says which build it is.

Automatic means required: this platform has no observe-only check, so a
failure here blocks the merge. The job gates on `fast_gate` for that reason —
a branch that does not compile fails there, and this job never runs to report
the same thing again.

By hand, which is the same script the job runs:

```bash
.codebase/preview/build.sh                          # labelled with the branch
PREVIEW_LABEL=mr2571 .codebase/preview/build.sh     # or name it yourself
PREVIEW_PLATFORMS=darwin/arm64 .codebase/preview/build.sh   # one platform, ~7s
```

Measured on a warm cache: 12-34s and 28.4 MB for both darwin platforms, 7s
and 14.6 MB for one. The output lands in `preview-package/`, which git does
not ignore — remove it before committing, the same way `release-candidate/`
is handled.

## Installing one

```bash
npm uninstall -g @larksuite/cli @byted/larksuite-cli
npm install -g ./byted-larksuite-cli-preview-<version>.tgz
lark-cli --version                    # the version names the merge request

npx skills add "$(npm root -g)/@byted/larksuite-cli-preview/skills/lark-<domain>" -y -g
```

All three packages provide the same `lark-cli` executable and cannot coexist.
Reinstall a released package to leave the preview.

## Why the job holds no credentials

It runs branch code on every merge request. A registry token reaching it would
let any author publish under the release name — the code that would use the
token is the code under review. So the job declares no `envs`,
`pipeline.test.sh` fails if a credential-shaped name appears anywhere in it,
and the package it builds is `private` as a second lock.

This is also why the tarball is an artifact rather than a registry version:
there is nothing to authenticate against.

## Next steps

Neither changes this directory's build.

- **Host the tarball behind a URL** so the install collapses to one line
  (`npm install -g https://.../<sha>.tgz`; npm takes an http tarball). An
  internal bucket with a lifecycle rule matches pkg.pr.new's ephemerality, and
  a write-only bucket credential has a far smaller blast radius than a
  registry token.
- **Post the install command to the merge request.** Whether this platform can
  comment from a pipeline is unconfirmed; until then the command is in the job
  log. The label falls back to the branch for the same reason — the platform
  variable carrying the merge request number is unconfirmed, and `fast_gate`
  already prints the injected `CI_*` names.
