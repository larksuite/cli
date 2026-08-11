---
name: lark-apps
version: 1.0.0
description: "妙搭（Spark/Miaoda）应用开发与托管：应用创建、本地全栈开发、云端生成迭代、创意设计（UI mockup / 可交互原型 / 线框图 / 落地页 / 仪表盘 / 幻灯片 deck / 视觉探索）、AI相关能力和飞书平台能力或者其他外部能力集成、日志/Trace/监控指标/PV/UV 查询、环境变量管理、应用协作者与协作权限设置、应用角色与成员管理、自动化触发器（定时/记录变更/Webhook/飞书审批）。当用户要开发/新建一个系统·工具·平台·应用，或要本地开发 / 云端开发 / 修改 / 部署 / 发布 / 上线 / 拿可分享链接，或用 HTML 做页面·网站·部署到妙搭，或要设计 / design / mockup / prototype / wireframe / 做 PPT / deck / 视觉探索，或提到妙搭/Spark/Miaoda（应用运行时域名形如 *.aiforce.cloud）、应用数据库、应用文件存储、开放 API Key、可见范围、应用协作者/开发权限、应用角色/角色成员、线上日志、接口请求量、错误量、延迟、访问量、环境变量、给妙搭应用配自动化任务/定时触发/审批通过后自动触发时使用。不负责普通云盘文件上传（lark-drive）、飞书文档编辑（lark-doc）、原生幻灯片创建（lark-slides）。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli apps --help; lark-cli apps +<cmd> --help"
---

# apps (v1)

妙搭应用属于用户资产。默认用 `--as user`；认证、scope、exit-10、高风险确认、`_notice` 等通用处理只读 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，不要在本 skill 里复制。妙搭应用有两条开发路径：**本地开发**（拉源码本地写）/ **云端会话**（妙搭 AI 生成）。

## 身份与授权

妙搭应用是用户的个人资产，统一 `--as user`（见开头）。已有用户身份可用时直接执行业务命令，**不要为了预防权限问题主动重新登录**，否则可能中断原任务并触发不必要的设备授权。仅当 CLI 明确返回未登录或缺少本域 scope 时，一次性执行：

```bash
lark-cli auth login --domain apps
```

因缺权限失败（`error.subtype == "missing_scope"`）时的通用处理见 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，同样按 `--domain apps` 授权；授权成功后只恢复原业务操作，不扩展任务范围。

## 意图路由

按具体操作查命令（开发路径先用下方「选择开发路径」判定表定好再进来取命令）：

