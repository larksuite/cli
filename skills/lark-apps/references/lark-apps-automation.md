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
- 「启用 / 启动已有 trigger」→ 先核对现有状态；只启用时不要修改源码或发布应用。
- 「换 / 重置 webhook 回调地址 / URL」→ `+automation-update --reset-url --app-env <preview|runtime>`
- 「换 / 重置 / 轮换 webhook token / bearer」→ `+automation-update --reset-token`
- 「触发器没反应 / enable 了不触发 / 为什么没执行 / 验证一下触发器」→ 先按「未触发时的诊断顺序」诊断；对 UPSERT 和 feishu-approval 仅验证配置边界，不承诺 handler 或 live 验证。

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

## 本地全栈 Trigger 闭环

当用户希望触发器实际执行业务代码时，先确认当前工作区是已初始化的应用项目，并读取其中与触发器任务匹配的 guide。

`--name` 是应用内唯一的 trigger 定位键；代码侧绑定名称必须与它逐字相同。不得用 trigger ID 或方法名代替它。具体 handler 语法和接入方式以项目 guide 为准。

### 仅创建/配置触发器

适用于 cron、record-change、webhook 和 feishu-approval。用 `+automation-create` 创建，并省略 `--status` 或显式传 `disabled`，然后报告 name 和 disabled 状态。

不要传 `--status enabled`，也不要写 handler、commit/push、release 或 enable；更不能把创建 API 成功称为“可运行”。默认 disabled 是这个意图的终点，不是稍后自动 enable 的待办。

### 仅启用已有 disabled trigger

用户只要求启用已存在且 disabled 的 trigger、没有要求修改代码或制造真实 runtime 事件时，先用 `+automation-get` 核对 name、类型和 disabled 状态，再用 `+release-list --status finished --page-size 1` 核对是否存在已完成线上 release。release history 只能证明当前线上应用有已发布版本，不能证明该 trigger name 已绑定 handler。不存在 finished release 时说明 enable 只会改变配置状态、当前没有可执行的线上版本；存在时说明它会对当前线上应用激活这条 trigger 配置。随后按用户要求执行 `+automation-enable`，再用 `+automation-get` 确认 enabled。

这条路径不得修改 handler、commit/push 或 release。未发布时不得自动创建 release，也不得声称 trigger 已开始实际运行。即使存在 finished release，也只能把 enable 报告为配置激活；没有 handler 来源或 runtime 结果时，不得声称业务 handler 已存在、已运行或可用。若用户期待尚未发布的本地改动生效，或检查后发现确实需要新增/修改 handler，转到下方“实现或更新 handler 后发布并启动/测试”路径；不要为单纯 enable 发布整个 `sprint/default`。

对 UPSERT 或 feishu-approval 只改变配置状态；由于本 guide 没有其已证实的 handler、投递或 live 验证契约，启用后也不得声称业务代码已运行或触发器已实测可用。

### 测试已有线上 trigger（不改代码）

用户要求测试已经发布的 trigger、没有要求修改 handler 时，先用 `+automation-get` 核对 name、类型、当前状态，再用 `+release-list --status finished --page-size 1` 确认应用存在 finished release，并说明本次测试覆盖当前线上代码。没有 finished release 时停止 runtime test，只报告配置状态；不得为测试自动修改源码、commit/push 或 release。release history 不证明该 name 已绑定 handler，真实 probe 的结果才是本次验证证据；若用户期待本地未发布改动，改走代码变更闭环。

记录测试前状态，并在任何临时 enable 之前完成两类授权和全部 preflight：测试请求已明确包含临时 enable，或另行取得 enable 授权；同时按下方“运行时验证的操作级授权”确定具体事件、影响、载荷、观察结果和清理。原本 disabled 时完成这些门槛后才临时 enable，并在验证结束后恢复 disabled；原本 enabled 时不要无意义切换状态。原本为 disabled 时，无论 probe 成功、失败、结果不确定，还是临时 enable 后提前结束或中断，最终都必须 `+automation-disable` 并回读 disabled，不得停在 enabled。测试意图本身不决定数据库记录、Webhook 请求或其他事件载荷。

### 仅完成 handler（不发布/不启用）

仅对 cron、webhook、record-change 的 `INSERT`、`UPDATE`、`DELETE` 使用此路径。

创建或定位已明确 name 的 disabled trigger，读取项目 guide，按其要求实现同名业务 handler，完成本地验证。只在既有 Git 确认或预授权下 commit/push；停止在 `+release-create` 和 `+automation-enable` 之前。用户没有明确“发布好”时，先问，不能默认把完整应用上线。

