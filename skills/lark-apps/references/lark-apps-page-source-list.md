# apps +page-source-list

`+page-source-list` 列出创意应用工程里的**一个目录**，不递归、不返回文件内容。运行时命令事实以 `lark-cli apps +page-source-list --help` 为准。

## 何时用

手里只有创意应用的分享链接（`/page/<meta_token>`），想先看清工程结构再决定读哪个文件。典型顺序：`+page-source-list` 摸结构 → `+page-source-get` 读具体文件。

**只需 view 权限**。这是它相对 `+export` 的核心区别：你是该应用的协作者、能打开页面，就能列出目录，不需要下载权限。

## 不要用它的时候

| 目标 | 用什么 |
|---|---|
| 读文件内容 | [`lark-apps-page-source-get.md`](lark-apps-page-source-get.md) |
| 要整份源码快照 / 离线分析 / 归档 | `+export`（需下载权限，产出 zip） |
| 看应用上传的文件存储（附件、图片） | `+file-list`。本命令读的是**代码仓库**，不是文件存储 |

## 只覆盖创意模式（git 源码）应用

非创意模式应用、或不提供 Git 源码的旧版应用，服务端返回 422。这不是权限问题，换参数无解。

## 读到的是「最后一次提交」，不是沙箱当前状态

返回平台默认选定的已提交源码版本。用户在沙箱里改了但没提交的内容**不在结果里**。看不到刚写的代码时先确认是否已提交，不要重试。

## 命令骨架

- `--meta-token` 必填，收**裸 token**：只传 `/page/<token>` 链接的最后一段。整条 URL 会在本地被拦下。
- `--path` 可选，相对工程根目录；省略时列根目录。
- `--page-size` 取 1..100，省略时服务端按 50 返回。
- `--page-token` 透传上一页的 `next_page_token`，原样传，不要改写。
- 必须用**用户身份**（`--as user`）调用。Agent Employee profile 暂不支持，会在鉴权阶段失败。

## 示例

```bash
lark-cli apps +page-source-list --meta-token <token>
lark-cli apps +page-source-list --meta-token <token> --path src --format pretty
lark-cli apps +page-source-list --meta-token <token> --path src --page-size 100
lark-cli apps +page-source-list --meta-token <token> --path src --page-token <next_page_token>
```

## 输出契约

JSON envelope 的 `data` 形如：

```json
{
  "items": [{"name": "main.ts", "path": "src/main.ts", "type": "file", "size": 1024}],
  "next_page_token": "...",
  "has_more": true
}
```

`meta.pagination` 给出 `complete` / `next_token`，`has_more` 为真时按 `next_token` 继续翻页。`--format pretty` 渲染成对齐表格，末行提示下一页的 `--page-token`。

`size` 是字节数；目录项为 0。

## 错误处理

| 情况 | 怎么办 |
|---|---|
| 401 未授权 | 缺 `spark:app:read` scope，按提示授权后重试 |
| 403 权限不足 | 当前登录用户对该创意页没有 view 权限。**能打开分享链接不等于已被授予协作者身份**，找所有者加协作者 |
| 404 | token 不对。确认传的是链接最后一段，且该页仍可访问 |
| 413 | 目录条目过多（单目录上限 1000）。收窄 `--path` 到子目录 |
| 422 | 应用或目录 entry 不受源码接口支持 |
| 429 | 触发限流，按指数退避重试 |

`--page-size` 超出 1..100 在本地直接报参数错误，不会发请求。
