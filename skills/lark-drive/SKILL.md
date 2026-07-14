---
name: lark-drive
version: 1.0.0
description: "飞书云空间（云盘/云存储）：管理 Drive 文件和文件夹，包含上传/下载、创建文件夹、复制/移动/删除、查看元数据、评论/权限/订阅、标题、版本和本地文件导入。用户需要整理云盘目录、处理云空间资源 URL/token、判断链接类型/真实 token/标题，或导入 Word/Markdown/Excel/CSV/PPTX/.base 为 docx/sheet/bitable/slides 时使用；doubao.com 云空间 URL/token 也按资源路径和 token 路由，不回退 WebFetch。不负责：文档内容编辑（走 lark-doc）、表格/Base 表内数据操作（走 lark-sheets/lark-base）、知识空间节点/成员管理（走 lark-wiki）、原生 Markdown 文件读写/patch/diff（走 lark-markdown）。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli drive --help"
---

# drive (v1)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

> **术语说明：** 飞书云空间也常被称为"云盘"、"云存储"、"网盘"或"我的空间"，这些说法通常指的是同一个产品，是飞书官方的云端文件存储与管理中心。

> **导入分流规则：** 如果用户要把本地 Excel / CSV / `.base` 快照导入成 Base / 多维表格 / bitable，必须优先使用 `lark-cli drive +import --type bitable`。不要先切到 `lark-base`；`lark-base` 只负责导入完成后的表内操作。

> **副本分流规则：** 如果用户要复制在线文档、创建文档副本、把文档复制到另一个文件夹，必须使用 `lark-cli drive files copy`。不要用 `drive +export` 下载后再 `drive +import` 上传，也不要用 `docs +fetch` + `docs +create` 重建正文；导出/导入只用于本地文件转换或离线产物。

## 快速决策

