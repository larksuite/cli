# apps +page-source-get

`+page-source-get` 读创意应用工程里的**一个 UTF-8 源文件**，原样输出到 stdout 或存到本地。运行时命令事实以 `lark-cli apps +page-source-get --help` 为准。

## 何时用

手里只有创意应用的分享链接（`/page/<meta_token>`），要看清某个文件里到底写了什么——页面文案、样式、脚本、配置。路径不确定时先 `+page-source-list` 摸结构。

**只需 view 权限**。你是该应用的协作者、能打开页面，就能读代码，不需要下载权限。

## 不要用它的时候

| 目标 | 用什么 |
|---|---|
| 不知道路径 / 要先看目录结构 | [`lark-apps-page-source-list.md`](lark-apps-page-source-list.md) |
| 要整份源码快照 / 离线分析 / 归档 / 全文检索 | `+export`（一次拿全量 zip，比逐个文件读便宜） |
| 要读应用上传的附件、图片 | `+file-download`。本命令读的是**代码仓库**，不是文件存储 |

## 只覆盖创意模式（git 源码）应用的文本文件

- 应用不是创意模式、或不提供 Git 源码 → 422。
- 二进制文件、非 UTF-8 内容 → 422。本命令不做二进制转码，也不做十六进制预览。
- 目录 → 用 `+page-source-list`。
- 符号链接 → 422，不做跳转解析。

## 读到的是「最后一次提交」，不是沙箱当前状态

返回平台默认选定的已提交源码版本。用户在沙箱里改了但没提交的内容**不在结果里**。读到的内容"少了刚写的代码"时先确认是否已提交，不要重试。

## 命令骨架

- `--meta-token` 必填，收**裸 token**：只传 `/page/<token>` 链接的最后一段。
- `--path` 必填，相对工程根目录的文件路径，正斜杠，不许 `..`。
- `--output` 可选，相对当前目录的落盘路径。
- 单文件上限 10 MiB（服务端与 CLI 同口径）。

## 示例

```bash
lark-cli apps +page-source-get --meta-token <token> --path src/main.ts
lark-cli apps +page-source-get --meta-token <token> --path src/main.ts --output ./main.ts
```

## 输出契约

不给 `--output` 时：**stdout 就是文件原始内容**，不加 JSON envelope、不补尾随换行，`--format` 只影响报错输出。因此 `--jq` 不可用（本地直接报参数错误）——拿到文件后再单独过滤。

给 `--output` 时：stdout 是 JSON envelope，含 `output`（落盘绝对路径）与 `size_bytes`；`--format pretty` 输出一行 `Saved <path> (<n> bytes)`。

不给 `--output` 时，stdout 写出前会按已启用的内容安全配置扫描；block 命中时拒绝输出，warn 命中时在 stderr 提示但保留原文。`--output` 落盘路径不扫描文件正文。

## 错误处理

| 情况 | 怎么办 |
|---|---|
| 401 / 403 | 按 `error.hint` 处理认证或权限问题，通用规则见 lark-shared。 |
| 400 | 路径指向目录。改用 `+page-source-list --path <目录>` |
| 404 | 路径在该提交里不存在（或 token 错）。用 `+page-source-list --path <父目录>` 核对真实文件名，注意大小写 |
| 413 | 文件超过 10 MiB。本命令不做分块读取，需要时改用 `+export` 取整包 |
| 422 | 应用不支持源码读取，或该路径是二进制 / 非 UTF-8 / 符号链接 |
| 429 | 触发限流，按指数退避重试 |