| 用户意图 | 先用 | 按需读取 |
|---|---|---|
| 创建**新**应用资产、拿 app_id | `+create` | [`lark-apps-create.md`](references/lark-apps-create.md) |
| 找已有 app_id、按名字过滤应用 | `+list --keyword <name>` | [`lark-apps-list.md`](references/lark-apps-list.md) |
| 查单个应用详情（类型、名称、发布状态等） | `+get --app-id <app_id>` | [`lark-apps-get.md`](references/lark-apps-get.md) |
| 改应用名或描述 | `+update` | [`lark-apps-update.md`](references/lark-apps-update.md) |
| HTML 应用 / 创意模式 — 写 HTML 页面/网站、静态页、PPT/deck、落地页、仪表盘、UI mockup、原型、线框图、视觉探索 | 加载 [`creative-design/creative-design.md`](creative-design/creative-design.md)（含完整开发与发布流程） | [`creative-design/creative-design.md`](creative-design/creative-design.md) |
| 旧版存量 HTML 应用（无 Git 管理）继续上传已有静态产物 | `+html-publish`（仅兼容旧链路；新建 html / 创意模式 / creative-design 产物不得使用） | [`lark-apps-html-publish.md`](references/lark-apps-html-publish.md) |
| 开发已有应用 / 初始化本地仓库（开发方式已定为本地后；先解析 app_id，勿 `+create` 新建） | `+init`（或手动 `+git-credential-init` + 原生 git）。**执行前必读** [`lark-apps-local-dev.md`](references/lark-apps-local-dev.md)，含端到端流程和领域规则 | [`lark-apps-init.md`](references/lark-apps-init.md), [`lark-apps-git-credential.md`](references/lark-apps-git-credential.md) |
| 本地开发时 `.env.local` 损坏/丢失，重新拉取启动期环境变量 | `+env-pull` | [`lark-apps-env-pull.md`](references/lark-apps-env-pull.md) |
| 管理应用环境变量（查看/设置/删除） | `+env-list`, `+env-set`, `+env-delete` | [`lark-apps-env.md`](references/lark-apps-env.md) |
| 查线上日志、Trace、请求数、错误率、延迟、CPU、memory、PV/UV/访问量 | `+log-list`, `+log-get`, `+trace-list`, `+trace-get`, `+metric-list`, `+analytics-list` | [`lark-apps-observability.md`](references/lark-apps-observability.md) |
| 看表 / 看结构 / 初始化多环境 / 导入导出数据 / 变更追溯 / 行级审计 / dev→online 发布 / 时间点恢复 / 查 DB 用量 | `+db-table-list`、`+db-table-get`、`+db-env-create`、`+db-data-export`/`+db-data-import`、`+db-changelog-list`、`+db-audit-status`/`+db-audit-enable`/`+db-audit-disable`/`+db-audit-list`、`+db-env-diff`/`+db-env-migrate`、`+db-recovery-diff`/`+db-recovery-apply`、`+db-quota-get` | [`lark-apps-db.md`](references/lark-apps-db.md) |
| 逐条执行 SQL（SELECT / DML / DDL）；建表 / 改表 / 写 SQL 的平台规范 | `+db-execute` | [`lark-apps-db-execute.md`](references/lark-apps-db-execute.md)（含「平台 SQL 规范」：审计列 / RLS / `user_profile` / 禁用 SQL / PG 陷阱） |
| 管理应用文件存储：上传/下载本地文件、列出/查看/删除已存文件、生成临时分享链接、查存储用量 | `+file-upload`/`+file-download`/`+file-list`/`+file-get`/`+file-sign`/`+file-delete`/`+file-quota-get` | [`lark-apps-file.md`](references/lark-apps-file.md) |
| 调试应用运行时缓存：查看/删除单个业务 key、清空指定环境缓存 | `+cache-get`/`+cache-delete`/`+cache-clear` | [`lark-apps-cache.md`](references/lark-apps-cache.md) |
| **部署/上线应用**（"部署""上线""推上去并部署""发布到云端"）；查发布状态/历史 | 本地开发链路先按 [`lark-apps-local-dev.md`](references/lark-apps-local-dev.md) 确认本次改动已 git commit + git push，再用 `+release-create` / `+release-get`；查历史用 `+release-list` | [`lark-apps-local-dev.md`](references/lark-apps-local-dev.md), [`lark-apps-release-create.md`](references/lark-apps-release-create.md), [`lark-apps-release-get.md`](references/lark-apps-release-get.md), [`lark-apps-release-list.md`](references/lark-apps-release-list.md) |
| 设置或查看运行时可见范围 | `+access-scope-set`, `+access-scope-get` | 对应 access-scope reference |
| 管理应用协作者（列出/添加/改权限/移除）或协作权限设置 | `+member-list`, `+member-add`, `+member-update`, `+member-remove`, `+member-settings-get`, `+member-settings-set` | 本文「应用协作者与协作权限设置」 |
| 创意模式（html）应用的评论相关操作 | 创意模式应用评论走 lark-drive 文档评论体系，读取 [`../lark-drive/SKILL.md`](../lark-drive/SKILL.md) 了解评论能力 | [`../lark-drive/SKILL.md`](../lark-drive/SKILL.md) |
| 管理 `app_...` 应用内角色、角色成员，或查询用户匹配角色 | `+role-list/get/create/update/delete`, `+role-member-list/add/remove`, `+role-match-list` | [`lark-apps-role.md`](references/lark-apps-role.md) |
| 云端 Agent 生成/迭代应用（开发方式已定为云端后） | `+session-create` -> `+chat` -> `+session-get` | [`lark-apps-cloud-dev.md`](references/lark-apps-cloud-dev.md) |
| 管理妙搭应用开放 API Key（创建/查看/启停/重置/删除凭证；密钥仅 create/reset 一次性返回） | `+openapi-key-list/get/create/update/enable/disable/delete/reset` | [`lark-apps-openapi-key.md`](references/lark-apps-openapi-key.md) |
| 管理妙搭应用自动化触发器（定时/记录变更/Webhook/飞书审批四类触发器的查询/创建/更新/启停；Webhook URL·Token 一次性回显、不落盘） | `+automation-list/get/create/update/enable/disable` | [`lark-apps-automation.md`](references/lark-apps-automation.md) |
| 查看某次会话某一轮（turn）的回复消息（含仍在生成中的本轮）/ 导出上一轮模型回复（"这一轮回复了什么""上一轮的回复""导出某轮消息"） | 先 `+session-get`（取 `latest_turn.turn_id`）-> `+session-messages-list --turn-id <id>`（仅 user 身份；分页用 `--page-token`） | [`lark-apps-session-messages-list.md`](references/lark-apps-session-messages-list.md) |
| 外部能力(AI模型能力和飞书平台能力)集成/插件/Plugin/Capability | `+plugin-install`, `+plugin-list`, `+plugin-uninstall` | [`lark-apps-plugin-install.md`](references/lark-apps-plugin-install.md), [`lark-apps-plugin-uninstall.md`](references/lark-apps-plugin-uninstall.md), [`lark-apps-plugin-list.md`](references/lark-apps-plugin-list.md) |
| 把一批 ID 在妙搭 user_id ↔ 飞书 open_id / union_id / 飞书 user_id 之间互转（例如拿到 open_id 但下游要 user_id） | `+user-id-convert --convert-type <方向> --ids <id1,id2,...>` | [`lark-apps-user-id-convert.md`](references/lark-apps-user-id-convert.md) |

