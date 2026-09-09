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
| `source.sh` | Tag equals the package version (public `scripts/release-preflight.js`, unchanged) and the tagged commit is in `origin/main` — stable **and** beta. Re-run by every stage. |
| `build.sh` | The unchanged public GoReleaser build, a toolchain check of every binary, then packaging. |
| `build-registry-package.js` | Derives the internal `package.json` from the public one and ships every GoReleaser binary under `bin/`. |
| `run.js` | Launcher shipped as the package's `scripts/run.js`: runs `bin/lark-cli-<os>-<arch>`, implements `lark-cli update` against the internal registry, silences the public update notifiers. |
| `postinstall.js` | Best-effort: deletes the binaries for other platforms after install. |
| `verify.sh` | `npm install -g` of the tarball into a scratch prefix; the CLI must report the candidate version, keep one binary, and route `update` through the launcher. |
| `publish.sh` | Publication policy from the public `scripts/release-publish-policy.js`, then `npm publish` of the verified tarball. |
| `registry-version.sh` | Tells "version exists" from "version missing" from "could not tell" on the registry. |
| `*.test.*` | Contract test of the pipeline, behavioural tests of the scripts against a real git repository and a stub `npm`, launcher tests. Run by the internal CI's `script_test` job. |

The binaries are the public build, byte for byte. What makes the package
internal is the package it ships in, the registry it is published to, and the
launcher in front of the binary. Versions and channels are the public ones:
`vX.Y.Z-beta.N` publishes under the `beta` dist-tag, `vX.Y.Z` under `latest`,
and the package version is the source `package.json` version.

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
| preflight | Tag/version mismatch, commit not in main, version already published | Fix the source and tag a new version. |
| build | Proxy, metadata or toolchain download failure | Re-run the job. A code or configuration failure needs a new version. |
| verify_install | The candidate does not install or reports the wrong version | The build is wrong; fix and tag a new version. Nothing was published. |
| publish | Network or registry error | Re-run the job: an already published identical tarball only has its dist-tag advanced, a channel that moved ahead is left alone. |
| publish | "different package integrity" | The registry holds other bytes under this version. Ship a new version. |

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
- **Package size.** Seven binaries make the tarball roughly 100 MB; the
  largest package published so far is 44 MB. bnpm's limit is not documented,
  so the first publish is the test. If it is refused, split the package per
  platform.
- **Concurrency.** Codebase has no concurrency groups. Two release tags
  pushed at once could race on the dist-tag; the publish policy re-reads the
  channel right before publishing, which narrows but does not close the window.
- **Version scheme.** Because the package version is the source version, a
  second internal release of the same upstream version needs a new version
  number in `package.json` on `main`, which then has to be merged with the next
  upstream bump. The earlier `-byted.N` suffix is not accepted by
  `scripts/release-preflight.js`.
