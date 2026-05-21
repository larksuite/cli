# apps +access-scope-set

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)。

设置应用的可用范围。三种 scope 形态互斥：`specific`（指定可见）、`public`（互联网公开）、`tenant`（企业全员）。

## 命令

```bash
# 指定可见 + 允许申请（targets 支持 user / department / chat 三种类型）
lark-cli apps +access-scope-set --app-id app_xxx \
  --scope specific \
  --targets '[{"type":"user","id":"ou_xxx"},{"type":"department","id":"od_xxx"},{"type":"chat","id":"oc_xxx"}]' \
  --apply-enabled \
  --approver ou_yyy

# 互联网公开 + 免登
lark-cli apps +access-scope-set --app-id app_xxx --scope public --require-login=false

# 企业全员
lark-cli apps +access-scope-set --app-id app_xxx --scope tenant
```

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--app-id <id>` | ✅ | 应用 ID |
| `--scope <enum>` | ✅ | `specific` / `public` / `tenant` |
| `--targets <json>` | scope=specific 必填 | targets JSON 数组，每项 `{"type":"user\|department\|chat", "id":"<id>"}` |
| `--apply-enabled` | scope=specific 可选 | 是否允许申请访问 |
| `--approver <ou_xxx>` | `--apply-enabled` 必填 | 申请审批人（**只能传一个 user open_id**，服务端限制） |
| `--require-login` | scope=public 必填 | 是否要求登录 |

## 互斥校验（Validate 阶段，不通过直接报错不发请求）

- `scope=specific`：必传 `--targets`；不允许 `--require-login`
- `scope=public`：必传 `--require-login`；不允许 `--targets` / `--apply-enabled` / `--approver`
- `scope=tenant`：不允许任何其它 flag
- `--targets` 内每项的 `type` 必须是 `user` / `department` / `chat` 之一

## 返回值

**成功：**

```json
{ "ok": true, "data": {} }
```

**API 失败：**

```json
{ "ok": false, "error": { "type": "api_error", "message": "...", "hint": "..." } }
```

**Validate 失败（互斥违反，CLI 本地校验）：**

```json
{ "ok": false, "error": { "type": "validation", "message": "--targets is required when --scope=specific" } }
```

## 字段语义

- 成功时 `data` 为空对象，CLI 端基于 `--scope` 构造给用户的报告语
- Validate 错的 `error.type=validation` 是本地校验，**不发请求**

## 典型场景

### 场景 1：用户说"把应用 X 开放给全员"

```bash
lark-cli apps +access-scope-set --app-id app_xxx --scope tenant
```

> 应用 `{app_id}` 可用范围已设为企业全员。

### 场景 2：用户说"把应用 X 设为互联网公开 + 免登"

```bash
lark-cli apps +access-scope-set --app-id app_xxx --scope public --require-login=false
```

> 应用 `{app_id}` 可用范围已设为互联网公开（免登）。

### 场景 3：用户说"只让 Alice 和 Bob 访问应用 X"

先用 `lark-cli contact +search-user --query Alice` 拿到 ou_id，再调：

```bash
lark-cli apps +access-scope-set --app-id app_xxx \
  --scope specific \
  --targets '[{"type":"user","id":"ou_alice"},{"type":"user","id":"ou_bob"}]'
```

> 应用 `{app_id}` 可用范围已设为指定可见，目标人数 2。

### 场景 4：用户说"开放给「项目讨论群」"

把群名转 chat_id：用 `lark-cli im +chat-search --query "项目讨论群"`，再调：

```bash
lark-cli apps +access-scope-set --app-id app_xxx \
  --scope specific \
  --targets '[{"type":"chat","id":"oc_xxx"}]'
```

### 场景 5：互斥违反

例如 `--scope tenant --targets ...` —— Validate 本地拦截。**不发请求**。

### 场景 6：API 失败

转述 `error.hint` / `error.message`。

## 协同命令

| 场景 | 命令 |
|---|---|
| 拿 app_id | `apps +list` |
| 把人名转 ou_id | `lark-cli contact +search-user --query <name>` |
| 把群名转 chat_id | `lark-cli im +chat-search --query <群名>` |

## 参考

- [lark-apps](../SKILL.md)
- [lark-shared](../../lark-shared/SKILL.md)