## 高频路径

- **Base 到应用数据库同步**：用户说“Base 同步到数据库 / 整库同步 / 多张表同步 / 批量任务重新启用 / operation-not-allowed”时，先读 [`lark-apps-db.md`](references/lark-apps-db.md) 的 Base 数据同步段落，再查 app_id 或处理授权。先形成计划再动手：`+db-sync-create` 一次只处理一张 Base 表，整库/多表必须拆成多份单表配置和多次 preview/create；batch/import 任务是一次性任务，不能重新 enable，遇 operation-not-allowed 先解释生命周期边界，再用 `+db-sync-get` 查状态/结果，持续同步要新建 streaming 任务。
- **性能/监控/观测指标**：用户问“接口请求量、错误量、错误率、接口慢、延迟、CPU、内存、最近一小时/七天趋势”时，不要去当前工作区搜索监控文件，也不要询问“监控数据在哪”。先按「app_id 获取」解析应用：`lark-cli apps +list --keyword "<应用名>" --as user`；拿到 `app_id` 后读 [`lark-apps-observability.md`](references/lark-apps-observability.md)，用 `+metric-list`。
- **请求量 + 错误量 + 延迟**：请求量/错误量用 `lark-cli apps +metric-list --app-id <app_id> --metric requests --since <range> --as user`（不传 `--series` 会同时返回 total/error）；延迟用 `--metric latency`（不传 `--series` 会返回 p50/p99）。如果用户给了具体接口，再加 `--api <path-or-name>`；不要臆造 group-by 参数。
- **PV/UV/访问量/活跃用户**：先解析 `app_id`，再用 `+analytics-list`，不要误用 `+metric-list`。
- **设置环境变量**：如果用户只给应用名，仍先 `+list --keyword` 解析 app_id；设置 online 环境且用户已经明确说“确认/直接执行”时，调用 `+env-set --environment online ... --yes`，不要再次要求确认。回复和日志摘要里只提 key / env / app，不回显真实 value；需要传复杂值时优先用 `@file` 或 stdin。
- **删除环境变量**：`+env-delete` 是破坏性操作。除非用户在同一轮已经明确确认删除这个 app/env/key，否则先向用户确认应用、环境、key 和删除后果；确认后再加 `--yes`。不要因为认证失败/重登完成就自动继续删除，必须保留确认门槛。

## 应用协作者与协作权限设置

这组命令管理妙搭应用的开发协作者和协作策略，不等同于 `+access-scope-*` 的运行时访问范围，也不等同于 `+role-*` 的应用内业务角色。所有命令使用 `app_...` 应用 ID 和 `--as user`。不要读取或判断 `app_type` 来预判支持范围，直接调用对应的协作者命令。

