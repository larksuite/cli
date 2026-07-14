# apps automation 触发器命令族 SOP

管理妙搭应用的自动化触发器（定时 / 记录变更 / Webhook / 飞书审批四类）。全部操作需 `--as user`（AuthType: user）。`--help` 是参数细节的完整来源；本文件只记录 Agent 不看就会做错的领域规则。

## 何时用本 skill（路由锚点）

**当用户消息里出现「妙搭应用名 / app_id」+ 以下任一意图，路由本 skill，不要走 lark-event 或 lark-openapi-explorer：**

- 「（每天 / 定时 / 每 N 小时 / 每周 X）自动跑 / 自动触发 / 定时同步」→ `+automation-create --trigger-type cron`
- 「数据表 / 记录 / 表里 X 字段（新增 / 更新 / 删除 / 变化）时（触发 / 通知 / 处理）」→ `+automation-create --trigger-type record-change`
- 「（webhook / 外部回调 / 外部系统调用 / HTTP 触发）」→ `+automation-create --trigger-type webhook`
- 「（审批 / 报销 / 请假 / 出差）（通过 / 拒绝 / 提交 / 撤回）后自动 X」→ `+automation-create --trigger-type feishu-approval`
- 「这个应用配了哪些（自动化 / 触发器 / 定时任务）」→ `+automation-list`
- 「（暂停 / 停用 / 先别自动跑 / 关掉自动触发）某个（触发器 / 定时任务 / 自动化）」→ `+automation-disable`（不是 update 改条件、不是 delete——本 skill 不提供删除）
- 「换 / 重置 webhook 回调地址 / URL」→ `+automation-update --reset-url --app-env <preview|runtime>`
- 「换 / 重置 / 轮换 webhook token / bearer」→ `+automation-update --reset-token`

**边界（防误路由）**：`lark-event` 是**实时事件流消费**（agent 长连接订阅事件），不管妙搭应用触发器的**配置**；用户说「配 / 设置一个触发器」而不是「订阅事件流」时，本 skill 才是正确选择。「审批通过触发」在妙搭应用语境下属于本 skill 的 `feishu-approval` 类型，不是 lark-event。

### 回应「怎么配」类问题的正确姿势

用户问「怎么配 / 怎么设置一个 X 触发器」时，**先展示完整命令模板 + 你对核心参数的推断**（让用户能确认你理解对了），再追问缺失的必填项（`--name` 之类）或可选项。**不要跳过展示、直接连环追问**，那样用户没法确认你有没有理解意图。

示范：用户说「报销审批一旦通过就自动触发处理，怎么配？」
- ✅ 正确：先写出「这是 feishu-approval 类型，命令模板：`apps +automation-create --app-id <id> --name <name> --trigger-type feishu-approval --event-type approval_instance --instance-status APPROVED [--approval-code <code>]`。需要你确认：(1) 触发器名 `<name>`；(2) 是否限定特定审批流程——限定就传 `--approval-code`（从飞书审批管理后台拿），不传则匹配所有审批定义」。
- ❌ 错误：直接问「叫什么名字？监听哪个审批？」——用户没法确认你有没有把「审批通过」映射到 `--event-type approval_instance --instance-status APPROVED`。

同理，cron/record-change/webhook 三类的「怎么配」都遵循此模式：先给命令 + 参数推断，后追问缺项。

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

```bash
+automation-create --app-id <id> --name daily --trigger-type cron \
  --cron '0 9 * * *' [--timezone Asia/Shanghai]
```

- `--cron` 是**五段式**（`minute hour day month weekday`），非六段。
- **最小间隔 30 分钟**：`--cron '* * * * *'`（每分钟）或 `*/n`（n<30）会被 CLI 本地拦截报错；后端也会二次校验。
- `--timezone` 缺省补 `Asia/Shanghai`（IANA 时区名）。

### record-change（记录变更）

