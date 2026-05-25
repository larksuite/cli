# base +form-detail

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

通过表单分享 Token 获取表单详情（含表单元信息、题目详情）。只读操作，不修改任何数据。

与 `+form-get` 的区别：`+form-get` 需要 `base-token` + `table-id` + `form-id`（从 Base 内部获取）；`+form-detail` 仅需 `share-token`（从分享链接获取，无需知道 Base/表信息）。

## 命令

```bash
# 通过 share_token 获取表单详情
lark-cli base +form-detail \
  --share-token <share_token>

# 以 pretty 格式展示（适合阅读 questions 结构）
lark-cli base +form-detail \
  --share-token <share_token> \
  --format pretty

# 使用 jq 过滤只看题目列表
lark-cli base +form-detail \
  --share-token <share_token> \
  --jq '.data.questions'

# 预览 API 调用（不执行）
lark-cli base +form-detail \
  --share-token <share_token> \
  --dry-run

# 使用应用身份（bot）
lark-cli base +form-detail \
  --share-token <share_token> \
  --as bot
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--share-token <token>` | 是 | 表单分享 Token（从表单分享链接中提取） |
| `--format` | 否 | 输出格式：json（默认）\| pretty \| table \| ndjson \| csv |
| `--as` | 否 | 身份：user（默认）\| bot |
| `--dry-run` | 否 | 预览 API 调用，不执行 |
| `--jq <expr>` | 否 | 用 jq 表达式过滤 JSON 输出 |

### 从分享链接提取 share-token

用户提供形如以下格式的表单分享链接时：

```text
https://bitable-test.feishu-boe.cn/share/base/form/shrbcvST8eZy0vk8zjVZ1CAXNye
```

**提取方式：** 取 URL 路径最后一段作为 `--share-token`。

以上述链接为例：

- `share-token` = `shrbcvST8eZy0vk8zjVZ1CAXNye`

```bash
lark-cli base +form-detail \
  --share-token shrbcvST8eZy0vk8zjVZ1CAXNye
```

## 输出格式

| 字段 | 类型 | 说明 |
|------|------|------|
| `base_token` | string | 所属多维表格 Base token |
| `name` | string | 表单名称 |
| `description` | string | 表单描述 |
| `questions[]` | array | 题目列表（含 id / title / type / required / description / filter） |

### questions 中每个题目的字段

#### 固定字段（所有题目共有）

| 字段 | 类型 | 是否必填 | 说明 |
|------|------|----------|------|
| `id` | string | 是 | 题目标识（对应 field_id） |
| `title` | string | 是 | 题目标题 |
| `type` | string | 是 | 字段类型（见下方类型对照表，与 [`lark-base-shortcut-field-properties.md`](lark-base-shortcut-field-properties.md) 对齐） |
| `required` | bool | 是 | 是否必填 |
| `description` | string | 否 | 题目描述 |
| `filter` | object | 否 | 题目显示条件（详见下方 filter 结构说明） |

#### 动态字段（按 type 不同而不同，直接平铺在 question 中）

除上述固定字段外，每种 `type` 还会携带该类型特有的配置字段（与 [`lark-base-shortcut-field-properties.md`](lark-base-shortcut-field-properties.md) 中的「常见补充字段」对应），例如：

- **text** → `style`（含 `style.type`: plain / phone / url / email / barcode）
- **number** → `style`（含 `style.type`: plain / currency / progress / rating 及其子配置）
- **select** → `multiple`（bool）、`options`（选项列表）或 `dynamic_options_source`
- **datetime / created_at / updated_at** → `style.format`
- **user / group_chat** → `multiple`
- **link** → `link_table`、`bidirectional`、`bidirectional_link_field_name`
- **formula** → `expression`
- **lookup** → `from`、`select`、`where`、`aggregate`
- **auto_number** → `style.rules`
- **attachment / location / checkbox / stage / created_by / updated_by** → 无额外动态字段

### filter 结构说明

`filter` 控制题目在表单中的显示/隐藏逻辑，由 `conjunction`（逻辑关系）和 `conditions`（条件列表）组成。