- `+member-list`、`+member-settings-get` 是只读命令，需要 `spark:app:read`。
- `+member-add`、`+member-update`、`+member-remove`、`+member-settings-set` 是高风险写命令，需要 `spark:app:write`。先用 `--dry-run` 核对目标、URL 和请求体；dry-run 不需要 `--yes`。用户已确认具体应用、成员/设置及影响，或已按下方「高影响动作：确认与预授权」对整条流程明确预授权时，真实执行加 `--yes`；否则在 dry-run 后停下请求确认。批量移除成员仍执行「禁止预授权判定底线」，不能从泛化的“直接做”推导出 `--yes`。
- 添加、更新、移除成员时必须显式提供匹配的外部 ID 类型，禁止传内部数字 ID、猜测类型或做隐式转换：用户 `--member-type openid --member-id ou_...`；群组 `--member-type openchat --member-id oc_...`；部门 `--member-type opendepartmentid --member-id od-...`。
- `+member-list --member-type` 的筛选枚举是响应对象类型 `user` / `department` / `chat`，与写命令的 ID 类型枚举不同。可再用 `--role view|edit|full_access` 筛选。
- `+member-list` 一次返回应用的全部直接协作者，不提供分页参数；可用 `--member-type` 和 `--role` 缩小结果范围。
- 成员响应不包含应用详情。需要名称、类型或发布状态时单独调用 `+get --app-id <app_id>`，不要期待成员分页重复返回 `app`。
- 收到 subtype `feature_not_available`（OpenAPI code `3340005`；直连服务可能为 `40005`）时，立即停止 CLI 自动化，不切换 `app_type`，也不尝试用 access scope、应用角色或其它成员命令绕过。向用户说明该应用暂不支持通过 lark-cli 设置协作者，并引导其在妙搭后台的权限设置中操作。
- `external_invite` 只在 `+member-settings-get` 的响应中读取，不能独立设置；它会跟随 `external_access`。CLI 不注册 `--external-invite`，需要改变外部协作能力时只设置 `--external-access`。
- `copy_download_by` 也只在 `+member-settings-get` 的响应中读取。CCM 当前明确不支持为妙搭对象写入复制、打印和下载权限，因此 CLI 不注册 `--copy-download-by`。保留读取结果，不要尝试写入，也不要改用其它权限字段模拟。

```bash
# 读取协作者和当前协作策略
lark-cli apps +member-list --app-id <app_id> --as user
lark-cli apps +member-settings-get --app-id <app_id> --as user

# 写操作先预览精确的 typed-ID 字段；确认后把 --dry-run 换成 --yes
lark-cli apps +member-add --app-id <app_id> --member-type openid --member-id ou_xxx --perm view --dry-run --as user
lark-cli apps +member-update --app-id <app_id> --member-type openchat --member-id oc_xxx --perm edit --dry-run --as user
lark-cli apps +member-remove --app-id <app_id> --member-type opendepartmentid --member-id od-xxx --dry-run --as user
lark-cli apps +member-settings-set --app-id <app_id> --external-access disabled --comment-by viewer --dry-run --as user
```

## 选择开发路径（进意图路由前先判这步）

新建必先定 **app_type** 和**开发方式**两件正交的事；修改已有先按「app_id 获取」指认到 app，指认不到就问用户，不擅自 `+create`。开发方式（本地 vs 云端）只看用户对"谁来写代码"的偏好，与应用复杂度、要不要数据库无关。

**app_type 三类边界**（先判"要不要把数据存到服务端"，再判"纯展示还是有交互"）：

