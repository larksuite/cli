
# okr +get

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

按 OKR ID 批量获取 OKR 详情，包含 Objective、Key Result 及进度信息。只读操作。

需要的 scopes: ["okr:okr:readonly"]

## 命令

```bash
# 获取单个 OKR 详情
lark-cli okr +get --okr-ids "okr_xxx"

# 批量获取多个 OKR（最多 10 个，逗号分隔）
lark-cli okr +get --okr-ids "okr_xxx,okr_yyy"

# 预览 API 调用
lark-cli okr +get --okr-ids "okr_xxx" --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--okr-ids <ids>` | 是 | 逗号分隔的 OKR ID（最多 10 个） |
| `--lang <lang>` | 否 | 语言：`zh_cn`（默认）或 `en_us` |
| `--format` | 否 | 输出格式：json（默认）\| pretty |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 工作流

1. 从 `+list` 的输出中获取 OKR ID，或由用户提供。
2. 执行 `lark-cli okr +get --okr-ids "..."`。
3. 展示完整的 OKR 结构：各 Objective 内容、进度、嵌套的 Key Result 及其进度。

## 参考

- [lark-okr](../SKILL.md) — 所有 OKR 命令
- [lark-shared](../../lark-shared/SKILL.md) — 认证与全局参数
