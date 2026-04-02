# drive +import

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

将本地文件（如 Word、TXT、Markdown、Excel 等）导入并转换为飞书在线云文档（docx、sheet、bitable）。底层统一通过 `POST /open-apis/drive/v1/import_tasks` 接口创建导入任务，并在 shortcut 内做有限次数轮询 `GET /open-apis/drive/v1/import_tasks/:ticket`。

## 命令

```bash
# 导入 Markdown 为新版文档 (docx)
lark-cli drive +import --file ./README.md --type docx

# 导入 Excel 为电子表格 (sheet)
lark-cli drive +import --file ./data.xlsx --type sheet

# 导入到指定文件夹，并指定导入后的文件名
lark-cli drive +import --file ./data.csv --type bitable --folder-token <FOLDER_TOKEN> --name "导入数据表"

# 预览底层调用链（上传 -> 创建任务 -> 轮询）
lark-cli drive +import --file ./README.md --type docx --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--file` | 是 | 本地文件路径，根据文件后缀名自动推断 `file_extension`，最大支持 20MB |
| `--type` | 是 | 导入目标云文档格式。可选值：`docx` (新版文档)、`sheet` (电子表格)、`bitable` (多维表格) |
| `--folder-token` | 否 | 目标文件夹 token，不传则请求中的 `point.mount_key` 为空字符串，Import API 会将其解释为导入到云空间根目录 |
| `--name` | 否 | 导入后的在线云文档名称，不传默认使用本地文件名去掉扩展名后的结果 |

## 行为说明

- **三步执行**：此 shortcut 内部封装了完整流程：
  1. 自动调用素材上传接口 (`/open-apis/drive/v1/medias/upload_all`) 获取源文件的 `file_token`
  2. 调用 `import_tasks` 接口发起导入任务，自动根据本地文件提取扩展名并构造挂载点（`mount_point`）参数
  3. 自动轮询查询导入任务状态；如果在内置轮询窗口内完成，则直接返回导入结果；如果仍未完成，则返回 `ticket`、当前状态和后续查询命令
- **默认根目录行为**：不传 `--folder-token` 时，shortcut 会保留空的 `point.mount_key`，Lark Import API 会将其视为“导入到调用者根目录”。

### 支持的文件类型转换

本地文件扩展名与目标云文档类型的对应关系如下：

| 本地文件扩展名 | 可导入为 | 说明 |
|--------------|---------|------|
| `.docx`, `.doc` | `docx` | Microsoft Word 文档 |
| `.txt` | `docx` | 纯文本文件 |
| `.md`, `.markdown`, `.mark` | `docx` | Markdown 文档 |
| `.html` | `docx` | HTML 文档 |
| `.xlsx`, `.xls` | `sheet`, `bitable` | Microsoft Excel 表格 |
| `.csv` | `sheet`, `bitable` | CSV 数据文件 |

> [!IMPORTANT]
> 文件扩展名与目标文档类型必须匹配，否则会返回验证错误：
> - 文档类文件（.docx, .doc, .txt, .md, .html）**只能**导入为 `docx`
> - 表格类文件（.xlsx, .xls, .csv）**只能**导入为 `sheet` 或 `bitable`
> - 例如：`.csv` 文件不能导入为 `docx`，`.md` 文件不能导入为 `sheet`

- 若导入任务执行失败，会返回失败时的 `job_status` 及错误信息。
- 若内置轮询超时但任务仍在处理中，shortcut 会成功返回，并带上：
  - `ready=false`
  - `timed_out=true`
  - `next_command`：可直接复制执行的后续查询命令，例如 `lark-cli drive +task_result --scenario import --ticket <TICKET>`
- 如果文件超过 20MB 上限，或者文件扩展名不被支持，执行时将抛出验证错误。

### 超时后的继续查询

当 `+import` 的内置轮询窗口结束但任务尚未完成时，使用返回结果中的 `ticket` 继续查询：

```bash
lark-cli drive +task_result --scenario import --ticket <TICKET>
```

> [!CAUTION]
> `drive +import` 是**写入操作** —— 执行前必须确认用户意图。

## 参考

- [lark-drive](../SKILL.md) -- 云空间全部命令
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