### 把 handler 发布好，但先不要启动

仅对 cron、webhook、record-change 的 `INSERT`、`UPDATE`、`DELETE` 使用此路径。先用 `+automation-get` 定位；不存在时用 `+automation-create` 创建同名 disabled trigger，再次回读确认。已存在时记录它是否 enabled。按项目 guide 完成同名业务 handler 并本地验证后，commit、`git push origin sprint/default`。若 trigger 已 enabled，先说明发布前必须临时停用以及可能造成的运行中断，并取得这次临时停用授权；未获授权时停止在发布前。取得授权后，在发布前执行 `+automation-disable`，并再次用 `+automation-get` 确认 disabled。随后发布完整应用：

```bash
lark-cli apps +release-create --as user --app-id <app_id> --branch sprint/default
```

若 `+release-create` 本身返回错误或未返回 `data.release_id`：视为确认未创建本轮 release（新代码未上线），原本 enabled 的 trigger 恢复 enabled 并回读、原本 disabled 的保持 disabled，然后停止；若因超时等导致创建结果未知，保持 disabled，先用 `+release-list --status finished --page-size 1` 核对是否已产生新 release 再决定。取得 `data.release_id` 后，对**这一轮** ID 调用 `+release-get`：`publishing` 时每 20 秒继续轮询，整体最多约 5 分钟；超时且状态仍不确定时报告 `release_id` 和当前 status，并保持 disabled；只有 `data.status=finished` 才算完成。确认 `failed` 且新代码未上线时，原本 enabled 的 trigger 恢复 enabled 并回读，原本 disabled 的保持 disabled。release 是整个应用上线，可能影响既有线上功能；未获得启动或测试授权时，finished 后始终保持 disabled，不执行 `+automation-enable`。

### 实现或更新 handler 后发布并启动/测试

仅当本轮确实需要新增或修改 cron、webhook、record-change 的 `INSERT`、`UPDATE`、`DELETE` handler，且用户要求把这次代码发布后启动或测试时，才使用此路径。按以下不可跳过的顺序执行：

1. 用 `+automation-get` 定位并记录发布前状态，再核对其 `--name`、类型并读取项目 guide；不存在时用 `+automation-create` 创建同名 trigger 并保持默认 disabled。
2. 按项目 guide 完成同名业务 handler 并本地验证。
3. 在 Git 已确认/预授权时 commit，然后执行 `git push origin sprint/default`。
4. 若 trigger 当前 enabled，先说明发布前必须临时停用以及可能造成的运行中断，并取得这次临时停用授权；未获授权时停止在发布前。取得授权后执行 `+automation-disable`，并再次用 `+automation-get` 确认 disabled；原本 disabled 时不要无意义切换状态。
5. 执行 `+release-create --branch sprint/default`。若该命令本身返回错误或未返回 `data.release_id`：视为确认未创建本轮 release（新代码未上线），原本 enabled 的 trigger 恢复 enabled 并回读、原本 disabled 的保持 disabled 后停止；若因超时等导致结果未知，保持 disabled，先用 `+release-list --status finished --page-size 1` 核对是否已产生新 release 再决定。取得 `data.release_id` 后进入下一步。
6. 对该 ID 执行 `+release-get`，只有 `data.status=finished` 才能继续；`publishing` 时每 20 秒继续轮询，整体最多约 5 分钟。超时且状态仍不确定时停止本轮轮询、报告 `release_id` 和当前 status，并保持 disabled；确认 `failed` 时报告发布失败，原本 enabled 的 trigger 仅在确认新代码未上线后恢复 enabled，原本 disabled 的保持 disabled。发布状态仍不确定时不得进入 enable、probe 或状态恢复分支。`is_published=true` 不能代替这轮发布完成。
7. **仅启动**：取得持续启动授权后执行 `+automation-enable`，并用 `+automation-get` 确认 enabled；到此结束，不制造 runtime probe。
8. **测试（含“启动并测试”）**：先按下节“运行时验证的操作级授权”完成全部 preflight，包括具体事件、sibling 影响、载荷、观察结果和清理；完成前保持 disabled，之后才执行 `+automation-enable` 并回读，再由已授权主体制造真实 runtime 条件并核验业务结果。若同时明确要求持续启动，只有 probe 成功后才保持 enabled。
9. 若用户仅要求测试而不是持续启动，只在本轮 release 已 `finished` 且 probe 成功后恢复到发布前状态：原本 disabled 或本轮新建的 trigger `+automation-disable` 并回读；原本 enabled 的可保持 enabled。无论用户是仅测试还是启动并测试，probe 失败、结果不确定或 enable 后提前结束时，一律 `+automation-disable` 并回读 disabled；不得把“发布前 enabled”当作失败后的恢复依据，因为本轮新代码已经上线。只有旧 release 已回滚并验证，或修复后重新发布且 probe 成功，才可再次 enabled。恢复失败时明确报告当前状态。

