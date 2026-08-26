# apps cache 域命令（应用运行时缓存调试）

调试妙搭应用的运行时缓存：查看某个缓存 key 的内容、删除单个 key、清空某个环境的全部缓存。缓存是应用为了加速而临时存放的数据，删除或清空后，应用下次用到时会自动重新取最新数据。命令事实以 `lark-cli apps +<cmd> --help` 为准；认证、`--as user`、exit 码、`_notice` 等通用处理见 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 与本域 [`SKILL.md`](../SKILL.md)。

## 何时用

用户要排查「某个缓存 key 里存的是什么 / 有没有命中」、想删掉某个 key 让应用下次拿到最新数据、或想清空某个环境的缓存做快速恢复时。

## 命令一览

| 命令 | 做什么 | 关键参数 |
|---|---|---|
| `+cache-get` | 查一个缓存 key 的内容与信息 | `--key`、`--environment`、`--format` |
| `+cache-delete` | 删一个缓存 key（重复删不会报错；不需 `--yes`） | `--key`、`--environment` |
| `+cache-clear` | 清空指定环境下的全部缓存（**高危，须先向用户二次确认**） | `--environment`、`--yes` |

> 所有命令都需 `--app-id`。

## 约定（先读）

- **环境 `--environment dev|online`（可省略）**：缓存按运行环境隔离。不指定时按应用当前的环境配置自动选择——有多环境的应用默认落到开发环境 `dev`，没有多环境的就是线上 `online`；返回结果里的 `environment` 会告诉你这次实际操作的是哪个环境。想固定就显式传。
- **缓存 key 用 `--key` 传**：传业务里使用的那个 key；是否合法（非空、长度等）由服务端校验，不合法会返回错误。
- **风险分级**：`+cache-clear` 会清掉整个环境的缓存，是高危操作，不带 `--yes` 会被确认关卡拦下，且**必须先拿到用户对本次清空的确认**（判据见 [+cache-clear](#cache-clear高危)）；`+cache-delete` 只删单个 key、影响小，不需 `--yes`。
- **`+cache-get` 的内容有两种展示**：`--format json`（默认）原样返回缓存内容，适合精确比对；`--format pretty` 会把内容格式化展开，更便于阅读。

## 各命令

### +cache-get
按 `--key` 查单个缓存。命中时返回：是否存在、剩余有效期（TTL）、内容及其大小；未命中（或已过期）时只返回 `exists=false`、不带内容。

> 每次查询都会连内容一起返回（没有「只看信息、不取内容」的模式），内容可能较大——只是想确认「在不在 / 还有多久过期」时，留意别占用太多上下文。

```bash
lark-cli apps +cache-get --app-id app_xxx --key spotbonus:2026:winners:list:v1
lark-cli apps +cache-get --app-id app_xxx --environment online --key <key> --format pretty
```

### +cache-delete
删一个缓存 key。**重复删、或删一个本就不存在的 key，都算成功**（返回 `deleted_key_count=0`）、不会报错；删中则返回 `deleted_key_count=1`。删掉后应用下次会自动重新取最新数据，影响小，故不需 `--yes`。

**响应里的 `deleted_key_count` 别读错**——它是「本次是否真的删掉了东西」的唯一判据：

| `deleted_key_count` | 含义 | 该怎么向用户表述 |
|---|---|---|
| `1` | 命中并删掉了 | 「已删除该 key」 |
| `0` | 请求成功，但没有删掉任何 key——这个 key **本来就不存在或已过期** | 「该 key 原本就不存在／已过期，无需删除」——**不要说成「已成功删除」** |

要证明「删除生效了」，用「删前 `+cache-get` 确认存在 → `+cache-delete` 拿到 `deleted_key_count=1` → 删后 `+cache-get` 得到 `exists=false`」这条链；只靠删后一次 miss 是不够的，因为 key 从一开始就不存在时（`deleted_key_count=0`）结果完全一样。

```bash
lark-cli apps +cache-delete --app-id app_xxx --environment dev --key <key>
```

### +cache-clear（高危）
清空当前应用在**指定环境**下的全部缓存，用于定位不到具体 key 时的快速恢复。影响面是整个环境，必须带 `--yes`；返回本次清除的 key 数量。

> [!CAUTION]
> **默认流程是「先确认、后执行」，不是「直接清」。** 除下表判定为「已确认」的情形外，**不允许在首次调用就自己带上 `--yes`**——用户提出清理请求 ≠ 用户确认了这次清理。
>
> 未拿到确认时，你只能做这两件事之一，然后**停下来等用户回话**：
> 1. 用 `--dry-run` 预览（不触发门禁、不产生任何真实清理），把将执行的请求给用户看；
> 2. 或者干脆不调命令，直接把「应用 + 环境 + 会清掉该环境全部缓存」讲清楚并请用户确认。
>
> 已经拿到确认后，才在原命令末尾补 `--yes` 执行。**看到 exit 10 / `confirmation_required` 不是「补 `--yes` 重试」的信号**，它只是告诉你门禁生效了；该不该补，取决于用户有没有确认过。

**什么算「已确认」（零歧义判据）**：看用户这轮的原话里，有没有对「清空这个环境」的授权表述。

| 用户原话 | 算不算确认 | 你该做什么 |
|---|---|---|
| 「帮我清一下 app_xxx 的 online 环境缓存」 | ❌ 不算（这是请求，不是确认） | 先 `--dry-run` 或直接请用户确认，**停下等回话** |
| 「清一下缓存」（连环境都没说） | ❌ 不算，且环境未定 | 请用户同时确认「清哪个环境」，**严禁自己选 `dev` 或 `online`** |
| 「我确认清 dev，不要动 online」 | ✅ 算（含确认表述 + 明确环境） | 显式带 `--environment dev --yes` 执行 |
| 「确认清 online，不用再问」／「是的，清吧」（承接你上一轮的确认提问） | ✅ 算 | 显式带 `--environment online --yes` 执行 |

线上环境额外一条：`--environment online` 是生产数据，**即使用户已明确指名 online，也仍需要上表意义上的确认表述**才可执行；缺确认就只出 `--dry-run` 预览。

```bash
# 1) 未确认：只预览，不清理（--dry-run 不触发门禁、不产生真实动作）
lark-cli apps +cache-clear --app-id app_xxx --environment online --dry-run

# 2) 用户确认后：补 --yes 执行
lark-cli apps +cache-clear --app-id app_xxx --environment dev --yes
```

## 错误与边界

- **key 不合法 / 缓存服务暂时不可用**：命令会返回带说明的错误，按 `error.hint` 转述给用户；「服务暂时不可用」这类可稍后重试。

## Agent 规则

- **写操作先定环境**：`+cache-clear` / `+cache-delete` 不指定 `--environment` 时会落到自动选中的环境——**没有多环境的应用会直接作用到线上 `online`（生产）**。不确定应用有没有多环境时，写操作显式传 `--environment`；纯查看（`+cache-get`）影响小，可以省略。
- **`+cache-clear` 一律先确认再清**：不带确认就执行是本域最容易犯的错。**「用户让我清缓存」不构成授权**——授权指用户对「清空这个环境」有明确确认表述（判据表见 [+cache-clear](#cache-clear高危)）。没有它，就只出 `--dry-run` 预览或口头确认请求，然后停下等回话；**不要在首次调用就自带 `--yes`，也不要看到 exit 10 就补 `--yes` 重试**。拿到确认后再补 `--yes`，并始终显式带 `--environment`。
- **排查缓存内容优先用 `+cache-get`**：想看结构化、易读的内容用 `--format pretty`；想拿原始内容做精确比对用默认 JSON。
- **删 key 前先对齐 key**：用户只描述了业务含义、没给准确 key 时，先确认再删——删错影响也有限（应用会自动重建），但仍应避免误删。
