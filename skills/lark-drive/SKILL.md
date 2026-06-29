---
name: lark-drive
version: 1.0.0
description: "飞书云空间（云盘/云存储）：管理 Drive 文件和文件夹，包含上传/下载、创建文件夹、复制/移动/删除、查看元数据、评论/权限/订阅、标题、版本和本地文件导入。用户需要整理云盘目录、处理云空间资源 URL/token，或导入 Word/Markdown/Excel/CSV/PPTX/.base 为 docx/sheet/bitable/slides 时使用；doubao.com 云空间 URL/token 也按资源路径和 token 路由，不回退 WebFetch。不负责：文档内容编辑（走 lark-doc）、表格/Base 表内数据操作（走 lark-sheets/lark-base）、知识空间节点/成员管理（走 lark-wiki）、原生 Markdown 文件读写/patch/diff（走 lark-markdown）。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli drive --help"
---

# drive

Read `../lark-shared/SKILL.md` first when auth, identity, scope, permission recovery, or `_notice` appears. For a plain Drive command, use this card first and avoid reading long references or source code unless the command fails with an unrecoverable schema/usage gap.

## Route

- Drive owns file/folder/docx/sheet/bitable/slides/wiki Drive URLs and tokens, including `doubao.com` resources with those paths.
- Local `.xlsx` / `.csv` / `.base` -> Base/bitable: first run `drive +import --type bitable`; use `lark-base` only after import completes.
- Local `.xlsx` / `.xls` / `.csv` -> online sheet: `drive +import --type sheet`.
- Local `.md` / `.docx` / `.doc` / `.txt` / `.html` -> online docx: `drive +import --type docx`.
- Local `.pptx` -> slides: `drive +import --type slides` (PPTX import limit 500MB).
- Native remote Markdown read/write/patch/diff/history diff: switch to `lark-markdown`; importing Markdown as docx stays here.
- Wiki space/node hierarchy and wiki membership: switch to `lark-wiki`; uploading a local file under a wiki node still uses `drive +upload --wiki-token`.
- Doc body read/edit/summary/images: switch to `lark-doc`. Sheet cells/formulas/styles: `lark-sheets`. Base tables/records/views: `lark-base`.

## Hot Commands

Use existing `lark-cli drive` commands. Do not inspect or edit Go source to solve user tasks.

| User intent | Command / next step | Minimal flags and notes |
|---|---|---|
| Inspect URL/token/type | `drive +inspect` | `--url <url>`; wiki URLs are unpacked to canonical `type/token/title/url`. |
| Search Drive objects | `drive +search` | `--query`, optional `--edited-since`, `--created-by-me`, `--mine`, `--doc-types`, `--sort`; project only needed fields in your answer. |
| Upload local file | `drive +upload` | `--file ./relative --parent-token <folder>` or `--wiki-token <wiki_node>`; validate local path first. |
| Download Drive file | `drive +download` | `--file-token <token> --output ./relative/path --overwrite` when user says replace/overwrite; `--output` must stay relative to cwd. |
| Export online doc | `drive +export` | `--token <doc_token> --doc-type docx|sheet|bitable|slides --file-extension <ext>`; follow with `+export-download` when it returns a file token/task. |
| Download export result | `drive +export-download` | Use the exported `file_token` and a relative output path. |
| Import local file | `drive +import` | `--file ./relative --type docx|sheet|bitable|slides --folder-token <folder>`; follow with `+task_result` for async status. |
| Async result | `drive +task_result` | Use for import/export/move/delete task tokens until success/failure is clear. |
| Create folder | `drive +create-folder` | `--parent-token <folder> --name <name>`. |
| Delete file/folder | `drive +delete` | High-risk write: require explicit user confirmation and pass `--yes`; folder delete may need `+task_result`. |
| Preview formats | `drive +preview` | `--file-token <token> --list-only` to list downloadable PDF/HTML/text/image previews; add `--output ./relative` only when downloading. |
| Cover presets | `drive +cover` | `--file-token <token> --list-only`; add `--spec <preset> --output ./relative` only when downloading. |
| Folder list | `drive files list` | Raw API; read `references/lark-drive-files-list.md` if pagination/params are unclear. |
| Rename title | `drive files patch` | Use `new_title` for docx/sheet/bitable/file/wiki/folder; get schema before raw call. |
| Copy/move | `drive files copy` or `drive +move` | Prefer shortcut for move; raw copy requires schema. |
| Comments | `drive +add-comment`; raw `file.comments *` | Full comment/list/resolve via raw resources; replies via `file.comment.replys *`. |
| Permissions | `+apply-permission` or `permission.*` | Read permission guide for collaborator/public/app authorization details. |
| Versions | `+version-history/get/revert/delete` | Supports user and bot; destructive delete/revert needs explicit user intent. |
| Quota | `drive quota_details get --as user` | User-only; `quota_detail_id` is current user id. |
| Pull folder to local | `drive +pull` | `--folder-token <folder> --local-dir ./relative --if-exists skip|smart|overwrite`; create local root dir first if needed. |
| Push local to folder | `drive +push` | `--local-dir ./relative --folder-token <folder> --if-exists skip|smart|overwrite`; deletion needs `--delete-remote --yes`. |
| Status diff | `drive +status` | `--local-dir ./relative --folder-token <folder> [--quick]`; compares local and Drive without mutating. |
| Two-way sync | `drive +sync` | `--local-dir ./relative --folder-token <folder> --on-conflict remote-wins|local-wins|keep-both|ask`; does not delete extras by default. |
| Knowledge organize | Reference only | For broad "organize Drive/knowledge base" planning, read `references/lark-drive-workflow-knowledge-organize.md`; default to plan only. |

## Fixed Workflows

- Export: `+export` -> if async or token returned, `+task_result` as needed -> `+export-download`.
- Import: `+import` -> `+task_result` until success -> route to `lark-doc` / `lark-sheets` / `lark-base` only for content/table work.
- Inspect then act: `+inspect --url <url>` -> use returned `type` and canonical token -> choose the matching command; do not guess wiki backing type.
- Sync: `+status` first when user asks compare/check -> `+sync` only when user asks to apply changes.
- Comments with replies: `file.comments list` -> `file.comment.replys list` per comment when replies are required.

## Guardrails

- A `/wiki/` token is a `wiki_token`, not a `file_token`; use `+inspect` before download/export/comment unless a command explicitly accepts wiki URL/token.
- If `+inspect`, `+upload`, or a read command returns permission denied, not found, or missing scope, stop and surface the recovery path. Do not switch to write APIs or unrelated skills to bypass it.
