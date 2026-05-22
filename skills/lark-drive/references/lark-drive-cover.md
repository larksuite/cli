## `drive +cover`

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、权限处理和安全规则。

列出或下载 Drive 文件的稳定封面预设。这个 shortcut 只暴露 `spec`，不暴露底层 `cover_option` 细节。

### 命令

```bash
# 列出内置封面规格
lark-cli drive +cover \
  --file-token "<FILE_TOKEN>" \
  --list-only

# 下载 square 规格封面
lark-cli drive +cover \
  --file-token "<FILE_TOKEN>" \
  --spec square \
  --output ./artifacts/report-cover

# 下载默认大图封面，并在文件冲突时覆盖
lark-cli drive +cover \
  --file-token "<FILE_TOKEN>" \
  --spec default \
  --output ./artifacts/report-cover.png \
  --if-exists overwrite
```

### 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--file-token` | 是 | Drive 文件 token |
| `--spec` | 条件必填 | 封面预设：`default` / `icon` / `grid` / `small` / `middle` / `big` / `square` |
| `--version` | 否 | 文件版本号 |
| `--list-only` | 否 | 仅返回可选规格，不下载 |
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
  - `selected_spec`
  - `output_path`
  - `status`

### 内置规格

- `default` -- 标准大图封面
- `icon` -- 列表小图标
- `grid` -- 网格/卡片流小封面
- `small` -- PC 小图
- `middle` -- 中等尺寸封面
- `big` -- 偏移动端的大图封面
- `square` -- 正方形裁剪封面

### 关键约束

- 不传 `--list-only` 时，必须显式传 `--spec` 和 `--output`
- `drive +cover` 只返回静态预设规格，不伪造后端“可下载状态”
- 不返回底层 `bus_type` / `platform` / `width` / `height` / `policy` 等实现细节
- 下载时直接调用 `preview_download`
- 未显式带扩展名时，会优先根据响应头补扩展名，缺失时回退到 `.png`

### 错误提示

- 下载某个 `--spec` 时如果返回 **HTTP 404**，表示这个文件**没有该规格对应的封面产物**，应视为“该规格不可用”，而不是默认按网络抖动或临时失败处理

### 参考

- [lark-drive](../SKILL.md) -- Drive 总入口
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
