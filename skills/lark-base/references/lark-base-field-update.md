# base +field-update

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

更新一个已有字段。

## 推荐命令

```bash
lark-cli base +field-update \
  --base-token <base_token> \
  --table-id <table_id> \
  --field-id <field_id> \
  --json '{"name":"状态","type":"select","multiple":false,"default_value":["Doing"],"options":[{"name":"Todo","hue":"Blue","lightness":"Lighter"},{"name":"Doing","hue":"Orange","lightness":"Light"},{"name":"Done","hue":"Green","lightness":"Light"}]}' \
  --yes

lark-cli base +field-update \
  --base-token <base_token> \
  --table-id <table_id> \
  --field-id <field_id> \
  --json '{"name":"负责人","type":"user","multiple":false,"default_value":null,"description":"用于标记记录的直接负责人"}' \
  --yes

lark-cli base +field-update \
  --base-token <base_token> \
  --table-id <table_id> \
  --field-id <field_id> \
  --json '{"name":"编号","type":"auto_number","style":{"rules":[{"type":"text","text":"TASK-"},{"type":"created_time","date_format":"yyyyMM"},{"type":"text","text":"-"},{"type":"incremental_number","length":4}]}}' \
  --yes
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token |
| `--table-id <id_or_name>` | 是 | 表 ID 或表名 |
| `--field-id <id_or_name>` | 是 | 字段 ID 或字段名 |
| `--json <body>` | 是 | 字段属性 JSON 对象 |
| `--yes` | 是 | 确认执行高风险字段更新 |

> 这是**高风险写入操作**。`+field-update` 使用 `PUT` 全量字段定义语义；改变字段类型或关键配置可能影响整列已有数据的解释、展示或可用性。CLI 层要求显式传 `--yes`；如果用户已经明确目标和期望更新，可直接执行并带上 `--yes`。

## API 入参详情

**HTTP 方法和路径：**

```
PUT /open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id
```

当 `--json.type` 是 `auto_number` 时，仍然走同一个 v3 字段更新接口：更新自动编号规则后，接口现状就会把新规则应用到已有编号（这是接口默认行为，只是 agent 通常不知道），因此**不需要**任何额外开关或参数。只需要正常提交目标自动编号字段定义即可；如果用户要求“将修改用于已有编号”，直接执行这次 `+field-update` 就能达到效果，不要在 `--json` 里额外添加任何参数去“触发”重排。

## JSON 值规范

- `--json` 必须是 **JSON 对象**，顶层直接传字段定义。
- 更新语义是 `PUT`（全量字段配置更新），不要只传零散片段；至少显式包含 `name`、`type`，并补齐该类型所需关键配置。
- 所有字段类型都支持可选 `description`；支持纯文本，也支持 Markdown 链接。
- 需要字段默认值时传 `default_value`，直接使用字段对应 CellValue；传 `null` 清空，省略表示不修改现有默认值。完整规则见 [lark-base-field-json.md](lark-base-field-json.md)。
- `select` 更新时：`options` 仍按对象数组传，避免混入无效字段。
- `link` 更新限制：
  - 不能把非 `link` 字段改成 `link`，也不能把 `link` 改成非 `link`。
  - 现有 `link` 字段的 `bidirectional` 不能改。
- `auto_number` 更新的 `style.rules` 支持 `text`、`created_time`、`incremental_number`。

**推荐更新示例**

```json
{
  "name": "状态",
  "type": "select",
  "multiple": false,
  "default_value": ["Doing"],
  "options": [
    { "name": "Todo", "hue": "Blue", "lightness": "Lighter" },
    { "name": "Doing", "hue": "Orange", "lightness": "Light" },
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
  "description": "用于标记记录的直接负责人"
}
```

## 返回重点

- 返回 `field` 和 `updated: true`。
- `updated:true` 只表示更新请求成功，不表示字段结构、已有记录值或下游能力已经完成验证。`+field-update` 无法知道更新前的字段类型，因此成功响应会推荐执行 `+field-get`；若发生类型转换，还要抽样读取记录值。
- 如果响应中的 `field.type` 与提交的 `type` 不一致，必须把它当作待核验的类型不匹配；不能返回完成态，也不能只根据其中任一类型推断更新成功。
- 如果 API 报告本次更新没有产生任何变更（no-op），命令会如实返回该错误；这通常说明目标字段已是期望状态，不要机械重试同一份 `+field-update`。需要确认当前字段完整状态时执行 `+field-get`。
- 如果返回 `field_get_recommended:true` 或 `next_step:"field_get"`，按提示读回字段；`auto_number` 更新后还应抽样读记录值确认编号已按新规则生成。

## 工作流


1. 建议先用 `+field-get` 拉现状，再做最小化修改。
2. `formula/lookup` 类型更新前先阅读对应指南。
3. 如果更新 `auto_number`，理解为“更新编号规则，同时把新规则应用到已有编号”；执行后按返回提示读回字段并在必要时抽样记录值。
4. 如果这次更新会改变字段 `type` 先按下方“字段类型变更规则”判断能否执行。如果不修改 `type`，大多数场景都相对安全。

## 字段类型变更规则

字段类型变更采用白名单机制：**只允许白名单转换**；未命中白名单时，**不建议用 CLI 转换字段类型** 除非用户明确知道风险并同意。

### 允许直接转换 type

先 `+field-get` / `+field-list` 看结构，再抽样读值；只有命中以下规则时，转换才是比较安全的。

#### 相对安全

| 目标类型 | 允许的源类型 | 说明 |
|------|------|------|
| `text` | `number`、`select`、`datetime`、`created_at`、`updated_at`、`location`（只保留 `full_address`）、`auto_number`、`checkbox` | 保留字符串表示；丢失原类型语义和结构化能力 |
| `number` | `text`、`number`、`datetime`、`created_at`、`updated_at`、`checkbox` | 保留可解析的数字值；无法解析的值会变空，原文本格式会丢失 |
| `datetime` | `text`、`number`、`datetime`、`created_at`、`updated_at` | 保留可解析的时间字符串和时间戳；无法解析的值会变空，原文本格式会丢失 |
| `select` | `text -> select`、`number -> select`、`single select -> multi select` | 只有完全匹配目标选项名的值会转成对应选项；没匹配上的值会被丢弃 |

#### 可执行但会截断 / 重算

- `select(multi) -> select(single)`: 只保留第一个值，其余值会被丢弃。
- `user(multi) -> user(single)`: 只保留第一个人员，其余值会被丢弃。
- `group_chat(multi) -> group_chat(single)`: 只保留第一个群，其余值会被丢弃。

#### 无状态字段可直接转换

- `created_at`、`created_by`、`updated_at`、`updated_by`、`formula`、`lookup`: 这类字段值由系统或计算逻辑生成，不承载独立存储数据；可以执行类型转换，不必担心破坏原始记录值，但仍要做下游读回验证。

### 一律不要用 CLI 转换

以下场景全部视为黑名单；默认要求用户改到 Web 页面手动完成，或改走“新建字段 + 数据迁移”。

- `any -> checkbox`
- `any -> user`
- `any -> group_chat`
- `any -> attachment`
- `any -> location`
- `link` 类型变更
- 任意涉及动态 / 静态选项来源切换的 `select` 类型变更

### 可例外继续执行的场景

只有在**整列数据丢失可接受**时，才允许对黑名单场景例外执行。

- `EmptyColumn`: 该列为空
- `FreshTableInit`: 新建空表初始化
- `PrimaryFieldBootstrap`: 主列不能删，只能更新完成初始化
- `ExplicitLossAccepted`: 用户明确接受整列数据丢失

不满足以上条件时，不要转换。

### 非白名单场景如何处理

- 命中白名单时：建议直接原地转换，再做读回验证。
- 未命中白名单时：先询问用户是否仍要执行转换，并明确说明风险：
  - 无状态字段除外；这类字段可以直接转换
  - 可能整列变空
  - 可能只保留第一个值
  - 可能只保留字符串表示，丢失原类型语义和结构化能力
  - 可能影响视图 / 筛选 / 排序 / 公式 / lookup / 写入引用
- 如果用户不接受风险：不要执行转换。

### 完成态验证

- `FieldReadback`: 读回字段结构，确认 `type` / `multiple` / `style` / `options`
- `NoopReadback`: `+field-update` 返回 no-op 错误时，只能说明 API 报告没有产生变更；可以跳过重复 update，但不能替代 `FieldReadback`
- `ValueReadback`: 抽样读回转换后的单元格值
- `DownstreamReadback`: 若涉及看板 / 分组 / 排序 / lookup / 公式，继续读回结果
- `CompletionRule`: 结构、值、下游能力都正确，才能回复“已完成”

## 坑点

- ⚠️ 这是全量字段属性更新语义，不是 patch。
- ⚠️ 这是高风险写入操作，执行时必须带 `--yes`。
- ⚠️ 当 `type` 是 `formula` 或 `lookup` 时，先阅读对应指南再执行。

## 参考

- 更新前读取当前字段，确认现有 `type` 和具体配置细节，再决定是原地更新还是新建字段迁移。
- [lark-base-field-json.md](lark-base-field-json.md) — 字段 JSON 规范（推荐）
- [formula-field-guide.md](formula-field-guide.md) — formula 指南（更新公式前必读）
- [lookup-field-guide.md](lookup-field-guide.md) — lookup 指南（更新查找引用前必读）
