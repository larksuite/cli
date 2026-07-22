# apps +create

创建妙搭应用。运行时命令事实以 `lark-cli apps +create --help` 为准。

## 何时用

用来创建应用资产并拿到 `app_id`。它不负责把自然语言需求交给云端 Agent：用户要“帮我生成/迭代应用”时，先创建 `full_stack` app，再进入 [`lark-apps-cloud-dev.md`](lark-apps-cloud-dev.md) 用 `+session-create` / `+chat` 提交需求。

## 命令骨架

- 必填：`--name`、`--app-type`。
- app type 语义取值为 `html` / `full_stack`；CLI 会把输入归一成小写后校验。
- 可选：`--description`、`--icon-url`。

## 示例

```bash
lark-cli apps +create --name "客户调研问卷" --app-type html

lark-cli apps +create --name "审批系统" --app-type full_stack \
  --description "部门审批系统，支持登录、提交申请、多级审批"

lark-cli apps +create --name "Demo" --app-type html --dry-run
```

## 输出契约

- 成功默认 JSON envelope 中读取 `data.app.app_id`，同时可用 `data.app.name` / `description` 向用户确认结果。
- pretty 输出只适合人看；后续命令需要 app_id 时，用 JSON 或 `--jq '.data.app.app_id'`。

## app type 与命名

- `--app-type` 取值与判定信号见 SKILL.md「选择开发路径」，此处不重复。
- 用户只给自然语言需求时，据此生成简洁的 `--name` 和一句 `--description` 直接创建；不满意再用 `+update` 改。

创建后按用户路径继续：

- 本地应用开发（含 html 和 full_stack）：读 [`lark-apps-local-dev.md`](lark-apps-local-dev.md)。
- 云端 Agent 生成/迭代：读 [`lark-apps-cloud-dev.md`](lark-apps-cloud-dev.md)。