以下以一个「活动报名」表单为例，其中「紧急联系人」题目的 filter 配置：

```json
{
  "conjunction": "and",
  "conditions": [
    {"field_name": "是否携带家属", "operator": "is", "value": ["是"]},
    {"field_name": "参与人数", "operator": "isGreater", "value": [1]}
  ]
}
```

> 以上述 JSON 为例：当题目「是否携带家属」的值为「是」**并且**题目「参与人数」大于 1 时，「紧急联系人」才会展示（`conjunction: "and"` 表示全部条件需同时满足；若为 `"or"` 则任一条件满足即显示）。

另一个常见场景——用 `or` 控制可选填的补充信息：

```json
{
  "conjunction": "or",
  "conditions": [
    {"field_name": "满意度评分", "operator": "isLessEqual", "value": [3]},
    {"field_name": "是否愿意回访", "operator": "is", "value": ["是"]}
  ]
}
```

> 即：评分 ≤ 3 **或** 愿意接受回访时，才展示「改进建议」文本框。

#### filter 字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `conjunction` | string | 条件间逻辑关系：`and`（全部满足） / `or`（任一满足） |
| `conditions[]` | array | 条件列表 |

#### conditions 中每个条件项的字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `field_name` | string | 所依赖的题目标题（引用其他题目的 title） |
| `operator` | string | 过滤操作符（见下方 operator 可选值） |
| `value` | array | 过滤值数组（部分 operator 不需要，如 `isEmpty` / `isNotEmpty`） |

#### operator 可选值

| operator | 含义 | 适用类型 |
|----------|------|----------|
| `is` | 等于 | 除附件外全部 |
| `isNot` | 不等于 | 除附件外全部 |
| `contains` | 包含 | 文本、选项、人员、群聊、地理位置 |
| `doesNotContain` | 不包含 | 文本、选项、人员、群聊、地理位置 |
| `isEmpty` | 为空 | 全部 |
| `isNotEmpty` | 不为空 | 全部 |
| `isGreater` | 大于 | 数字、日期时间 |
| `isGreaterEqual` | 大于等于 | 数字、日期时间 |
| `isLess` | 小于 | 数字、日期时间 |
| `isLessEqual` | 小于等于 | 数字、日期时间 |

> **附件（attachment）特殊说明：** 仅支持 `isEmpty` 和 `isNotEmpty`，不支持 `is` / `isNot` / `contains` 及比较操作符。

#### value 的格式（按所依赖题目的类型区分）

| 所依赖题目类型 | value 格式 | 示例 |
|----------------|-----------|------|
| 文本类（text / phone / email / url 等） | 字符串数组 | `["1", "2"]` |
| 数字类（number） | 数字数组 | `[1, 2]` |
| 选项类（select / multi_select） | 选项名称数组 | `["选项A", "选项B"]` |
| 人员类（user） | open_id 数组 | `["ou_d57864434a537020cf7a4a681d393e2d"]` |
| 群聊类（group_chat） | open_id 数组 | `["oc_f62478de5cc958583191e778db972603"]` |
| 地理位置（location） | 地点名称数组 | `["北京总部"]` |
| 日期时间类（datetime） | 时间字符串数组，固定格式 `yyyy-MM-dd HH:mm:ss` | `["2026-05-07 14:30:00"]` |
| 关联（link / duplexlink） | 记录 ID 数组 | `["recxxxxxxx", "recyyyyyyy"]` |

### type 可选值

与 [`lark-base-shortcut-field-properties.md`](lark-base-shortcut-field-properties.md) 中的字段类型完全对齐。

