
# okr +list

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

查看用户的 OKR 列表。默认查看当前登录用户在当前活跃周期的 OKR。只读操作。

需要的 scopes: ["okr:okr:readonly"]

> **⚠️ 注意：** 此 API 仅支持 user 身份调用。**不可使用 bot 身份，否则调用将失败。**

## 命令

```bash
# 查看我的 OKR（默认当前用户 + 自动检测当前周期）
lark-cli okr +list

# 指定用户
lark-cli okr +list --user-id "ou_xxx"

# 指定周期
lark-cli okr +list --period-id "period_xxx"

# 指定用户 + 周期
lark-cli okr +list --user-id "ou_xxx" --period-id "period_xxx"

# 预览 API 调用，不执行
lark-cli okr +list --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--user-id <id>` | 否 | 用户 `open_id`，省略则使用当前登录用户 |
| `--period-id <id>` | 否 | OKR 周期 ID。省略则自动检测当前活跃周期。可通过 `+periods` 获取可用周期列表 |
| `--lang <lang>` | 否 | 语言：`zh_cn`（默认）或 `en_us` |
| `--offset <n>` | 否 | 分页偏移（默认 0） |
| `--limit <n>` | 否 | 每页数量（默认 10，最大 10） |
| `--format` | 否 | 输出格式：json（默认）\| pretty |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 工作流

1. 如果用户想看"我的 OKR"，直接运行 `lark-cli okr +list` 即可（无需 `--user-id`）。
2. 如果用户想看特定周期的 OKR，先运行 `lark-cli okr +periods` 获取周期 ID，再传入 `--period-id`。
3. 如果用户想看他人的 OKR，需获取对方的 `open_id`（例如通过 `lark-cli contact +get-user`），再传入 `--user-id`。
4. 展示结果：列出各 Objective 及其 Key Result，附带进度百分比。

## 参考

- [lark-okr](../SKILL.md) — 所有 OKR 命令
- [lark-shared](../../lark-shared/SKILL.md) — 认证与全局参数
