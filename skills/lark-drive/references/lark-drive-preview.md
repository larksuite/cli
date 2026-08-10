## `drive +preview`

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、权限处理和安全规则。

查看或下载 Drive 文件内容，或列出并获取文件可用的预览产物。对象是 Drive **文件**，也支持 Wiki URL / node token（CLI 先把 Wiki 节点解析到底层文件，`obj_type` 必须是 `file`）。这个 shortcut 不猜测默认类型：

- 如果只需要查看或下载文件内容，或不关心 PDF/text/image 等转换预览，优先使用 `--type source_file --output <path>`
- 只想看候选项时，用 `--list-only`
- 如果需要服务端生成的预览效果，例如 doc/docx 的 PDF 版式预览，先用 `--list-only` 查看候选项，再按候选项选择 `--type pdf` / `text` / `image` 等
- 想下载时，必须显式传 `--type` 和 `--output`
- 如果 `--list-only` 没有可用预览候选项，或错误提示明确建议使用 `--type source_file`，可以改用 `--type source_file --output <path>` 查看文件内容；资源不存在、token 无效等终态错误需要先修正输入
- 如果某个候选项还在生成中，会返回结构化错误并提示先重新 `--list-only`

### 命令

```bash
# 查看文件内容
lark-cli drive +preview \
  --file-token "<FILE_TOKEN>" \
  --type source_file \
  --output ./artifacts/source

# 推荐：直接传 URL，CLI 自动解析类型和 token
lark-cli drive +preview \
  --url "https://example.feishu.cn/file/<FILE_TOKEN>" \
  --list-only

# Wiki URL 也可直接传，CLI 会先解析到底层 obj_token/obj_type（obj_type 必须是 file）
lark-cli drive +preview \
  --url "https://example.feishu.cn/wiki/<WIKI_NODE_TOKEN>" \
  --type source_file \
  --output ./artifacts/source

# 只有裸 Wiki node token 时，显式传 --wiki-token
lark-cli drive +preview \
  --wiki-token "<WIKI_NODE_TOKEN>" \
  --list-only

# 列出可用预览候选项
lark-cli drive +preview \
  --file-token "<FILE_TOKEN>" \
  --list-only

# 下载 PDF 预览
lark-cli drive +preview \
  --file-token "<FILE_TOKEN>" \
  --type pdf \
  --output ./artifacts/report

# 下载文本预览，并在目标已存在时自动改名
lark-cli drive +preview \
  --file-token "<FILE_TOKEN>" \
  --type text \
  --output ./artifacts/report \
  --if-exists rename

# 指定版本号查询/下载
lark-cli drive +preview \
  --file-token "<FILE_TOKEN>" \
  --version "12" \
  --type html \
  --output ./artifacts/report.html
```

### 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--file-token` | 条件必填 | Drive 文件 token；与 `--url` / `--wiki-token` 三选一 |
| `--url` | 条件必填 | 飞书文件 URL 或 Wiki URL；CLI 自动解析类型和 token |
| `--wiki-token` | 条件必填 | 裸 Wiki node token；CLI 先解析到底层 Drive 文件 |
| `--type` | 条件必填 | 预览类型；优先使用 `--list-only` 返回的 `type`，如 `pdf` / `html` / `text` / `png` / `jpg` / `source_file` |
| `--version` | 否 | 文件版本号 |
| `--list-only` | 否 | 仅返回候选项，不下载 |
| `--output` | 条件必填 | 下载到本地的输出路径 |
| `--if-exists` | 否 | 输出冲突策略：`error`（默认）/ `overwrite` / `rename` |

### 输出约定

- 查询态返回：
  - `mode=list`
  - `file_token`
  - `candidates[]`
  - `next_action`
- 下载态返回：
  - `mode=download`
  - `file_token`
  - `selected_type`
  - `output_path`
  - `status`

### 候选项字段

`candidates[]` 中每个对象包含：

- `type`
- `type_code`
- `label`
- `status`
- `status_code`
- `downloadable`
- `reason`（可选）

### 关键约束

- 不传 `--list-only` 时，必须显式传 `--type` 和 `--output`
- 不会隐式选择“第一个候选项”作为默认下载目标
- `--type source_file` 用于查看文件内容，不依赖 `--list-only` 返回的候选项；它适合读取或保存源内容，不等同于 PDF/text/image 等转换预览
- 候选项状态来自后端 `preview_status` 枚举，例如 `READY` / `PROCESSING` / `FAILED` / `NO_SUPPORT`
- 本地文件名在未显式带扩展名时，会结合响应头自动补扩展名
- Wiki URL / 裸 Wiki node token 会先解析到底层文档，解析后会在输出里附带 `wiki_token` 和 `wiki_node`（含底层 `obj_token`/`obj_type`）；`obj_type` 必须是 `file`。如果 Wiki 指向 `docx` / `sheet` / `bitable` / `slides` 等在线文档，`+preview` 无法直接处理，CLI 会返回 typed validation error，并在 hint 中提示改用 [lark-drive-export](lark-drive-export.md)

### 参考

- [lark-drive](../SKILL.md) -- Drive 总入口
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
