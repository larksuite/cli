# markdown +create

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

在 Drive 中创建一个原生 Markdown 文件（`.md`），支持创建到普通 Drive 文件夹或 Wiki 节点下。

## 命令

```bash
# 直接用行内内容创建
lark-cli markdown +create \
  --name README.md \
  --content '# Hello'

# 从本地 .md 文件创建
lark-cli markdown +create \
  --file ./README.md

# 从本地文件读取内容，但仍走 --content
lark-cli markdown +create \
  --name README.md \
  --content @./README.md

# 从 stdin 读取内容
printf '# Hello\n\nfrom stdin\n' | \
  lark-cli markdown +create \
    --name README.md \
    --content -

# 创建到指定文件夹
lark-cli markdown +create \
  --folder-token fldcn_xxx \
  --file ./README.md

# 创建到指定文件夹（可直接传 Drive folder URL）
lark-cli markdown +create \
  --folder-token "https://feishu.cn/drive/folder/fldcn_xxx" \
  --file ./README.md

# 创建到指定 wiki 节点
lark-cli markdown +create \
  --wiki-token wikcn_xxx \
  --file ./README.md

# 创建到指定 wiki 节点（可直接传 wiki URL）
lark-cli markdown +create \
  --wiki-token "https://feishu.cn/wiki/wikcn_xxx" \
  --file ./README.md

# 预览底层请求
lark-cli markdown +create \
  --name README.md \
  --content '# Hello' \
  --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--folder-token` | 否 | 目标 Drive 文件夹 token 或 Drive folder URL；与 `--wiki-token` 互斥；省略时创建到根目录 |
| `--wiki-token` | 否 | 目标 wiki 节点 token 或 wiki URL；与 `--folder-token` 互斥；传入后自动映射为 `parent_type=wiki` |
| `--name` | 条件必填 | 文件名，**必须显式带 `.md` 后缀**；使用 `--content` 时必填；使用 `--file` 时可省略，默认取本地文件名 |
| `--content` | 条件必填 | Markdown 内容；与 `--file` 互斥；支持直接传字符串、`@file`、`-`（stdin） |
| `--file` | 条件必填 | 本地 `.md` 文件路径；与 `--content` 互斥 |

## 关键约束

- `--content` 与 `--file` 必须二选一
- `--folder-token` 与 `--wiki-token` 互斥
- `--folder-token` 只能是 Drive 文件夹；不要传 wiki/doc/sheet/base/file token 或 URL
- `--wiki-token` 只能是 Wiki 节点；如果只有 docx/sheet/base 等文档 URL，先用 `lark-cli wiki +node-get --node-token <url>` 解析出 `node_token`
- `--name` 必须带 `.md` 后缀
- `--file` 指向的本地文件名也必须带 `.md` 后缀
- 传 `--wiki-token` 时，返回值中不会附带 `/file/<token>` URL，因为 wiki 承载文件没有稳定的独立 file URL

## 返回值

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "file_token": "boxcnxxxx",
    "file_name": "README.md",
    "size_bytes": 1234
  }
}
```

> [!IMPORTANT]
> 如果 Markdown 文件是**以应用身份（bot）创建**的，如 `lark-cli markdown +create --as bot`，在创建成功后，CLI 会**尝试为当前 CLI 用户自动授予该文件的 `full_access`（可管理权限）**。
>
> 以应用身份创建时，结果里会额外返回 `permission_grant` 字段，明确说明授权结果：
> - `status = granted`：当前 CLI 用户已获得该文件的可管理权限
> - `status = skipped`：本地没有可用的当前用户 `open_id`，因此不会自动授权；可提示用户先完成 `lark-cli auth login`，再让 AI / agent 继续使用应用身份（bot）授予当前用户权限
> - `status = failed`：Markdown 文件已创建成功，但自动授权用户失败；会带上失败原因，并提示稍后重试或继续使用 bot 身份处理该文件
>
> `permission_grant.perm = full_access` 表示该资源已授予“可管理权限”。
>
> **不要擅自执行 owner 转移。** 如果用户需要把 owner 转给自己，必须单独确认。

## 失败处理

- `not_found` / `1061044`：父目录或 wiki 节点不存在，或 token 类型放错参数。修正 `--folder-token` / `--wiki-token` 后再试，不要重复提交同一参数。
- `quota_exceeded` / `1061101`：目标存储空间配额已满。释放空间、换父目录/节点或请管理员扩容后再试。
- `permission_denied` / `missing_scope`：区分身份处理。`--as user` 看用户授权和目标 ACL；`--as bot` 看应用 scope 与目标目录/节点 ACL。
- `rate_limit`：停止立即重试，使用退避。
- `server_error` / `233523001`：可以稍后有限重试；若重复出现，保留 `log_id` / request id 给服务端排查。

## 参考

- [lark-markdown](../SKILL.md) — Markdown 域总览
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
