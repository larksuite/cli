# base +form-submit

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

通过表单分享链接填写并提交多维表格表单。仅支持分享模式（share_token），支持填写普通字段值和上传本地文件作为附件。

## 填写前必读：先获取表单详情

**在调用 `+form-submit` 之前，必须先使用 [`+form-detail`](lark-base-form-detail.md) 获取表单详情。** 原因如下：

1. **字段类型匹配**：每个题目的 `type` 决定了值的格式（文本、数字、选项、人员、日期等），需根据类型正确构造 `fields` 中的值
2. **必填校验**：通过 `questions[].required` 判断哪些题目为必填项，避免遗漏
3. **显示条件过滤**：部分题目带有 `filter`（显示/隐藏逻辑），需根据用户已填的其他题目值判断该题目是否应该出现——**不应填写被 filter 隐藏的题目**
4. **获取 base_token（附件场景必用）**：`+form-detail` 返回的 `data.base_token` 是该表单所属的多维表格标识。当表单包含附件字段时，提交时必须通过 `--base-token` 传入此值，因为附件需要上传到该 Base 的 Drive Media 中

典型流程：

```bash
# 1️⃣ 先获取表单详情，了解所有题目
lark-cli base +form-detail --share-token <share_token>

# 2️⃣ 根据返回的 questions 列表，按 type 格式化值、检查 required、判断 filter 条件

# 3️⃣ 再提交
lark-cli base +form-submit \
  --share-token <share_token> \
  --json '{"fields":{...}}'
```

详见 [`lark-base-form-detail.md`](lark-base-form-detail.md) 中的「questions 结构说明」和「filter 结构说明」。

## 命令

```bash
# 基本提交（填写普通字段）
lark-cli base +form-submit \
  --share-token <share_token> \
  --json '{"fields":{"服务评分":5,"评价内容":"服务态度好"}}'

# 带附件提交（需要额外提供 --base-token）
lark-cli base +form-submit \
  --share-token <share_token> \
  --base-token <base_token> \
  --json '{
    "fields": {"服务评分": 5, "评价内容": "好"},
    "attachments": {
      "附件字段名": ["./report.pdf", "./photo.png"],
      "另一个附件字段": ["./doc.docx"]
    }
  }'

# 使用应用身份（bot）
lark-cli base +form-submit \
  --share-token <share_token> \
  --json '{"fields":{...}}' \
  --as bot

# 预览 API 调用（不实际执行）
lark-cli base +form-submit \
  --share-token <share_token> \
  --json '{"fields":{...}}' \
  --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--share-token <token>` | 是 | 表单分享 Token（必填），从表单分享链接中提取 |
| `--base-token <token>` | 条件必填 | Base token；**当 `--json` 包含 `attachments` 时必须提供**，用于将附件上传到 Base Drive Media |
| `--json <json>` | 是 | JSON 对象，包含 `"fields"`（普通字段值）和 `"attachments"`（附件上传），详见下方说明 |
| `--format` | 否 | 输出格式：json（默认）\| pretty \| table \| ndjson \| csv |
| `--as` | 否 | 身份：user（默认）\| bot |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

### --json 结构说明

`--json` 是一个 JSON 对象，包含两个部分：

#### fields（普通字段）

`fields` 中的单元格值写法与 [`lark-base-cell-value.md`](lark-base-cell-value.md) 完全对齐，填写前应先阅读该文档了解各类型的构造规则：

```json
{
  "文本字段": "Hello World",
  "电话字段": "13800000000",
  "超链接字段": "https://example.com",
  "数字字段": 12.5,
  "单选字段": "选项A",
  "多选字段": ["选项A", "选项B"],
  "时间字段": "2026-04-27 14:30:00",
  "复选框字段": true,
  "人员字段": [{ "id": "ou_7094d131420c8749632145f08fbf114a" }],
  "关联字段": [{ "id": "recXXXXXXXXXXXX" }],
  "地理位置字段": { "lng": 116.397428, "lat": 39.90923 }
}
```

> **注意：附件类型字段不要写在 `fields` 里。** `fields` 中不包含附件，附件有独立的填写方式，见下方「attachments（附件上传）」章节。

> 自动编号、公式、创建/修改人、创建/修改时间等系统字段会自动填入，无需手动传入。

#### attachments（附件上传）

**附件字段的填写方式与 `fields` 中的普通单元格完全不同**，不能在 `fields` 里传 `file_token` 或其他附件格式。必须将附件字段单独放在 `--json` 的顶层 `attachments` 对象中，值为**本地文件路径数组**（不是 token）：

```json
{
  "attachments": {
    "附件字段名": ["./report.pdf", "./photo.png"],
    "另一个附件字段": ["./doc.docx"]
  }
}
```

CLI 收到路径后会自动完成以下流程：
1. 校验所有文件（存在性、大小 ≤2GB、常规文件）
2. 并行上传到 Base Drive Media（并发上限 5，跨字段重复路径自动去重）
3. 获取 `file_token` 后合并到最终表单提交内容中

> 与 [`lark-base-cell-value.md`](lark-base-cell-value.md) 中 Record 场景的附件写法不同：Record 写入时附件走独立的 `+record-upload-attachment` 命令；而 `+form-submit` 只需在 `attachments` 中传本地路径，上传由 CLI 内部自动完成。

### 从分享链接提取 share-token

用户提供形如以下格式的表单分享链接时：

```
https://www.example.com/share/base/form/shrbcvST8eZy0vk8zjVZ1CAXNye
```

**提取方式：** 取 URL 路径最后一段作为 `--share-token`。

以上述链接为例：

- `share-token` = `shrbcvST8eZy0vk8zjVZ1CAXNye`

```bash
lark-cli base +form-submit \
  --share-token shrbcvST8eZy0vk8zjVZ1CAXNye \
  --json '{"fields":{...}}'
```

## 输出格式

| 字段 | 类型 | 说明 |
|------|------|------|
| `can_submit_again` | bool | 是否可以再次填写 |

```json
{
  "ok": true,
  "data": {
    "can_submit_again": true
  }
}
```

## 提示

- 本命令仅支持通过表单分享链接（share_token）提交，不支持通过 base_token + table_id + view_id 方式提交
- **当 `--json` 包含 `attachments` 时，必须额外提供 `--base-token`**，因为附件上传到 Base Drive Media 需要指定目标 Base
- 附件字段只需在 `--json.attachments` 中提供本地路径即可，CLI 自动完成校验、并行上传、Token 获取和合并写入
- 限流：单应用 20 QPS，单用户 5 QPS
- 权限要求：`base:form:update`；使用 attachments 时还需 `docs:document.media:upload`

## 参考

- [lark-base](../SKILL.md) — 多维表格全部命令
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
- [lark-base-form](references/lark-base-form.md) — 表单管理总览
