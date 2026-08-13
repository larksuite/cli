# agents list

> **前置条件：** 先读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)（认证、身份、安全规则）。

发现层命令。无参数时列出已注册的 provider 及其元数据，**不调用任何 API**；带 scheme 时枚举该 provider 下的 agent 实例（catalog 型必可枚举；instance 型是否支持见 provider 文件）。只读。

## 命令

```bash
# 列 provider（默认 JSON 信封）
lark-cli agents list

# 二级发现：枚举某 provider 下的 agent
lark-cli agents list <scheme>

# 人类可读（带表头 TSV）
lark-cli agents list --format pretty
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `[scheme]` | 否 | 省略=列 provider；给定=枚举该 provider 下的 agent |
| `--format json\|pretty` | 否 | 默认 `json`；`pretty` 为带表头 TSV |
| `--jq` | 否 | jq 过滤（强制 JSON） |

## 输出（`agents list`）

`data.providers[]` 每个已注册 provider 一条。示例（base，真实输出；完整 provider 清单见 [SKILL.md「Provider 目录」](../SKILL.md)）：

```json
{
  "ok": true,
  "data": {
    "providers": [
      {
        "scheme": "base",
        "label": "Base Assistant",
        "agent_ref_format": "base:<agent_id>",
        "kind": "catalog",
        "agent_id_source": "Use the fixed agent reference base:assistant"
      }
    ]
  },
  "meta": { "count": 1 }
}
```

字段消费方式：

- **`agent_ref_format`**：告诉用户 agent_ref 怎么写（`<provider>:<agent_id>`，`<agent_id>` 整体替换）。
- **`agent_id_source`**：拿 agent_id 的路径文案，用户没有 agent_id 时照这个引导。
- **`kind`**：`catalog` = ref 指向目录内条目，**必可枚举**（`agents list <scheme>` 注册期强制支持）；`instance` = ref 指向一个具体 agent 实例，能否枚举取决于服务端 List API（见 provider 文件）。

## 二级发现（`agents list <scheme>`）

- provider 支持枚举（catalog 型必支持）→ 返回 `{"agents": [{agent_ref, name, description?}]}`。**catalog 型**（离线有限集，不分页）条数在 `meta.count`，用 `.meta.count // 0` 消费；**instance 型**走服务端分页，条数在 `meta.pagination.items`（见下方「分页」）。空列表且无下一页时整个 `meta` 省略。示例（base，catalog 型，真实输出）：

```json
{
  "ok": true,
  "data": {
    "agents": [
      {
        "agent_ref": "base:assistant",
        "name": "Base Assistant",
        "description": "Handles multi-component Base construction and restructuring, plus user-facing data retrieval and analysis. Use Base CLI shortcuts for a single atomic edit or record create, update, or delete."
      }
    ]
  },
  "meta": { "count": 1 }
}
```

- provider 不支持枚举（部分 instance 型）→ 本地报错 `unsupported_capability`（exit 2），message 为 `provider '<scheme>' does not support listing agents`，hint 直接给出该 provider 的 agent_id 获取路径（即 `agent_id_source` 文案）——别编清单、别重试，把 hint 原样转达用户。

**分页（仅 instance 型枚举）**：instance 型的 `agents list <scheme>` 走服务端 List API，支持 `--page-size N`（1-100，默认 20）+ `--page-token <token>`；响应带 `meta.pagination`（`complete` / `next_token` / `items`）和 `meta.next` 翻页命令（照 `meta.next` 执行即可）。**catalog 型（如 base）是离线有限集，不分页**，`--page-size` / `--page-token` 在该路径被忽略。

## 错误目录

| 触发 | subtype | exit | message / hint（真实输出） |
|---|---|---|---|
| 未知 scheme（如 `agents list nosuch`） | invalid_argument | 2 | message 形如 `unknown agent provider 'nosuch', currently registered: <已注册 scheme 全集>`（列表随注册变化，勿硬编码断言）；hint `run lark-cli agents list to see the available providers` |
| `agents list <scheme>`（该 provider 不支持枚举） | unsupported_capability | 2 | 见上方「二级发现」说明 |

## `agents list <scheme>` 的业务参数

- `--param key=value`（可重复）：**仅在带 scheme 时有意义**；按该 provider 声明的 `list_parameters` 校验（在无 scheme 的 `agents list` 输出 `providers[]` 里查看——list 时你手上还没有 agent_ref，参数发现面就在这里；`list_parameters` 是 omitempty 字段，只有声明了 list 参数的 provider 才带，上方示例里 base 没有该字段即零参数）。无 scheme 带 `--param` 报 `invalid_argument`；catalog 型 provider 的枚举是纯离线操作、不接受任何 `--param`。
- 参数错误一次报全（`params[]` 每条带原因），hint 指向 `providers[].list_parameters`。

## 参考

- [lark-agents](../SKILL.md) — agent 全部动词
