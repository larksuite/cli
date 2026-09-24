# base +form-query-page-get / +form-query-page-create

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

> [!CAUTION]
> 这是**写入操作** — 执行前必须向用户确认。

表单查询页让提交者在提交之后回查自己填过的内容。它由一个查询视图承载，视图 ID 就是 `query_view_id`；一个表单最多关联一个查询页。读取用 `+form-query-page-get`，创建或恢复关联用 `+form-query-page-create`。

## 何时使用

- 用户要给表单开"提交后可查询/可修改自己的记录"的入口时，先读后写。
- 收到一次创建失败、需要判断"到底建出来没有、要不要重建"时，本文档的恢复流程是唯一正确解法。
- 只想改题目、分享开关或页面布局时不要用本组命令，分别走 `+form-questions-*`、`+form-share-*`、`+form-display-settings-*`。

## 工作流：先 GET，再决定 POST 怎么发

这条顺序是硬要求，不能跳：

1. 先执行 `+form-query-page-get`。
   - `configured=false` 且 `query_view_id=null`：还没有查询页，可以新建。
   - `configured=true`：已经有查询页，直接把返回的 `query_view_id` 报告给用户，**不要再创建**，再创建会多出一个查询页。
2. 需要新建时，执行不带 `--query-view-id` 的 `+form-query-page-create`，请求体是 `{}`，响应里 `reused=false`。
3. **如果创建已经尝试过一次并失败**，从报错的 `hint` 里取出 `query_view_id`（形如 `error.value={"failed_step":"associate","query_view_id":"vewQuery"}`），用 `--query-view-id <id>` 再请求一次来恢复关联，响应里 `reused=true`。
   **绝不能**用新的空 POST 去"透明重建"——那会新建第二个查询页，把上一轮半成品变成孤儿视图。

```bash
# 1. 查当前关联
lark-cli base +form-query-page-get \
  --base-token <base_token> --table-id <table_id> --form-id <form_id>

# 2. 未关联时新建
lark-cli base +form-query-page-create \
  --base-token <base_token> --table-id <table_id> --form-id <form_id>

# 3. 上一次创建失败后，用同一个 ID 恢复
lark-cli base +form-query-page-create \
  --base-token <base_token> --table-id <table_id> --form-id <form_id> \
  --query-view-id vewQuery
```

## 参数

| 参数 | 命令 | 必填 | 说明 |
|---|---|---|---|
| `--base-token` | 两条 | 是 | 表单所属 Base |
| `--table-id` | 两条 | 是 | 表单所属 Table |
| `--form-id` | 两条 | 是 | 表单 ID |
| `--query-view-id` | create | 否 | 已有查询视图 ID。不传就发 `{}` 新建；传了表示用这个视图恢复关联 |

## 返回结构

`+form-query-page-get` 的 `data`：

```json
{"query_view_id": "vewQuery", "configured": true}
```

未关联时：

```json
{"query_view_id": null, "configured": false}
```

`+form-query-page-create` 的 `data` 在上面两个字段之外多一个 `reused`：

```json
{"query_view_id": "vewQuery", "configured": true, "reused": false}
```

- `reused=false`：这次是首次新建。
- `reused=true`：这次是用传入的 `query_view_id` 恢复了关联，没有新建视图。

## 参数边界

`--query-view-id` 要么不传（新建），要么传一个非空 ID（恢复）。传空串会被 CLI 本地拒绝并提示省略该 flag，请求不会发出。

## 错误与重试

创建失败时，恢复所需的信息由 CLI 并进报错的 `hint`，形如：

```text
error.value={"completed_steps":["create"],"failed_step":"associate","query_view_id":"vewQuery"}
```

**不要去错误输出里找 `data` 字段**：CLI 的错误信封只有固定字段，服务端响应的 `data` 整块在错误分支被丢弃，这三个键是被单独摘出来并进 `hint` 的（服务端侧的原始位置是 `data.error.value`）。把 `hint` 里 `error.value=` 后面那段按 JSON 解析即可。

| 字段 | 含义 |
|---|---|
| `query_view_id` | 已经建出来的查询视图 ID，重试时必须原样回传给 `--query-view-id` |
| `completed_steps` | 已完成的步骤列表 |
| `failed_step` | 失败所处的步骤 |

步骤枚举共三个，按执行顺序：

| 步骤 | 含义 |
|---|---|
| `create` | 创建查询视图 |
| `associate` | 把查询视图关联到表单 |
| `readback` | 回读校验关联结果 |

重试规则：

- `failed_step` 是 `associate` 或 `readback` 时，视图已经建出来了，**必须**带 `--query-view-id` 重试。
- `failed_step` 是 `create` 且 `value` 里没有 `query_view_id` 时，才可以用空 POST 重新新建。
- **只看到 `query_view_id`、没有 `failed_step` 时，一律按「必须带 ID 重试」处理**：恢复标识过长时 CLI 会退化成只带这一个键。拿到了服务端错误体时，能不能重建的判据是有没有 `query_view_id`，不是 `failed_step` 缺不缺；没拿到错误体的情况见下一条。
- **超时或网络错误、没拿到服务端错误体时，视为状态不确定**：此时 hint 里没有 `error.value`，但不代表服务端没建出视图，**不许**据此发空 POST。先回到第 1 步执行 `+form-query-page-get`：`configured=true` 就直接报告返回的 `query_view_id`；仍是 `configured=false` 时，如实告诉用户上一次创建结果未知、可能留下未关联的查询视图，由用户决定是否再创建。
- 任何情况下都不要"先删再建"，也不要在不确定状态下连发多次空 POST。

## 参考

- [lark-base](../SKILL.md) — 多维表格全部命令
- [lark-base-form-share.md](lark-base-form-share.md) — 表单分享开关与访问范围
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
