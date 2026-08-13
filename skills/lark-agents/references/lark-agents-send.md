# agents send

> **前置条件：** 先读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)。调 send **前先查参数**：card 的 `has_parameters` 含 `send` 时，跑 `agents card <ref> --operation send` 拿参数声明（不含则无需任何 `--param`）；所需 scope 见对应 provider 文件（card 不含 scope），通用流程见 [前置准备](../SKILL.md)。

向远程 agent 发一条消息：不带 `--context-id/--task-id` 起一个**新任务**；带 `--context-id`（可选 `--task-id`）向同一多轮上下文**续发**；带 `--answer` 回答任务停着的 `input_required` **问题组**（答法一条规则：给选项键 `<qid>=<option_id>`、给文字 `<qid>.text=<文本>`；`--text` 是整体附言，永远不是某道题的答案）。写操作。

> **`--file` 会把本地文件上传到远端 provider，内容离开本机、不可撤回。** CLI 强制确认门：真实 send 带 `--file` 须加 `--yes`，否则报 `confirmation_required`（exit 10）不上传；`--dry-run` 不上传、免 `--yes`。加 `--yes` 前先与用户确认。

## 命令

```bash
# 起新任务，立即返回 task_id/context_id/state（send 只 fire、不等结果）
lark-cli agents send <provider>:<agent_id> --text "<消息内容>"
# 轮询进度用 task get --watch（照 meta.next 给的命令，默认有界 90s）：
lark-cli agents task get <provider>:<agent_id> <task-id> --watch --timeout 90s

# 客户端预演：本地校验并打印将发的请求，不调 API（永远可用）
lark-cli agents send <provider>:<agent_id> --text "x" --dry-run

# 多轮续聊（同一会话追问）：起新一轮任务
lark-cli agents send <provider>:<agent_id> --context-id <ctx-id> --text "<追问>"

# 回答 input_required 问题组（一组一条命令原子交清；照抄 task get 输出 meta.next 的模板填空）
lark-cli agents send <provider>:<agent_id> --context-id <ctx-id> --task-id <task-id> \
  --answer <qid1>=<option_id> \        # 选择题：值必须命中 option_id（拼错报错，不会被当成文字）
  --answer <qid2>.text="<文本>" \       # 给文字：问答题的正常答案 / 选择题"都不想选"的逃生，同一写法
  --answer <qid3>=<option_id> --answer <qid3>=<option_id>   # 多选=同 key 重复

# 带文件（外发到远端；上传成功后才发消息，任一文件失败即中止）
lark-cli agents send <provider>:<agent_id> --text "看这份表" --file ./report.xlsx
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `<agent_ref>` | 是 | `<provider>:<agent_id>` |
| `--text` | 视情况 | 消息的**自由文本部分**：起任务/续聊的正文（必填，空报 `invalid_argument`，exit 2），或随 `--answer` 的整体附言（可省）。**永远不是某道题的答案** |
| `--param key=value` | 视声明 | 可重复；按 **send 这个动词**的参数声明校验（`--operation send` 查看）。校验规则与 object 点路径/JSON 传法的唯一权威见 [card「字段语义」](lark-agents-card.md)；错误一次报全且每条带完整声明（见下方错误目录） |
| `--file <path>` | 否 | 可重复；**文件外发**到远端 provider（内容离机、不可撤回）。本地先校验：仅相对路径（限 CWD 内）、文件必须存在且非目录，违规一次报全（`invalid_argument`，exit 2，dry-run 同样校验）。对 `file_input=false` 的 agent，`--file` 一律报 `unsupported_capability`（**能力门先于 dry-run**，dry-run 也拦）；对支持 file_input 的 agent，真实 send 须配 `--yes`（见下），`--dry-run` 则不上传、免 `--yes`、在 `would_send.files` 列出 |
| `--yes` | 视上 | 确认 `--file` 外发；真实 send 带 `--file` 时必填，否则报 `confirmation_required`（exit 10）不上传 |
| `--context-id` | 否 | 续同一会话；省略=新会话，结果回显新 `context_id` |
| `--task-id` | 否 | 回应某任务；**须与 `--context-id` 同用**，否则报错 |
| `--answer <key>=<value>` | 否 | 可重复；回答 `input_required` 问题组（见 `agents task get` 的 `input_required.questions[]`）。key 只有两种合法形态：`<question_id>`（值=option_id，多选重复同 key，相同值自动去重）或 `<question_id>.text`（值=文字，每题至多一条）；**须与 `--context-id/--task-id` 同用**；对 card `input_required=false` 的 agent 离线报 `unsupported_capability`。空值/非法 key 一次报全（exit 2） |
| `--dry-run` | 否 | 本地校验+打印请求，不调 API（永远可用；跳过 scope preflight 与 `--file` 确认门，但**仍过能力门**——对 `file_input`/`input_required=false` 的 agent，`--file`/`--answer` 同样报 `unsupported_capability`，与真实 send 一致，不会给出误导性的"预演成功"） |
| `--as` / `--format json\|pretty` / `--jq` | 否 | 通用；默认 `json` |

## 输出

send 立即返回当前任务。结构示例（`agents send <agent_ref> --text "分析一下上季度销售数据"`，下例已是终态；provider 未终态时返回 `submitted`/`working`，`meta.next` 会推有界轮询命令 `task get <agent_ref> <task-id> --watch --timeout 90s`）：

```json
{ "ok": true, "identity": "bot",
  "data": {
    "task_id": "task_ad9acc62af31", "context_id": "ctx_fb95c586fa03",
    "state": "completed", "is_terminal": true,
    "created_at": "2026-07-12T09:57:58Z", "updated_at": "2026-07-12T09:57:58Z",
    "messages": [
      { "role": "user", "parts": [ { "type": "text", "text": "分析一下上季度销售数据" } ] },
      { "role": "agent", "parts": [ { "type": "text", "text": "上季度销售额环比增长 12%……" } ] }
    ]
  },
  "meta": { "next": [ { "label": "查看任务详情与产物",
    "command": "lark-cli agents task get <agent_ref> task_ad9acc62af31 --as bot" } ] } }
