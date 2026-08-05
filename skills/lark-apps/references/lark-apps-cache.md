# apps cache 域命令（应用运行时缓存调试）

调试妙搭应用的运行时缓存：查看某个缓存 key 的内容、删除单个 key、清空某个环境的全部缓存。缓存是应用为了加速而临时存放的数据，删除或清空后，应用下次用到时会自动重新取最新数据。命令事实以 `lark-cli apps +<cmd> --help` 为准；认证、`--as user`、exit 码、`_notice` 等通用处理见 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 与本域 [`SKILL.md`](../SKILL.md)。

## 何时用

用户要排查「某个缓存 key 里存的是什么 / 有没有命中」、想删掉某个 key 让应用下次拿到最新数据、或想清空某个环境的缓存做快速恢复时。

## 命令一览

| 命令 | 做什么 | 关键参数 |
|---|---|---|
| `+cache-get` | 查一个缓存 key 的内容与信息 | `--key`、`--environment`、`--format` |
| `+cache-delete` | 删一个缓存 key（重复删不会报错；不需 `--yes`） | `--key`、`--environment` |
| `+cache-clear` | 清空指定环境下的全部缓存（**高危**） | `--environment`、`--yes` |

> 所有命令都需 `--app-id`。

## 约定（先读）

- **环境 `--environment dev|online`（可省略）**：缓存按运行环境隔离。不指定时按应用当前的环境配置自动选择——有多环境的应用默认落到开发环境 `dev`，没有多环境的就是线上 `online`；返回结果里的 `environment` 会告诉你这次实际操作的是哪个环境。想固定就显式传。
- **缓存 key 用 `--key` 传**：传业务里使用的那个 key；是否合法（非空、长度等）由服务端校验，不合法会返回错误。
- **风险分级**：`+cache-clear` 会清掉整个环境的缓存，是高危操作，不带 `--yes` 会被确认关卡拦下；`+cache-delete` 只删单个 key、影响小，不需 `--yes`。
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
删一个缓存 key。**重复删、或删一个本就不存在的 key，都算成功**（返回删除数量 0）、不会报错；删中则返回删除数量 1。删掉后应用下次会自动重新取最新数据，影响小，故不需 `--yes`。

```bash
lark-cli apps +cache-delete --app-id app_xxx --environment dev --key <key>
```

### +cache-clear（高危）
清空当前应用在**指定环境**下的全部缓存，用于定位不到具体 key 时的快速恢复。影响面是整个环境，必须带 `--yes`；返回本次清除的 key 数量。动手前可先 `--dry-run` 预览将要执行的操作。

```bash
lark-cli apps +cache-clear --app-id app_xxx --environment dev --yes
```

## 错误与边界

- **key 不合法 / 缓存服务暂时不可用**：命令会返回带说明的错误，按 `error.hint` 转述给用户；「服务暂时不可用」这类可稍后重试。

## Agent 规则

- **写操作先定环境**：`+cache-clear` / `+cache-delete` 不指定 `--environment` 时会落到自动选中的环境——**没有多环境的应用会直接作用到线上 `online`（生产）**。不确定应用有没有多环境时，写操作显式传 `--environment`；纯查看（`+cache-get`）影响小，可以省略。
- **`+cache-clear` 会清掉整个环境的缓存**：执行前先跟用户确认环境无误、说明会清掉该环境全部缓存。已明确授权可直接带 `--yes`；遇到确认关卡（`confirmation_required`，exit 10）按 lark-shared 约定与用户确认后再补 `--yes` 重试，不要静默追加。
- **排查缓存内容优先用 `+cache-get`**：想看结构化、易读的内容用 `--format pretty`；想拿原始内容做精确比对用默认 JSON。
- **删 key 前先对齐 key**：用户只描述了业务含义、没给准确 key 时，先确认再删——删错影响也有限（应用会自动重建），但仍应避免误删。
