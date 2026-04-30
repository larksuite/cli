# markdown +overwrite

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

覆盖更新 Drive 中已有的原生 Markdown 文件，并返回覆盖后的新版本号。

## 命令

```bash
# 用行内内容覆盖
lark-cli markdown +overwrite \
  --file-token boxcnxxxx \
  --content '# Updated'

# 用本地 .md 文件覆盖
lark-cli markdown +overwrite \
  --file-token boxcnxxxx \
  --file ./README.md

# 覆盖内容时顺便显式指定新文件名
lark-cli markdown +overwrite \
  --file-token boxcnxxxx \
  --name NEW-README.md \
  --content '# Updated'

# 用 --content 从本地文件读取
lark-cli markdown +overwrite \
  --file-token boxcnxxxx \
  --content @./README.md

# 用 stdin 覆盖
printf '# Updated\n' | \
  lark-cli markdown +overwrite \
    --file-token boxcnxxxx \
    --content -

# 预览底层请求
lark-cli markdown +overwrite \
  --file-token boxcnxxxx \
  --content '# Updated' \
  --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--file-token` | 是 | 目标 Markdown 文件 token |
| `--name` | 否 | 显式指定覆盖后的文件名；必须带 `.md` 后缀。传入时优先使用它 |
| `--content` | 条件必填 | 新 Markdown 内容；与 `--file` 互斥；支持直接传字符串、`@file`、`-`（stdin） |
| `--file` | 条件必填 | 本地 `.md` 文件路径；与 `--content` 互斥 |

## 关键约束

- `--content` 与 `--file` 必须二选一
- 如果传了 `--name`，直接使用它作为覆盖后的文件名
- 如果没传 `--name` 且使用 `--content`，默认保留远端原文件名
- 如果没传 `--name` 且使用 `--file`，默认使用本地文件名
- `--file` 指向的本地文件名必须带 `.md` 后缀
- 覆盖成功后 **必须** 返回 `version`

## 返回值

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "file_token": "boxcnxxxx",
    "file_name": "README.md",
    "version": "7633658129540910621",
    "size_bytes": 2048
  }
}
```

其中：

- `version` 是覆盖写入后的新版本号
- `size_bytes` 是本次覆盖后的内容大小

## 参考

- [lark-markdown](../SKILL.md) — Markdown 域总览
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
