# base +field-update

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

更新一个已有字段。

> **Select 选项更新特别警告：** `+field-get` 返回的 `options` 可能不完整。增加、删除或更新单选/多选选项时，禁止直接基于 `+field-get.options` 构造更新 payload；必须先用分页接口 `+field-search-options` 取得全部选项，再生成最终选项集并执行全量覆盖。

## 推荐命令

```bash
lark-cli base +field-update \
  --base-token <base_token> \
  --table-id <table_id> \
  --field-id <field_id> \
  --json '{"name":"状态","type":"select","multiple":false,"default_value":["Doing"],"options":[{"name":"Todo","hue":"Blue","lightness":"Lighter"},{"name":"Doing","hue":"Orange","lightness":"Light"},{"name":"Done","hue":"Green","lightness":"Light"}]}' \
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

## Select 选项更新强制流程

单选或多选字段的选项操作必须遵守以下完整性约束：

1. **选项查询必须使用 `+field-search-options`。** 该命令是分页接口；从第一页开始，使用 `--limit` / `--offset` 持续读取，直到接口明确没有后续页，才能得到可用于写入的完整选项集。查询某个选项是否存在时也使用该命令，不要只检查 `+field-get.options`。
2. **`+field-get.options` 不保证完整。** `+field-get` 用于读取字段名称、类型和其他字段配置；其响应中的 `options` 可能只返回前一部分。必须检查 `remaining_options_count`：只要该值大于 `0`，就表示仍有选项未返回，禁止把当前 `options` 当作全集，也禁止用 `len(options)` 声称字段的选项总数。
3. **先收齐、再变换、最后覆盖。** `+field-update` 是全量 `PUT`。增加、删除或更新选项时，先用 `+field-search-options` 分页收齐全部现有选项；然后在完整集合上执行增加、删除、重命名、颜色调整等目标变换，得到最终选项集；最后把这个最终选项集放入完整字段定义并调用 `+field-update --yes`。
4. **完整性无法证明就停止写入。** 任一分页读取失败、提前终止或无法确认已经读取全部选项时，不得执行 `+field-update`。应说明当前结果不完整并继续补齐读取；无法补齐时如实报告，不能用部分选项覆盖字段。

```text
field-get（字段其他配置，并检查 remaining_options_count）
    + field-search-options（offset=0 开始，分页拉取全部选项）
    + 在完整选项集上增加 / 删除 / 更新
    + field-update（提交最终完整字段定义，全量 PUT）
```

## API 入参详情

**HTTP 方法和路径：**

```
PUT /open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id
```

## JSON 值规范

- `--json` 必须是 **JSON 对象**，顶层直接传字段定义。
- 更新语义是 override 式的完整覆盖 `PUT`，不是 partial update；先读取当前定义，再提交整个字段需要保留的可写配置，不要只传零散片段。
- 所有字段类型都支持可选 `description`；支持纯文本，也支持 Markdown 链接。
- 需要字段默认值时传 `default_value`，直接使用字段对应 CellValue；传 `null` 清空。完整规则见 [Field Schema](lark-base-field-schema.md)。
- `select` 更新时：`options` 仍按对象数组传，避免混入无效字段；该数组必须来自上方流程收齐并变换后的最终完整选项集，不能直接使用 `+field-get.options`。
- `link` 更新限制：
  - 不能把非 `link` 字段改成 `link`，也不能把 `link` 改成非 `link`。
  - 现有 `link` 字段的 `bidirectional` 不能改。
- 更新 `auto_number.style.rules` 会按新规则更新已有记录的编号；规则结构见 [Field Schema](lark-base-field-schema.md)。

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

## 返回重点

- 返回 `field` 和 `updated: true`。
- 按返回的 `next_step` 和 `verification_hint` 继续；类型转换涉及已有值时抽样读取记录。

## 工作流


1. 先用 `+field-get` 读取当前字段名称、类型和其他配置，并检查 `remaining_options_count`。若操作 Select 选项，再按“Select 选项更新强制流程”使用 `+field-search-options` 分页读取全部选项；只改变目标属性，并把需要保留的其他可写配置与最终完整选项集一并写回。
2. `formula/lookup` 类型更新前先阅读对应指南。
3. 如果这次更新会改变字段 `type`，先按下方“字段类型变更规则”判断能否执行。如果不修改 `type`，大多数场景都相对安全。

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

1. 该列为空。
2. 正在初始化新建的空表。
3. 主字段不能删除，需要通过更新完成初始化。
4. 用户明确接受整列数据丢失。

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

## 坑点

- ⚠️ 这是全量字段属性更新语义，不是 patch。
- ⚠️ Select 选项必须通过 `+field-search-options` 分页读取完整；`+field-get.options` 可能不全，必须检查 `remaining_options_count`。
- ⚠️ 增加、删除或更新选项必须在完整选项集上生成最终结果后再全量覆盖；分页未完成时禁止调用 `+field-update`。
- ⚠️ 这是高风险写入操作，执行时必须带 `--yes`。
- ⚠️ 当 `type` 是 `formula` 或 `lookup` 时，先阅读对应指南再执行。

## 参考

- 更新前读取当前字段，确认现有 `type` 和具体配置细节，再决定是原地更新还是新建字段迁移。
- [Field Schema](lark-base-field-schema.md) — 字段 JSON 规范（推荐）
- [Formula Field](lark-base-field-formula.md) — 更新公式前必读
- [Lookup Field](lark-base-field-lookup.md) — 更新查找引用前必读
