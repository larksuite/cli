
# okr +progress-add

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

为 OKR 的 Objective 或 Key Result 添加进展记录。写操作。

需要的 scopes: ["okr:okr.progress:writeonly"]

## 命令

```bash
# 为 Key Result 添加纯文本进展
lark-cli okr +progress-add \
  --target-id "kr_xxx" \
  --target-type key_result \
  --text "本周完成了 80% 的开发工作"

# 为 Objective 添加进展
lark-cli okr +progress-add \
  --target-id "obj_xxx" \
  --target-type objective \
  --text "各子项目均按计划推进"

# 使用富文本 JSON（从文件读取）
lark-cli okr +progress-add \
  --target-id "kr_xxx" \
  --target-type key_result \
  --content @progress.json

# 预览 API 调用
lark-cli okr +progress-add \
  --target-id "kr_xxx" \
  --target-type key_result \
  --text "测试" \
  --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--target-id <id>` | 是 | 目标 Objective 或 Key Result 的 ID |
| `--target-type <type>` | 是 | 目标类型：`objective` 或 `key_result`（也兼容 `2`/`3`） |
| `--text <text>` | 否* | 纯文本进展内容（自动转换为飞书富文本格式） |
| `--content <json>` | 否* | 富文本 JSON 内容。支持 `@file`（从文件读取）和 `-`（从 stdin 读取） |
| `--source-title <title>` | 否 | 来源标题（默认 "lark-cli"） |
| `--source-url <url>` | 否 | 来源 URL（默认 "https://github.com/larksuite/cli"），必须以 http/https 开头 |
| `--data <json>` | 否 | 完整 JSON payload（覆盖其他参数） |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

\* `--text`、`--content`、`--data` 三者至少提供一个。

## 工作流

1. 获取目标 ID：使用 `+list` 或 `+get` 获取 Objective 或 Key Result 的 ID。
2. 与用户确认：目标、进展内容、目标类型。
3. 执行 `lark-cli okr +progress-add --target-id "..." --target-type key_result --text "..."`。
4. 展示结果：progress record ID。

> [!CAUTION]
> 这是一个**写操作** — 执行前必须确认用户意图。

## 参考

- [lark-okr](../SKILL.md) — 所有 OKR 命令
- [lark-shared](../../lark-shared/SKILL.md) — 认证与全局参数
