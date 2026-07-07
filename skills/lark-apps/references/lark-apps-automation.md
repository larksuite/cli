# apps automation 触发器命令族 SOP

管理妙搭应用的自动化触发器（定时 / 记录变更 / Webhook / 飞书审批四类）。全部操作需 `--as user`（AuthType: user）。`--help` 是参数细节的完整来源；本文件只记录 Agent 不看就会做错的领域规则。

## 命令路由

| 命令 | 用途 | Risk |
|---|---|---|
| `+automation-list` | 列出应用所有触发器（可按类型过滤、`--all` 聚合翻页） | read |
| `+automation-get` | 查看单个触发器完整配置（Webhook Bearer Token 恒脱敏） | read |
| `+automation-create` | 创建触发器，四类共用一条命令，按 `--trigger-type` 分派 | write |
| `+automation-update` | 改条件/描述，或经专用 flag 管理 Webhook URL·Token | high-risk-write |
| `+automation-enable` | 启用触发器（`status→enabled`，开始自动触发） | write |
| `+automation-disable` | 停用触发器（`status→disabled`，停止触发，不删除） | write |

触发器以 **应用内唯一的 `--name`** 定位（不是 id）。所有单条命令都用 `--app-id` + `--name`；名字忘了先 `+automation-list` 查。

## 四类触发器 payload

`--trigger-type` 用面向 Agent 的 kebab-case（`cron` / `record-change` / `webhook` / `feishu-approval`），CLI 内部转 snake_case 下推。类型专属 flag 只在对应类型生效。

### cron（定时）

```
+automation-create --app-id <id> --name daily --trigger-type cron \
  --cron '0 9 * * *' [--timezone Asia/Shanghai]
```

- `--cron` 是**五段式**（`minute hour day month weekday`），非六段。
- **最小间隔 30 分钟**：`--cron '* * * * *'`（每分钟）或 `*/n`（n<30）会被 CLI 本地拦截报错；后端也会二次校验。
- `--timezone` 缺省补 `Asia/Shanghai`（IANA 时区名）。

### record-change（记录变更）

```
+automation-create --app-id <id> --name onUpd --trigger-type record-change \
  --table <table_id> --event UPDATE [--fields '["status"]']
```

- `--event` 是**大写枚举**：`INSERT` / `UPDATE` / `UPSERT` / `DELETE`（CLI 会 uppercase，但请按枚举传）。
- `--table` 为 dataloom 表 id，必填。
- `--fields` 是 JSON 字符串数组，仅对 `UPDATE`/`UPSERT` 有意义；`'["*"]'` 表示监听所有字段；不传表示不限定字段。

### webhook（外部回调）

```
+automation-create --app-id <id> --name hook --trigger-type webhook \
  [--white-ip-list '["1.1.1.1","2.2.2.2"]']
```

- 创建时可选 `--white-ip-list`（JSON 字符串数组）限制回调来源 IP。
- 回调 URL 分 **preview / runtime 两套**，创建时不回显；用 `+automation-get` 查当前配置，用 `+automation-update --reset-url --app-env <preview|runtime>` 轮换。
- Bearer Token 是回调鉴权凭证，见下方「凭证脱敏与一次性回显」。

### feishu-approval（飞书审批）

```
+automation-create --app-id <id> --name apv --trigger-type feishu-approval \
  --event-type approval_instance --instance-status APPROVED [--approval-code <code>]
```

- `--event-type` 必填，取 `approval_instance` 或 `approval_task`，决定状态用哪套 flag：
  - `approval_instance` → `--instance-status`（可重复），合法枚举：`PENDING` `APPROVED` `REJECTED` `CANCELED` `DELETED` `REVERTED` `OVERTIME_CLOSE` `OVERTIME_RECOVER`
  - `approval_task` → `--task-status`（可重复），合法枚举：`REVERTED` `PENDING` `APPROVED` `REJECTED` `TRANSFERRED` `ROLLBACK` `DONE` `OVERTIME_CLOSE` `OVERTIME_RECOVER`
- 状态按 event-type 分桶校验；传错桶的状态会被 CLI 本地拦截。