| 信号 | 判定 |
|---|---|
| 含数据库 / 后端持久化：登录 / 增删改查 / 报名·投票·站会存记录 / 多人协作 / 泛称"系统·工具"且明确要存数据 | `app_type=full_stack` |
| 纯静态展示（给人"看"的物料，无 JS 交互）：PPT/deck / demo / 落地页 / 海报 / UI mockup / 线框图 / 静态仪表盘 / 视觉探索 | `app_type=html`，加载 [`creative-design/creative-design.md`](creative-design/creative-design.md)（含完整开发与发布流程） |
| 有 JS 交互但无数据库（给人"用"的前端应用）：可交互原型 / SPA / 表单校验 / 动态计算 / 调用外部 API / 泛称"工具·系统"但未明确要存数据 | `app_type=frontend`（**默认倾向**：用户未明确提出数据库需求时默认引导 frontend，不默认 full_stack） |
| 类型模糊（尤其"要不要存数据"不清） | **追问**，话术偏向 frontend，例："看起来是个前端应用，需要保存数据吗？"；确认要存数据再转 full_stack，确认纯展示再转 html |
| 用户要自己写 / 本地 IDE·code agent / 拉源码到本地 / 交研发 | 本地开发，读 [`lark-apps-local-dev.md`](references/lark-apps-local-dev.md) |
| 让妙搭 AI 云端生成 / 对话式 / 自己不碰代码 | 云端会话，读 [`lark-apps-cloud-dev.md`](references/lark-apps-cloud-dev.md) |
| 未表达"谁来写"偏好 | **必须先问**（本地代码开发 vs 云端 AI 生成）；选定前不擅自选边、不暗示默认，不得以"需求不模糊"为由跳过提问直接 `+init` / `git clone` / `+session-create` / 首轮 `+chat` |
| 修改已有 + 当前目录是 `.spark/meta.json` 项目 | 直接继续本地按意图路由，不必问也不必判云端 |
| 修改已有 + 有云端偏好 | 云端会话；未表达偏好且非本地项目 → 默认本地；判不准先问 |

**类型升级**：`frontend` 应用后续需要数据库/后端能力时，本地 CLI 不提供类型升级；引导用户到云端会话（打开 `https://miaoda.feishu.cn/app/{app_id}`），用自然语言描述后端需求（如"给这个应用加登录和数据存储"）即可触发升级，无需特殊指令。

## 发布态护栏

- **发布意图判定**：用户要"可访问 / 线上 / 分享 / 新链接 / 上线" = 发布意图，先走发布链路、确认完成再给链接。
- 完成 ≠ 发布：云端会话完成 / `+list is_published=true` 都不代表最新内容已部署。
- 开发态链接 `https://miaoda.feishu.cn/app/{app_id}`（full_stack / frontend 应用）：进应用编辑/开发态、管理与继续开发应用的入口，也是 frontend 升级为 full_stack 的入口（云端会话）。创意模式（html）应用开发态和发布态是同一个链接，无需额外提供开发态链接。
- 发布态链接来源：`+release-get` 轮询 `finished` 给 `online_url` / `failed` 给 `error_logs`（html / frontend / full_stack 统一走 `+release-get`）。
- html 应用的主链路是创意模式开发方式：按 [`lark-apps-local-dev.md`](references/lark-apps-local-dev.md) 初始化仓库、在仓库内产出 HTML 及关联文件，并通过 git commit / git push / `+release-create` / `+release-get` 发布部署。任何 git 操作（clone / pull / push）报错时，先执行 `lark-cli apps +git-credential-init --app-id <app_id> --as user` 刷新本地 Git 凭证，再重试原 git 命令。如果刷新凭证也失败，**停止并向用户报告**：原始 git 错误、凭证刷新失败原因，以及是否可能是当前环境（操作系统、沙箱）限制导致（如 macOS Keychain 在沙箱中不可用、Linux 加密文件目录不可写等）。不要改走 `+html-publish`，也不要把 `+html-publish` 当作本地开发链路的 fallback。
- 创意模式（html）应用的链接格式为 `https://{租户域名}/page/{meta_token}`，**开发态和发布态是同一个链接**（区别于 full_stack 应用两者分开）。此链接形似飞书文档链接。`+get --app-id <meta_token>` 可获取应用信息（含 `app_id`），`+get --app-id <app_id>` 可获取 `meta_token`。看到 `/page/xxx` 链接时，它是妙搭创意模式应用，不要当成飞书文档跳过。

## 平台资源与应用源码边界

