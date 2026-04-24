---
name: lark-event
version: 1.0.0
description: "飞书实时事件监听/订阅/消费：通过 `lark-cli event consume <EventKey>` 以 NDJSON 流式接收飞书实时事件（当前覆盖 IM 11 个 EventKey：消息接收、表情回复、群成员变更等），适用于写飞书机器人（bot）、实时消息处理、长跑监听等场景。支持 `--max-events` / `--timeout` bounded 执行和子进程 ready marker 契约，适合 AI agent 作为子进程启动。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli event --help"
---

# Lark Events

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md) 了解认证、`--as user/bot` 切换、`Permission denied` 处理和安全规则。

## 30 秒看懂

抓 1 条消息事件看 shape（本 SKILL 最常跑的一条命令）：

```bash
lark-cli event consume im.message.receive_v1 --max-events 1 --timeout 30s --as bot
# 然后到 bot 所在会话发一条消息
```

**预期 stderr**（关键几行）：

```
[event] ready event_key=im.message.receive_v1              ← 订阅就绪契约标记；此后事件保证送 stdout
[source] feishu-websocket: connected                        ← 上游 WebSocket 连上
[event] exited — received 1 event(s) in 3s (reason: limit)  ← 达到 --max-events 正常退出 (exit 0)
```

**预期 stdout**（单行 NDJSON）：

```json
{"type":"im.message.receive_v1","event_id":"...","message_id":"om_x...","chat_id":"oc_...","chat_type":"p2p","message_type":"text","sender_id":"ou_...","content":"{\"text\":\"hi\"}"}
```

**父进程调用范式**：block 读 stderr 到 `[event] ready` 这行，之后并发读 stdout 处理事件。**不要 sleep 兜底**。

## 核心命令

| 命令 | 用途 |
|------|------|
| `lark-cli event list [--json]` | 列出所有可订阅的 EventKey |
| `lark-cli event schema <EventKey> [--json]` | 查 EventKey 的参数和输出 schema |
| `lark-cli event consume <EventKey> [flags]` | 前台阻塞消费事件（stdout NDJSON） |
| `lark-cli event status [--json] [--fail-on-orphan]` | 查本机 bus 守护进程状态 |
| `lark-cli event stop [--all] [--force]` | 停 bus 守护进程 |

## AI 调用路线

三步闭环：

1. `lark-cli event list --json` → 在合法 key 里选
2. `lark-cli event schema <key> --json` → 读 `resolved_output_schema` + `root_path_hint` 确定字段路径
3. `lark-cli event consume <key> [--jq '<expr>']` → 消费

## AI 子进程契约

### Ready marker

`event consume` 的 stderr 固定打 `[event] ready event_key=<key>`。**父进程 block 读 stderr 到这行再开始读 stdout**。

### stdin EOF 视为优雅退出

`event consume` 把 stdin 关闭当退出信号（为 AI 子进程场景设计）。`< /dev/null` / `nohup` / systemd 默认 `StandardInput=null` 起会立刻优雅退出（stderr `reason: signal`）。要持续跑：

- 给 stdin 一个永不 EOF 的源：`< <(tail -f /dev/null)`
- 或走 bounded：`--max-events N` / `--timeout D`

### 退出码 & Reason

`event consume` 退出时 stderr 最后一行是 `[event] exited — received N event(s) in Xs (reason: ...)`。

| exit code | reason | 触发 |
|---|---|---|
| 0 | `reason: limit` | 达到 `--max-events` |
| 0 | `reason: timeout` | 达到 `--timeout` |
| 0 | `reason: signal` | Ctrl+C / SIGTERM / stdin EOF |
| 非 0 | `Error: ...`（无 exited 行）| 启动/运行期异常（权限、网络、参数、配置） |

AI 编排按 `reason` 区分"业务到期"（limit/timeout/signal 均 exit 0）和"故障"（非 0）。

### 永远不 `kill -9`

**避免 `kill -9` consume 进程**：对**提前订阅类**的 EventKey，`kill -9` 会跳过 OAPI unsubscribe，造成服务端订阅残留（症状：重启报订阅已存在、事件重复推送）。优先用 SIGTERM / 关 stdin 停。

### 一个 consume 一个 EventKey（多 key = 多 shell）

命令只接 1 个位置参数；不支持 `k1,k2` 或通配符。监听 N 个 key 开 N 个子进程 —— **有意为之**：

- 每进程 stdout 一种 shape，AI 不写 dispatcher
- 故障隔离（一个 key 挂不牵连其他）
- 独立 `--as` / `--jq` / `--max-events` / `--timeout`

N 个 consume 共用同一 bus 守护进程（UDS 本地 IPC），资源开销小

## Output Schema 指南

`event schema <key> --json` 输出里这三段最关键：

| 字段 | 含义 |
|---|---|
| `root_path_hint` | jq 路径前缀：`"."`（flat）或 `".event"`（V2 信封）。写 jq 直接看这个，不用猜 |
| `params`（`omitempty`） | 这个 key 接受的 `--param`：含 `name` / `type` / `required` / `enum` / `default` / `description`。**字段不存在 = 这个 key 不接受任何 `--param`** |
| `resolved_output_schema.properties.*.format` | 字段语义标记：`open_id` / `chat_id` / `timestamp_ms` / `email` 等。可用于反查 API 或格式转换 |

**schema 长什么样，jq 就怎么写**。

## 常用 Flag

| Flag | 说明 |
|---|---|
| `--param key=value` / `-p` | 业务参数（可重复；多值用逗号）。不存在的 key 会报错并列出合法名 |
| `--jq <expr>` | jq 过滤/变形每条事件；表达式输出空则跳过该条 |
| `--max-events N` | 收到 N 条后退出。默认 0 = 无限 |
| `--timeout D` | D（如 `30s`、`2m`）后退出。默认 0 = 不超时。与 `--max-events` 取先达到者 |
| `--output-dir <dir>` | 事件按文件写入（仅相对路径，防路径穿越）|
| `--quiet` | 抑制 stderr 提示。**AI 不要用**——会压制 ready marker |
| `--as user\|bot\|auto` | 强制身份；默认 `auto`，解析到 EventKey 声明的 `AuthTypes[0]` |

输出是 **NDJSON**（一行一个 JSON 对象）。pretty-print 用 `| jq .`。

## 辅助命令（生命周期 / 排障）

| 命令 | 用途 |
|---|---|
| `lark-cli auth status` | 当前身份 + token + scope 概览（JSON） |
| `lark-cli event status --json` | bus 守护进程状态（running / pid / uptime / active_consumers） |
| `lark-cli event stop --all --force` | 停所有 bus 守护并强清 stale socket（含有 active consumer 的也强停） |

## 业务索引

| 主题 | Reference | 覆盖 |
|---|---|---|
| IM | [`references/lark-event-im.md`](references/lark-event-im.md) | 11 个 IM key（消息/群/成员三组） + Shape 速览 + 从 0 到 1 样本 + 5 个场景 pipeline（按会话类型/按消息类型/单群过滤/私聊自动回复/多 key 组合）+ 关键字段注意（`.content` fromjson、sender_id、@bot 识别） |
