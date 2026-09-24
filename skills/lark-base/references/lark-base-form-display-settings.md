# base +form-display-settings-get / +form-display-settings-update

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

> [!CAUTION]
> 这是**写入操作** — 执行前必须向用户确认。

表单展示设置控制填写页的分页方式：所有题目在一页里连续排列（传统模式），还是一次只显示一题（一页一题）。读取用 `+form-display-settings-get`，切换用 `+form-display-settings-update`。

## 何时使用

- 用户说"一页一题""一次只显示一道题""改回一页显示全部""传统表单样式"时。
- 改完布局要向用户确认现状时，用 GET 独立核实，不要只凭写请求的响应下结论。
- 主题色和背景当前 CLI 不支持，如实告诉用户，不要找别的命令代替；表单标题和描述用 `+form-update`；题目本身走 `+form-questions-*`。

## 命令

```bash
# 读取当前布局
lark-cli base +form-display-settings-get \
  --base-token <base_token> --table-id <table_id> --form-id <form_id>

# 切换成一页一题（不做版本校验）
lark-cli base +form-display-settings-update \
  --base-token <base_token> --table-id <table_id> --form-id <form_id> \
  --display-mode one_question_per_page

# 切回传统模式，并要求版本必须仍是 12
lark-cli base +form-display-settings-update \
  --base-token <base_token> --table-id <table_id> --form-id <form_id> \
  --display-mode traditional --revision 12
```

## 参数

| 参数 | 命令 | 必填 | 说明 |
|---|---|---|---|
| `--base-token` | 两条 | 是 | 表单所属 Base |
| `--table-id` | 两条 | 是 | 表单所属 Table |
| `--form-id` | 两条 | 是 | 表单 ID |
| `--display-mode` | update | 是 | `traditional` 或 `one_question_per_page` |
| `--revision` | update | 否 | 期望的当前版本号（整数）。传了就必须与服务端一致，不传则跳过版本校验 |

`--display-mode` 只有两个取值：

| 取值 | 含义 |
|---|---|
| `traditional` | 所有题目在同一页连续显示 |
| `one_question_per_page` | 一页一题，填完一题翻到下一题 |

## 返回结构

GET 和 PATCH 的 `data` 结构一致：

```json
{"display_mode": "one_question_per_page", "revision": 13}
```

请求体形态：

```json
{"display_mode": "one_question_per_page", "revision": 12}
```

```json
{"display_mode": "traditional"}
```

`--revision` 走 PATCH 语义——只有显式传入时才会出现在请求体里，不传就完全不发这个字段。

## 输出口径

- `display_mode` 对外一律是字符串。SDK 内部另有一套数字枚举，**不要**把数字透出到回复、日志或文档里，也不要在解释时说"模式 1 / 模式 2"。
- 本命令只切分页方式。用户如果同时提到主题色、背景，要说明 CLI 当前不支持；提到标题或描述，改用 `+form-update`。
- 写成功后建议独立执行一次 `+form-display-settings-get` 核实 `display_mode` 和新的 `revision`，再向用户汇报。

## 错误与重试

| 情况 | 表现 | 处理 |
|---|---|---|
| `--revision` 与服务端当前版本不一致 | 返回 conflict | 重新 `+form-display-settings-get` 拿最新 `revision`，确认没有他人并发改动后再带新版本号重试 |
| `--display-mode` 传了枚举外的值 | 本地校验直接拒绝 | 只用 `traditional` / `one_question_per_page` |
| `--revision` 传负数 | 本地校验直接拒绝 | 省略该参数即可跳过版本校验 |

`revision` 每次成功写入都会推进，所以"带旧 revision 重试"必然继续 conflict；重试前一定要重新读一次。

## 参考

- [lark-base](../SKILL.md) — 多维表格全部命令
- [lark-base-form-questions-create.md](lark-base-form-questions-create.md) — 表单题目的创建与字段类型
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