```

`meta.next` 是建议命令（**显式传过 `--as` 时会原样带上同身份**，保证非默认身份的链条照抄可复现；没传则不带，下一条与 shortcut 家族一致走默认身份解析）：无 `template` 字段的可直接照抄——如上例的 `task get <agent_ref> task_ad9acc62af31 --as bot`（终态任务直接看详情）；未终态时推的是 `task get ... --watch --timeout 90s`，同样照抄、轮询到停轮询条件（权威定义见 [SKILL.md 核心概念](../SKILL.md)）。`template:true` 的含 `<...>` 占位符，先**整体替换**再执行，出现在三类场景：`input_required` 续发命令（照 [SKILL.md 工作流](../SKILL.md) 第 4 步，该态是否出现见 provider 文件）、`auth_required` 授权后的重查命令、产物下载 / 必填参数的占位（如 `-o <保存路径>`）。

## 错误目录（精确断言 `subtype`+exit）

本地校验（不发请求）：

| 触发 | subtype | exit | message / hint（真实输出） |
|---|---|---|---|
| 缺 `--text` | invalid_argument | 2 | `--text must contain non-whitespace characters`；hint 提示若在答题改用 `--answer` |
| `--task-id` 缺 `--context-id` | invalid_argument | 2 | `--task-id must be used together with --context-id` |
| `--answer` 缺 `--context-id/--task-id` | invalid_argument | 2 | `answering a question group requires both --context-id and --task-id`；hint 指向照抄 meta.next 模板 |
| `--answer` 键/值违规 | invalid_argument | 2 | `invalid --answer: ...` 一次报全：非 `key=value` 形、key 非法（仅 `<qid>` 与 `<qid>.text` 两种形态，`.txt`/`.TEXT`/`-` 打头都不行）、空值（`an empty answer means nothing`——不想答的题不要带 key）、同题 `.text` 重复 |
| `--answer` 但 card `input_required=false` | unsupported_capability | 2 | 该 agent 不会提问，`--answer` 无处可答（离线拦截，不发请求） |
| 答案内容违规（服务端） | invalid_argument | 1 | `N 个答案有问题` + `params[]` 每条 `{name: <key>, reason: <枚举>, spec: <题目声明>}`；reason ∈ `unknown_question`（键陈旧/拼错——组可能已换，先 task get 重看）/ `invalid_option` / `missing`（缺必答）/ `count_violation`（单选给多值等）/ `conflict`（互斥，如 skip+实值）。**修正后整组重发（含未报错的题）** |
| 组已被答掉（服务端） | failed_precondition | 1 | `任务 '...' 已不在等待输入` + 机器可读 `resolved_answers`（已受理的答案）——转告用户结果即可，别重试 |
| 传了未声明的 `--param` | invalid_argument | 2 | `unknown parameter foo (send accepts: ...)`；参数声明在别的动词上时报 `does not apply to send (declared on: task_list)`；`param` 字段为 `param:foo` |
| 多处参数问题 | invalid_argument | 2 | 一次报全：message 为 `send parameter validation failed: N problems (see params)`，`params[]` 每条含 `{name, reason, spec?}`（已声明参数的违规带 spec = 完整声明，可据此直接修；未知/重复/格式错的条目看 reason/suggestions） |
| enum / 类型 / 范围violation | invalid_argument | 2 | `must be one of low\|normal\|high` / `must be an integer` / `must be within 1..100`——错误消息即修复指令 |
| 未知 scheme | invalid_argument | 2 | message 形如 `unknown agent provider '<scheme>', currently registered: <已注册 scheme 全集>`（列表随注册变化，勿硬编码断言）；hint 指向 `agents list` |
| `--file` 路径非法/不存在/是目录 | invalid_argument | 2 | `invalid --file path: <path> (only relative paths inside the CWD are accepted)`（或 `does not exist or is not readable`/`is a directory`，多个违规一次报全）；hint `--file accepts only existing files at relative paths inside the current directory; fix each one and resend`。先于能力门与确认门 |
| `--file` 但 card `file_input=false` | unsupported_capability | 2 | 该 agent 不收文件外发，`--file` 无处可传（离线拦截，不发请求；**dry-run 亦拦**，不会给出误导性预演成功）。先于确认门 |
| `--file` 真实 send 缺 `--yes` | confirmation_required | 10 | `--file uploads local file content to the remote agent (it leaves this machine and cannot be recalled)`；hint `add --yes to confirm sending these files`。仅在 provider 支持 file_input 时触发；`--dry-run` 免此门 |
| 缺 scope（user/bot） | missing_scope | 3 | 本地 preflight，附 `missing_scopes` + 可照抄 hint；语义与修复路径（user≠bot）的唯一权威见 [SKILL.md 前置准备](../SKILL.md) 第 2/3 条。`--dry-run` 跳过此检查（真实调用按当前身份的声明检查：bot 侧 best-effort 降级、未声明 scope 的身份整体跳过） |

服务端错误：通用规则见 [SKILL.md「服务端错误」](../SKILL.md)，业务错误码目录见对应 provider 文件。

> `data.state=failed/rejected` 是**任务失败**（`ok:true`，别当传输错误重试）；error 对象才是传输/协议失败。

## 参考

- [lark-agents](../SKILL.md) — agent 全部动词
