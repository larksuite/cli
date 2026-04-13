
# okr +progress-get

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

按 ID 获取进展记录详情，包含进展内容、目标信息、修改时间。只读操作。

需要的 scopes: ["okr:okr.progress:readonly"]

## 命令

```bash
# 获取进展记录详情
lark-cli okr +progress-get --progress-id "prog_xxx"

# 预览 API 调用
lark-cli okr +progress-get --progress-id "prog_xxx" --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--progress-id <id>` | 是 | 进展记录 ID |
| `--format` | 否 | 输出格式：json（默认）\| pretty |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 工作流

1. 从 `+get` 输出中获取进展记录 ID（各 Objective/KR 包含 `progress_record_list`）。
2. 执行 `lark-cli okr +progress-get --progress-id "..."`。
3. 展示进展内容、目标信息、修改时间。

## 参考

- [lark-okr](../SKILL.md) — 所有 OKR 命令
- [lark-shared](../../lark-shared/SKILL.md) — 认证与全局参数