- 用户要**复制文档 / 创建副本 / 另存为副本**时，使用 `lark-cli drive files copy`。先用 `lark-cli schema drive.files.copy --format json` 确认参数；如果来源是 wiki URL/token，先用 `lark-cli drive +inspect` 获取底层 `token` 和 `type`，不要把 wiki token 直接当 `file_token`。`params.file_token` 传源文档 token，`data.folder_token` 传目标文件夹 token，`data.name` 传副本名称，`data.type` 传源文件类型（如 `docx` / `sheet` / `bitable` / `slides`）。示例：`lark-cli drive files copy --params '{"file_token":"<DOC_TOKEN>"}' --data '{"folder_token":"<FOLDER_TOKEN>","name":"<COPY_NAME>","type":"docx"}'`。如返回 `confirmation_required`，按 `lark-shared` 高风险审批协议向用户确认后，在原命令末尾追加 `--yes` 重试。
- 用户要**识别飞书 / doubao 云空间 URL 的类型和 token**时，可以先按 URL 路径形态做轻量判断；当路径已明确指向 docx / sheet / bitable / slides / file / folder 等资源时，可直接提取对应 token/type。传入 wiki URL、需要识别标题或 canonical URL、URL/token 有歧义，或后续操作依赖底层真实资源时，再使用 `lark-cli drive +inspect --url '<url>'` 进行识别；具体用法、失败处理和边界见 [`references/lark-drive-inspect.md`](references/lark-drive-inspect.md)。
- 高风险写操作（删除、公开权限修改、owner 转移、版本删除/回滚、批量移动/覆盖/同步）必须同时满足三个条件才执行：目标已解析为该操作可直接使用的执行对象，执行细节已明确到可直接调用命令（例如删除的 file-token/type、公开权限修改的共享范围、owner 转移的目标 owner、版本删除/回滚的 version id、移动/覆盖/同步的目标位置和冲突策略），且用户在本轮明确确认执行这些具体目标和执行细节。用户只说“删除没用的文件”“开放/共享给大家”“改成开放”“覆盖/移动这些”只表示目标状态；先只读发现并列出候选、权限档位或执行方案，停止等待用户确认。
- 用户要**检查 / 治理文档权限、公开范围、链接分享、外部访问、复制下载权限、密级标签、owner 转移**，或要“权限风险报告、收紧权限、申请查看 / 编辑权限、转移 / 批量转移 owner”，必须先阅读 [`references/lark-drive-workflow.md`](references/lark-drive-workflow.md)，再按其中 `Workflow Registry` 进入 [`permission_governance`](references/lark-drive-workflow-permission-governance.md) workflow。
- 用户要**整理云盘 / 文件夹 / 文档库 / 知识库 / 个人文档库**，或要“盘点目录结构、找出未归档/临时/重复/空目录、生成整理方案”，必须先阅读 [`references/lark-drive-workflow.md`](references/lark-drive-workflow.md)，再按其中 `Workflow Registry` 进入 [`knowledge_organize`](references/lark-drive-workflow-knowledge-organize.md) workflow。默认只生成方案；创建目录、移动资源、申请权限都必须单独确认。
- 用户要**搜文档 / Wiki / 电子表格 / 多维表格 / 云空间（云盘/云存储）对象**，优先使用 `lark-cli drive +search`。自然语言里"最近我编辑过的"、"我创建的"（→ `--created-by-me`，原始创建者语义）、"我负责/owner 的"（→ `--mine`，owner 语义）、"最近一周我打开过的 xxx"、"某人 owner 的 docx" 等直接映射到扁平 flag，避免手写嵌套 JSON。
- 用户要**获取文档评论列表**时，优先使用 `lark-cli drive +list-comments --url '<url>'`，不要优先手写 `drive file.comments list`；支持妙搭 apps 的 `/page/<token>` URL；具体使用方式先阅读 [`references/lark-drive-list-comments.md`](references/lark-drive-list-comments.md)。
- 妙搭 apps 评论场景：除新增全文/局部评论不支持外，评论列表、批量查询、解决/恢复、回复创建/读取/更新/删除、reaction 添加/删除等评论管理能力已支持；使用原生命令时文档类型传 `apps`（`file_type=apps`），裸 token 调 shortcut 时传 `--type apps`。
- 用户要**根据文档评论定位正文位置**，例如 根据评论 review 文档、根据评论内容回看文档、区分多处相同引用文本时，对于 docx 类型（`file_type=docx`）的文档支持通过 `drive +list-comments --need-relation` 返回评论位置，其他类型会静默忽略该参数；具体用法需要先阅读 [`references/lark-drive-comment-location.md`](references/lark-drive-comment-location.md) 了解。
- 用户给出 doubao.com 的云空间资源 URL/token，或明确提到豆包里的 file/folder/docx/sheet/bitable/wiki 资源时，仍按资源类型、URL 路径和 token 路由到本 skill；不要因为域名不是飞书而回退到 WebFetch。
- 用户要把本地 `.xlsx` / `.csv` / `.base` 导入成 Base / 多维表格 / bitable，第一步必须使用 `lark-cli drive +import --type bitable`。
- 用户要把本地 `.md` / `.docx` / `.doc` / `.txt` / `.html` 导入成在线文档，使用 `lark-cli drive +import --type docx`。
- 用户要把本地 `.pptx` 导入成飞书幻灯片，使用 `lark-cli drive +import --type slides`；当前 PPTX 导入上限是 500MB。
- 批量执行 `drive +import` 且目标是同一个位置（同一 `--folder-token`、默认根目录，或同一 `--target-token`）时，必须串行执行；不要并发导入到同一位置，服务端可能返回并发冲突错误。
- 用户要在 Drive 里上传、创建、读取、局部 patch 或覆盖更新**原生 `.md` 文件**（不是导入成 docx），切到 [`lark-markdown`](../lark-markdown/SKILL.md)。
- 用户要比较原生 `.md` 文件的**历史版本差异**，或比较远端 Markdown 与本地草稿，切到 [`lark-markdown`](../lark-markdown/SKILL.md) 的 `lark-cli markdown +diff`；需要版本号时先用 `drive +version-history`。
- 用户要查看、下载、回滚或删除文件的**历史版本**，使用 `drive +version-history`、`drive +version-get`、`drive +version-revert`、`drive +version-delete`；这组命令同时支持 `--as user` 和 `--as bot`，自动化场景优先 `--as bot`。
- 用户要把本地 `.xlsx` / `.xls` / `.csv` 导入成电子表格，使用 `lark-cli drive +import --type sheet`。
- 用户要在云空间（云盘/云存储）里新建文件夹，优先使用 `lark-cli drive +create-folder`。
- 用户要查看某个文件有哪些可下载预览格式，或想下载 PDF / HTML / 文本 / 图片等预览产物，使用 `lark-cli drive +preview`。
- 用户要获取某个文件的封面图，优先使用 `lark-cli drive +cover`；先 `--list-only` 看规格，再选 `--spec` 下载。
- 用户要导出云文档时，优先使用 `lark-cli drive +export --url '<文档 URL>' --file-extension <格式>`；详细参数、Wiki token 和错误码处理见 [`references/lark-drive-export.md`](references/lark-drive-export.md)。
- 用户要把本地文件上传到知识库 / 文档库里的某个 wiki 节点下时，仍然使用 `lark-cli drive +upload --wiki-token <wiki_token>`；不要误切到 `wiki` 域命令。
- `lark-base` 只负责导入完成后的 Base 内部操作（表、字段、记录、视图），不要在“本地文件 -> Base”这一步提前切到 `lark-base`。
- 用户给的是 wiki URL / token，且后续还没明确底层资源类型时，先用 `lark-cli drive +inspect` 解包；`+inspect` 失败后不要自动切到别的写接口继续尝试，先按错误提示处理权限、scope 或链接问题。
- `drive +inspect` / `drive +upload` 遇到 `not found`、`permission denied`、`missing scope` 时，默认停止重试；只有 `rate limit` 或临时网络错误才适合有限重试。

