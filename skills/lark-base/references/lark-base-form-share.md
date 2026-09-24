# base +form-share-get / +form-share-update

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

> [!CAUTION]
> 这是**写入操作** — 执行前必须向用户确认。

表单分享控制"这张表单能不能被填、谁能填、填的人要不要登录、提交人身份是否匿名"。读取用 `+form-share-get`，修改用 `+form-share-update`。

## 何时使用

- 用户说"把表单发出去""开启/关闭分享""只让公司内部填""允许任何人填""匿名收集""必须登录才能提交"时。
- 题目内容、显隐顺序、页面布局、二维码都**不在**本组命令范围内：题目走 `+form-questions-*`，显隐顺序走 `+view-set-visible-fields`，布局走 `+form-display-settings-*`，二维码走 `+form-share-qrcode-*`。
- 分享链接本身通过 `+form-share-get` 的返回读取，不要自己拼 URL。

## 命令

`+form-share-update` **一次只接受一个字段**——这不是建议，是命令的本地校验：一次传多个会直接被拒绝（见下方「错误与重试」）。要改多项就分多条命令依次执行，每条之间不需要重新读取。

```bash
# 1) 先读现状
lark-cli base +form-share-get \
  --base-token <base_token> --table-id <table_id> --form-id <form_id>

# 2) 每条命令只改一个字段
lark-cli base +form-share-update \
  --base-token <base_token> --table-id <table_id> --form-id <form_id> --enabled=true

lark-cli base +form-share-update \
  --base-token <base_token> --table-id <table_id> --form-id <form_id> --access-scope anyone

lark-cli base +form-share-update \
  --base-token <base_token> --table-id <table_id> --form-id <form_id> --allow-anonymous=true

# 3) 写后回读验收
lark-cli base +form-share-get \
  --base-token <base_token> --table-id <table_id> --form-id <form_id>
```

## 参数

| 参数 | 命令 | 必填 | 类型 | 说明 |
|---|---|---|---|---|
| `--base-token` | 两条 | 是 | string | 表单所属 Base |
| `--table-id` | 两条 | 是 | string | 表单所属 Table |
| `--form-id` | 两条 | 是 | string | 表单 ID |
| `--enabled` | update | 四选一 | bool | 分享总开关 |
| `--access-scope` | update | 四选一 | string | 访问范围，见下表 |
| `--allow-anonymous` | update | 四选一 | bool | 提交人身份是否匿名 |
| `--require-login` | update | 四选一 | bool | 提交前是否必须登录 |

后四个是"四选一"：一次调用必须且只能出现其中一个。

`--access-scope` 的取值：

| 取值 | 含义 |
|---|---|
| `invite` | 仅被邀请的人可填 |
| `tenant` | 组织内成员可填 |
| `anyone` | 任何拿到链接的人可填 |

`--allow-anonymous` 管的是**提交人身份是否记名**，`--require-login` 管的是**填之前要不要登录**，两者互相独立，不要互相替代，也不要因为改了一个就顺手改另一个。

## 布尔 flag 必须写成等号形式

`--enabled` / `--allow-anonymous` / `--require-login` 三个布尔 flag **必须写成 `--enabled=true`、`--require-login=false` 这种等号形式**：

| 写法 | 实际结果 |
|---|---|
| `--enabled=true` / `--enabled=false` | ✅ 正确，分别发送 `true` / `false` |
| `--enabled true` / `--enabled false` | ❌ 都报 `positional arguments are not supported (got ["true"]); pass values via flags`（括号里是你写的那个值） |

两种空格写法**结果相同**，都在发请求前退出，不会改动任何设置——照报错把值挪到等号后面即可，原来写 `false` 就写 `--enabled=false`，不要一律改成 `=true`。

原因是布尔 flag 不吃后面那个值——`--enabled` 出现即为 `true`，紧跟的 `true` / `false` 会掉出去变成位置参数，被 runner 的位置参数校验拒绝。

非布尔 flag（`--access-scope`、`--base-token`、`--table-id`、`--form-id`）用空格分隔正常，写不写等号都行。

## 请求与返回

`+form-share-update` 是 PATCH：只有显式传入的字段才会进请求体，没传的字段服务端保持不变。`allow_anonymous` 和 `require_login` 包在 `settings` 里，`enabled` 和 `access_scope` 在顶层。

```json
{"enabled": true}
{"access_scope": "anyone"}
{"settings": {"allow_anonymous": true}}
{"settings": {"require_login": false}}
```

`+form-share-get` 回显分享状态、访问范围、settings 和分享链接，是唯一的验收证据——改完用它回读确认，不要只凭写请求的响应下结论。

## 错误与重试

| 情况 | 表现 | 处理 |
|---|---|---|
| 一次传了多个字段 | `share update accepts exactly one field; do not combine --enabled, --access-scope` | 拆成多条命令依次执行 |
| 一个字段都没传 | `exactly one share field must be provided` | 补上要改的那个 flag |
| 布尔 flag 用空格分隔 | `positional arguments are not supported (got ["true"])` | 改成 `--enabled=true` |
| `--access-scope` 传枚举外的值 | 本地校验直接拒绝 | 只用 `invite` / `tenant` / `anyone` |

分享没开启时（`enabled` 为 false），`+form-share-qrcode-get` / `+form-share-qrcode-download` 拿不到二维码；要二维码先把分享开起来。

## 参考

- [lark-base](../SKILL.md) — 多维表格全部命令
- [lark-base-form-share-qrcode.md](lark-base-form-share-qrcode.md) — 分享开启后取二维码
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
