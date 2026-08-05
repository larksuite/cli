# apps file 域命令（应用存储）

管理妙搭应用的文件存储：上传 / 下载本地文件、列出与查看已存文件、生成临时分享链接、批量删除、查看用量。运行时命令事实以 `lark-cli apps +<cmd> --help` 为准；认证、`--as user`、exit 码、`_notice` 等通用处理见 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 与本域 [`SKILL.md`](../SKILL.md)。

## 何时用

用户要在某个妙搭应用里上传 / 下载 / 列出 / 删除文件、拿文件的临时分享链接、或看存储用量时。普通飞书云盘走 [`lark-drive`](../../lark-drive/SKILL.md)；数据库里的表数据走 `+db-*`。

## 命令一览

| 命令 | 做什么 | 关键参数 |
|---|---|---|
| `+file-list` | 列出文件，可按名/路径/类型/大小/上传时间过滤 | `--app-id`、过滤器、`--page-size`/`--page-token` |
| `+file-get` | 查单个文件的元数据 | `--app-id`、`--path` |
| `+file-sign` | 生成有时效的下载链接（用于分享 / 直接下载） | `--app-id`、`--path`、`--expires-in` |
| `+file-download` | 把远端文件保存到本地 | `--app-id`、`--path`、`--output` |
| `+file-upload` | 上传本地文件到应用存储 | `--app-id`、`--file` |
| `+file-delete` | 按路径批量删除文件 | `--app-id`、`--path`（可重复）、`--yes` |
| `+file-quota-get` | 查应用的文件存储用量 | `--app-id` |

## 寻址与约定（先读）

- **远端文件统一用 `--path` 精确寻址**（远端路径，带前导 `/`）。只知道文件名时，先用 `+file-list --name <名>` 定位拿到 `path`，再做后续操作。
- **本地文件 / 输出路径用工作目录内的相对路径**（如 `--file ./report.pdf`、`--output ./out.png`）；路径在别处时先 `cd` 过去或改成相对路径。
- 上传只接收本地 `--file`：文件名沿用本地文件名，远端路径由平台分配、全局唯一（无需也无法手填）。
- file 域不区分环境，没有 `--env`。

## 各命令

### +file-list
列出应用文件，支持精确过滤：`--name`（文件名）、`--path`（远端路径）、`--type`（MIME 类型）、`--size-gt`/`--size-lt`（字节）、`--uploaded-since`/`--uploaded-until`（上传时间区间，时间格式见末尾）。分页 `--page-size`（默认 20，范围 1..200）/ `--page-token`。列表每项给名称、路径、大小、类型、上传时间（pretty 表格即这 5 列）；上传者、下载地址（如有）仅在 JSON 输出里，单文件详情用 `+file-get`。

```bash
lark-cli apps +file-list --app-id app_xxx
lark-cli apps +file-list --app-id app_xxx --type image/png --uploaded-since 7d
```

### +file-get
按 `--path` 查单个文件的元数据。路径不存在时返回明确的「文件不存在」错误。

```bash
lark-cli apps +file-get --app-id app_xxx --path /1858537546760216.png
```

### +file-sign
为指定文件生成一个**有时效的下载链接**——适合发给用户分享、或直接下载。`--expires-in` 设有效期秒数（默认 1 天，最长 30 天）。`pretty` 模式只输出链接本身，便于复制 / 管道；要把到期时间一并告诉用户时用默认 JSON 输出（含到期时间）。

```bash
lark-cli apps +file-sign --app-id app_xxx --path /1858537546760216.png --expires-in 3600
```

### +file-download
把远端文件保存到本地。`--output` 指定保存路径，缺省时按远端文件名保存到当前目录。

```bash
lark-cli apps +file-download --app-id app_xxx --path /1858537546760216.png --output ./logo.png
```

### +file-upload
上传一个本地文件。文件名沿用本地文件名（特殊字符做 URL 编码透传；以 `.` 开头的隐藏文件名会加 `_` 前缀，避免下载回本地时覆盖隐藏文件），远端路径由平台分配。单文件上限 100 MB。

```bash
lark-cli apps +file-upload --app-id app_xxx --file ./report.pdf
```

### +file-delete（高危）
按路径批量删除，`--path` 可重复传多个。删除是高危操作，必须带 `--yes`；缺省会被确认关卡拦下。**逐项返回结果**：部分文件删除失败（如某个路径不存在）不影响其余文件，整体仍算成功，失败项在结果里单独标出原因。

```bash
lark-cli apps +file-delete --app-id app_xxx --path /1858537546760216.png --yes
lark-cli apps +file-delete --app-id app_xxx --path /a.png --path /b.png --yes
```

### +file-quota-get
查应用的文件存储用量（已用量、文件数；配额接入后还会给总配额与使用率）。

```bash
lark-cli apps +file-quota-get --app-id app_xxx
```

## 时间格式（`--uploaded-since` / `--uploaded-until`）

按用户口语自然传入即可，支持：
- 相对时间 `7d` / `2h` / `30s`（从现在往前推）
- 日期 `2026-04-15`
- 日期时间 `2026-04-15T10:00:00`
- 带时区的 ISO 8601 `2026-04-15T10:00:00Z` / `2026-04-15T10:00:00+08:00`

> **时区**：不带时区的 `日期` / `日期时间` 按**运行机器的本地时区**解析（再归一化到 UTC 发给服务端）。CI（UTC）与本地（如 UTC+8）跑同一条命令，过滤边界会差几小时；要精确到某时区时显式写 ISO 8601 带偏移（如 `...+08:00` / `...Z`）。

## Agent 规则

- 寻址一律用 `--path`；用户只给文件名时先 `+file-list --name <名>` 定位，多个同名再让用户确认。
- 上传 / 下载的本地路径用工作目录内相对路径；不在当前目录就 `cd` 过去或改相对路径。
- 用户要「分享链接 / 临时下载地址」时用 `+file-sign`，把返回的链接转述给用户。
- 删除前判断意图：已明确要删且授权时可直接带 `--yes`；不确定删哪些时先 `+file-list` 给用户确认。批量删除部分失败不报错，按逐项结果向用户说明哪些成功、哪些没删掉及原因。