```bash
+automation-create --app-id <id> --name onUpd --trigger-type record-change \
  --table <table_name> --event UPDATE [--fields '["status"]']
```

- `--event` 是**大写枚举**：`INSERT` / `UPDATE` / `UPSERT` / `DELETE`（CLI 会 uppercase，但请按枚举传）。
- `--table` 是应用数据库里的**表名**（对应 `+db-table-list` / `+db-table-get` 输出里 `.name` 字段的值），必填。妙搭应用的 dataloom 表以名称作为稳定标识符，没有独立的 `table_id`。
- `--fields` 是 JSON 字符串数组，仅对 `UPDATE`/`UPSERT` 有意义；`'["*"]'` 表示监听所有字段；不传表示不限定字段。

### webhook（外部回调）

```bash
+automation-create --app-id <id> --name hook --trigger-type webhook \
  [--white-ip-list '["1.1.1.1","2.2.2.2"]']
```

- 创建时可选 `--white-ip-list`（JSON 字符串数组）限制回调来源 IP。
- 回调 URL 分 **preview / runtime 两套**，创建时不回显；用 `+automation-get` 查当前配置，用 `+automation-update --reset-url --app-env <preview|runtime>` 轮换。
- Bearer Token 是回调鉴权凭证，见下方「凭证脱敏与一次性回显」。

### feishu-approval（飞书审批）

```bash
+automation-create --app-id <id> --name apv --trigger-type feishu-approval \
  --event-type approval_instance --instance-status APPROVED [--approval-code <code>]
```

- `--event-type` 必填，取 `approval_instance` 或 `approval_task`，决定状态用哪套 flag：
  - `approval_instance` → `--instance-status`（可重复）
  - `approval_task` → `--task-status`（可重复）
- **领域规则**：状态按 `event-type` 分桶校验，两桶枚举**不完全相同**（`PENDING`/`APPROVED`/`REJECTED`/`REVERTED`/`OVERTIME_CLOSE`/`OVERTIME_RECOVER` 两桶共享；`TRANSFERRED`/`ROLLBACK`/`DONE` 仅 task 有；`CANCELED`/`DELETED` 仅 instance 有）；传错桶的状态会被 CLI 本地拦截，错误信息会打印该桶的合法值列表。具体枚举见命令 `--help`。

## approval-code 获取路径

`--approval-code` **可选**。不传时匹配所有审批定义；要限定某个审批流程时，从**飞书审批管理后台**获取具体的 code 传给它。触发器 OpenAPI 不提供审批定义查询能力，具体 code 需去审批管理后台查。

## 凭证脱敏与一次性回显（安全关键）

