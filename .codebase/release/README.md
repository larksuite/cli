# Internal Release

How a `v*` tag on `code.byted.org/lark/cli` becomes `@byted/larksuite-cli` on
`https://bnpm.byted.org`. Everything internal lives in this directory and in
`.codebase/pipelines/release.yaml`; nothing outside `.codebase/` is changed.
The public counterpart is `.github/workflows/release.yml`.

## Pieces

| Piece | Role |
| --- | --- |
| `../pipelines/release.yaml` | Tag-triggered pipeline: preflight, build, verify_install, publish. Its header explains each departure from the public workflow. |
| `config.env` | Package name, registry and Go toolchain pin. The only place these are spelled out. |
| `source.sh` | Tag equals the package version and the channel's source policy holds — both from the public `scripts/release-preflight.js`, unchanged: a **stable** tag must point at a commit in `origin/main`, a **beta** tag may be cut from a feature branch. Re-run by every stage. |
| `build.sh` | The unchanged public GoReleaser build, a toolchain check of every binary, then packaging. |
| `build-registry-package.js` | Derives the internal `package.json` files from the public one: the launcher package plus one platform package per GoReleaser binary. |
| `run.js` | Launcher shipped as the package's `scripts/run.js`: runs the binary from the installed platform package (or from `bin/` when bundled, as in a preview), implements `lark-cli update` against the internal registry, silences the public update notifiers. |
| `postinstall.js` | Preview package only: best-effort deletion of the bundled binaries for other platforms after install. |
| `verify.sh` | `npm install -g` of the launcher tarball with this platform's package tarball into a scratch prefix; the CLI must report the candidate version, have exactly one binary, and route `update` through the launcher. |
| `publish.sh` | Publication policy from the public `scripts/release-publish-policy.js` applied to every package, then `npm publish` of the verified tarballs, platform packages first. |
| `registry-version.sh` | Tells "version exists" from "version missing" from "could not tell" on the registry. |
| `*.test.*` | Contract test of the pipeline, behavioural tests of the scripts against a real git repository and a stub `npm`, launcher tests. Run by the internal CI's `script_test` job. |

The binaries are the public build, byte for byte. What makes the package
internal is the package it ships in, the registry it is published to, and the
launcher in front of the binary. Versions and channels are the public ones:
`vX.Y.Z-beta.N` publishes under the `beta` dist-tag, `vX.Y.Z` under `latest`,
and the package version is the source `package.json` version.

## Package layout

One release is eight packages at the same version:

| Package | Contents | Installed when |
| --- | --- | --- |
| `@byted/larksuite-cli` | `scripts/run.js` (the launcher), `CHANGELOG.md`, `LICENSE`, `README.md`; no binary. Names every platform package below as an `optionalDependency` pinned to its own version. | always |
| `@byted/larksuite-cli-<goos>-<goarch>` | `bin/lark-cli` (or `lark-cli.exe`) for one platform, `os`/`cpu` restricted to it. Seven of them: `darwin`/`linux`/`windows` × `amd64`/`arm64`, plus `linux-riscv64`. | only on a matching host |

npm resolves the optional dependencies, keeps the one whose `os`/`cpu` match
the host, and skips the rest without downloading them. A cold `npm install -g`
therefore fetches the launcher (tens of KB) and one binary (about 14 MB
compressed) rather than every platform, and needs no postinstall to prune. The
launcher finds the binary by resolving `<its own name>-<goos>-<goarch>` from
its install location, which also works when the two packages are installed
from tarballs side by side, as `verify.sh` does before anything is published.

This is the layout esbuild, swc and Biome use for the same problem. The
platform packages have no `bin` field: the launcher owns the `lark-cli`
command, and two links to the same name would fight.

### Installing without npm

A platform package is a plain tarball with the binary at `package/bin/lark-cli`,
so an environment without Node — a cold-started sandbox, a container image —
can take it straight from the registry:

