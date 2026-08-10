# base +field-create

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

创建一个或多个字段；同一表的多个字段默认使用一次 JSON 数组输入。预计串行运行时间超过 caller/tool timeout 时按时间预算拆分，不按固定条数切块。

## Agent 最小工作流

1. 先判断是不是 `formula` / `lookup`。
2. 如果是：先读对应 guide。
3. 没读 guide 前，不要直接创建 formula / lookup 字段。
4. 读完 guide 后，再构造 `--json` 并创建字段。
5. 如果是跨表 formula / lookup，再补查**目标表**的结构。

## 推荐命令

```bash
lark-cli base +field-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --json '{"name":"预算","type":"number","style":{"type":"plain","precision":2}}'

lark-cli base +field-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --json '{"name":"状态","type":"select","multiple":false,"default_value":["Todo"],"options":[{"name":"Todo","hue":"Blue","lightness":"Lighter"},{"name":"Done","hue":"Green","lightness":"Light"}]}'

lark-cli base +field-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --json '{"name":"负责人","type":"user","multiple":false,"default_value":[{"$slot":"current_user"}],"description":"用于标记记录的直接负责人；协作约定可参考[团队字段约定](https://example.com/field-spec)"}'

# 多个字段复用相同字段 JSON 形状，一次传非空数组
lark-cli base +field-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --json '[{"name":"备注","type":"text"},{"name":"优先级","type":"select","multiple":false,"options":[{"name":"高"},{"name":"低"}]}]'
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token |
| `--table-id <id_or_name>` | 是 | 表 ID 或表名 |
| `--json <body>` | 是 | 单个字段 JSON 对象，或多个字段对象组成的非空数组 |

## API 入参详情

**HTTP 方法和路径：**

```
POST /open-apis/base/v3/bases/:base_token/tables/:table_id/fields
```

## JSON 值规范

- `--json` 接受单个字段 **JSON 对象**，也接受多个字段对象组成的非空数组；不要再套 `fields` 等外层对象。
- 数组按顺序创建字段，遇到首个失败即停止且不自动回滚已创建字段；需要原子写入时不要假设数组具备事务语义。
- 每个字段对象最少包含：`name`、`type`。
- 所有字段类型都支持可选 `description`；支持纯文本，也支持 Markdown 链接，如 `协作约定可参考[团队字段约定](https://example.com/field-spec)`。
- 需要字段默认值时传 `default_value`，直接使用字段对应 CellValue；`datetime` / `user` 的动态填充用 `$slot`。完整规则见 [lark-base-field-json.md](lark-base-field-json.md)。
- `type` 不同，必填子字段不同：
  - `select`：`multiple` 控制是否多选，`options` 定义静态选项，`dynamic_options_source` 定义动态选项来源。静态与动态选项配置二选一，不能同时传。
  - `link`：必须有 `link_table`，可选 `bidirectional`、`bidirectional_link_field_name`。
  - `formula`：必须有 `expression`；先读 formula guide，再创建。
  - `lookup`：必须有 `from`、`select`、`where`；先读 lookup guide，再创建。

**正确（base +field-create）**

```json
{
  "name": "状态",
  "type": "select",
  "multiple": false,
  "default_value": ["Todo"],
  "options": [
    { "name": "Todo", "hue": "Blue", "lightness": "Lighter" },
    { "name": "Done", "hue": "Green", "lightness": "Light" }
  ]
}
```

**字段说明示例**

```json
{
  "name": "负责人",
  "type": "user",
  "multiple": false,
  "description": "用于标记记录的直接负责人；协作约定可参考[团队字段约定](https://example.com/field-spec)"
}
```

## 返回重点

- 单字段返回 `field` 和 `created: true`；多字段完整返回服务端 `fields`、`total` 和 `created: true`。
- 大数组成功时若不需要逐字段 ID，可追加 `--jq 'if .ok then (.data | {created,total,field_get_recommended,next_step,verification_hint}) else . end'` 控制 stdout 大小；失败分支仍保留完整部分失败明细。需要逐字段 ID 时不要使用该投影。
- 数组部分失败返回 `ok:false`、`summary` 和有序 `items`，保留已创建字段及 ID、失败项和未执行项。`failed` 项保留 `type`、`subtype`、`code`、`hint`、`retryable`、`log_id`、`troubleshooter`，以及原 typed error 已有的扩展字段，例如权限错误的 `missing_scopes`、`identity`、`console_url` 或安全策略错误的 `challenge_url`；扩展键与部分失败账本的 `index`、`status`、`field`、`error` 冲突时，以带 `error_` 前缀的无冲突别名输出（例如 `field` → `error_field`）。
- 部分失败统一返回 `next_step:"inspect_items"`；`field_get_recommended` 仅表示已创建字段是否建议读回。`retryable:true` 只表示该 `failed` 项可原样自动重试；否则先按该项 `hint` 完成授权或修正输入，再重新提交该项。`not_attempted` 项应单独继续。
- 调用方超时且未收到命令终态输出时，不要重投整个数组；先按本次提交的字段名定向读回，再只提交缺失项。没有写前快照时，读回命中的同名项只能标记为 `ambiguous`，不得计作本轮 `created`。
- 完整成功且返回 `field_get_recommended:false`、`next_step:"done"` 时直接结束；除非用户明确要求读回或额外属性，否则不要再执行 `+field-list/get`。确需核验时用 `--jq` 过滤 `+field-list`，不要把全部字段打印进上下文。
- `field_get_recommended:true` 表示完成当前 `next_step` 后按 `verification_hint` 读回；完整成功时 `next_step:"field_get"` 表示可直接读回。`formula`、`lookup`、`link`、`auto_number` 等字段更适合读回确认服务端最终结构。

## 工作流

1. formula / lookup 字段必须先阅读对应指南；没读之前不要直接创建。
2. 创建简单字段时，优先相信命令返回；只有用户要求精确核对额外属性，或返回建议读回时，才继续执行 `+field-get`。

## 坑点

- ⚠️ 这是写入操作，执行前必须确认。
- ⚠️ 当 `type` 是 `formula` 或 `lookup` 时，先读对应 guide，再创建。
- ⚠️ 不要把“每次创建后都 `+field-get`”当作固定流程；按返回里的 `field_get_recommended` 和 `next_step` 决定是否读回。

## 参考

- [lark-base-field-json.md](lark-base-field-json.md) — 字段 JSON 规范（推荐）
- [formula-field-guide.md](formula-field-guide.md) — formula 指南（创建公式必读）
- [lookup-field-guide.md](lookup-field-guide.md) — lookup 指南（创建查找引用必读）