## 修改标题
- 使用 `drive files patch` 命令，通过new_title字段可以修改标题，支持 docx、sheet、bitable、file、wiki、folder 类型

## 核心概念

### 文档类型与 Token

飞书开放平台中，不同类型的文档有不同的 URL 格式和 Token 处理方式。在进行文档操作（如添加评论、下载文件等）时，必须先获取正确的 `file_token`。

### 文档 URL 格式与 Token 处理

| URL 格式 | 示例                                                      | Token 类型 | 处理方式 |
|----------|---------------------------------------------------------|-----------|----------|
| `/docx/` | `https://example.larksuite.com/docx/doxcnxxxxxxxxx`    | `file_token` | URL 路径中的 token 直接作为 `file_token` 使用 |
| `/doc/` | `https://example.larksuite.com/doc/doccnxxxxxxxxx`     | `file_token` | URL 路径中的 token 直接作为 `file_token` 使用 |
| `/wiki/` | `https://example.larksuite.com/wiki/wikcnxxxxxxxxx`    | `wiki_token` | 不能直接当底层 `file_token`；优先用 `drive +inspect` 解包获取 `obj_token` |
| `/sheets/` | `https://example.larksuite.com/sheets/shtcnxxxxxxxxx`  | `file_token` | URL 路径中的 token 直接作为 `file_token` 使用 |
| `/page/` | `https://example.feishu.cn/page/N1BWmMrqndT5ZcamAIBcnvDLnOf/` | apps token | 妙搭 apps 类型；用于评论列表时直接作为 `file_token`，`file_type=apps` |
| `/drive/folder/` | `https://example.larksuite.com/drive/folder/fldcnxxxx` | `folder_token` | URL 路径中的 token 作为文件夹 token 使用 |

### Wiki 链接特殊处理

```bash
lark-cli drive +inspect --url 'https://xxx.feishu.cn/wiki/wikcnXXX'
```

知识库链接背后可能是 docx、sheet、bitable、slides、file 等不同对象。后续要做评论、下载、导出或内容读取时，优先用 `drive +inspect` 拿到 `type`、`token`、`title`、`url`；完整手动解析和跨 skill 路由见共享文档 [`lark-wiki-token-routing.md`](../lark-shared/references/lark-wiki-token-routing.md)。不要只根据 `/wiki/<token>` 猜底层类型。

### 常见操作 Token 需求

| 操作 | 需要的 Token | 说明 |
|------|-------------|------|
| 读取文档内容 | `file_token` / 通过 `docs +fetch` 自动处理 | `docs +fetch` 支持直接传入 URL |
| 添加局部评论（划词评论） | `file_token` | 传 `--block-id` 时，`drive +add-comment` 会创建局部评论；`docx` 支持文本定位或 block_id，`sheet` 使用 `<sheetId>!<cell>`，`slides` 使用 `<slide-block-type>!<xml-id>`；Base 只有记录局部评论，定位为 file_token(base_token) + `--block-id <table-id>!<record-id>!<view-id>` |
| 添加全文评论 | `file_token` | 不传 `--block-id` 时，`drive +add-comment` 默认创建全文评论；支持 `docx`、旧版 `doc` URL、白名单扩展名的 Drive file，以及最终解析为 `doc`/`docx`/`file` 的 wiki URL |
| 下载文件 | `file_token` | 从文件 URL 中直接提取 |
| 上传文件 | `folder_token` / `wiki_node_token` | 目标位置的 token |
| 列出文档评论 | URL 或 `file_token` | 优先使用 `drive +list-comments --url '<url>'`；wiki URL/token 会自动解析到底层真实 token/type；妙搭 apps URL 使用 `/page/<token>` |