```bash
version=1.0.95   # or: curl -fsSL https://bnpm.byted.org/@byted/larksuite-cli/latest | jq -r .version
curl -fsSL "https://bnpm.byted.org/@byted/larksuite-cli-linux-amd64/-/larksuite-cli-linux-amd64-${version}.tgz" \
  | tar -xz --strip-components=2 -C /usr/local/bin package/bin/lark-cli
# The public binary's own updater and update notices point at @larksuite/cli
# on npmjs.org; the launcher normally switches them off. Do it here instead.
export LARKSUITE_CLI_NO_UPDATE_NOTIFIER=1
export LARKSUITE_CLI_NO_SKILLS_NOTIFIER=1
lark-cli --version
```

The binary is the public build and runs on its own, but there is no launcher
in front of it: **do not run `lark-cli update` on such an install.** That
command is the binary's own updater, which knows only `@larksuite/cli` on
npmjs.org and would install the public build over this one (or fail without
npm). Upgrading is fetching the next version's tarball the same way. The two
variables above silence the update and skills notices that would otherwise
point at the public release; set them in the image or profile.

## One-time setup (before the first tag)

1. **Publish token.** Create the pipeline variable `BNPM_AUTH_TOKEN` under
   Pipelines → Variables with a token that may publish `@byted/larksuite-cli`
   on bnpm.byted.org. A pipeline that references an undefined variable fails to
   parse, and a re-run never sees a variable added later.
2. **Protected tags.** Restrict creation of `v*` tags to the release owners.
   This is the approval boundary: there is no Environment approval, and
   `manual: true` gates only MR-triggered jobs.
3. **Runner access.** build reaches `goproxy.byted.org` (modules, the pinned
   toolchain and its checksum database under `/sumdb/sum.golang.org`),
   `mirrors.byted.org` (Node) and `open.feishu.cn` (API metadata);
   verify_install and publish reach `bnpm.byted.org`.

## Cutting a release

1. Merge the release change into `main`: `package.json` and `package-lock.json`
   at the new version, `CHANGELOG.md` updated.
2. From an up-to-date `main` checkout run `./scripts/tag-release.sh`. It checks
   the version, requires HEAD to be the tip of `origin/main`, and pushes the tag
   to `origin`, which is the internal repository.
3. Watch the `Release` pipeline for the tag. `verify_install` proves the
   tarball installs and runs; `publish` is the only job that holds the token.
4. Install the result on a real machine. The public package cannot stay
   installed next to it — both provide the `lark-cli` executable:

   ```bash
   npm uninstall -g @larksuite/cli
   npm install -g @byted/larksuite-cli@latest --registry https://bnpm.byted.org
   lark-cli --version
   ```

## Cutting a beta from a branch

A beta exists to be installed before the work lands, so `source.sh` accepts a
beta tag on a feature branch; only stable tags must come from `main`.
`scripts/tag-release.sh` is the public stable path and refuses any branch but
`main`, so a branch beta is tagged by hand after the same two checks it makes:

```bash
node scripts/release-preflight.js --tag "vX.Y.Z-beta.N"   # tag == package.json == package-lock.json
git push origin HEAD                                      # the tagged commit must be on origin
git tag "vX.Y.Z-beta.N" && git push origin "refs/tags/vX.Y.Z-beta.N"
```

The version bump lives on the branch and must not reach `main` as it is: before
merging, restore `main`'s version or raise it to the next release number, or
`main` would carry a prerelease version that is older than what `latest`
already serves.

## Installing and updating

```bash
npm uninstall -g @larksuite/cli                    # same `lark-cli` executable; the two cannot coexist
npm install -g @byted/larksuite-cli --registry https://bnpm.byted.org
lark-cli update --check                            # compares with the internal registry's `latest`
lark-cli update                                    # npm install -g of the new version, verify, sync skills
```

`lark-cli update` is served by the package's launcher, not by the binary: it
reads `latest` from the internal registry, runs `npm install -g` from it,
checks `lark-cli --version`, then syncs the public skills bundle exactly like
the public updater (`npx skills add https://open.feishu.cn/lark-cli/skills/regular`).
`--check` and `--json` are supported; other flags of the public `update` are
not. A pnpm install is told the `pnpm add -g` command to run instead. There is
no proactive update notice: releases are announced, and `update --check` is the
on-demand check.

