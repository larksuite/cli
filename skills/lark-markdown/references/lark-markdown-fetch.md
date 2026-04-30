# markdown +fetch

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

读取 Drive 中原生 Markdown 文件的内容；也支持把内容保存到本地。

## 命令

```bash
# 直接返回 Markdown 文本
lark-cli markdown +fetch --file-token boxcnxxxx

# 保存到本地
lark-cli markdown +fetch \
  --file-token boxcnxxxx \
  --output ./README.md

# 传目录时，使用远端文件名保存到该目录下
lark-cli markdown +fetch \
  --file-token boxcnxxxx \
  --output ./downloads/

# 覆盖已存在文件
lark-cli markdown +fetch \
  --file-token boxcnxxxx \
  --output ./README.md \
  --overwrite

# 预览底层请求
lark-cli markdown +fetch \
  --file-token boxcnxxxx \
  --output ./README.md \
  --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--file-token` | 是 | 目标 Markdown 文件 token |
| `--output` | 否 | 本地保存路径；既可传具体文件名，也可传目录路径。传目录时使用远端文件名保存；省略时直接返回 Markdown 内容 |
| `--overwrite` | 否 | 覆盖已存在的本地输出文件；仅在传入 `--output` 时生效 |

## 返回值

不传 `--output`：

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "file_token": "boxcnxxxx",
    "file_name": "README.md",
    "content": "# Hello\n",
    "size_bytes": 8
  }
}
```

传入 `--output`：

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "file_token": "boxcnxxxx",
    "file_name": "README.md",
    "saved_path": "/abs/path/README.md",
    "size_bytes": 8
  }
}
```

## 参考

- [lark-markdown](../SKILL.md) — Markdown 域总览
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
