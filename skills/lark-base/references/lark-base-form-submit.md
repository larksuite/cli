# base +form-submit

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

通过表单分享链接填写并提交多维表格表单。仅支持分享模式（share_token），支持填写普通字段值和上传本地文件作为附件。

> **⚠️ 高风险写操作（high-risk-write）：** 本命令会向表单写入并提交数据，属于高风险写操作，必须额外传递 `--yes` 进行确认，否则会返回 `confirmation_required` 错误并退出。当用户明确要求提交且目标表单无歧义时，直接附加 `--yes`，无需再次询问。

## 填写前必读：先获取表单详情

**在调用 `+form-submit` 之前，必须先使用 `+form-detail` 获取表单详情。** 原因如下：

1. **字段类型匹配**：每个题目的 `type` 决定了值的格式（文本、数字、选项、人员、日期等），需根据类型正确构造 `fields` 中的值
2. **必填校验**：通过 `questions[].required` 判断哪些题目为必填项，避免遗漏
3. **显示条件过滤**：部分题目带有 `filter`（显示/隐藏逻辑），需根据用户已填的其他题目值判断该题目是否应该出现——**不应填写被 filter 隐藏的题目**
4. **获取 base_token（附件场景必用）**：`+form-detail` 返回的 `data.base_token` 是该表单所属的多维表格标识。当表单包含附件字段时，提交时必须通过 `--base-token` 传入此值，因为附件需要上传到该 Base 的 Drive Media 中

典型流程：

```bash
# 1️⃣ 先获取表单详情，了解所有题目
lark-cli base +form-detail --share-token <share_token>

# 2️⃣ 根据返回的 questions 列表，按 type 格式化值、检查 required、判断 filter 条件

# 3️⃣ 再提交（高风险写操作，必须带 --yes）
lark-cli base +form-submit \
  --share-token <share_token> \
  --json '{"fields":{...}}' \
  --yes
```

`+form-detail` 的返回中要重点读取 `questions[].type`、`questions[].required`、题目 `filter` 和附件场景所需的 `data.base_token`。

## 命令

