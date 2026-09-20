# apps +create

创建妙搭应用。运行时命令事实以 `lark-cli apps +create --help` 为准。

## 何时用

用来创建应用资产并拿到 `app_id`。它不负责把自然语言需求交给云端 Agent：用户要“帮我生成/迭代应用”时，先按 SKILL.md「选择开发路径」判定的 `--app-type`（有数据库需求→`full_stack`，纯前端交互未提数据库→默认 `frontend`）创建 app，再进入 [`lark-apps-cloud-dev.md`](lark-apps-cloud-dev.md) 用 `+session-create` / `+chat` 提交需求。

## 命令骨架

- 必填：`--name`、`--app-type`。
- app type 取值为小写 `html` / `frontend` / `full_stack`；框架按枚举精确校验（不做大小写归一），非法值直接报错。
- 可选：`--description`、`--icon-url`。
- 可选：`--access-scope`，**仅当用户明确提出可见范围诉求时才用**，见下方「按需：创建时设定可见范围」。

## 示例

```bash
lark-cli apps +create --name "客户调研问卷" --app-type html

lark-cli apps +create --name "JSON 格式化工具" --app-type frontend \
  --description "纯前端交互工具，无需数据库"

lark-cli apps +create --name "审批系统" --app-type full_stack \
  --description "部门审批系统，支持登录、提交申请、多级审批"

lark-cli apps +create --name "Demo" --app-type html --dry-run
```

## 输出契约

- 成功默认 JSON envelope 中读取 `data.app.app_id`，同时可用 `data.app.name` / `description` 向用户确认结果。
- pretty 输出只适合人看；后续命令需要 app_id 时，用 JSON 或 `--jq '.data.app.app_id'`。

## 按需：创建时设定可见范围

**默认不要传 `--access-scope`**：用户没提可见范围就省略，交服务端默认。只有用户明确提出诉求（"只给我自己看" / "组织内可见" / "公开到互联网"）时才用，取值小写 `self` / `tenant` / `public`。

```bash
lark-cli apps +create --name "内部工具" --app-type frontend --access-scope tenant
```

只有这三档粗粒度，且**只在创建这一次生效**：应用建好后再传该 flag 不会改动它的可见范围。裸 HTML 单文件/目录首发不必先走本命令，`+deploy --file-path/--dir` 自动建应用时同样接受该 flag，见 [`lark-apps-deploy.md`](lark-apps-deploy.md)。

传了之后，**是否生效以 `data.access_scope_result` 为准**（没传该 flag 时响应里不会有这个字段）。设置失败不影响创建，应用照样建好可用，只是可见范围没变：

- `applied: true` → 可以明确告诉用户已设为该范围。
- `applied: false` → 按下表转述，并**引导用户到界面上自行调整**。
- 字段缺失（服务端较旧）→ 只说"已按 `<scope>` 创建"，不要断言"已设为组织内可见 / 已公开"。

| `reason_code` | 含义 | 告诉用户怎么办 |
|---|---|---|
| `policy_denied` | 租户的分享策略或文档密级管控拒绝了这次设置。放大范围（`tenant` / `public`）会触发，收窄范围（`self`）不会。**这是策略限制，不是故障** | 引导用户到应用的分享权限设置页自行调整 |
| `downstream_error` | 下游调用失败，属临时故障 | 稍后在应用的分享权限设置页重新设置即可 |
| `unsupported_app_type` | 最终创建出的应用类型不支持该设置，参数被忽略 | 无需处理，说明这类应用没有该配置项即可 |

**三种情况都不要重新创建应用重试**：可见范围只在创建那一刻应用，重试等于多建一个应用，而 apps 域没有删除接口。`message` 字段是服务端给的可直接展示文案，可以原样转述给用户。

## app type 与命名

- `--app-type` 取值与判定信号见 SKILL.md「选择开发路径」，此处不重复。
- 用户只给自然语言需求时，据此生成简洁的 `--name` 和一句 `--description` 直接创建；不满意再用 `+update` 改。

创建后按用户路径继续：

- 本地应用开发（含 html / frontend / full_stack）：读 [`lark-apps-local-dev.md`](lark-apps-local-dev.md)。
- 云端 Agent 生成/迭代：读 [`lark-apps-cloud-dev.md`](lark-apps-cloud-dev.md)。