- `apps` 命令的 `--path`、`--file`、`--output` 等路径参数只接受当前工作目录（cwd）下的相对路径，传绝对路径会报错。如果目标文件不在 cwd 下，先 `cd` 到目标目录再执行命令。
- 图片、字体、音视频等资源型文件属于平台资源，不应提交到 git 仓库、引用本地路径或以 base64 内联到源码中。先通过 `lark-cli apps +file-upload --app-id <app_id> --file <local_path>` 上传到应用文件存储，拿到返回的远端 URL 后在代码中引用。上传返回的链接按 app 隔离，不同应用必须各自重新上传，不能跨应用复用同一链接。详情读 [`lark-apps-file.md`](references/lark-apps-file.md)。
- `apps +role-*` 只管理平台角色资源；修改已初始化应用的源码（包括当前目录已经是应用项目）时，先查看工作区 `.agents/skills/`，完整读取与任务匹配的领域 skill，再按其路由读取所需 reference。角色鉴权或运行态角色管理读应用内 `authz-guide`，不能用本 skill 的平台命令参考推断运行时合同。
- `lark-cli` 只用于开发过程中的平台资源核验或变更。应用运行时代码必须使用工程内领域 skill 规定的 SDK，禁止通过 `exec` 或子进程调用 `lark-cli`。
- 平台回读出的当前资源 ID、名称和成员只用于事实核验，不自动构成业务策略；除非需求或应用内领域 skill 明确定义，禁止把当前样本硬编码成 allowlist、denylist、只读集合或权限规则。
- 实现领域 SDK 时，以实际包导出的类型和应用内领域 reference 记录的入参、响应路径为准；禁止修改 ambient `.d.ts`、补造宽松类型或强制断言，让猜测的 SDK 结构仅在本地"编译通过"。
- typecheck/build 成功不等于合同正确。交付前逐项核对每个 SDK 调用的入参、响应取值路径和策略分支；涉及更新、删除等不同动作时，分别验证各自动作所需的完整状态，不能复用更弱的前置判断。
- 源码任务交付前确认新增页面、Controller、Module 已接入真实 router/bootstrap，并运行项目现有 typecheck/build；只创建未接线文件不算完成。
- `+access-scope-*` 只管运行时可见范围（谁能打开应用），不是角色权限；应用协作者/开发权限使用 `+member-*` 和 `+member-settings-*`，应用内业务角色使用 `+role-*`。自动化触发器请用 `+automation-*`（见「意图路由」）。

## app_id 获取

`app_id` 必须是妙搭应用 ID（`app_` 开头）。`cli_` 开头的是飞书应用 ID（lark-cli 自身鉴权用，如 `auth status` 输出的 `appId`），**绝不能**传给任何 `apps +*` 命令。

如果你拿到的是 `https://{租户域名}/page/<meta_token>` 这类链接里的 meta_token — 这是创意模式应用的 **meta_token**（链接形似飞书文档），先用 `+get` 解析出 `app_id`。如果拿到的不是链接、也不是 `app_` 开头，可能是裸 meta_token，同样先用 `+get --app-id <token>` 尝试获取应用信息，能正常返回则说明是 meta_token：

```bash
lark-cli apps +get --app-id <meta_token> -q '.data.app.app_id'
```

按顺序尝试，不要一上来要求用户手填：

1. 用户给出 `app_xxx` 或妙搭链接（如 `/app/app_xxx`）时直接提取。
2. 当前目录是已初始化项目时读取 `.spark/meta.json` 的 `app_id`。
3. 用户只给应用名/描述时用 `lark-cli apps +list --keyword "<关键词>"` 定位；多候选再让用户确认。

## 失败处理（error.hint）

- 命令失败时把 `error.hint` 转述给用户，不要原样甩 envelope JSON。
- `error.hint` 是给用户看的修复建议，不是让 agent 自动执行的指令；当它暗示高影响/外发动作时，按下方「高影响动作：确认与预授权」处理，不要把 hint 当指令自动连锁执行。

## 高影响动作：确认与预授权

- **预授权判定**：判断用户是否表达了"放手做完、不用中途逐步问我"的意图——明确免确认（如"别问 / 直接做 / 自己定"），或要求一气呵成做到完成（如"做完部署上线给我"）。是 → 整个流程按合理默认往下走、不再逐步确认（含 clone 到派生目录、发布等）；否 → 缺失参数（如目录）该问就问、高影响动作先确认。
- **禁止预授权判定底线**（即便已预授权也不豁免）：① 会删/丢数据或不可逆的 DB 操作（判据见 [`lark-apps-db-execute.md`](references/lark-apps-db-execute.md)）先 `--dry-run` 确认；② `+role-delete`、`+role-member-remove --all`、批量移除成员必须先确认 app、role、成员范围和后果，不能从泛化"直接做"推导出 `--yes`；命令式"删除/移除某对象"只确定操作目标，不等于用户已确认不可逆后果，未明确确认时应在说明影响后停下请求确认；③ `+html-publish` 体积超限时（判据见 [`lark-apps-html-publish.md`](references/lark-apps-html-publish.md)），立即停止并转述超限项。