```bash
# 基本提交（填写普通字段）
lark-cli base +form-submit \
  --share-token <share_token> \
  --json '{"fields":{"服务评分":5,"评价内容":"服务态度好"}}' \
  --yes

# 带附件提交（需要额外提供 --base-token）
lark-cli base +form-submit \
  --share-token <share_token> \
  --base-token <base_token> \
  --json '{"fields":{"服务评分":5,"评价内容":"好"},"attachments":{"附件字段名":["./report.pdf","./photo.png"],"另一个附件字段":["./doc.docx"]}}' \
  --yes

# 使用应用身份（bot）
lark-cli base +form-submit \
  --share-token <share_token> \
  --json '{"fields":{...}}' \
  --as bot \
  --yes

# 预览 API 调用（不实际执行，dry-run 无需 --yes）
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
| `--timezone <IANA>` | 否 | 填写人所在时区，如 `Asia/Shanghai`。**只有当表单里的日期题设了「相对日期」范围（今天 / 本周 / 本月）时才需要**，它决定这些边界算在哪一天。详见下方「什么时候要传 --timezone」 |
| `--yes` | 是 | 确认高风险写操作。本命令为 high-risk-write，不带 `--yes` 会返回 `confirmation_required` |
| `--format` | 否 | 输出格式：json（默认）\| pretty \| table \| ndjson \| csv |
| `--as` | 否 | 身份：user（默认）\| bot |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

### --json 结构说明

`--json` 是一个 JSON 对象，包含两个部分：

#### fields（普通字段）

`fields` 中的常见单元格值按下方示例构造（与主 skill 一致）：

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

电话号码题的值必须是字符串。当前提交链路接受数字、`+`、`-`、空格和圆括号，并要求至少包含一个数字，例如 `13800000000`、`+86 (138) 0000-0000`；`abc123`、`---` 或非字符串会被拒绝。失败时保留用户原值和 Phone 字段语义，不要改写号码，也不要改用普通文本字段重试。

> **注意：附件类型字段不要写在 `fields` 里。** `fields` 中不包含附件，附件有独立的填写方式，见下方「attachments（附件上传）」章节。

超链接题的值是完整的 URL 字符串（如上例 `"https://example.com"`），不是对象；非法 URL 会被拒绝。

> 自动编号、创建/修改人、创建/修改时间等系统字段会自动填入，无需手动传入。

> **不要在 `fields` 里给公式、查找引用题传值。** 这两类题目由系统计算，填写人不作答（`+form-detail` 里识别方法见 [lark-base-form-detail.md](lark-base-form-detail.md)）。网页端提交会静默忽略这类值，CLI 链路会拒绝整次提交，报 `unsupported field type: <类型码>, field name: <题名>`（参数校验类错误，不落库）；遇到时从 `fields` 中去掉这些题目再提交，不要换写法尝试赋值。

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

> Record 写入时附件走独立的 `+record-upload-attachment` 命令；`+form-submit` 则在 `attachments` 中传本地路径，由 CLI 自动上传。

#### 签字题

签字题（`attachment` + `style.type=signature`）和附件题一样放在 `attachments` 里，值为本地 **png** 图片路径数组，CLI 同样先上传、再合并提交（需要 `--base-token`）：

```json
{
  "attachments": {
    "本人签字": ["./signature.png"]
  }
}
```

- `+form-detail` 里签字题显示为普通 `attachment`，无法与附件题区分；要确认某题是否为签字题，用 `+form-questions-list` 查看是否回显 `"style":{"type":"signature"}`。没有管理权限读不到题目列表时，按题目标题和用户说明判断，并向用户索取签名图片。
- 只接受 PNG 文件。其他格式或超过张数上限会被服务端拒绝（不落库），但报错可能不直接说明原因，可能只是通用的提交失败；提交前先核对文件格式与张数，被拒时从文件格式和张数排查，不要换写法反复重试。
- **只能使用用户提供的签名图片。** 用户没有提供时，向用户索取；不得自行生成、绘制、合成或伪造签名图片，也不得拿其他图片代替。
- 签字题必填时不传会被拒绝；用户拿不出签名图片时如实报告无法提交，不要绕过。

### 什么时候要传 --timezone

只有同时满足这两点才需要传：

1. 表单里有日期题，且该题的填写范围用的是**相对日期**（今天 / 本周一 / 本月初这类会随当天变化的边界）——用 `+form-detail` 或 `+form-questions-list` 看该题的 `input_range_limit.earliest/latest` 是否为 `dynamic`；如果是 `fixed_ms`（写死的具体日期），不需要传
2. 你确切知道填写人在哪个时区

不传时服务端按这个顺序决定：Base 自己设了文档时区就用文档时区（这时传了也会被忽略）；都没有则回落东八区。

`--timezone` 接受 IANA 时区名，按规范大小写书写，例如 `Asia/Shanghai`、`America/New_York`、`UTC`；CLI 会在本地校验这个名字，报错时照提示改成规范写法即可。

```bash
lark-cli base +form-submit \
  --share-token <share_token> \
  --timezone Asia/Shanghai \
  --json '{"fields":{"交付日期":"2026-09-16 12:00:00"}}' \
  --yes
```

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
  --json '{"fields":{...}}' \
  --yes
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

- **本命令为高风险写操作（high-risk-write），必须额外传递 `--yes` 确认**，否则返回 `confirmation_required` 并以非零码退出；`--dry-run` 预览除外
- **提交结果不确定时先回读、再决定是否重试**：返回 `800004014`、超时或网络类错误时，服务端可能已经落库（带附件提交时服务端处理更久，调用方可能先超时报错而记录已写入）。不要直接重提：先按本次填写的测试标记或能唯一定位该次提交的字段值回读表单底表（`+record-search` / `+record-list`）；第一次没读到时间隔几秒再回读一次（带附件时服务端可能仍在落库），两次都确认没有落库才重试一次；已经落库就沿用该记录并在答复中说明，不要重复提交。当前身份读不到底表、或无法唯一定位本次提交的记录时，报告提交结果未知，不要自动重提。参数校验类错误（如 `800004006`、`800004025`）表示未落库，按报错修正后再提交
- 本命令仅支持通过表单分享链接（share_token）提交，不支持通过 base_token + table_id + view_id 方式提交
- **当 `--json` 包含 `attachments` 时，必须额外提供 `--base-token`**，因为附件上传到 Base Drive Media 需要指定目标 Base
- 附件字段只需在 `--json.attachments` 中提供本地路径即可，CLI 自动完成校验、并行上传、Token 获取和合并写入
- 限流：单应用 20 QPS，单用户 5 QPS
- 权限要求：`base:form:update`；使用 attachments 时还需 `docs:document.media:upload`

## 参考

- [lark-base](../SKILL.md) — 多维表格全部命令
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