## approval-code 获取路径

`--approval-code` **可选**。不传表示匹配所有审批定义；如需限定某个审批流程，`--approval-code` 从**飞书审批管理后台**获取（触发器 OpenAPI 不提供审批定义查询能力）。

## 凭证脱敏与一次性回显（安全关键）

- `+automation-get` / `+automation-list`：**恒不返回明文 Bearer Token**——`trigger_condition.token_value` 被抹为 `null`。用户想知道「token 是什么」时，list/get 都查不到明文。
- `+automation-update --enable-token` / `--reset-token`：明文 Bearer Token **仅当次 stdout 回显一次**，同时 stderr 打印一次性告警：
  ```
  warning: this bearer token is shown only once and is NOT stored by lark-cli — copy it now and store it in your own secret manager.
  ```
- Webhook URL 同理：`--reset-url` 后新 URL 仅当次回显一次，旧 URL 立即失效。
- CLI 不落盘任何明文 token/URL（不写 cache / config / recent / debug log / 错误信息）。
- **Token 丢失只能 reset**：找不回，唯一恢复方式是 `+automation-update --reset-token`（旧 token 同时失效）。

## 高危确认

`+automation-update` 整体是 `high-risk-write`，任何一次调用都需显式 `--yes`；缺少时框架会要求确认（退出码 10）。**不要自动补 `--yes`**——需用户明确确认后再加。以下 Webhook 动作 flag 尤其不可逆：

- `--reset-url`（旧回调 URL 立即失效，需配 `--app-env preview|runtime`）
- `--reset-token`（旧 token 立即失效）
- `--disable-token`（关闭 token 校验，**不可逆**）

四个 Webhook 动作 flag（`--reset-url` / `--enable-token` / `--disable-token` / `--reset-token`）**每次只能传一个**。不确定影响时先跑 `--dry-run` 看将发出的请求（不含明文）。

## ⚠️ 安全告警：无鉴权公网回调组合态

`--disable-token`（关闭 Bearer Token 校验，不可逆）**叠加** `--white-ip-list '[]'`（清空 IP 白名单）会让 Webhook 触发器进入「**无鉴权公网回调**」组合态——**任何来源都能触发该 Webhook**，没有任何一道防线拦截。

- 两道防线：Token 校验（谁能调）+ IP 白名单（从哪能调）。**不要同时关闭这两道防线。**
- 若确需关闭 Token（例如对端无法带 Bearer 头），务必**保留 IP 白名单**收敛来源；反之若要放开 IP，务必**保留 Token 校验**。
- 用户要求同时做这两件事时，先明确复述这个组合态的后果并请其确认，不擅自执行。

## 默认 disabled

`+automation-create` 创建后触发器**默认 disabled**，不会自动触发。需 `+automation-enable` 才开始按条件自动运行（且触发器执行的是**线上已发布**的应用代码——应用未发布时即便 enable 也不会有实际效果）。

## 常见错误与决策场景

| 现象 / 用户意图 | 正确处理 |
|---|---|
| 创建报名字冲突（`--name` 应用内唯一） | 换名或加后缀重试 |
| cron 报非法 / 间隔过小 | 检查是否五段式、分钟字段是否 `*` 或 `*/n`(n<30) |
| `--reset-url` 报缺 app-env | 补 `--app-env preview` 或 `--app-env runtime` |
| 想把 cron 触发器改成 webhook（跨类型改） | update 不支持换类型；需删旧建新（新建一个 webhook 触发器） |
| 触发器 enable 了但不触发 | 确认应用**已发布**；触发器跑的是线上已发布代码 |
| 「token 泄露了」 | 优先 `+automation-update --reset-token --yes` 轮换（旧 token 立即失效），而非直接 disable-token 关校验 |
| 「回调 URL 泄露了」 | `+automation-update --reset-url --app-env <env> --yes` 轮换 |

## 不在本 skill 范围

- 审批定义查询、Webhook 消费端实现、实时触发日志 tail：本期不支持。
- 身份选择、权限不足处理、exit-10 审批、通用「禁输出密钥」红线、高风险操作通用框架：见 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，不在此重复。
