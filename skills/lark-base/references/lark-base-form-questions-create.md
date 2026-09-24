# base +form-questions-create

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

向多维表格表单/问卷中批量添加问题。可以新建字段并作为题目，也可以把已有字段加到表单中作为题目而不新建字段。

## 命令

```bash
# 添加一个文本必填问题
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"text","title":"您的姓名是？","required":true}]'

# 添加多个问题（按顺序排列）
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"text","title":"您的姓名是？","required":true},{"type":"text","title":"您的联系方式是？","required":false}]'

# 添加单选题（带选项）
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"select","title":"满意度评价","required":true,"multiple":false,"options":[{"name":"非常满意","hue":"Green"},{"name":"满意","hue":"Blue"},{"name":"一般","hue":"Yellow"}]}]'

# 添加评分题
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"number","title":"服务评分","style":{"type":"rating","icon":"star","min":1,"max":5}}]'
  
# 添加带描述的问题（纯文本）
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"text","title":"您的姓名","description":"请填写真实姓名"}]'
# 添加带描述的问题（含链接）
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"text","title":"反馈建议","description":"更多详情请查看[帮助文档](https://example.com/help)"}]'  

# 添加带显隐条件（visible_rule）的问题：当「是否需要发票」选择「是」时才显示「发票抬头」
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"select","title":"是否需要发票","required":true,"options":[{"name":"是","hue":"Blue"},{"name":"否","hue":"Gray"}]},{"type":"text","title":"发票抬头","visible_rule":{"logic":"and","conditions":[["是否需要发票","==","是"]]}}]'

# 把已有字段作为题目加到表单中，不新建字段
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"use_existing_field":true,"field_id":"fldEmail","title":"你的邮箱","description":"用于接收回执","required":true}]'
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token（base_token） |
| `--table-id <id>` | 是 | 数据表 ID |
| `--form-id <id>` | 是 | 表单 ID |
| `--questions <json>` | 是 | 问题 JSON 数组，最多 10 个（见下方格式） |
| `--format` | 否 | 输出格式：json（默认）\| pretty \| table \| ndjson \| csv |
| `--as` | 否 | 身份：user（默认）\| bot |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## `--questions` 格式

`--questions` 是 1~10 个问题对象的数组。每个对象二选一：

- 新建字段题目：创建一个新字段，并把该字段作为表单题目。
- 已有字段题目：把一个已存在字段加入表单，只改变该字段在表单中的可见性，不创建字段。

### 形态 A：新建字段题目

新建字段题目会在数据表中创建新字段，返回的 question `id` 就是新字段的 `field_id`。CLI 当前要求每个新建字段题目显式传 `title` 和 `type`。

| 字段                    | 必填 | 说明 |
|-----------------------|------|------|
| `title`               | **是** | 问题标题（字段名） |
| `type`                | **是** | 题目类型：`text`、`number`、`select`、`datetime`、`user`、`attachment`、`location` |
| `description`         | 否 | 问题描述（纯文本或 Markdown 链接，如 `[文本](https://example.com)`） |
| `required`            | 否 | 是否必填（true/false） |
| `option_display_mode` | 否 | 选项展示方式（仅 `select` 有效）：`0`=下拉，`1`=纵向（默认），`2`=横向 |
| `multiple`            | 否 | 是否多选（`select`/`user` 类型有效，bool） |
| `options`             | 否 | 选项列表（仅 `select` 有效）：`[{"name":"选项1","hue":"Blue"}]`，hue 可选：`Red`/`Orange`/`Yellow`/`Green`/`Blue`/`Purple`/`Gray` |
| `style`               | 否 | 字段样式配置（见下方说明） |
| `visible_rule`        | 否 | 题目显隐条件（见下方「`visible_rule` 显隐条件」） |
| `input_range_limit`   | 否 | 输入范围限制：限制填写的字数 / 数值区间 / 可选日期区间（见下方「`input_range_limit` 输入范围限制」） |

### 形态 B：已有字段题目

已有字段题目只把一个已存在字段加入表单，不新建字段，也不改变已有记录数据。适合把之前用 `+form-questions-delete --keep-field` 移出表单的题目重新加回，或把表里已有字段补充为表单题目。

| 字段                    | 必填 | 说明 |
|-----------------------|------|------|
| `use_existing_field`  | **是** | 固定传 `true`，表示使用已有字段 |
| `field_id`            | **是** | 已有字段的 ID 或字段名；推荐字段 ID，避免同名字段歧义。引用长度 1~100，较长字段名请改用字段 ID |
| `title`               | 否 | 题目标题；省略时使用字段名 |
| `description`         | 否 | 问题描述（纯文本或 Markdown 链接，如 `[文本](https://example.com)`） |
| `required`            | 否 | 是否必填（true/false），默认 false |
| `option_display_mode` | 否 | 选项展示方式（仅已有字段为 `select` 时有效）：`0`=下拉，`1`=纵向（默认），`2`=横向 |
| `visible_rule`        | 否 | 题目显隐条件（见下方「`visible_rule` 显隐条件」） |
| `input_range_limit`   | 否 | 输入范围限制。本形态推荐分两步：先把字段加进表单，再用 `+form-questions-update` 按题目 `id` 设置，见「`input_range_limit` 输入范围限制」末尾 |

已有字段题目不要携带字段定义属性，例如 `type`、`style`、`options`、`multiple`、`name`。服务端使用 strict schema，误传不属于该形态的字段会被拒绝。

### `style` 字段说明

| 类型 | style 结构 | 说明 |
|------|------|------|
| `text` | `{"type":"plain"}` | 当前仅支持 `plain` |
| `number` | `{"type":"plain","precision":2}` | precision 为小数位数 |
| `number`（评分） | `{"type":"rating","icon":"star","min":1,"max":5}` | icon 可选：`star`/`heart`/`thumbsup`/`fire`/`smile`/`lightning`/`flower`/`number` |
| `datetime` | `{"format":"yyyy/MM/dd"}` | format 可选：`yyyy/MM/dd`、`yyyy/MM/dd HH:mm`、`MM-dd`、`MM/dd/yyyy`、`dd/MM/yyyy` |

### `visible_rule` 显隐条件

> **仅当用户明确要求为题目设置显隐条件（显示/隐藏逻辑）时，才需要读下面的结构说明；否则忽略本节。**

`visible_rule` 控制题目在表单中的显示/隐藏：当条件满足时题目显示，不满足时隐藏；不传或 `conditions` 为空数组则题目始终显示。

- **结构与视图筛选 `filter` 完全一致**，即 `{logic?, conditions?}`，共用同一套公共协议。
- 与视图 `filter` 唯一的区别：`conditions` 中的 `field` 引用的是**同一表单内其他题目的题目名称或题目 ID**（推荐用题目 ID 以避免重名歧义），而不是数据表字段。
- **只能引用前序题目**：条件只能引用排在当前题目之前的题目——创建时按 `questions` 数组顺序判定（可引用同批次更靠前的新题目或表单中已有题目），不支持循环引用。
- 引用的题目必须真实存在，否则会报错。
- 列出题目（`+form-questions-list`）会在每个题目对象中**原样返回** `visible_rule`；未设置显隐条件的题目返回 `null` 或 `conditions` 为空数组。

```json
{
  "logic": "and",
  "conditions": [
    ["是否需要发票", "==", "是"],
    ["报销金额", ">=", 1000]
  ]
}
```

详细的 `visible_rule` 结构（顶层规则、operator 列表、各题目类型的 value 写法）请阅读 [lark-base-filter-condition.md](lark-base-filter-condition.md)。

### `input_range_limit` 输入范围限制

> **仅当用户明确要求限制填写内容的范围（字数、数值区间、可选日期区间）时，才需要读下面的结构说明；否则忽略本节。**

用户提出「最多/至少 N 个字、只能填 0~100、只能选今天起 7 天内」这类填写范围要求时，交付物就是这里的 `input_range_limit`：写进 `description` 描述文案、改成 `required`、或提交后再人工核对，都不构成实现——前端不会据此拦截填写。

`input_range_limit` 是**题目级**配置，与 `required`、`visible_rule` 同级，不是字段定义属性。创建题目时把规则放进**同一个 question 对象**里，不要另起一次调用。清除已有限制传 `"input_range_limit": null`。

三类限制用 `type` 判别：

| type | 参数 | 规则 |
|---|---|---|
| `text_length` | `min_length`?、`max_length`? | 用户说了哪一侧就只写哪一侧；边界是正整数（不接受 0），两侧都给时 `min_length <= max_length` |
| `number_range` | `min_value`?、`max_value`? | 至少给一个有限数字边界，允许 0 及负数，`min_value <= max_value` |
| `date_range` | `earliest`?、`latest`? | 每个边界写成 `{"fixed_ms":整数毫秒}` 或 `{"dynamic":{"anchor":...,"offset_days":...}}`，**二者互斥**；至少给一个边界 |

三类的最小 JSON 形状：

```json
{"type":"text_length","min_length":1,"max_length":100}
{"type":"number_range","min_value":0,"max_value":100}
{"type":"date_range","earliest":{"dynamic":{"anchor":"today","offset_days":0}},"latest":{"dynamic":{"anchor":"today","offset_days":7}}}
{"type":"date_range","earliest":{"fixed_ms":1790784000000},"latest":{"fixed_ms":1791302400000}}
```

上面这个固定区间例子换算成北京时间是 `2026-10-01 00:00:00` ~ `2026-10-07 00:00:00`，也就是**允许选 10 月 1 日到 10 月 7 日**——`latest` 取的是 10-07 当天零点，而不是 10-08 零点。

硬规则：

- **用户只说了一侧就只写一侧**：「至少 N 个字」只写 `min_length`，「最多 N 个字」只写 `max_length`，不要补全另一侧。缺省的那一侧表示不限制，擅自补上会把用户没要求的限制写进表单。`number_range` 的 `min_value` / `max_value`、`date_range` 的 `earliest` / `latest` 同理。
- **不写就是省略该键，不要传 `null`**：边界键只接受数字，传 `null` 在 CLI 本地就会被拒（`--questions item N.input_range_limit.min_length must be a JSON number, not null`），请求不会发出。`null` 只用在整个 `input_range_limit` 上，表示清除该题的全部限制。
- 字段名不接受别名，一律 snake_case；`min_length` / `max_length` / `min_value` / `max_value` / `fixed_ms` 都必须是 JSON **数字**，不要转成字符串。
- `anchor` 只能取 `today` / `monday` / `friday` / `saturday` / `sunday` / `month_start` / `month_end`；`offset_days` 是整数天，可正可负。
- **`fixed_ms` 就填那一天的 00:00:00，不要 +1 天**：两个边界都是**闭区间**，`latest` 填当天零点就已经包含当天全天。用户说「最晚 9 月 30 日」就写 9-30 的 00:00:00，不要写成 10-01 的零点，也不要写 9-30 的 23:59:59——这两种写法都会把可选范围多放一天，表单上显示成能选到 10 月 1 日。`earliest` 同理，填当天零点即可。
- 同一个边界里 `fixed_ms` 和 `dynamic` 只能出现一个。
- 两端**都**写 `fixed_ms` 时，CLI 本地就会比较先后，`earliest` 晚于 `latest` 直接被拒。一端 `dynamic` 时不比较——相对日期要到运行期才知道落在哪天。
- `+form-questions-list` 会对每个题目回显 `input_range_limit`，没有配置的题目返回 `null`；这是唯一的验收证据。

#### 相对日期怎么算：anchor + offset_days 映射表

协议里**没有**「7 天前」「30 天后」这类具名枚举，所有相对日期都只能由 `anchor` 加 `offset_days` 组合出来。按下表把用户口语翻译成写法，不要发明 anchor 名：

| 用户说法 | 写法 |
|---|---|
| 今天 | `{"anchor":"today","offset_days":0}` |
| 昨天 | `{"anchor":"today","offset_days":-1}` |
| 明天 | `{"anchor":"today","offset_days":1}` |
| 7 天前 / 7 天后 | `{"anchor":"today","offset_days":-7}` / `{"anchor":"today","offset_days":7}` |
| 30 天前 / 30 天后 | `{"anchor":"today","offset_days":-30}` / `{"anchor":"today","offset_days":30}` |
| 本周一 / 本周五 / 本周六 / 本周日 | `{"anchor":"monday","offset_days":0}` / `{"anchor":"friday","offset_days":0}` / `{"anchor":"saturday","offset_days":0}` / `{"anchor":"sunday","offset_days":0}`（自然周：周一开始、周日结束） |
| 月初 / 月末 | `{"anchor":"month_start","offset_days":0}` / `{"anchor":"month_end","offset_days":0}` |

用户说的是固定的绝对日期（如「只能选 2026 年 10 月 1 日到 10 月 7 日」）才用 `fixed_ms`；说的是「从今天起 / 未来 N 天 / 本周内」这类跟着运行日期滚动的范围，必须用 `dynamic`，不得把当天换算成 `fixed_ms` 固化。

#### 产品侧边界值

| 限制 | 边界 |
|---|---|
| 文本字数 | 最小字数下限 1，最大字数上限 100000 |
| 数字范围 | 可负、可小数；缺省的那一侧即不限 |
| 日期范围 | **只到日，不支持时分秒** |

#### 可直接运行的完整命令

```bash
# 文本题限制 10~200 字
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"text","title":"意见反馈","required":true,"input_range_limit":{"type":"text_length","min_length":10,"max_length":200}}]'

# 数字题限制 0~150（允许 0）
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"number","title":"年龄","required":true,"input_range_limit":{"type":"number_range","min_value":0,"max_value":150}}]'

# 日期题限制「今天起 7 天内」（相对日期，跟着运行日期滚动）
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"datetime","title":"预约日期","required":true,"input_range_limit":{"type":"date_range","earliest":{"dynamic":{"anchor":"today","offset_days":0}},"latest":{"dynamic":{"anchor":"today","offset_days":7}}}}]'

# 日期题限制固定区间（用户点名了绝对日期时才用 fixed_ms）
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"datetime","title":"活动日期","input_range_limit":{"type":"date_range","earliest":{"fixed_ms":1790784000000},"latest":{"fixed_ms":1791302400000}}}]'


# 回读验收：确认每道题的 input_range_limit 与要求一致
lark-cli base +form-questions-list \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id>
```

已有字段题目（形态 B）要加限制时，先用 `use_existing_field:true` + `field_id` 把字段加进表单，再用 `+form-questions-update` 按题目 `id` 设置 `input_range_limit`；`+form-questions-update` 是全量覆盖，写法和保留规则见 [lark-base-form-questions-update.md](lark-base-form-questions-update.md)。

## 输出格式

返回创建成功的问题列表：

```json
{
  "ok": true,
  "data": {
    "items": [
      {"id": "q_001", "title": "您的姓名是？", "required": true}
    ]
  }
}
```

## 工作流

> [!CAUTION]
> 这是**写入操作** — 执行前必须向用户确认。

1. 先确定表单所属的真实 `table_id`，并在整个表单管理工作流中复用它；仅在 ID 缺失或归属不明确时调用 `+table-list`。
2. 用 `+form-questions-list` 查看现有问题。问题 `id` 是承载该问题的 `field_id`，不是独立于数据表的临时 ID。
3. 需要把表里已有字段加进表单时，先用 `+field-list` 确认真实字段 ID 和字段类型，再用 `use_existing_field:true` + `field_id`；字段已经是可见题目时不要重复创建，改用 `+form-questions-update`。
4. 除非用户明确要求同名的独立问题，否则目标标题已经存在时用 `+form-questions-update` 更新必填状态、标题或描述；不要创建同名问题后再删除旧问题。
5. 创建确实不存在的问题，或用户明确要求的同名独立问题，并报告新建的问题 ID。

`+form-questions-delete` 默认会删除承载问题的数据表字段及记录数据；如果只是想把题目移出表单并保留字段，必须用 `+form-questions-delete --keep-field`。移出后可用本文的已有字段题目形态加回。

## 参考

- [lark-base](../SKILL.md) — 多维表格全部命令
- [lark-base-filter-condition.md](lark-base-filter-condition.md) — `visible_rule` / `filter` 条件结构公共协议
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
