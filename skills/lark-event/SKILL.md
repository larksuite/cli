---
name: lark-event
version: 1.2.0
description: "飞书事件实时订阅：通过 `lark-cli event consume <EventKey>` 消费 IM 消息等实时事件。支持 `--max-events` / `--timeout` bounded 执行，适合 AI agent 作为子进程启动。业务细节在 references 下按业务域拆分。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli event --help"
---

# Lark Events

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md) 了解认证、`--as user/bot` 切换、`Permission denied` 的处理和安全规则。

## 核心命令

| 命令 | 用途 |
|------|------|
| `lark-cli event list [--json]` | 列出所有可订阅的 EventKey |
| `lark-cli event schema <EventKey> [--json]` | 查看指定 EventKey 的参数、schema|
| `lark-cli event consume <EventKey> [flags]` | 前台阻塞式消费事件（Ctrl+C / SIGTERM / 关 stdin 退出） |
| `lark-cli event status [--fail-on-orphan] [--json]` | 查看本机 bus 守护进程状态 |
| `lark-cli event stop [--all] [--force] [--json]` | 停止 bus 守护进程 |

**AI 调用路线**：`event list --json` → 选定 EventKey → `event schema <key> --json` 拿 schema → 写 `--jq` → `event consume`。具体 flag 查 `event consume --help`。

**子进程启动契约**：`event consume` 的 stderr 出现 `[event] ready event_key=<key>` 后，之后到达的事件保证送到 stdout。父进程 block 读 stderr 到这行再开始读 stdout，**不要 sleep 兜底**。

**一个 consume 一个 EventKey**：命令只接 1 个位置参数，不支持 `key1,key2` 或通配符。监听 N 个 key 要开 N 个子进程。

**抓样本看 payload 形态**：`event consume <key> --max-events 1 --timeout 30s` —— 收到 1 条或 30 秒后退出，常用于探测事件的 shape。

**不要 `kill -9` consume 进程**：跳过 cleanup 会留下 orphan 本地 bus（用 `event status --fail-on-orphan` 检测、`event stop --all --force` 清理）；有 PreConsume 的 EventKey 还会在服务端残留订阅。

## 业务索引

| 业务 | Reference | 覆盖 |
|---|---|---|
| IM | [`references/lark-event-im.md`](references/lark-event-im.md) | 11 个 IM key（消息、群、reaction） |

## 输出形状来自 Output Schema

`event schema <key>` 的 Output Schema 给出字段路径、类型、是否 optional：

- 顶层有 `{schema, header, event}` → V2 信封结构，字段路径走 `.event.xxx`
- 顶层直接是扁平字段 → 路径走 `.xxx`

**schema 长什么样，JQ 就怎么写**。

`Output Schema` 里字段除了 `type` / `description`，还可能带：
- `enum: [...]` — 该字段只能取这几个值
- `format: "open_id" / "chat_id" / "email" / "timestamp_ms" / ...` — 语义标记，可用于反查 API 或格式转换

具体业务的 shape 差异和字段语义见对应 reference。

## 常用 Flag（consume）

| Flag | 说明 |
|---|---|
| `--param key=value` / `-p` | 业务参数（可重复；多值用逗号）；取值见 `event schema <key>` 和对应业务 reference |
| `--jq <expr>` | JQ 过滤/变形每条事件；表达式输出空则跳过该条 |
| `--max-events N` | 收到 N 条事件后退出。默认 0 = 无限 |
| `--timeout D` | D（如 `30s`、`2m`）后退出。默认 0 = 不超时。和 `--max-events` 取先达到者 |
| `--quiet` | 抑制 stderr 状态提示。**AI 不要用**——会压制 ready marker |
| `--output-dir <dir>` | 事件按文件写入目录，不走 stdout |
| `--as user/bot/auto` | 强制身份；默认用 EventKey 声明的 `AuthTypes[0]` |

输出是 **NDJSON**（一行一个 JSON 对象）。pretty-print 用 `| jq .`。
