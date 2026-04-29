# markdown +create

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

在 Drive 中创建一个原生 Markdown 文件（`.md`）。

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

# 预览底层请求
lark-cli markdown +create \
  --name README.md \
  --content '# Hello' \
  --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--folder-token` | 否 | 目标 Drive 文件夹 token；省略时创建到根目录 |
| `--name` | 条件必填 | 文件名，**必须显式带 `.md` 后缀**；使用 `--content` 时必填；使用 `--file` 时可省略，默认取本地文件名 |
| `--content` | 条件必填 | Markdown 内容；与 `--file` 互斥；支持直接传字符串、`@file`、`-`（stdin） |
| `--file` | 条件必填 | 本地 `.md` 文件路径；与 `--content` 互斥 |

## 关键约束

- `--content` 与 `--file` 必须二选一
- `--name` 必须带 `.md` 后缀
- `--file` 指向的本地文件名也必须带 `.md` 后缀

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

当以 `--as bot` 创建且自动为当前 CLI 用户授予可管理权限时，结果还会额外带上 `permission_grant`。

## 参考

- [lark-markdown](../SKILL.md) — Markdown 域总览
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
