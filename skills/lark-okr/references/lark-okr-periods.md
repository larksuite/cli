
# okr +periods

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

列出 OKR 周期。用于获取 `period_id`，作为其他 OKR 命令的前置查询。只读操作。

需要的 scopes: ["okr:okr.period:readonly"]

## 命令

```bash
# 列出 OKR 周期
lark-cli okr +periods

# 指定分页大小
lark-cli okr +periods --page-size 20

# 预览 API 调用
lark-cli okr +periods --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--page-token <token>` | 否 | 分页 token，用于翻页 |
| `--page-size <n>` | 否 | 每页数量（默认 10） |
| `--format` | 否 | 输出格式：json（默认）\| pretty |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 输出说明

- pretty 格式下，当前活跃周期会标注 `[current]`。
- 输出包含周期名称、ID、起止时间。

## 工作流

1. 运行 `lark-cli okr +periods` 列出所有可用周期。
2. 找到目标周期的 `id`（当前活跃周期标有 `[current]`）。
3. 将周期 ID 传入 `+list --period-id "..."` 等命令。

## 参考

- [lark-okr](../SKILL.md) — 所有 OKR 命令
- [lark-shared](../../lark-shared/SKILL.md) — 认证与全局参数
