
# drive +export

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

把 `doc` / `docx` / `sheet` / `bitable` / `slides`（也支持 Wiki URL / Wiki node token 自动解包）导出到本地文件。这个 shortcut 内置有限轮询：

- 如果导出任务在轮询窗口内完成，会直接下载到本地目录
- 如果轮询结束仍未完成，会返回 `ticket`、`ready=false`、`timed_out=true` 和 `next_command`
- 后续继续查结果时，改用 `drive +task_result --scenario export`
- 拿到 `file_token` 后，改用 `drive +export-download`

## 命令

```bash
# 推荐：直接传 URL，CLI 自动解析类型和 token
lark-cli drive +export \
  --url "https://example.feishu.cn/docx/<DOCX_TOKEN>" \
  --file-extension pdf

# Wiki URL 也推荐直接传，CLI 会先解析到底层 obj_token/obj_type
lark-cli drive +export \
  --url "https://example.feishu.cn/wiki/<WIKI_NODE_TOKEN>" \
  --file-extension pdf

# 只有裸 Wiki node token 时，显式传 --doc-type wiki，让 CLI 先解析到底层文档类型
lark-cli drive +export \
  --token "<WIKI_NODE_TOKEN>" \
  --doc-type wiki \
  --file-extension pdf

# 导出新版文档为 pdf，默认保存到当前目录
lark-cli drive +export \
  --token "<DOCX_TOKEN>" \
  --doc-type docx \
  --file-extension pdf

# 导出旧版文档为 docx
lark-cli drive +export \
  --token "<DOC_TOKEN>" \
  --doc-type doc \
  --file-extension docx

# 导出 docx 为 markdown（Lark-flavored Markdown）
# 注意：markdown 只支持 docx
lark-cli drive +export \
  --token "<DOCX_TOKEN>" \
  --doc-type docx \
  --file-extension markdown

# 导出电子表格为 xlsx
lark-cli drive +export \
  --token "<SHEET_TOKEN>" \
  --doc-type sheet \
  --file-extension xlsx \
  --output-dir ./exports

# 导出幻灯片为 pptx
lark-cli drive +export \
  --token "<SLIDES_TOKEN>" \
  --doc-type slides \
  --file-extension pptx \
  --output-dir ./exports

# 导出幻灯片为 pdf
lark-cli drive +export \
  --token "<SLIDES_TOKEN>" \
  --doc-type slides \
  --file-extension pdf \
  --output-dir ./exports

# 指定本地文件名（会按导出格式自动补扩展名）
lark-cli drive +export \
  --token "<DOCX_TOKEN>" \
  --doc-type docx \
  --file-extension pdf \
  --file-name "weekly-report.pdf" \
  --output-dir ./exports

# 导出电子表格或多维表格为 csv 时，必须传 sub_id
lark-cli drive +export \
  --token "<SHEET_OR_BITABLE_TOKEN>" \
  --doc-type "<sheet|bitable>" \
  --file-extension csv \
  --sub-id "<SUB_ID>" \
  --output-dir ./exports

# 导出多维表格为 .base 快照（只支持 bitable）
lark-cli drive +export \
  --token "<BITABLE_TOKEN>" \
  --doc-type bitable \
  --file-extension base \
  --output-dir ./exports

# 导出多维表格结构为 .base 快照（仅导出表结构，不导出记录数据）
lark-cli drive +export \
  --token "<BITABLE_TOKEN>" \
  --doc-type bitable \
  --file-extension base \
  --only-schema \
  --output-dir ./exports

# 允许覆盖已存在文件
lark-cli drive +export \
  --token "<DOCX_TOKEN>" \
  --doc-type docx \
  --file-extension pdf \
  --overwrite
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 与 `--token` 二选一 | 源文档 URL，推荐优先使用；CLI 自动解析类型和 token，Wiki URL 会解析到底层 `obj_token/obj_type` |
| `--token` | 与 `--url` 二选一 | 源文档裸 token；裸 token 必须同时传 `--doc-type`。裸 Wiki node token 必须传 `--doc-type wiki`，CLI 会先解析到底层 `obj_token/obj_type` |
| `--doc-type` | 条件必填 | 源文档类型：`doc` / `docx` / `sheet` / `bitable` / `slides` / `wiki`；仅当使用裸 `--token` 时必填，使用 `--url` 时自动推断。`wiki` 只用于裸 Wiki node token，解析后会按真实底层类型发起导出 |
| `--file-extension` | 是 | 导出格式：`docx` / `pdf` / `xlsx` / `csv` / `markdown` / `base` / `pptx` |
| `--sub-id` | 条件必填 | 当 `sheet` / `bitable` 导出为 `csv` 时必填 |
| `--only-schema` | 否 | 仅当 `--doc-type bitable --file-extension base` 时可用；只导出多维表格结构，不导出记录数据 |
| `--file-name` | 否 | 覆盖默认本地文件名；如未带扩展名，会按 `--file-extension` 自动补齐 |
| `--output-dir` | 否 | 本地输出目录，默认当前目录 |
| `--overwrite` | 否 | 覆盖已存在文件 |

## 关键约束

- 推荐优先传 `--url`，不要从 URL 手工拆 token 和 type；尤其是 Wiki URL，CLI 会自动解包到底层资源
- `--url` 和 `--token` 互斥
- 裸 `--token` 必须传 `--doc-type`；裸 Wiki node token 使用 `--doc-type wiki`
- `doc` 支持导出为 `docx` / `pdf`
- `docx` 支持导出为 `docx` / `pdf` / `markdown`
- `sheet` 支持导出为 `xlsx` / `csv`
- `bitable` 支持导出为 `xlsx` / `csv` / `base`
- `slides` 支持导出为 `pptx` / `pdf`
- `csv` 只支持 `sheet` / `bitable`，且必须带 `--sub-id`
- `--only-schema` 只支持 `bitable` 导出为 `.base`，用于仅导出表结构
- 如果格式不匹配，CLI 会返回 typed validation error，并在 `hint` 中给出可重试的 `--file-extension` 建议；例如 `docx + csv` 会提示改用 `docx/pdf/markdown`，或改传 sheet/bitable URL
- shortcut 内部固定有限轮询：最多 10 次，每次间隔 5 秒
- 轮询超时不是失败；会返回 `ticket`、`timed_out=true` 和 `next_command`，供后续继续查询

## 错误码处理

| 错误码 | 含义 | 处理方式 |
|--------|------|----------|
| `1069914` | token 非法或 token/type 不匹配；常见原因是把 Wiki node token 当作底层 `docx` / `sheet` / `bitable` token 使用，没有传 `--doc-type wiki` | 优先改用 `--url <Wiki URL>`；只有裸 Wiki token 时，用 `--token <WIKI_NODE_TOKEN> --doc-type wiki`。不确定 token 类型时，先用 `lark-cli drive +inspect --url <TOKEN> --type wiki` 检查是否能解包为 Wiki node；如果不是 Wiki token，再检查 token 来源、`--doc-type` 是否与实际资源类型一致 |
| `1069902` | 没有当前导出任务所需权限 | 不要直接重试同一命令；先确认当前 `--as` 身份是否能访问该文档、是否有下载/导出权限，以及文档是否受分享、密级或租户策略限制。需要补权限时，让文档 owner 或管理员授权后再执行 |
| `99991679` | 缺少 OpenAPI scope | 按错误 envelope 中的 `missing_scopes` / `required_scope` / `hint` 补齐授权；常见方式是重新执行 `lark-cli auth login --scope "<缺失 scope>"`。补 scope 前不要反复重试导出命令 |

## 推荐续跑方式

```bash
# 第一步：先尝试直接导出
lark-cli drive +export \
  --url "<DOCX_URL>" \
  --file-extension pdf \
  --file-name "weekly-report.pdf"

# 如果返回 ready=false / timed_out=true，再继续查
lark-cli drive +task_result \
  --scenario export \
  --ticket "<TICKET>" \
  --file-token "<DOCX_TOKEN>"

# 查到 file_token 后下载
lark-cli drive +export-download \
  --file-token "<EXPORTED_FILE_TOKEN>" \
  --file-name "weekly-report.pdf" \
  --output-dir ./exports
```

## 参考

- [lark-drive](../SKILL.md) -- 云空间（云盘/云存储）全部命令
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