| type 值 | 含义 | 常见动态字段 |
|----------|------|-------------|
| `text` | 文本（含电话/邮箱/链接/条码等子类型） | `style` |
| `number` | 数字（含货币/进度/评分等子类型） | `style` |
| `select` | 选项（单选/多选由 `multiple` 区分） | `multiple`、`options` / `dynamic_options_source` |
| `datetime` | 日期时间 | `style.format` |
| `user` | 人员 | `multiple` |
| `group_chat` | 群组 | `multiple` |
| `attachment` | 附件 | 无 |
| `location` | 地理位置 | 无 |
| `checkbox` | 复选框 | 无 |
| `link` | 关联 | `link_table`、`bidirectional`、`bidirectional_link_field_name` |
| `formula` | 公式 | `expression` |
| `lookup` | 引用 | `from`、`select`、`where`、`aggregate` |
| `auto_number` | 自动编号 | `style.rules` |
| `created_at` | 创建时间 | `style.format` |
| `updated_at` | 更新时间 | `style.format` |
| `created_by` | 创建人 | 无 |
| `updated_by` | 更新人 | 无 |
| `stage` | 阶段 | 无 |

```json
{
  "ok": true,
  "data": {
    "base_token": "DBALKJKLHDLJ",
    "name": "2026 年度技术大会报名",
    "description": "请填写参会信息，带 * 为必填项",
    "questions": [
      {
        "id": "fldzaYFpb6",
        "required": true,
        "title": "姓名",
        "type": "text"
      },
      {
        "id": "fldCoBpOlx",
        "required": true,
        "title": "手机号",
        "type": "text",
        "style": { "type": "phone" }
      },
      {
        "id": "fldmmhZFCs",
        "required": false,
        "title": "公司邮箱",
        "type": "text",
        "style": { "type": "email" }
      },
      {
        "id": "fldhqmqCj8",
        "required": true,
        "title": "参会日期",
        "type": "datetime",
        "style": { "format": "yyyy-MM-dd" }
      },
      {
        "id": "fldlyRrfrN",
        "required": true,
        "title": "参与人数",
        "type": "number"
      },
      {
        "id": "fldRakYky3",
        "required": false,
        "title": "是否携带家属",
        "type": "select",
        "multiple": false,
        "options": [
          { "name": "是", "hue": "Green", "lightness": "Lighter" },
          { "name": "否", "hue": "Gray", "lightness": "Lighter" }
        ]
      },
      {
        "id": "fldyrOO0X4",
        "required": false,
        "title": "紧急联系人",
        "type": "text",
        "filter": {
          "conjunction": "and",
          "conditions": [
            {"field_name": "是否携带家属", "operator": "is", "value": ["是"]},
            {"field_name": "参与人数", "operator": "isGreater", "value": [1]}
          ]
        }
      },
      {
        "id": "fldM9AsRc2",
        "required": false,
        "title": "上传简历",
        "type": "attachment",
        "filter": {
          "conjunction": "or",
          "conditions": [
            {"field_name": "是否携带家属", "operator": "isNotEmpty"}
          ]
        }
      },
      {
        "id": "fldN7PsWx1",
        "required": true,
        "title": "所属部门",
        "type": "user",
        "multiple": false
      },
      {
        "id": "fldKq3mTz8",
        "required": true,
        "title": "参会主题",
        "type": "select",
        "multiple": true,
        "options": [
          { "name": "AI 与大模型", "hue": "Purple", "lightness": "Lighter" },
          { "name": "云原生", "hue": "Blue", "lightness": "Lighter" },
          { "name": "工程效能", "hue": "Orange", "lightness": "Lighter" },
          { "name": "前端技术", "hue": "Carmine", "lightness": "Lighter" }
        ]
      }
    ]
  }
}
```

## 提示

- `share_token` 从表单分享链接中提取，格式通常为 `shr` + 随机字符串（如 `shrbcvST8eZy0vk8zjVZ1CAXNye`）
- 返回的 `questions` 列表可直接用于构造 `+form-submit` 的 `--json.fields` 参数
- `questions[].title` 对应题目标题，可用于 `+form-submit` 的字段名映射
- 如果需要通过 Base 内部路径操作表单，使用 `+form-get`（需要 base-token / table-id / form-id）
- 权限要求：`base:form:read`

## 参考

- [lark-base](../SKILL.md) — 多维表格全部命令
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
- [lark-base-form-submit](lark-base-form-submit.md) — 获取详情后可用 submit 填写提交
