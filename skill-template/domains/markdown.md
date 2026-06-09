## Markdown

**身份：Markdown 文件通常属于用户云空间资源，优先使用 `--as user`。首次以 `user` 身份访问前执行 `lark-cli auth login`。**

## 快速决策

- 用户要**上传、创建一个原生 `.md` 文件**，使用 `lark-cli markdown +create`
- 用户要**比较原生 `.md` 文件的历史版本差异**，或比较远端 Markdown 与本地草稿，使用 `lark-cli markdown +diff`
- 用户要**读取 Drive 里某个 `.md` 文件内容**，使用 `lark-cli markdown +fetch`
- 用户要对 Markdown 文件做**局部文本替换 / 正则替换**，优先使用 `lark-cli markdown +patch`
- 用户要**覆盖更新 Drive 里某个 `.md` 文件内容**，使用 `lark-cli markdown +overwrite`
- 用户要先拿 Markdown 文件的历史版本号，再做比较、下载或回滚，先用 [`lark-drive`](../../skills/lark-drive/SKILL.md) 的 `lark-cli drive +version-history`
- 用户要把本地 Markdown **导入成在线新版文档（docx）**，不要用本域，改用 [`lark-drive`](../../skills/lark-drive/SKILL.md) 的 `lark-cli drive +import --type docx`
- 用户要对 Markdown 文件做**rename / move / delete / 搜索 / 权限 / 评论**等云空间操作，不要留在本域，切到 [`lark-drive`](../../skills/lark-drive/SKILL.md)

## 核心边界

- 本域处理的是 **Drive 中作为普通文件存储的 Markdown**，不是 docx 文档
- `markdown +patch` 的语义是：**先完整下载 Markdown，再本地替换，再整文件覆盖上传**
- `markdown +patch` 不是服务端原子 patch；它是 CLI 侧编排出来的局部更新能力
- `markdown +patch` 当前只支持**单组** `--pattern` / `--content`
- `markdown +patch` 替换后的最终内容**不能为空**；CLI 会拒绝上传空文件，因为 Drive 不支持零字节 Markdown，且空文件通常是误操作

## 不在本域范围

- 将 Markdown 导入为飞书在线文档 → [`lark-drive`](../../skills/lark-drive/SKILL.md)
- 文件搜索、权限、评论、移动、删除等云空间管理 → [`lark-drive`](../../skills/lark-drive/SKILL.md)
