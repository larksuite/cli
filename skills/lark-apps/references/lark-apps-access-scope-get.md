# apps +access-scope-get

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)。

获取应用当前的可用范围配置。一次 `GET /apps/{appId}/access-scope` 调用，响应原样透传服务端契约（字符串 scope 枚举 + 拆分数组）。

## 命令

```bash
lark-cli apps +access-scope-get --app-id app_xxx
```

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--app-id <id>` | ✅ | 应用 ID |

## 返回值

**成功（specific，三种 target 类型混合）：**

```json
{
  "ok": true,
  "data": {
    "scope": "Range",
    "users": ["ou_xxx", "ou_yyy"],
    "departments": ["od_xxx"],
    "chats": ["oc_xxx"],
    "apply_config": {
      "enabled": true,
      "approvers": ["ou_approver"]
    }
  }
}
```

**成功（public + 免登）：**

```json
{ "ok": true, "data": { "scope": "All", "require_login": false } }
```

**成功（tenant）：**

```json
{ "ok": true, "data": { "scope": "Tenant" } }
```

**失败：**

```json
{ "ok": false, "error": { "type": "api", "message": "...", "hint": "..." } }
```

## 字段语义

- `scope` 是**字符串枚举**：
  - `"All"` = 互联网公开 — 对应 `apps +access-scope-set --scope public`
  - `"Tenant"` = 组织内 — 对应 `--scope tenant`
  - `"Range"` = 部分人员 — 对应 `--scope specific`
- `users` / `departments` / `chats` 三个数组（仅 `scope="Range"` 时）：服务端拆分形态，CLI 不合并回统一 targets
- `apply_config`（可选，仅 `scope="Range"` 且申请开启时）：含 `enabled` 和 `approvers`（只允许一个 user open_id）
- `require_login`（仅 `scope="All"` 时）：bool

## 典型场景

### 场景 1：查看当前应用对谁可见

```bash
lark-cli apps +access-scope-get --app-id app_xxx
```

按 `scope` 值组装报告：
- `scope="All"` → "应用 `{app_id}` 当前互联网公开（require_login={require_login}）"
- `scope="Tenant"` → "应用 `{app_id}` 当前对企业全员可见"
- `scope="Range"` → "应用 `{app_id}` 当前指定可见，包含 N 个用户 / M 个部门 / K 个群"

### 场景 2：把 GET 响应拼回 `+access-scope-set` 命令（复制 / 备份可用范围）

```bash
# 拼一个 --targets JSON 数组（jq）
lark-cli apps +access-scope-get --app-id app_src -q '
  .data
  | (.users        // [] | map({type:"user",       id:.}))
  + (.departments // [] | map({type:"department", id:.}))
  + (.chats        // [] | map({type:"chat",       id:.}))
'
```

得到 `[{"type":"user","id":"ou_x"}, ...]` 数组，可作为 `apps +access-scope-set --targets '...'` 的入参。

## 协同命令

| 场景 | 命令 |
|---|---|
| 设置可用范围 | `apps +access-scope-set` |
| 拿 app_id | 从用户提供的妙搭应用链接 `https://miaoda.feishu.cn/app/app_xxx` 的 `/app/` 后面提取，或让用户直接给 `app_xxx` 字符串（详见 `../SKILL.md`） |

## 参考

- [lark-apps](../SKILL.md)
- [lark-shared](../../lark-shared/SKILL.md)
