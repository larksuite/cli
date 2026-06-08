# markdown +patch

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

对 Drive 中已有的原生 Markdown 文件做局部文本替换，并返回是否实际写入了新版本。

## 命令

```bash
# 字面量替换
lark-cli markdown +patch \
  --file-token boxcnxxxx \
  --pattern 'hello markdown' \
  --content 'hello patched'

# 正则替换（RE2）
lark-cli markdown +patch \
  --file-token boxcnxxxx \
  --regex \
  --pattern 'hello (.+)' \
  --content 'hi $1'

# 正则 pattern 含特殊字符时要显式转义
lark-cli markdown +patch \
  --file-token boxcnxxxx \
  --regex \
  --pattern 'version \\(1\\.0\\)' \
  --content 'version (2.0)'

# 删除匹配内容
lark-cli markdown +patch \
  --file-token boxcnxxxx \
  --pattern ' debug' \
  --content ''

# --pattern / --content 也支持 @file
lark-cli markdown +patch \
  --file-token boxcnxxxx \
  --pattern @./pattern.txt \
  --content @./replacement.md

# 从 stdin 读取 replacement
printf 'hi patched\n' | \
  lark-cli markdown +patch \
    --file-token boxcnxxxx \
    --pattern 'hello markdown' \
    --content -

# 预览底层编排
lark-cli markdown +patch \
  --file-token boxcnxxxx \
  --pattern 'hello markdown' \
  --content 'hello patched' \
  --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--file-token` | 是 | 目标 Markdown 文件 token |
| `--pattern` | 是 | 要匹配的文本；默认按字面量处理；支持直接传字符串、`@file`、`-`（stdin） |
| `--content` | 是 | 替换后的内容；支持直接传字符串、`@file`、`-`（stdin）；允许空字符串 `''`，表示删除匹配内容 |
| `--regex` | 否 | 将 `--pattern` 按 Go RE2 正则解释；`--content` 支持 `$1` 这类分组替换；如果需要字面 `$`，请写成 `$$` |

## 关键约束

- 当前只支持**单组** `--pattern` / `--content`
- `--pattern` 必须显式传入且不能为空字符串
- `--content` 必须显式传入，但允许为空字符串
- 未加 `--regex` 时，行为等价于对整份 Markdown 文本执行 `strings.ReplaceAll`
- 加了 `--regex` 时，行为等价于对整份 Markdown 文本执行 RE2 全量替换；`--content` 里的 `$1`、`${name}` 会按 Go regexp replacement template 解释，字面 `$` 请写成 `$$`
- 替换后的最终 Markdown 不能为空；如果 patch 结果是空字符串，CLI 会直接报错，不会上传空文件，因为 Drive 不支持零字节 Markdown，且空文件通常是误操作
- `0` 命中时命令仍然成功返回，但不会上传新版本

## Good / Bad

```bash
# BAD: pattern 含正则特殊字符但未转义，容易匹配错误位置
lark-cli markdown +patch \
  --file-token boxcnxxxx \
  --regex \
  --pattern 'version (1.0)' \
  --content 'version (2.0)'

# GOOD: 显式转义括号和点号
lark-cli markdown +patch \
  --file-token boxcnxxxx \
  --regex \
  --pattern 'version \\(1\\.0\\)' \
  --content 'version (2.0)'
```

## 实现边界

- 该命令的内部语义是：**download -> local replace -> overwrite upload**
- 它不是服务端原子 patch；如果有人在你下载后、上传前更新了同一文件，本次 patch 仍可能覆盖那次中间修改
- 它不会返回详细匹配位置，只返回命中数量
- `--dry-run` 会同时展示两种可能的上传路径：`upload_all`（小文件）和 `upload_prepare/upload_part/upload_finish`（大文件分片上传）

## 返回值

命中并写入新版本：

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "updated": true,
    "mode": "literal",
    "match_count": 1,
    "version": "7639217385152646325",
    "size_bytes_before": 39,
    "size_bytes_after": 41
  }
}
```

未命中：

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "updated": false,
    "mode": "literal",
    "match_count": 0,
    "version": "",
    "size_bytes_before": 41,
    "size_bytes_after": 41
  }
}
```

其中：

- `updated` 表示本次是否真的上传了新版本
- `mode` 为 `literal` 或 `regex`
- `match_count` 是匹配次数
- `version` 只有在 `updated=true` 时才会有值
- `size_bytes_before` / `size_bytes_after` 分别是替换前后的 Markdown 大小

## 适用场景

- 只需要替换一小段 Markdown 文本，而不想自己手动 `fetch -> edit -> overwrite`
- 需要基于正则做简单批量替换
- 需要判断“这次是否真的改到了内容”

## 不适用场景

- 需要 rename / move / delete / permission / comment 管理：切到 [`lark-drive`](../../lark-drive/SKILL.md)
- 需要多组 patch 一次完成：当前不支持，改为多次调用 `markdown +patch`
- 需要真正原子更新：当前能力不提供

## 参考

- [lark-markdown](../SKILL.md) — Markdown 域总览
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