## Recovery

Re-run the failed **job**, never the pipeline; the `release-candidate`
artifact lives a week and nothing else in the run has to be repeated.

| Failed job | Cause | Action |
| --- | --- | --- |
| preflight | Tag/version mismatch, stable commit not in main, version already published | Fix the source and tag a new version. |
| build | Proxy, metadata or toolchain download failure | Re-run the job. A code or configuration failure needs a new version. |
| verify_install | The candidate does not install or reports the wrong version | The build is wrong; fix and tag a new version. Nothing was published. |
| publish | Network or registry error | Re-run the job: every already published identical tarball only has its dist-tag advanced, a channel that moved ahead is left alone, and the packages not yet published are published. Platform packages go first, so a launcher on the registry always has its platform packages there too. |
| publish | "different package integrity" | The registry holds other bytes under this version for the named package. Ship a new version. |

A published version is never overwritten or unpublished; a fix is the next
version. A tag whose run failed before `publish` can be deleted and re-created
after the fix (`git push origin :refs/tags/vX.Y.Z`).

## Rehearsal without publishing

```bash
git init --bare /tmp/rehearsal.git && git remote add rehearsal /tmp/rehearsal.git
git push rehearsal HEAD:refs/heads/main && git tag vX.Y.Z && git push rehearsal vX.Y.Z
RELEASE_REMOTE=rehearsal GOPROXY=https://goproxy.byted.org bash .codebase/release/build.sh vX.Y.Z
bash .codebase/release/verify.sh release-candidate
rm -rf release-candidate && git tag -d vX.Y.Z && git remote remove rehearsal
```

`GORELEASER=/path/to/goreleaser` skips installing GoReleaser through the proxy.
The candidate is written to `release-candidate/` at the repository root, which
is not ignored by git; remove it before the next GoReleaser run, which refuses
a dirty tree.

## Known gaps

- **Two update implementations.** The binary's own updater knows only
  `@larksuite/cli` on npmjs.org and would install that over this package, so
  the launcher owns `update` and the binary never sees it. The launcher covers
  npm installs, `--check` and `--json`; pnpm installs and the other public
  `update` flags are not automated. The binary's update notices are switched
  off because they would point at the public updater.
- **No install wizard.** `lark-cli install` (the public onboarding wizard) is
  not shipped; use `lark-cli config init` and `lark-cli auth login`.
- **Skills.** `skills` still syncs from the public skills index.
- **macOS signing and notarization.** Internal binaries are unsigned, like the
  earlier hand-built `@byted/lark-cli@1.0.42-byted.1`. A CLI started from a
  terminal is not Gatekeeper-checked, but this is not parity with the public
  release.
- **Optional dependencies are optional.** An install run with
  `--omit=optional` (or `--no-optional`) gets the launcher without a binary.
  The launcher then names the platform package and the `npm install -g`
  command that adds it; it does not download anything itself. A platform
  package added that way is its own global root and is not replaced when the
  launcher is next upgraded under the same setting, so the launcher refuses
  to run a platform package whose version is not its own and names the
  install command again, rather than reporting one version from `--version`
  and another from `update`.
- **Eight publishes per release.** The registry sees eight packages instead
  of one; `preflight` only checks the launcher for "already published", and
  `publish` reconciles each package independently, so a partial run is
  completed by re-running the job rather than tagging again.
- **Concurrency.** Codebase has no concurrency groups. Two release tags
  pushed at once could race on the dist-tag; the publish policy re-reads the
  channel right before publishing, which narrows but does not close the window.
- **Version scheme.** Because the package version is the source version, a
  second internal release of the same upstream version needs a new version
  number in `package.json` on `main`, which then has to be merged with the next
  upstream bump. The earlier `-byted.N` suffix is not accepted by
  `scripts/release-preflight.js`.
