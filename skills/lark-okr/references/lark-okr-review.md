
# okr +review

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

查询指定用户在指定周期的 OKR 复盘信息，包含周期复盘链接和进展报告链接。只读操作。

需要的 scopes: ["okr:okr.review:readonly"]

## 命令

```bash
# 查询单个用户的复盘
lark-cli okr +review --user-ids "ou_xxx" --period-id "period_xxx"

# 查询多个用户的复盘（最多 5 个，逗号分隔）
lark-cli okr +review --user-ids "ou_xxx,ou_yyy" --period-id "period_xxx"

# 预览 API 调用
lark-cli okr +review --user-ids "ou_xxx" --period-id "period_xxx" --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--user-ids <ids>` | 是 | 逗号分隔的用户 `open_id`（最多 5 个） |
| `--period-id <id>` | 是 | OKR 周期 ID。可通过 `+periods` 获取 |
| `--format` | 否 | 输出格式：json（默认）\| pretty |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 工作流

1. 获取用户 ID：通过 `lark-cli contact +get-user` 或由用户提供。
2. 获取周期 ID：通过 `lark-cli okr +periods` 或由用户提供。
3. 执行 `lark-cli okr +review --user-ids "..." --period-id "..."`。
4. 展示复盘信息，包括周期复盘 URL 和进展报告 URL。

## 参考

- [lark-okr](../SKILL.md) — 所有 OKR 命令
- [lark-shared](../../lark-shared/SKILL.md) — 认证与全局参数