### 评论能力入口

- 添加评论优先使用 [`+add-comment`](references/lark-drive-add-comment.md)：review / 审阅 / 校对场景默认尽量创建局部评论，不要把多个可定位问题合并为一条全文评论。
- 获取评论列表优先使用 [`+list-comments`](references/lark-drive-list-comments.md)：推荐传 `--url`，支持 wiki 自动解包；参数细节见 reference。
- 评论查询、统计、排序、回复限制，先读 [`lark-drive-comments-guide.md`](references/lark-drive-comments-guide.md)。
- 需要根据评论定位正文位置时，先确认目标是 `file_type=docx`，再读 [`lark-drive-comment-location.md`](references/lark-drive-comment-location.md)，并使用 `drive +list-comments --need-relation`；其他文档类型会静默忽略该参数。
- reaction / 表情相关操作先读 [`lark-drive-reactions.md`](references/lark-drive-reactions.md)；只有用户明确需要 reaction 信息时才带 `need_reaction=true`。
- `drive +add-comment` 的 `--content` 需要传 `reply_elements` JSON 数组字符串，例如 `--content '[{"type":"text","text":"正文"}]'`。
- `slides` 评论要求显式传 `--block-id <slide-block-type>!<xml-id>`；CLI 会将其拆分后写入 `anchor.block_id` 和 `anchor.slide_block_type`。其中 `<xml-id>` 是 PPT XML 协议中的元素 `id`；不支持 `--selection-with-ellipsis` 和 `--full-comment`。
- 评论写入内容（添加评论、回复评论、编辑回复）里的文本不能直接出现 `<`、`>`；提交前必须先转义：`<` -> `&lt;`，`>` -> `&gt;`。
- 使用 `drive +add-comment` 时，shortcut 会对 `type=text` 的文本元素自动做上述转义兜底；如果直接调用 `drive file.comments create_v2`、`drive file.comment.replys create`、`drive file.comment.replys update`，则需要在请求里自行传入已转义的内容。
- Base 记录局部评论使用 `--type bitable` / `--type base` 或 `/base/`、`/bitable/`、wiki Base 链接；`bitable` 和 Base 是同一概念，`bitable` 是内部代号、Base 是产品名，裸 token 推荐传 `bitable`，`base` 仅作为兼容别名兜底。
- Base 不支持全局评论，所有评论都挂在记录上；定位信息必须是 file token（base token）+ `--block-id <table-id>!<record-id>!<view-id>`，其中 table/record/view ID 通常分别以 `tbl`/`rec`/`vew` 开头。view_id 只决定被提及时点击通知打开哪个视图，不影响评论挂载点；只要在同一记录上都能看到评论，但必须传，否则通知无法确定跳转视图。ID 可通过 [`lark-base`](../lark-base/SKILL.md) 获取。
- 如果 wiki 解析后不是 `doc`/`docx`/`file`/`sheet`/`slides`/`bitable`/`base`，不要用 `+add-comment`。
- 如果需要更底层地直接调用评论 V2 协议，再走原生 API：先执行 `lark-cli schema drive.file.comments.create_v2`，再执行 `lark-cli drive file.comments create_v2 ...`。全文评论省略 `anchor`；docx/sheet/slides 局部评论传 `anchor.block_id`，Base 记录局部评论传 `anchor.block_id`（table_id）、`anchor.base_record_id`、`anchor.base_view_id`。
- 直接调用原生 `drive.file.comments.*` / `drive.file.comment.replys.*` 评论 Base 文档时，`file_type` 填 `bitable`，不要填 `base`。

### 典型错误与解决方案

| 错误信息 | 原因 | 解决方案 |
|----------|------|----------|
| `not exist` | 使用了错误的 token | 检查 token 类型，wiki 链接必须先查询获取 `obj_token` |
| `permission denied` | 没有相关操作权限 | 引导用户检查当前身份对文档/文件是否有相应操作权限；如果需要，可以授予相应权限 |
| `invalid file_type` | file_type 参数错误 | 根据 `obj_type` 传入正确的 file_type（docx/doc/sheet/slides/bitable/apps） |
| `232140101` / `232140100` / `233523001`（常见于 `drive +import` 的 `job_error_msg`） | 同一位置下存在并发导入 / 创建操作 | 批量导入到同一文件夹、根目录或同一 `--target-token` 时改为串行执行；每个失败项每次重试前等待几秒，总共最多重试 3 次，仍失败就停止并报告冲突 |

