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

# 添加电话号码题（规范写法）
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"text","style":{"type":"phone"},"title":"联系电话","required":true}]'

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
  --questions '[{"use_existing_field":true,"field_id":"fldPhone","title":"联系电话","description":"用于活动当天联系","required":true}]'

# 添加超链接题（text + url 样式；等价别名写法 {"type":"url","title":"作品链接"}）
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"text","style":{"type":"url"},"title":"作品链接"}]'

# 添加签字题（attachment + signature 样式）
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"attachment","style":{"type":"signature"},"title":"本人签字","required":true}]'

# 添加公式题（只读计算题；[单价]、[数量] 必须是表里已存在的字段）
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"formula","title":"总价","expression":"[单价]*[数量]"}]'

# 添加查找引用题（只读计算题；按本表「所在部门」到「部门表」查出负责人，「所在部门」必须已存在）
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"lookup","title":"部门负责人","from":"部门表","select":"负责人","where":{"logic":"and","conditions":[["部门名称","==",{"type":"field_ref","field":"所在部门"}]]},"aggregate":"raw_value"}]'
```

## `+form-create` 自动添加的题目

`+form-create` 会把表里支持表单收集的字段全部加入新表单，包括电话号码、超链接、签字、公式、查找引用字段。不要再用本命令为这些字段重复建题；公式、查找引用等题型的约束见下方「超链接、公式、查找引用、签字题」。

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
| `type`                | **是** | 题目类型：`text`、`number`、`select`、`datetime`、`user`、`attachment`、`location`、`formula`、`lookup`；电话号码首选 `text` + `style.type:phone`，兼容别名为 `phone`；超链接题写 `text` + `style.type=url`（或别名 `url`），签字题写 `attachment` + `style.type=signature`，见下方「超链接、公式、查找引用、签字题」 |
| `description`         | 否 | 问题描述（纯文本或 Markdown 链接，如 `[文本](https://example.com)`） |
| `required`            | 否 | 是否必填（true/false） |
| `option_display_mode` | 否 | 选项展示方式（仅 `select` 有效）：`0`=下拉，`1`=纵向（默认），`2`=横向 |
| `multiple`            | 否 | 是否多选（`select`/`user` 类型有效，bool） |
| `options`             | 否 | 选项列表（仅 `select` 有效）：`[{"name":"选项1","hue":"Blue"}]`，hue 可选：`Red`/`Orange`/`Yellow`/`Green`/`Blue`/`Purple`/`Gray` |
| `style`               | 否 | 字段样式配置（见下方说明） |
| `expression`          | `formula` 必填 | 公式表达式，语法与 `+field-create` 的 formula 相同 |
| `from` / `select` / `where` | `lookup` 必填 | 查找引用定义，写法与 `+field-create` 的 lookup 相同 |
| `aggregate`           | 否 | `lookup` 的聚合方式，省略时默认 `raw_value`，取值与 `+field-create` 的 lookup 相同 |
| `visible_rule`        | 否 | 题目显隐条件（见下方「`visible_rule` 显隐条件」） |
| `input_range_limit`   | 否 | 输入范围限制：限制填写的字数 / 数值区间 / 可选日期区间（见下方「`input_range_limit` 输入范围限制」）；电话号码、超链接、公式、查找引用、签字题不支持 |

### 形态 B：已有字段题目

已有字段题目只把一个已存在字段加入表单，不新建字段，也不改变已有记录数据。包括 Phone 在内的受支持字段可用此形态加入或恢复；适合把之前用 `+form-questions-delete --keep-field` 移出表单的题目重新加回，或把表里已有字段补充为表单题目。

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

表里已有的电话号码、超链接、公式、查找引用、签字字段也可以用本形态加入表单，或把被隐藏的这类题目恢复显示。加入后的约束与新建时相同，见下方「超链接、公式、查找引用、签字题」一节。

### `style` 字段说明

| 类型 | style 结构 | 说明 |
|------|------|------|
| `text` | `{"type":"plain"}` / `{"type":"phone"}` / `{"type":"url"}` | `plain` 为普通文本；`phone` 创建真实电话号码字段，GET 统一回显 `type:text + style.type:phone`；`url` 为超链接题（底层字段为超链接 Url）。`email`、`barcode` 不支持作为表单题目 |
| `number` | `{"type":"plain","precision":2}` | precision 为小数位数 |
| `number`（评分） | `{"type":"rating","icon":"star","min":1,"max":5}` | icon 可选：`star`/`heart`/`thumbsup`/`fire`/`smile`/`lightning`/`flower`/`number` |
| `datetime` | `{"format":"yyyy/MM/dd"}` | format 可选：`yyyy/MM/dd`、`yyyy/MM/dd HH:mm`、`MM-dd`、`MM/dd/yyyy`、`dd/MM/yyyy` |
| `attachment`（签字） | `{"type":"signature"}` | 签字题；不传 `style` 为普通附件题 |

电话号码题的规范形态是 `{"type":"text","style":{"type":"phone"}}`。`{"type":"phone"}` 仅作为兼容别名：省略 style 时服务端会展开为规范形态，显式 `style.type:"phone"` 也可用；若别名同时携带 `plain`、`email`、`url`、`barcode` 或未知 style，整批请求会在写入前失败。不要删除 style 后重试，也不要把已有 Phone 字段改成 plain；服务端未就绪或拒绝请求时，应保留电话号码语义并如实报告错误。

电话号码题不支持 `input_range_limit`（字数 / 位数限制），号码字符由提交链路校验。用户要求给电话号码题限制字数或位数时——**即使用户明确点名要“只能填 11 位”**——也必须保留电话号码题型：新建题仍用 `style.type:phone` 创建、不带 `input_range_limit`；已有电话题不加限制。然后在答复中如实说明电话号码题不支持字数/位数限制、号码格式由提交时校验。不得为了加限制把 Phone 字段用 `+field-update` 改成普通文本，也不得改建一道普通文本题加 `text_length` 来替代；字段类型一旦改掉，电话语义和已有号码的格式就丢了。

`email`、`barcode` 两种文本样式目前不能作为表单题目，已有的 Email / Barcode 字段也不能直接加入表单。用户要求收集这类信息时不要因此缺项：新建题目时省略 `style` 按普通 `text` 题收集，并在答复中说明该题不带对应格式校验，这一步无需另行征得同意。表里已有的 Email / Barcode 字段承载着用户数据的格式语义，不能为了加入表单擅自改样式：先向用户说明限制并征得同意，同意后才用 `+field-update` 把 `style.type` 改为 `plain` 再加入表单，并说明改后不再有对应格式校验；用户不同意或无法确认时保留该字段不改，说明该题受平台限制无法加入表单。这条回退只适用于 `email`、`barcode`；电话号码、超链接、公式、查找引用、签字题被拒时都不得退成普通文本，见下方各题型的约束。

货币（`currency`）和进度（`progress`）**不是表单支持的题型**：新建题时会被拒绝（`800010529`），也**不能**用 `+field-create` 先建货币 / 进度字段、或建好普通数字题后用 `+field-update` 改成货币 / 进度格式来达成——这两种做法得到的仍是表单不支持的题型，等同违规交付。用户要求货币 / 进度格式时，建 `plain` 数字题（按需设 `precision`），并明确告诉用户表单里只能按普通数字收集、无法显示货币符号或进度条。

### 超链接、公式、查找引用、签字题

**超链接**

- 写法：`{"type":"text","style":{"type":"url"},"title":"作品链接"}`，或别名 `{"type":"url","title":"作品链接"}`（不传 `style` 时自动展开为 `text` + `url`）。底层字段类型为超链接（Url）。
- 别名 `url` 不能再带非 `url` 的 `style`，例如 `{"type":"url","style":{"type":"plain"}}`：冲突时**整批拒绝**，同批其他合法题目也不会创建，修正后整批重发。

**公式（只读计算题）**

- 写法：`{"type":"formula","title":"总价","expression":"[单价]*[数量]"}`。`expression` 语法与 `+field-create` 的 formula 完全相同，写之前读 [lark-base-field-formula.md](lark-base-field-formula.md)。
- 表达式编译失败返回 `800010221`（validation_error），整批不创建任何字段。
- 表达式引用的字段必须**已存在于表中**。同一批新建的题目不能被本批公式引用：先调用一次创建被引用的题目，再单独调用一次创建公式题。

**查找引用（只读计算题）**

- **先确认来源存在**：`from` 指定的来源表（或 `select` 字段）在 Base 里不存在时，停下来告诉用户并询问来源；不要自己新建来源表、更不要编造示例数据去“凑”出查找结果。只有用户明确要求新建来源表并给出数据时才建。
- 写法：`{"type":"lookup","title":"部门负责人","from":"部门表","select":"负责人","where":{"logic":"and","conditions":[["部门名称","==",{"type":"field_ref","field":"所在部门"}]]},"aggregate":"raw_value"}`。`from` / `select` / `where` / `aggregate` 与 `+field-create` 的 lookup 写法完全一致，写之前读 [lark-base-field-lookup.md](lark-base-field-lookup.md)。
- `where` 至少一个条件。条件第一个元素是**来源表**（`from`）的字段；`{"type":"field_ref","field":...}` 引用的是**当前表**的字段，通常是本表单里排在前面的另一道题，填写人答完那道题后，本题据此实时查出值。只有 `empty` / `non_empty` 条件可以省略比较值、写成二元组，其他运算符都要给比较值。
- 被 `field_ref` 引用的当前表字段必须**已存在**；同一批新建的题目不能被引用，需先建被引用题，再单独调用一次建查找引用题。
- 来源表不存在返回 `800030104`，来源表字段不存在返回 `800030201`。

**签字**

- 写法：`{"type":"attachment","style":{"type":"signature"},"title":"本人签字","required":true}`。
- 填写时签名以 png 图片提交，提交方式见 [lark-base-form-submit.md](lark-base-form-submit.md)。

**共同约束**

- 公式、查找引用是只读计算题，填写人不作答：
  - 设 `required:true` 会被拒（`800010541`），`+form-questions-create` 与 `+form-questions-update` 都一样。
  - 不能被其他题目的 `visible_rule` 引用（`800010542`）。
- 签字题被其他题目的 `visible_rule` 引用时，只能用 `empty` / `non_empty`；用 `==` 带值会被拒（`800010506`）。
- 四种题型都不支持 `input_range_limit`（电话号码题同样不支持），传了会报错。
- 这四种题型创建被拒时，**如实报告错误码和原因**，按报错修正后重试；不得退化成普通文本题或普通附件题交付，也不要把公式、查找引用改成让填写人手填的题目。
- 题目语义是链接 / 网址 / 主页时一律建超链接题。用户的附加要求超链接题做不到时（如字数限制 `input_range_limit`、要提交一个不合法的链接），也**不能为了满足它而改建普通文本题**：照常建超链接题，并说明该要求无法设置 / 该链接会被拒；要提交不合法链接时，提交前指出并向用户确认，或照提交并如实报告被拒。
- 题型限制同样作用于底层字段：**不要先用 `+field-create` 建出表单不支持的字段（如 `email`/`barcode` 样式文本、`currency`/`progress` 数字），再加入表单或用 `+form-questions-update` 改标题来绕过；也不要建好题目后再用 `+field-update` 把题目的底层字段改成这些样式**。有的表单会把表里新建的字段自动显示成题目，绕过后得到的仍是表单不支持的题型。需要邮箱、条码时建普通文本题（不带 style）并说明不做格式校验；需要金额、进度时建普通数字题并说明。
- 表单题目接口的业务错误码在 CLI 输出里可能被归一成 `code: 1`，判断原因以 `message` / `hint` 为准；本节写的错误码用于对照服务端日志。

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

例外：`input_range_limit` 只适用于普通文本（`text_length`）、普通数字（`number_range`）、日期（`date_range`）题。电话号码、超链接、签字、公式、查找引用题不支持；用户对这些题提范围要求时，保持原题型，如实说明该限制无法设置，**不要为了加上限制而改成普通文本 / 数字题**。

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
