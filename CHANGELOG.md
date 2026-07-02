# Changelog

All notable changes to this project will be documented in this file.

## [v1.0.64] - 2026-07-02

### Features

- **im**: Upgrade card send to Card 2.0 with full component reference (#1688)
- **im**: Add `+chat-members-list` shortcut for member listing (#1398)
- **okr**: Semi-plain text format with mention position preservation and `patch` shortcut (#1671)

### Bug Fixes

- **cli**: Point permission-apply link at official `/page/scope-apply` entry (#1722)
- **cli**: Improve secure label error handling (#1707)
- **cli**: Reduce public content token false positives
- **cli**: Increase npm registry fetch timeout to 15s during update check (#1724)
- **doc**: Align word statistics compound tokens (#1706)

### Documentation

- **approval**: Add detailed command-to-reference mapping for the approval skill (#1630)
- **doc**: Support `reference_map` in docs (#1690)
- **slides**: Refresh generation guidance — add constraints, drop template toolchain, and inline lint XML fixtures

## [v1.0.62] - 2026-07-01

### Features

- **vc**: Add meeting message send shortcut (#1643)
- **doc**: Add document word statistics helper (#1697)
- **cli**: Interactive upgrade prompt for bare `lark-cli` invocation (#1498)
- **install**: Fail closed when `checksums.txt` is missing during install (#1503)

### Bug Fixes

- **drive**: Improve batch failure handling for push/pull/sync (#1703)
- **base**: Support JSON array input for field create (#1661)
- **task**: Expose completion state in `my tasks` output (#1641)
- **cli**: Reduce public content credential false positives (#1700)

## [v1.0.61] - 2026-06-30

### Features

- **apps**: Add `db`, `file`, `openapi-key` and observability shortcuts (#1596)
- **identity**: Add `whoami` command showing effective identity (#1666)
- **docs**: Add reference map flags (#1547)

### Bug Fixes

- **identity**: Correct identity diagnosis under external credential providers (#1693)
- **cli**: Harden git credential error handling (#1676)

### Documentation

- **doc**: Guide document copy skill usage (#1673)
- **doc**: Fix lark-doc media token examples (#1662)

## [v1.0.60] - 2026-06-29

### Features

- **affordance**: Per-command usage guidance system with markdown source (#1565)
- **event**: Support VC meeting lifecycle events (#1632)
- **sheets**: Use `office_sheet_file` parent_type for imported office spreadsheets (#1606)
- **authorization**: Expand lark-shared auth guidance and assert clean logout JSON (#1598)
- **transport**: Add `LARK_CLI_NO_PROXY_WARN` to silence proxy warning (#1647)

### Bug Fixes

- **install**: Load `@clack/prompts` via dynamic import to avoid `ERR_REQUIRE_ESM` (#1652)

### Tests

- **doc**: Derive fetch test flag defaults from `v2FetchFlags` (#1428)

### Build

- **ci**: Reduce public content false positives

## [v1.0.59] - 2026-06-26

### Features

- **slides**: Add `+replace-pages` and `xml get` shortcuts, and expose the presentation URL (#1585)
- **minutes**: Support speaker list and no-Lark speaker replace (#1594)
- **calendar/vc/minutes**: Optimize and extend calendar, vc, minutes, and note shortcuts and skills (#1571)

### Bug Fixes

- **docs**: Hide docs `api-version` compat flag (#1580)

## [v1.0.58] - 2026-06-25

### Features

- **sheets**: Typed table I/O and error contract, workbook import/export, and skill refresh (#1355)
- **base**: Add Base URL and title resolve shortcuts (#1338)
- **drive**: Add `+member-add` shortcut with wiki space member collection collaborator support (#1204)
- **doc**: Support `create` title option (#1536)
- **doc**: Add `im-markdown` output format for doc fetch (#1550)
- **whiteboard**: Export whiteboard as SVG and update whiteboard via SVG (#1559)
- **card**: Support `card.action.trigger` event with auto-fetched card content (#1528)
- **task**: Add task event consumer (#1510)

### Bug Fixes

- **doc**: Prefix docs resource shortcuts (#1564)
- **binding**: Skip unix mode audit on Windows (#1525)

### Documentation

- **approval**: Sync approval skill for meta API commands (#1499)
- **doc**: Restore lark-doc style requirements (#1579)
- **im**: Document `chat.nickname` get/update/delete (#1378)
- **im**: Clarify audio message opus requirement (#1271)

### Build

- **ci**: Add public content safeguards and reduce false positives

## [v1.0.57] - 2026-06-23

### Features

- **slides**: Add `+screenshot` to capture slide page images (or render a single `<slide>` XML snippet), returning the local file path instead of Base64 (#1358)
- **base**: Support record comments (#1043)
- **search**: Surface search API notices (#1413)

### Bug Fixes

- **mail**: Resolve folder/label filter once per `+triage list` call (#1512)
- **meta**: Backfill enum value descriptions from options (#1541)
- **cli**: Add missing CLI headers for git credential helper (#1539)

### Documentation

- **doc**: Refine rich block, path, and block ID guidance (#1508)
- **mail**: Trim lark-mail skill context (#1527)
- **drive**: Add permission governance workflow guidance (#1292)

### Build

- **ci**: Bind semantic review to workflow run head (#1551)

## [v1.0.56] - 2026-06-18

### Features

- **apps**: Add `+session-messages-list` for session turn reply messages (#1402)

### Bug Fixes

- **api**: Align API success envelopes (#1489)
- **base**: Reject out-of-range pagination flags (#1495)

### Refactor

- Retire legacy error envelopes and enforce typed contract (#1449)

### Documentation

- **skills**: Soften lark-doc style guidance (#1463)

### Build

- Add CI quality gate with semantic review

## [v1.0.55] - 2026-06-16

### Features

- **vc**: Support agent meeting event workflows (#1483)
- **drive**: Support exporting Base structure snapshots (#1481)
- **doc**: Add docx cover resource commands (#1468)
- **doc**: Support `lang` for docx fetch v2 (#1459)
- **event**: Optimize subscription precheck, links, and consumer guard (#1447)

### Bug Fixes

- **drive**: Validate drive import folder target (#1485)

## [v1.0.54] - 2026-06-15

### Features

- **mail**: Auto-attach default signature on send/reply/forward (#1415)
- **drive**: Support `original_creator_ids` filter in search (#1046)
- **cli**: Simplify proxy plugin warning and gate it on TTY (#1448)

### Bug Fixes

- **doc**: Fix docs fetch and update ergonomics (#1466)
- **vfs**: Reject blank local paths (#1460)
- **vfs**: Reject Windows absolute paths cross-platform (#1401)
- **event**: Clarify remote bus blocker recovery (#1454)

### Refactor

- Converge command pipelines onto a typed metadata model + catalog (#1191)

### Documentation

- **im**: Document `@mention` format per message type (text/post/card) (#1419)
- **doc**: Clarify lark-doc create title guidance (#1474)
- **skills**: Add rename prompt for import without `--name` (#1461)
- **apps**: Drop Miaoda brand word from apps command help text (#1399)

## [v1.0.53] - 2026-06-12

### Features

- **auth**: Revoke user tokens server-side on `auth logout` (#1434)
- **auth**: Add `--json` flag support to auth subcommands (#1431)
- **token**: Mint TAT via unified OAuth v3 Token Endpoint (#1408)
- **note**: Split note into a dedicated domain with `+detail` and `+transcript` flows (#1345, #1417, #1435)
- **im**: Unify sort flags into `--sort` field and `--order` direction (#1302)

### Bug Fixes

- **apps**: Read release error_logs from `data.error_logs` in `+release-get` (#1436)

### Documentation

- **skills**: Optimize whiteboard skill (#1371)
- **skills**: Optimize okr skill (#1368)

## [v1.0.52] - 2026-06-11

### Features

- **events**: Per-resource subscription identity + Match hook (#1185)
- **apps**: Emit typed error envelopes across the apps domain (#1288)
- **wiki**: Emit typed error envelopes across the wiki domain (#1350)
- **im**: Add `--chat-modes` filter to chat search (#1317)
- **apps**: Exclude `.git` directory from `+html-publish` package (#1396)
- **build**: Support riscv64 prebuilt binaries in release and install pipeline

### Bug Fixes

- **apps**: Support git credential dry-run (#1390)
- **whiteboard**: Fix parsing empty whiteboard content (#1391)
- **build**: Make `-race` flag arch-conditional to support riscv64

### Documentation

- **im**: Document `chat.user_setting` batch_query/batch_update (#1339)
- **im**: Document `chat.managers` and `chat.moderation` API resources (#1294)
- **skills**: Optimize lark-drive skill routing (#1284)
- **skills**: Expand cite user guidance and fix typos (#1394)

## [v1.0.51] - 2026-06-10

### Features

- **apps**: Support multi dev modes (#1175)
- **im**: Complete audio/post rendering and add opt-in `--download-resources` (#1245)
- **base**: Configure initial base table schema (#1377)
- **vc**: Add recording event support (#1369)
- **minutes**: Replace words for transcript (#1372)
- **markdown**: Emit typed error envelopes across the markdown domain (#1347)
- **sheets**: Emit typed error envelopes across the sheets domain (#1348)
- **slides**: Emit typed error envelopes across the slides domain (#1349)

### Documentation

- **skills**: Warn about `@file` absolute path restriction in lark-doc skills (#1375)
- **skills**: Remove unsupported ⚠️ from callout emoji list (#1374)

## [v1.0.50] - 2026-06-09

### Features

- **doc**: Emit typed error envelopes across the doc domain (#1346)
- **event**: Emit typed error envelopes across the event domain (#1289)
- **contact**: Emit typed error envelopes across the contact domain (#1287)
- **sheets**: Guard `+csv-put --csv` against a path passed without `@` (#1337)
- **cli**: Adjust agent timeout hint output conditions (#1328)

### Bug Fixes

- **drive**: Add `@file`/stdin support to `+add-comment --content` (#1343)
- **slides**: Build create URL locally instead of drive metas call (#1329)
- **cli**: Clarify `--block-id` supports comma-separated batch delete in help text (#1336)

### Documentation

- **doc**: Replace append with `block_insert_after` in skeleton workflow guidance (#1340)
- **doc**: Document `<folder-manager>` resource block (#1168)
- **drive**: Add drive comment location guidance (#1258)

## [v1.0.49] - 2026-06-08

### Features

- **events**: Add whiteboard event domain with per-board subscription (#1265)
- **im**: Support feed group (#1102)
- **im**: Add feed shortcut create, list, and remove shortcuts (#1273)
- **im**: Format feed group error handling (#1308)
- **im**: Return typed error envelopes across the im domain (#1230)
- **base**: Emit typed error envelopes across the base domain (#1248)
- **calendar**: Emit typed error envelopes across the calendar domain (#1232)
- **task**: Emit typed error envelopes across the task domain (#1231)
- **okr,whiteboard**: Emit typed error envelopes across both domains (#1236)
- **minutes,vc**: Emit typed error envelopes across both domains (#1234)
- **markdown**: Harden create upload failures (#1325)
- **drive**: Harden inspect shortcut failures (#1324)
- **slides**: Add IconPark lookup for Lark slides (#1123)
- **doc**: Remove docs v1 API (#1291)
- **cli**: Add `skills` command to read embedded skill content (#1318)
- **cli**: Fetch official skills index (#1301)
- **shared**: Document relative-path-only file arguments (#1319)
- **scopes**: Clear `recommend.allow` scope auto-approve overrides (#1272)
- **shortcuts**: Check shortcut example commands against the live CLI tree (#1244)

### Bug Fixes

- **events**: Keep bounded event consume runs alive after stdin EOF (#1285)
- **drive**: Use docs secure label read scope (#1281)

### Documentation

- **approval**: Restructure skill with intent table and scope boundaries (#1307)
- **skills**: Tighten drive and markdown guardrails (#1326)
- **skills**: Optimize calendar, vc, and minutes skill guidance (#1269)
- **markdown**: Add markdown domain template (#1293)
- **markdown**: Improve lark-markdown skill guidance (#1279)
- **doc**: Improve lark-doc skill guidance (#1283)
- **wiki**: Optimize skill guidance and routing boundaries (#1275)
- **slides**: Tighten routing/boundary and reconcile in-slide whiteboard (#1169)

## [v1.0.48] - 2026-06-04

### Features

- **mail**: Preserve mailbox context in `+triage` output for public mailboxes (#1238)
- **contact**: Add contact skill domain guidance (#1144)

### Bug Fixes

- **skills**: Use JSON skills list during update (#1251)

### Documentation

- **drive**: Refine lark-drive knowledge organize workflow (#1253)
- **vc-agent**: Require explicit leave request (#1260)
- **slides**: Add whiteboard element documentation and improve slide guidance (#1029)

## [v1.0.47] - 2026-06-03

### Features

- **sheets**: Add spec-driven shortcut package with backward-compatible wrapper (#1220)
- **base**: Add base block shortcuts (#1044)
- **im**: Complete card message format (#1198)
- **im**: Improve markdown guidance for messages (#1237)
- **vc**: Forward invite call-id on meeting join (#1243)
- **drive**: Emit typed error envelopes across the drive domain (#1205)
- **common**: Emit typed validation errors from shared shortcut pre-checks (#1242)
- **mail**: Validate `message_ids` in `+messages` before batch get (#1202)
- **wiki**: Support `appid` member type (#1235)
- **cli**: Add `--json` flag as no-op alias for `--format json` (#1104)
- **config**: Validate credentials after `config init` (#1151)

### Bug Fixes

- **skills**: Recover empty fallback for skills to update (#1233)

## [v1.0.46] - 2026-06-02

### Features

- **im**: Add card message format support (#1218)
- **im**: Resolve markdown blank-line formatting inconsistency in post messages (#1216)
- **vc**: Inline transcript from artifacts API and add keywords (#1206)
- **transport**: Add proxy plugin mode for CLI HTTP transport (#1181)
- **agent**: Increase agent trace max length to 1024 (#1211)
- **shortcuts**: Unconditionally inject `--format` flag for all shortcuts (#1156)

### Bug Fixes

- **cli**: Remove FLAGS section from root `--help` (#1226)
- **cli**: Stop root `--help` listing per-command flags as global (#1223)

### Refactor

- **transport**: Own all HTTP transport in `internal/transport`, fix util layering inversion (#1213)

### Documentation

- **base**: Optimize base skill references (#1171)
- **drive**: Add Lark Drive knowledge organization workflow (#1028)

## [v1.0.45] - 2026-06-01

### Features

- **errors**: Add typed envelope contract for auth-domain errors (#1135)
- **platform**: Support multiple policy rules per plugin (#1182)

### Bug Fixes

- **vc**: Add domain boundaries and enrich `+notes` (#1172)
- **whiteboard**: Fix whiteboard skill (#1180)

### Refactor

- **auth**: Update login hint and split-flow docs (#1201)

## [v1.0.44] - 2026-05-29

### Features

- **base**: Add dashboard block data shortcut and workflow docs (#1067)
- **im**: Support `--types` flag for listing p2p single chats in `chat-list` (#1077)
- **agent**: Add agent header support (#1158)

### Bug Fixes

- **im**: Correct 64-bit MP4 box size handling to prevent panic on crafted media (#1165)
- **install**: Detect curl version before using `--ssl-revoke-best-effort` (#1124)
- **vc**: Correct `--minute-token` to `--minute-tokens` in recording reference (#1170)
- **whiteboard**: Fix whiteboard skill (#1166)

## [v1.0.43] - 2026-05-28

### Features

- **event**: Support `note` generated event (#1159)
- **config**: Decouple `--lang` preference from TUI display language (#1132)
- **mail**: Add HTML lint library with Larksuite-native autofix for `lark-mail` (#1019)

### Bug Fixes

- **config**: Propagate `Lang` across credential boundary; respect `CurrentApp` in priorLang (#1157)
- **config**: Allow lark-channel bind source override (#1154)
- **im**: Clarify `messages-send` dry-run chat membership (#1150)
- **base**: Include `log_id` in attachment media errors (#1133)

### Performance

- **im**: Parallelize reactions, thread_replies, and merge_forward fetches (#1146)

### Documentation

- **im**: Update IM skill urgent APIs (#1153)

## [v1.0.42] - 2026-05-27

### Features

- **mail**: Add `+draft-send` shortcut for batch draft sending (#1017)
- **im**: Enrich messages with reactions and output `update_time` (#1095)
- **schema**: Output JSON spec envelope for all API commands (#1048)
- **event**: Support `vc` / `note` / `minute` events (#1113)
- **drive**: Add secure label shortcuts (#985)
- **affordance**: Use description and command in affordance example schema (#1126)

### Bug Fixes

- **docs**: Remove unsupported `fetch` text format (#1109)

### Refactor

- **auth**: Drop duplicate top-level user fields in `status` (#1128)

### Documentation

- **doc**: Document block anchor URLs in `lark-doc` skill (#1120)
- **whiteboard**: Improve SVG/Mermaid instructions (#1097)

## [v1.0.41] - 2026-05-26

### Features

- **minutes**: Add minutes edit shortcuts (#1036)
- **minutes**: Get minutes keywords (#1079)
- **slides**: Support importing pptx as slides (#1068)
- **config**: Add `keychain-downgrade` subcommand (macOS) (#1085)
- **errors**: Add structured CLI error contract (#984)
- **apps**: Replace `+html-publish` cwd hard-reject with credential-file scan (#1072)

### Bug Fixes

- **drive**: Support doubao drive inspect URL variants (#1106)
- **skills**: Sync skills incrementally during update (#1042)
- **apps**: Read app object from `data.app` for `+create` and `+update` (#1087)
- **common**: Escape special chars in multipart form filenames (#1037)
- **auth**: Remove fenced code block guidance from auth URL output hints (#1088)

### Documentation

- **skills**: Fix agent routing for doubao.com URLs (#1082)
- **task**: Require `--complete=false` for pending standup summaries (#1101)
- **base**: Document UI-only field settings (#1078)
- **contributing**: Clarify contributor guidance (#1096)

## [v1.0.40] - 2026-05-25

### Features

- **wiki**: Add exponential backoff retry for `+node-create` lock contention (#1012)
- **auth**: Add `auth qrcode` subcommand and update auth docs/hints (#968)

### Bug Fixes

- **wiki**: Rename `+node-get --token` to `--node-token`, keep alias (#1074)
- **output**: Classify wiki lock-contention error (131009) with retry hint (#1014)
- **contact**: Add actionable hint when fanout search all-fail with no API code (#1054)
- **permission**: Annotate auto-grant permission failures with `required_scope` and `console_url` (#1045)
- **validation**: Use `ErrValidation` instead of `fmt.Errorf` in `Validate` paths (#1001)

### Documentation

- **skills**: Add 云盘/云存储 alias alongside 云空间 for agent clarity (#1073)
- **task**: Refresh `lark-task` shortcut docs (#1057)

## [v1.0.39] - 2026-05-22

### Features

- **slides**: Add `+export` shortcut to export slides (#988)
- **sidecar**: Support multi-client identity isolation in `server-demo` via per-client HMAC keys, preventing UAT cross-contamination when multiple CLI sandboxes share one sidecar (#934)
- **im**: Support Markdown image rendering in post content (#893)

### Bug Fixes

- **scope**: Add 22 new scope entries to scope priorities (#1050)

### Documentation

- **base**: Update location `full_address` guidance (#754)
- **apps**: Refine `lark-apps` skill description and surface, document `index.html` / `--path` hard constraints (#1040)

## [v1.0.38] - 2026-05-22

### Features

- **apps**: Gate the Miaoda apps domain off on the Lark brand — the `apps` shortcut subtree returns a structured brand-restriction error, `auth login --domain apps` is rejected, `--domain all` skips it, and `spark:*` scopes are no longer requested (#1025)

## [v1.0.37] - 2026-05-21

### Features

- **apps**: Add miaoda apps domain with 6 shortcuts covering `+create` / `+update` / `+list` / `+access-scope-get` / `+access-scope-set` / `+html-publish` (#1002)

### Bug Fixes

- **permission**: Surface auto-grant skipped/failed cases via stderr warnings and a `hint` field in the `permission_grant` JSON output (#1015)
- **sheets**: Use `FileIO` for `+write-image` input so stdin / `-` works consistently (#996)

## [v1.0.36] - 2026-05-21

### Features

- **drive/markdown**: Return real tenant URLs for `drive +upload` and `markdown +create` (#992)

### Bug Fixes

- **auth**: Return validation error when `--scope` is empty in `auth check` (#999)

### Documentation

- **lark-drive**: Improve search evidence guidance (#864)

## [v1.0.35] - 2026-05-20

### Features

- **markdown**: Support wiki node target in `+create` (#883)
- **markdown**: Add `+diff` shortcut (#876)
- **base**: Add form `+detail` / `+submit` shortcuts (#759)
- **skills**: Add incremental skills sync (#965)
- **doc**: Warn before overwrite when document contains whiteboard or file blocks (#825)

### Documentation

- **im**: Clarify media key formats for message media flags (#991)
- **im**: Add media-preview reference (#990)
- **drive**: Migrate `docs +search` to `drive +search` and fix `creator_ids` owner semantic (#951)
- **drive**: Prefer local comments for drive reviews (#981)
- **wiki**: Add wiki base fast path (#982)

## [v1.0.34] - 2026-05-19

### Features

- **drive**: Switch markdown export to V2 `docs_ai` fetch API (#948)
- **drive**: Add `+inspect` shortcut for document URL inspection with wiki unwrapping (#947)
- **wiki**: Add `+node-get` / `+node-delete` / `+space-create` shortcuts (#904)
- **base**: Support Base attachment APIs (#887)
- **mail**: Validate `bot` + `mailbox=me` and add dynamic `--as` help tests (#895)
- **mail**: Expose draft priority in `--inspect` projection and document `--set-priority` (#779)

### Bug Fixes

- **identitydiag**: Harden verify path and tighten status semantics (#961)
- **wiki**: Surface real node URL for `+node-create` / `+node-copy` (#960)
- **auth**: Split bot and user identity diagnostics (#957)
- **base**: Address Base attachment review follow-ups (#958)
- **docs**: Clarify `replace_all` selection errors (#954)

### Documentation

- **drive**: Clarify add comment constraints (#967)
- **lark-im**: Clarify message activity search (#865)

### Tests

- Verify e2e resource cleanup (#949)
- **lint**: Exclude `bidichk` from test files (#959)

## [v1.0.33] - 2026-05-18

### Features

- **markdown**: Add `+patch` shortcut (#857)
- **slides**: Improve slide planning and validation guidance (#847)
- **drive**: Add `+sync` workflow for Drive directories (#873)
- **drive**: Add drive version shortcut (#841)
- **extension**: Plugin / Hook framework with command pruning (#910)

### Bug Fixes

- **sheets**: Explicitly document safe JSON unmarshal ignore in `DryRun` (#935)
- **base**: Mark base field update high risk (#936)
- **auth**: Guide agents to yield during auth device flow (#933)

### Documentation

- **lark-wiki**: Correct the `--as` default-identity claim (#919)

### Tests

- Drop stale e2e `--yes` flags (#920)

## [v1.0.32] - 2026-05-15

### Features

- **doc**: Add `--width`/`--height` flags to `docs +media-insert` (#832)
- **wiki**: Add `+space-list` / `+node-list` / `+node-copy` shortcuts (#392)

### Bug Fixes

- **drive**: Preserve parent token on nested overwrite (#908)
- **selfupdate**: Use `LookPath` instead of `Executable` for binary verification (#886)
- **registry**: Wait for background meta refresh before test reset (#894)

### Documentation

- **doc**: Add SVG whiteboard support to `lark-doc` v2 skill (#901)
- **drive**: Add permission public patch error guidance (#863)

## [v1.0.31] - 2026-05-14

### Features

- **install**: Skip interactive prompts in non-TTY environments (#888)
- **update**: Recommend `lark-cli update` over `npm install` for AI agents (#884)
- **im**: Add `--exclude-muted` to `+chat-search` and new `+chat-list` shortcut (#820)
- **auth**: Add `--exclude` flag and allow combining `--scope` with `--domain`/`--recommend` (#844)
- **drive**: Add modified-time smart sync mode (#859)
- **approval**: Add `tasks.add_sign` and `tasks.rollback` methods (#867)

## [v1.0.30] - 2026-05-13

### Features

- **im**: Add `--chat-mode topic` to `+chat-create` (#790)

### Bug Fixes

- **auth**: Support comma-separated `--scope` in `auth login` (#764)
- **auth**: Clarify URL handling in auth messages and docs (#856)
- **bind**: Accept `~/` paths in OpenClaw secret references (#839)

### Tests

- **update**: Isolate stamp writes from real `~/.lark-cli/skills.stamp` (#858)

## [v1.0.29] - 2026-05-12

### Features

- **vc**: Add agent meeting join, leave, and events shortcuts (#824)
- **mail**: Add unknown-flag fuzzy match for `lark-cli mail` commands (#806)
- **whiteboard**: Pin `whiteboard-cli` to `v0.2.11` in `lark-whiteboard` skill (#850)

### Bug Fixes

- Silence misleading "skills not installed" startup notice (#801)

### Documentation

- **base**: Refine data analysis SOP wording (#784, #849)
- Update README capability descriptions (#793)

## [v1.0.28] - 2026-05-11

### Features

- **im**: Support UAT for `messages.forward` and add `threads.forward` (#689)
- **im**: Add flag shortcuts `+flag-create` / `+flag-list` / `+flag-cancel` for message bookmarks (#770)

### Bug Fixes

- **drive**: Handle duplicate remote sync paths (#803)

### Documentation

- **im**: Name `--query` / `--member-ids` in `+chat-search` shortcut row (#812)

## [v1.0.27] - 2026-05-09

### Features

- **config**: Add `lark-channel` as a bind source (#786)

### Bug Fixes

- **install**: Fix installation errors when PowerShell is disabled by Group Policy (#789)

### Documentation

- **task**: Clarify task member id types in references (#777)

## [v1.0.26] - 2026-05-08

### Features

- **im**: Add `message_app_link` to message outputs (#668)
- **auth**: Add scope hint for missing authorization errors (#776)

### Bug Fixes

- **base**: Clean error detail output (#783)
- **whiteboard**: Reclassify `+update` as `write` risk (#775)

### Documentation

- **mail**: Add data integrity and write-confirmation rules (#749)

## [v1.0.25] - 2026-05-07

### Features

- Add skills version drift notice and unify update flow (#723)

### Bug Fixes

- Remove misleading default value from `--as` flag help text (#769)
- Handle negative truncate lengths (#744)
- Reject invalid JSON pointer escapes (#741)
- Migrate task shortcut errors to structured `output.Errorf`/`ErrValidation` (#740)

### Documentation

- Clarify base `user_open_id` guidance (#763)

## [v1.0.24] - 2026-05-06

### Features

- **sheets**: Add sheet management shortcuts (#722)
- **base**: Support batch record get and delete (#630)
- **task**: Add upload task attachment shortcut (#736)
- **drive**: Pre-flight 10000-rune total cap for `+add-comment` `reply_elements` (#605)

### Bug Fixes

- **auth**: Handle missing scopes and device flow improvements (#752)
- Add url to markdown `+create` output (#753)

### Documentation

- Refine field update conversion guidance (#748)

## [v1.0.23] - 2026-04-30

### Features

- **drive**: Add `+pull` shortcut for one-way Drive → local mirror (#696)
- **drive**: Add `+push` shortcut for one-way local → Drive mirror (#709)
- **drive**: Add `+status` shortcut for content-hash diff (#692)
- **drive**: Support `--file-name` for drive export (#685)
- **base**: Add markdown output for record reads (#726)
- **minutes**: Add media upload shortcut (#725)
- **doc**: Warn when callout uses `type=` without `background-color` (#467)
- **cmdutil**: Support `@file` for params and data (#724)
- Add markdown shortcuts and skill docs (#704)

### Documentation

- **doc**: Guide lark-doc v2 usage (#710)
- **minutes**: Clarify minutes file-to-notes routing (#732)

## [v1.0.22] - 2026-04-29

### Features

- **task**: Add resource agent & `agent_task_step_info` (#693)
- **task**: Support app task members by id (#712)
- **contact**: Add `--queries` multi-name fanout to `+search-user` (#707)
- **slides**: Add slide templates with template-first skill guidance (#684)
- **mail**: Support calendar events in emails (#646)
- **install**: Honor `npm_config_registry` for binary URL resolution with npmmirror fallback (#690)

### Bug Fixes

- **install**: Make Windows zip extraction resilient (#713)
- **config/init**: Respect `--brand` flag in `--new` mode (#711)

### Documentation

- **base**: Clarify base search routing (#708)
- **base**: Align base skills and view config contracts (#653)

## [v1.0.21] - 2026-04-28

### Features

- **contact**: Add search filters and richer profile fields to `+search-user` (#648)
- **common**: Backfill resource URL when create APIs omit it (#680)
- **risk**: Add risk tiering for command sensitivity classification (#633)
- **okr**: Add progress records support (#574)
- **calendar**: Enhance event search and meeting room finding (#679)
- **event**: Add event subscription & consume system (#654)
- **drive**: Extend `+add-comment` to support slides targets (#674)
- **slides**: Add font management for slides (#681)

### Bug Fixes

- **cmdutil**: Default flag completions to disabled (#688)
- **e2e/wiki**: Pass `obj_type` when deleting wiki nodes in cleanup (#687)
- **readme**: Fix readme statistics (#691)

## [v1.0.20] - 2026-04-27

### Features

- **drive**: Add `+search` shortcut with flat filter flags (#658)
- **mail**: Support sharing emails to IM chats via `+share-to-chat` (#637)
- **calendar**: Add `+update` shortcut (#678)
- **im**: Add `--at-chatter-ids` filter to `+messages-search` (#612)
- **pagination**: Preserve pagination state on truncation and natural end (#659)
- **lark-im**: Add `chat.members.bots` to skill docs (#616)

### Bug Fixes

- **strict-mode**: Reject explicit `--as` instead of silently overriding it (#673)
- **whiteboard**: Manual disable edge case for svg compatibility (#661)

### Documentation

- **lark-drive**: Add missing import command examples (#669)
- **readme**: Add Project (Meegle) to Features table (#660)

## [v1.0.19] - 2026-04-24

### Features

- **mail**: Add read receipt support — `--request-receipt` on compose, `+send-receipt` / `+decline-receipt` for response
- **doc**: Add v2 API for `docs +create` / `+fetch` / `+update` (#638)
- **im**: Request thread roots for chat message list (#635)
- **drive**: Support wiki node targets in `+upload` (#611)
- **config**: Block `auth` / `config` when external credential provider is active (#627)
- **whiteboard**: Pin `whiteboard-cli` to `v0.2.10` in `lark-whiteboard` skill (#649)

## [v1.0.18] - 2026-04-23

### Features

- **base**: Support `.base` import and export for bitable (#599)
- **config**: Add `config bind` for per-Agent credential isolation (#515)
- **slides**: Add `+replace-slide` shortcut for block-level XML edits (#516)
- **wiki**: Add `+delete-space` shortcut with async task polling (#610)
- **doc**: Add `--from-clipboard` flag to `docs +media-insert` (#508)
- **minutes**: Unify minute artifacts output to `./minutes/{minute_token}/` (#604)
- Add configurable content-safety scanning (#606)
- **install**: Add SHA-256 checksum verification to `install.js` (#592)
- **whiteboard**: Pin `whiteboard-cli` to `^0.2.9` (#617)

### Bug Fixes

- **drive**: Escape angle brackets in comment text (#632)
- **im**: Unify `messages-search` pagination int flags (#446)
- **im**: Fix markdown URL rendering issues in post content (#206)

### Documentation

- **base**: Refine record cell value guidance (#636)

## [v1.0.17] - 2026-04-22

### Features

- **im**: Use `Content-Disposition` filename when downloading message resources (#536)
- **drive**: Add `+apply-permission` to request doc access (#588)
- Support record share link (#466)
- **whiteboard**: Add image support to `whiteboard-cli` skill (#553)
- **cmdutil**: Add `X-Cli-Build` header for CLI build classification (#596)

### Bug Fixes

- **base**: Add default-table follow-up hint to `base-create` (#600)
- Skip flag-completion registration outside completion path (#598)
- Add `record-share-link-create` in `SKILL.md` (#597)
- **mail**: Remove leftover conflict marker in skill docs (#594)

### Documentation

- **drive**: Clarify that comment listing defaults to unresolved comments only (#609)
- **doc**: Fix `--markdown` examples that teach literal `\n` (#602)
- **mail**: Remove `get_signatures` from skill reference, exposed via `+signature` instead (#545)

## [v1.0.16] - 2026-04-21

### Features

- **mail**: Support large email attachments (#537)
- **mail**: Add draft preview URL to draft operations (#438)
- **doc**: Add pre-write semantic warnings to `docs +update` (#569)
- **doc**: Add `--selection-with-ellipsis` position flag to `+media-insert` (#335)
- **calendar**: Support event share link and error details (#583)

### Bug Fixes

- **doc**: Preserve round-trip formatting in `+fetch` output (#469)
- **docs**: Validate `--selection-by-title` format early (#256)
- **whiteboard**: Register `+media-upload` shortcut and add whiteboard parent type

### Refactor

- Split `Execute` into `Build` + `Execute` with explicit IO and keychain injection (#371)
- **auth**: Simplify scope reporting in login flow (#582)

## [v1.0.15] - 2026-04-20

### Features

- **sheets**: Add float image shortcuts (#494)
- **approval**: Document `remind` and `initiated` methods in skill (#554)

### Bug Fixes

- **base**: Preserve attachment metadata on base uploads (#563)
- **base**: Fix role view and record default permission on edit (#530)
- **sheets**: Normalize single-cell range in `+set-style` and `+batch-set-style` (#548)
- **im**: Cap `basic_batch` user_ids at 10 per API limit (#551)
- **install**: Refine install wizard messages (#529)
- **whiteboard**: Deprecate old `lark-whiteboard-cli` skill (#547)

## [v1.0.14] - 2026-04-17

### Features

- **mail**: Add email priority support for compose and read (#538)
- **mail**: Support scheduled send (#534)
- **drive**: Support sheet cell comments in `+add-comment` (#518)
- **doc**: Add `--file-view` flag to `+media-insert` (#419)
- **base**: Auto grant current user for bot create and copy (#497)
- **base**: Add identity priority strategy and error handling (#505)
- **auth**: Improve login scope handling and messages (#523)
- Add OKR business domain (#522)

### Documentation

- **wiki**: Improve wiki skill docs and add wiki domain template (#512)
- **task**: Document `custom_fields` and `custom_field_options` API resources and permissions (#524)

### Refactor

- **skills**: Introduce `lark-doc-whiteboard.md` and streamline whiteboard workflow (#502)

## [v1.0.13] - 2026-04-16

### Features

- **im**: Support user access token for file, image, audio, and video upload, aligning upload and send identity with `--as` flag (#474)
- **drive**: Add `drive +create-folder` shortcut with root-folder fallback and bot-mode auto-grant (#470)
- **wiki**: Add bot-mode auto-grant support to `wiki +node-create` (#470)
- **doc**: Default `skip_task_detail` in `docs +fetch` to reduce unnecessary task detail expansion (#471)

### Bug Fixes

- **im**: Preserve original URL filename for uploaded file messages instead of generic `media.ext` names (#514)
- **whiteboard**: Use atomic overwrite API parameter for `whiteboard +update`, replacing read-then-delete approach (#483)

### Documentation

- **base**: Unify record batch write limit to 200 and enforce serial writes for continuous operations (#499)
- **base**: Remove redundant reference documentation and command grouping chapters from SKILL.md (#500)

### CI

- Consolidate workflows into layered CI pyramid with single `results` gate (#510)

## [v1.0.12] - 2026-04-15

### Features

- Add guided npm install flow that installs or upgrades the CLI, installs AI skills, and walks through app config and auth login (#464)
- **mail**: Add email signature support with `+signature`, `--signature-id` compose flags, and draft signature edit operations (#485)
- **mail**: Return recall hints for sent emails when recall is available (#481)
- **slides**: Add `+media-upload` and support `@path` image placeholders in `+create --slides` (#450)

### Documentation

- **mail**: Add recipient search guidance to the mail skill workflow (#437)
- **calendar/vc**: Route past meeting queries to `lark-vc` and clarify historical date matching in skills (#482, #480)

## [v1.0.11] - 2026-04-14

### Features

- **sheets**: Add dropdown shortcuts for data validation management (`+set-dropdown`, `+update-dropdown`, `+get-dropdown`, `+delete-dropdown`) (#461)
- **task**: Add task search, tasklist search, related-task, set-ancestor, and subscribe-event shortcuts (#377)
- Streamline interactive login by removing the extra auth confirmation step (#451)

### Bug Fixes

- **base**: Validate JSON object inputs for base shortcuts and reject `null` objects (#458)

### Documentation

- **sheets**: Document value formats for formulas and special field types (#456)
- **readme**: Add Attendance to the features table (#460)

## [v1.0.10] - 2026-04-13

### Features

- **im**: Support im oapi range download for large files (#283)
- **sheets**: Add filter view and condition shortcuts (#422)
- **wiki**: Add wiki move shortcut with async task polling (#436)
- **drive**: Add drive `+create-shortcut` shortcut (#432)
- **drive**: Add drive files patch metadata API (#444)
- **task**: Support `--section-guid` flag in tasklist-task-add shortcut (#430)

### Bug Fixes

- **base**: Support large base attachment uploads (#441)
- **config**: Clarify init copy for TTY, preserve original for AI (#448)
- **im**: Reject `--user-id` under bot identity for chat-messages-list (#340)
- **mail**: Add missing scopes for mail `+watch` shortcut (#357)
- **mail**: Restrict `--output-dir` to current working directory (#376)

### Documentation

- **wiki**: Add wiki member operations to lark-wiki skill (#417)
- **task**: Document sections API resources, permissions, and URL parsing (#430)
- **doc**: Clarify when markdown escaping is needed (#312)

## [v1.0.9] - 2026-04-11

### Features

- Add attendance `user_task.query` (#405)
- Support minutes search (#359)
- **slides**: Add slides `+create` shortcut with `--slides` one-step creation (#389)
- **slides**: Return presentation URL in slides `+create` output (#425)
- **sheets**: Add dimension shortcuts for row/column operations (#413)
- **sheets**: Add cell operation shortcuts for merge, replace, and style (#412)
- **drive**: Add drive folder delete shortcut with async task polling (#415)

### Documentation

- **drive**: Add guide for granting document permission to current bot (#414)

## [v1.0.8] - 2026-04-10

### Features

- Add `update` command with self-update, verification, and rollback (#391)
- Add `--file` flag for multipart/form-data file uploads (#395)
- Support file comment reply reactions (#380)
- **base**: Add `+dashboard-arrange` command for auto-arranging dashboard blocks layout and `text` block type with Markdown support (#388)
- **base**: Add record batch `+add` / `+set` shortcuts (#277)
- **base**: Add `+record-search` for keyword-based record search (#328)
- **base**: Add view visible fields `+get` / `+set` shortcuts (#326)
- **base**: Add record field filters (#327)
- **base**: Optimize workflow skills (#345)
- **calendar**: Add room find workflow (#403)
- **mail**: Add `--page-token` and `--page-size` to mail `+triage` (#301)
- **whiteboard**: Add `+query` shortcut and enhance `+update` with Mermaid/PlantUML support (#382)

### Bug Fixes

- Improve error hints for sandbox and initialization issues (#384)
- Fix markdown line breaks support (#338)
- Return raw base field and view responses (#378)
- **base**: Return raw table list response and clarify sort help (#393)
- **calendar**: Add default video meeting to `+create` (#383)
- **mail**: Replace `os.Exit` with graceful shutdown in mail watch (#350)

### Documentation

- **base**: Document Base attachment download via docs `+media-download` (#404)
- Reorganize lark-base skill guidance (#374)

## [v1.0.7] - 2026-04-09

### Features

- Auto-grant current user access for bot-created docs, sheets, imports, and uploads (#360)
- **mail**: Add `send_as` alias support, mailbox/sender discovery APIs, and mail rules API
- **vc**: Extract note doc tokens from calendar event relation API (#333)
- **wiki**: Add wiki node create shortcut (#320)
- **sheets**: Add `+write-image` shortcut (#343)
- **docs**: Add media-preview shortcut (#334)
- **docs**: Add support for additional search filters (#353)

### Bug Fixes

- **api**: Support stdin and quoted JSON inputs on Windows (#367)
- **doc**: Post-process `docs +fetch` output to improve round-trip fidelity (#214)
- **run**: Add missing binary check for lark-cli execution (#362)
- **config**: Validate appId and appSecret keychain key consistency (#295)

### Refactor

- Route base import guidance to drive `+import` (#368)
- Migrate mail shortcuts to FileIO (#356)
- Migrate drive/doc/sheets shortcuts to FileIO (#339)
- Migrate base shortcuts to FileIO (#347)

### Documentation

- **lark-doc**: Document advanced boolean and intitle search syntax for AI agents (#210)

### Chore

- Add depguard and forbidigo rules to guide FileIO adoption (#342)

## [v1.0.6] - 2026-04-08

### Features

- Improve login scope validation and success output (#317)
- **task**: Support starting pagination from page token (#332)
- Support multipart doc media uploads (#294)
- **mail**: Auto-resolve local image paths in all draft entry points (#205)
- **vc**: Add `+recording` shortcut for `meeting_id` to `minute_token` conversion (#246)

### Bug Fixes

- Resolve concurrency races in RuntimeContext (#330)
- **config**: Save empty config before clearing keychain entries (#291)
- Reject positional arguments in shortcuts (#227)
- Improve raw API diagnostics for invalid or empty JSON responses (#257)
- **docs**: Normalize `board_tokens` in `+create` response for mermaid/whiteboard content (#10)
- **task**: Clarify `--complete` flag help for `get-my-tasks` (#310)
- **help**: Point root help Agent Skills link to README section (#289)

### Documentation

- Clarify `--complete` flag behavior in `get-my-tasks` reference (#308)

### Refactor

- Migrate VC/minutes shortcuts to FileIO (#336)
- Migrate common/client/IM to FileIO and add localfileio tests (#322)

## [v1.0.5] - 2026-04-07

### Features

- **drive**: Support multipart upload for files larger than 20MB (#43)
- Add darwin file master key fallback for keychain writes (#285)
- Add strict mode identity filter, profile management and credential extension (#252)

### Bug Fixes

- **mail**: Restore CID validation and stale PartID lookup lost in revert (#230)
- **base**: Clarify table-id `tbl` prefix requirement (#270)
- Fix parameter constraints for LarkMessageTrigger (#213)

### Documentation

- Fix root calendar example (#299)
- Fix README auth scope and api data flag (#298)
- Clarify task guid for applinks (#287)
- Clarify lark task guid usage (#282)
- **lark-base**: Add `has_more` guidance for record-list pagination (#183)

### Tests

- Isolate registry package state in tests (#280)

### CI

- Add scheduled issue labeler for type/domain triage (#251)
- **issue-labels**: Reduce mislabeling and handle missing labels (#288)
- Map wiki paths in pr labels (#249)

## [v1.0.4] - 2026-04-03

### Features

- Support user identity for im `+chat-create` (#242)
- Implement authentication response logging (#235)
- Support im chat member delete and add scope notes (#229)

### Bug Fixes

- **security**: Replace `http.DefaultTransport` with proxy-aware base transport to mitigate MITM risk (#247)
- **calendar**: Block auto bot fallback without user login (#245)

### Documentation

- **mail**: Add identity guidance to prefer user over bot (#157)

### Refactor

- **dashboard**: Restructure docs for AI-friendly navigation (#191)

### CI

- Add a CLI E2E testing framework for lark-cli, task domain testcase and ci action (#236)

## [v1.0.3] - 2026-04-02

### Features

- Add `--jq` flag for filtering JSON output (#211)
- Add `+download` shortcut for minutes media download (#101)
- Add drive import, export, move, and task result shortcuts (#194)
- Support im message send/reply with uat (#180)
- Add approve domain (#217)

### Bug Fixes

- **mail**: Use in-memory keyring in mail scope tests to avoid macOS keychain popups (#212)
- **mail**: On-demand scope checks and watch event filtering (#198)
- Use curl for binary download to support proxy and add npmmirror fallback (#226)
- Normalize escaped sheet range separators (#207)

### Documentation

- **mail**: Clarify JSON output is directly usable without extra encoding (#228)
- Clarify docs search query usage (#221)

### CI

- Add gitleaks scanning workflow and custom rules (#142)

## [v1.0.2] - 2026-04-01

### Features

- Improve OS keychain/DPAPI access error handling for sandbox environments (#173)
- **mail**: Auto-resolve local image paths in draft body HTML (#139)

### Bug Fixes

- Correct URL formatting in login `--no-wait` output (#169)

### Documentation

- Add concise AGENTS development guide (#178)

### CI

- Refine PR business area labels and introduce skill format check (#148)

### Chore

- Add pull request template (#176)

## [v1.0.1] - 2026-03-31

### Features

- Add automatic CLI update detection and notification (#144)
- Add npm publish job to release workflow (#145)
- Support auto extension for downloads (#16)
- Remove useless files (#131)
- Normalize markdown message send/reply output (#28)
- Add auto-pagination to messages search and update lark-im docs (#30)

### Bug Fixes

- **base**: Use base history read scope for record history list (#96)
- Remove sensitive send scope from reply and forward shortcuts (#92)
- Resolve silent failure in `lark-cli api` error output (#85)

### Documentation

- **base**: Clarify field description usage in json (#90)
- Update Base description to include all capabilities (#61)
- Add official badge to distinguish from third-party Lark CLI tools (#103)
- Rename user-facing Bitable references to Base (#11)
- Add star history chart to readmes (#12)
- Simplify installation steps by merging CLI and Skills into one section (#26)
- Add npm version badge and improve AI agent tip wording (#23)
- Emphasize Skills installation as required for AI Agents (#19)
- Clarify install methods as alternatives and add source build steps

### CI

- Improve CI workflows and add golangci-lint config (#71)

## [v1.0.0] - 2026-03-28

### Initial Release

The first open-source release of **Lark CLI** — the official command-line interface for [Lark/Feishu](https://www.larksuite.com/).

### Features

#### Core Commands

- **`lark api`** — Make arbitrary Lark Open API calls directly from the terminal with flexible parameter support.
- **`lark auth`** — Complete OAuth authentication flow, including interactive login, logout, token status, and scope management.
- **`lark config`** — Manage CLI configuration, including `init` for guided setup and `default-as` for switching contexts.
- **`lark schema`** — Inspect available API services and resource schemas.
- **`lark doctor`** — Run diagnostic checks on CLI configuration and environment.
- **`lark completion`** — Generate shell completion scripts for Bash, Zsh, Fish, and PowerShell.

#### Service Shortcuts

Built-in shortcuts for commonly used Lark APIs, enabling concise commands like `lark im send` or `lark drive upload`:

- **IM (Messaging)** — Send messages, manage chats, and more.
- **Drive** — Upload, download, and manage cloud documents.
- **Docs** — Work with Lark documents.
- **Sheets** — Interact with spreadsheets.
- **Base** — Manage multi-dimensional tables.
- **Calendar** — Create and manage calendar events.
- **Mail** — Send and manage emails.
- **Contact** — Look up users and departments.
- **Task** — Create and manage tasks.
- **Event** — Subscribe to and manage event callbacks.
- **VC (Video Conference)** — Manage meetings.
- **Whiteboard** — Interact with whiteboards.

#### AI Agent Skills

Bundled AI agent skills for intelligent assistance:

- `lark-im`, `lark-doc`, `lark-drive`, `lark-sheets`, `lark-base`, `lark-calendar`, `lark-mail`, `lark-contact`, `lark-task`, `lark-event`, `lark-vc`, `lark-whiteboard`, `lark-wiki`, `lark-minutes`
- `lark-openapi-explorer` — Explore and discover Lark APIs interactively.
- `lark-skill-maker` — Create custom AI skills.
- `lark-workflow-meeting-summary` — Automated meeting summary workflow.
- `lark-workflow-standup-report` — Automated standup report workflow.
- `lark-shared` — Shared skill utilities.

#### Developer Experience

- Cross-platform support (macOS, Linux, Windows) via GoReleaser.
- Shell completion for Bash, Zsh, Fish, and PowerShell.
- Bilingual documentation (English & Chinese).
- CI/CD pipelines: linting, testing, coverage reporting, and automated releases.

[v1.0.64]: https://github.com/larksuite/cli/releases/tag/v1.0.64
[v1.0.62]: https://github.com/larksuite/cli/releases/tag/v1.0.62
[v1.0.61]: https://github.com/larksuite/cli/releases/tag/v1.0.61
[v1.0.60]: https://github.com/larksuite/cli/releases/tag/v1.0.60
[v1.0.59]: https://github.com/larksuite/cli/releases/tag/v1.0.59
[v1.0.58]: https://github.com/larksuite/cli/releases/tag/v1.0.58
[v1.0.57]: https://github.com/larksuite/cli/releases/tag/v1.0.57
[v1.0.56]: https://github.com/larksuite/cli/releases/tag/v1.0.56
[v1.0.55]: https://github.com/larksuite/cli/releases/tag/v1.0.55
[v1.0.54]: https://github.com/larksuite/cli/releases/tag/v1.0.54
[v1.0.53]: https://github.com/larksuite/cli/releases/tag/v1.0.53
[v1.0.52]: https://github.com/larksuite/cli/releases/tag/v1.0.52
[v1.0.51]: https://github.com/larksuite/cli/releases/tag/v1.0.51
[v1.0.50]: https://github.com/larksuite/cli/releases/tag/v1.0.50
[v1.0.49]: https://github.com/larksuite/cli/releases/tag/v1.0.49
[v1.0.48]: https://github.com/larksuite/cli/releases/tag/v1.0.48
[v1.0.47]: https://github.com/larksuite/cli/releases/tag/v1.0.47
[v1.0.46]: https://github.com/larksuite/cli/releases/tag/v1.0.46
[v1.0.45]: https://github.com/larksuite/cli/releases/tag/v1.0.45
[v1.0.44]: https://github.com/larksuite/cli/releases/tag/v1.0.44
[v1.0.43]: https://github.com/larksuite/cli/releases/tag/v1.0.43
[v1.0.42]: https://github.com/larksuite/cli/releases/tag/v1.0.42
[v1.0.41]: https://github.com/larksuite/cli/releases/tag/v1.0.41
[v1.0.40]: https://github.com/larksuite/cli/releases/tag/v1.0.40
[v1.0.39]: https://github.com/larksuite/cli/releases/tag/v1.0.39
[v1.0.38]: https://github.com/larksuite/cli/releases/tag/v1.0.38
[v1.0.37]: https://github.com/larksuite/cli/releases/tag/v1.0.37
[v1.0.36]: https://github.com/larksuite/cli/releases/tag/v1.0.36
[v1.0.35]: https://github.com/larksuite/cli/releases/tag/v1.0.35
[v1.0.34]: https://github.com/larksuite/cli/releases/tag/v1.0.34
[v1.0.33]: https://github.com/larksuite/cli/releases/tag/v1.0.33
[v1.0.32]: https://github.com/larksuite/cli/releases/tag/v1.0.32
[v1.0.31]: https://github.com/larksuite/cli/releases/tag/v1.0.31
[v1.0.30]: https://github.com/larksuite/cli/releases/tag/v1.0.30
[v1.0.29]: https://github.com/larksuite/cli/releases/tag/v1.0.29
[v1.0.28]: https://github.com/larksuite/cli/releases/tag/v1.0.28
[v1.0.27]: https://github.com/larksuite/cli/releases/tag/v1.0.27
[v1.0.26]: https://github.com/larksuite/cli/releases/tag/v1.0.26
[v1.0.25]: https://github.com/larksuite/cli/releases/tag/v1.0.25
[v1.0.24]: https://github.com/larksuite/cli/releases/tag/v1.0.24
[v1.0.23]: https://github.com/larksuite/cli/releases/tag/v1.0.23
[v1.0.22]: https://github.com/larksuite/cli/releases/tag/v1.0.22
[v1.0.21]: https://github.com/larksuite/cli/releases/tag/v1.0.21
[v1.0.20]: https://github.com/larksuite/cli/releases/tag/v1.0.20
[v1.0.19]: https://github.com/larksuite/cli/releases/tag/v1.0.19
[v1.0.18]: https://github.com/larksuite/cli/releases/tag/v1.0.18
[v1.0.17]: https://github.com/larksuite/cli/releases/tag/v1.0.17
[v1.0.16]: https://github.com/larksuite/cli/releases/tag/v1.0.16
[v1.0.15]: https://github.com/larksuite/cli/releases/tag/v1.0.15
[v1.0.14]: https://github.com/larksuite/cli/releases/tag/v1.0.14
[v1.0.13]: https://github.com/larksuite/cli/releases/tag/v1.0.13
[v1.0.12]: https://github.com/larksuite/cli/releases/tag/v1.0.12
[v1.0.11]: https://github.com/larksuite/cli/releases/tag/v1.0.11
[v1.0.10]: https://github.com/larksuite/cli/releases/tag/v1.0.10
[v1.0.9]: https://github.com/larksuite/cli/releases/tag/v1.0.9
[v1.0.8]: https://github.com/larksuite/cli/releases/tag/v1.0.8
[v1.0.7]: https://github.com/larksuite/cli/releases/tag/v1.0.7
[v1.0.6]: https://github.com/larksuite/cli/releases/tag/v1.0.6
[v1.0.5]: https://github.com/larksuite/cli/releases/tag/v1.0.5
[v1.0.4]: https://github.com/larksuite/cli/releases/tag/v1.0.4
[v1.0.3]: https://github.com/larksuite/cli/releases/tag/v1.0.3
[v1.0.2]: https://github.com/larksuite/cli/releases/tag/v1.0.2
[v1.0.1]: https://github.com/larksuite/cli/releases/tag/v1.0.1
[v1.0.0]: https://github.com/larksuite/cli/releases/tag/v1.0.0