### 权限能力入口

- 用户要管理 Drive 文档/文件协作者、公开权限、授权当前应用访问文档，或处理 `permission.public.patch` 的 `91009` / `91010` / `91011` / `91012` 错误时，先读 [`lark-drive-permission-guide.md`](references/lark-drive-permission-guide.md)。
- 用户只是没有访问权限并希望向 owner 申请访问，优先使用 [`+apply-permission`](references/lark-drive-apply-permission.md)。
- 普通 scope、身份或登录问题仍按 [`lark-shared`](../lark-shared/SKILL.md) 处理；不要把租户安全策略、对外分享、密级拦截简单归类为缺 scope。

## 不在本 skill 范围

- 文档正文读取、总结、创建、编辑、图片/附件插入或下载：使用 [`lark-doc`](../lark-doc/SKILL.md)。
- 电子表格单元格、筛选、公式、样式等表内操作：使用 [`lark-sheets`](../lark-sheets/SKILL.md)。
- Base / 多维表格内部的表、字段、记录、视图、仪表盘等操作：使用 [`lark-base`](../lark-base/SKILL.md)。
- 知识空间、Wiki 节点层级、空间成员管理：使用 [`lark-wiki`](../lark-wiki/SKILL.md)；上传本地文件到 wiki 节点仍用 `drive +upload --wiki-token`。
- 原生 Markdown 文件读取、写入、patch、diff：使用 [`lark-markdown`](../lark-markdown/SKILL.md)；把 Markdown 导入成在线 docx 才用 `drive +import --type docx`。

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli drive +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut | 说明 |
|----------|----------|
| [`+search`](references/lark-drive-search.md) | 搜索文档、Wiki、表格、文件夹等云空间对象；支持 `--edited-since`、`--created-by-me`、`--mine`、`--doc-types` 等扁平 flag；区分 original creator 与 owner 语义。 |
| [`+upload`](references/lark-drive-upload.md) | 上传本地文件到 Drive 文件夹或 wiki 节点。 |
| [`+create-folder`](references/lark-drive-create-folder.md) | 新建 Drive 文件夹，支持父文件夹与 bot 创建后自动授权。 |
| [`+download`](references/lark-drive-download.md) | 下载 Drive 文件到本地。 |
| [`+preview`](references/lark-drive-preview.md) | 查看或下载文件的 PDF / HTML / 文本 / 图片等预览产物。 |
| [`+cover`](references/lark-drive-cover.md) | 查看或下载文件封面图规格。 |
| [`+status`](references/lark-drive-status.md) | 比较本地目录与 Drive 文件夹差异；默认按 SHA-256 精确比较，`--quick` 使用修改时间近似比较。 |
| [`+pull`](references/lark-drive-pull.md) | 从 Drive 拉取文件到本地目录，支持重复远端路径处理和增量模式。 |
| `+sync` | 双向同步本地目录与 Drive 文件夹：拉取 `new_remote`、推送 `new_local`，`modified` 按 `--on-conflict=remote-wins\|local-wins\|keep-both\|ask` 处理；`--quick` 用修改时间近似比较；`--on-duplicate-remote` 支持 `fail` / `newest` / `oldest`；只同步 `type=file`，跳过在线文档和 shortcut，且不会删除两端多余文件。 |
| [`+push`](references/lark-drive-push.md) | 将本地目录推送到 Drive 文件夹，支持 skip / smart / overwrite 与确认后删除远端。 |
| [`+create-shortcut`](references/lark-drive-create-shortcut.md) | 在另一个文件夹里创建现有 Drive 文件的快捷方式。 |
| [`+add-comment`](references/lark-drive-add-comment.md) | 给 doc/docx/file/sheet/slides/base(bitable) 添加评论，也支持解析到这些类型的 wiki URL；评论统计、回复和 reaction 细则见 [`lark-drive-comments-guide.md`](references/lark-drive-comments-guide.md)。 |
| [`+list-comments`](references/lark-drive-list-comments.md) | 获取 doc/docx/sheet/file/slides/base(bitable)/apps 评论列表；优先传 URL，支持 wiki 自动解包和妙搭 `/page/<token>` URL。 |
| [`+export`](references/lark-drive-export.md) | 将 doc/docx/sheet/bitable/slides 导出为本地文件。 |
| [`+export-download`](references/lark-drive-export-download.md) | 根据导出产物的 file_token 下载文件。 |
| [`+import`](references/lark-drive-import.md) | 将本地文件导入为飞书在线文档、表格、多维表格或幻灯片。 |
| [`+version-history`](references/lark-drive-version-history.md) | 查看文件历史版本。 |
| [`+version-get`](references/lark-drive-version-get.md) | 下载指定历史版本。 |
| [`+version-revert`](references/lark-drive-version-revert.md) | 回滚到指定历史版本。 |
| [`+version-delete`](references/lark-drive-version-delete.md) | 删除指定历史版本。 |
| [`+move`](references/lark-drive-move.md) | 移动 Drive 文件或文件夹；Wiki 层级移动走 `lark-wiki`。 |
| [`+delete`](references/lark-drive-delete.md) | 删除 Drive 文件或文件夹，文件夹删除会轮询异步任务。 |
| [`+task_result`](references/lark-drive-task-result.md) | 查询 import/export/move/delete 等异步任务结果。 |
| [`+inspect`](references/lark-drive-inspect.md) | 检视 URL 的类型、标题和 canonical token；wiki URL 会自动解包到底层文档。 |
| [`+apply-permission`](references/lark-drive-apply-permission.md) | 以 user 身份向文档 owner 申请访问权限。 |
| [`+member-add`](references/lark-drive-member-add.md) | 添加一个或最多 10 个 Drive 文档、文件、文件夹或 wiki 节点协作者/授权成员；封装 Drive permission member create/batch_create，真实写入需要 `--yes`。 |
| [`+secure-label-list`](references/lark-drive-secure-label.md) | 列出当前用户可用的密级标签。 |
| [`+secure-label-update`](references/lark-drive-secure-label.md) | 更新 Drive 文件或文档的密级标签。 |