没有通用的 `automation-debug` 或 trigger 日志 shortcut。缺少安全事件入口、匹配环境或可观察结果时，记录 blocked，不能编造测试成功。

### 运行时验证的操作级授权

启用 trigger 的授权不等于制造 runtime 事件的授权，测试授权也不等于任意数据库写入授权。cron 可等待计划时间；webhook 只能向既有 runtime URL 发送已授权、安全且不泄露凭证的请求。record-change 在执行任何 DML 前，必须明确并取得覆盖以下作用域的授权：环境、表、操作、精确测试记录或筛选条件、payload、预期结果和清理方式。

优先使用专用测试记录，不要任取线上业务记录。用户已明确授权精确、可撤回的测试夹具及其清理时，不机械追加一轮确认；目标或影响仍不清楚时必须停下。record-change probe 前先执行 `+automation-list --trigger-type record-change --all`，检查同一环境、表和操作可能命中的其他 enabled trigger；若存在 sibling match，必须说明聚合业务影响并取得覆盖这些影响的授权，或换成隔离夹具/经授权临时停用后再测。`UPDATE` 要限定精确条件并保留恢复方式；`INSERT` 要预先约定清理；恢复 UPDATE 或清理 INSERT 也可能再次触发自动化，必须纳入影响说明和授权。`DELETE` 必须遵循 [lark-apps-db-execute.md](lark-apps-db-execute.md)：先 `SELECT count(*)`、执行 `--dry-run`，展示影响后取得针对该删除目标的明确授权，再带 `--yes` 执行；清理动作若包含未预先授权的删除，同样走该门槛。

缺少安全、已授权且可清理的事件入口时，记录 blocked，不得用“测试一下”推导任意 online 数据写入。

### UPSERT 与飞书审批边界

record-change 的 UPSERT 可创建 disabled 配置，但当前没有已证实的运行时代码契约；不得静默按 UPDATE 处理，也不得承诺 handler 或 live 验证。

feishu-approval 可创建 disabled 配置，并读取或更新 `event_type`、对应 status 和可选 `approval_code`。当前没有已证实的运行时 handler 契约或实际投递验证；不要把 enable 或审批 API 成功称为业务代码已执行。

### 未触发时的诊断顺序

按 `--name` / 项目 guide 要求的代码接入 → 本轮 release `finished` → enabled 状态 → 类型条件、环境和已有日志的顺序排查。客户审批投递故障属于服务端事件投递排查，不要归因于此 SOP 或改写无关业务代码。

## 常见错误与决策场景

| 现象 / 用户意图 | 正确处理 |
|---|---|
| 创建报名字冲突（`--name` 应用内唯一） | 换名或加后缀重试 |
| cron 报非法 / 间隔过小 | 检查是否五段式、分钟字段是否 `*` 或 `*/n`(n<30) |
| `--reset-url` 报缺 app-env | 补 `--app-env preview` 或 `--app-env runtime` |
| 想把 cron 触发器改成 webhook（跨类型改） | update 不支持换类型，本 skill 也不提供删除。旧触发器只能 `+automation-disable` 停用（保留在应用里），另建一个 webhook 触发器；若要真正清理旧触发器，请到妙搭 web 手动删除 |
| 触发器 enable 了但不触发 | 已证实的 cron、webhook、record-change（INSERT/UPDATE/DELETE）按「未触发时的诊断顺序」排查；UPSERT 和 feishu-approval 仅核对配置边界，不承诺 handler 或 live 验证。 |
| 「token 泄露了」 | 优先 `+automation-update --reset-token --yes` 轮换（旧 token 立即失效），而非直接 disable-token 关校验 |
| 「回调 URL 泄露了」 | `+automation-update --reset-url --app-env <env> --yes` 轮换 |

## 不在本 skill 范围

- 审批定义查询、Webhook 消费端实现、实时触发日志 tail：本期不支持。
- 身份选择、权限不足处理、exit-10 审批、通用「禁输出密钥」红线、高风险操作通用框架：见 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)，不在此重复。