- `+automation-get` / `+automation-list`：**恒不返回明文 Bearer Token**——`trigger_condition.token_value` 被抹为 `null`。用户想知道「token 是什么」时，list/get 都查不到明文。
- `+automation-update --enable-token` / `--reset-token`：明文 Bearer Token **仅当次 stdout 回显一次**，同时 stderr 打印一次性告警：
  ```text
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

### 执行前必须完成的确认步骤（高危写强制协议）

**在带 `--yes` 执行任何高危写之前，Agent 必须先完成以下 3 件事**，缺一不可——即使用户口气很急、即使命令一眼就明：

1. **确认目标唯一**：不允许"猜名字"或"批量试所有可能的名字"。若不确定 `--name`，先 `+automation-list --app-id <id>` 让用户在候选中点名；`--name` 不明的绝不执行写操作，更不要 for 循环批量试。
2. **确认可选参数已定**：`--reset-url` 必须由用户明确指定 `--app-env preview` 还是 `runtime`；不要默认取 runtime 或 preview。同一触发器的 preview/runtime 是两条独立的 URL，误重置另一条不可回退。
3. **告知不可逆后果并等确认**：把即将发生的 3 件事复述给用户——（a）旧 URL/Token 立即永久失效；（b）新 URL/Token 仅当次回显一次、CLI 不保存；（c）本次操作无法撤销——等用户回复"确认"再加 `--yes` 跑。

只要有一项没做，就先跟用户对齐、不要执行。这些是 skill 层的护栏，不是 CLI 层的（CLI 只强制 `--yes`，不强制上面 3 件事）。

## ⚠️ 安全告警：无鉴权公网回调组合态

`--disable-token`（关闭 Bearer Token 校验，不可逆）**叠加** `--white-ip-list '[]'`（清空 IP 白名单）会让 Webhook 触发器进入「**无鉴权公网回调**」组合态——**任何来源都能触发该 Webhook**，没有任何一道防线拦截。

- 两道防线：Token 校验（谁能调）+ IP 白名单（从哪能调）。**不要同时关闭这两道防线。**
- 若确需关闭 Token（例如对端无法带 Bearer 头），务必**保留 IP 白名单**收敛来源；反之若要放开 IP，务必**保留 Token 校验**。
- 用户同时要求「关 token 校验 + 清空 IP 白名单」时，Agent 的正确响应是**在识别到该请求的第一时间**（不要等命令跑失败才补警告）向用户输出以下 3 件事，再等确认——不要只描述"没有任何防线"就停下：
  1. 复述后果：这会形成无鉴权公网回调，任何来源都能触发。
  2. **主动给出替代方案**：明确建议"要么只关 Token 保留 IP 白名单，要么只放开 IP 保留 Token"，让用户在保留一道防线的两条备选里选一条。
  3. 只有用户明确回复"我理解风险、就是要两道都关"时，才继续按高危写协议（见上节「执行前必须完成的确认步骤」）走。

## 默认 disabled

`+automation-create` 创建后触发器**默认 disabled**，不会自动触发。需 `+automation-enable` 才开始按条件自动运行（且触发器执行的是**线上已发布**的应用代码——应用未发布时即便 enable 也不会有实际效果）。

**Agent 行为约束**：用户只说"创建/配一个触发器"时，**不要**主动在同一个 turn 里 `+automation-enable`。让用户自己在下一轮决定是否启用；主动启用会：
- 让 webhook 类型立即可被外部调用（原本用户可能只是想"备好 URL 稍后用"）
- 让 cron 到点真实触发（原本用户可能想"先建好观察配置"）
- 让 record-change 立即响应表变更

创建成功后的推荐话术：`已创建 <name>，当前 disabled；需要真正开始自动运行时告诉我，我用 +automation-enable 启用它。` **不要**在创建成功后立即启用，即使 skill 里说"需 enable 才自动触发"——这条是给用户的说明，不是给 agent 的行动指令。

## 常见错误与决策场景

| 现象 / 用户意图 | 正确处理 |
|---|---|
| 创建报名字冲突（`--name` 应用内唯一） | 换名或加后缀重试 |
| cron 报非法 / 间隔过小 | 检查是否五段式、分钟字段是否 `*` 或 `*/n`(n<30) |
| `--reset-url` 报缺 app-env | 补 `--app-env preview` 或 `--app-env runtime` |
| 想把 cron 触发器改成 webhook（跨类型改） | update 不支持换类型，本 skill 也不提供删除。旧触发器只能 `+automation-disable` 停用（保留在应用里），另建一个 webhook 触发器；若要真正清理旧触发器，请到妙搭 web 手动删除 |
| 触发器 enable 了但不触发 | 确认应用**已发布**；触发器跑的是线上已发布代码 |
| 「token 泄露了」 | 优先 `+automation-update --reset-token --yes` 轮换（旧 token 立即失效），而非直接 disable-token 关校验 |
| 「回调 URL 泄露了」 | `+automation-update --reset-url --app-env <env> --yes` 轮换 |

## 不在本 skill 范围

- 审批定义查询、Webhook 消费端实现、实时触发日志 tail：本期不支持。
- 身份选择、权限不足处理、exit-10 审批、通用「禁输出密钥」红线、高风险操作通用框架：见 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，不在此重复。