## API Resources

```bash
lark-cli schema drive.<resource>.<method>   # 调用 API 前必须先查看参数结构
lark-cli drive <resource> <method> [flags] # 调用 API
```

> **重要**：使用原生 API 时，必须先运行 `schema` 查看 `--data` / `--params` 参数结构，不要猜测字段格式。
>
> **高频原生命令：** 读取 Drive 文件夹清单时使用 `drive files list`，使用前先读 [`references/lark-drive-files-list.md`](references/lark-drive-files-list.md)，按模板通过 `--params` 传参并手动处理分页；不要把 `--page-all` 输出直接交给 JSON 解析脚本。

### files

  - `copy` — 复制文件；在线文档创建副本的首选能力，完整参数见上方“快速决策”，不要用 `drive +export` / `drive +import` 绕行复制
  - `create_folder` — 新建文件夹
  - `list` — 获取文件夹下的清单；使用前阅读 [`references/lark-drive-files-list.md`](references/lark-drive-files-list.md)
  - `patch` — 修改文件标题

### file.comments

  - `batch_query` — 批量获取评论
  - `create_v2` — 添加全文/局部（划词）评论
  - `list` — 分页获取文档评论
  - `patch` — 解决/恢复 评论

### file.comment.replys

  - `create` — 添加回复
  - `delete` — 删除回复
  - `list` — 获取回复
  - `update` — 更新回复

### permission.members

  - `auth` — 
  - `create` — 增加协作者权限
  - `transfer_owner` — 

### metas

  - `batch_query` — 获取文档元数据

### user

  - `remove_subscription` — 取消订阅用户、应用维度事件
  - `subscription` — 订阅用户、应用维度事件（本次开放评论添加事件）
  - `subscription_status` — 查询用户、应用对指定事件的订阅状态

### file.statistics

  - `get` — 获取文件统计信息
    - 获取 docx / 文件统计信息时，建议优先使用 typed flags：`lark-cli drive file.statistics get --file-token <token> --file-type <type> --format json`；`--params` JSON 也支持，适合批量拼装或 raw 参数场景。

### file.view_records

  - `list` — 获取文档的访问者记录
    - 查看 docx 最近访问记录、返回 open_id、最多 N 条时，建议优先使用 typed flags：`lark-cli drive file.view_records list --file-token <docx_token> --file-type docx --page-size <N> --viewer-id-type open_id --format json`；`--params` JSON 也支持，适合批量拼装、分页续跑或 raw 参数场景。

### file.comment.reply.reactions

  - `update_reaction` — 添加/删除 reaction

### quota_details

  - `get` — 获取当前用户的容量信息，包含各业务使用量、租户配额是否超限、用户配额、所在部门配额
    - 仅支持 `--as user`，不要使用默认的 bot 身份
    - `quota_detail_id` 传当前用户的 `user_id`
