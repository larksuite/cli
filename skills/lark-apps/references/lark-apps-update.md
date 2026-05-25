# apps +update

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)。

部分更新一个妙搭应用的元信息（名字 / 描述）。**只把传入的字段发给服务端，未传字段保持不变**。

## 命令

```bash
lark-cli apps +update --app-id app_xxx --name "调研问卷 v2"
lark-cli apps +update --app-id app_xxx --description "新描述"
lark-cli apps +update --app-id app_xxx --name "v2" --description "新描述"
```

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--app-id <id>` | ✅ | 应用 ID |
| `--name <str>` | ❌ | 新名字 |
| `--description <str>` | ❌ | 新描述 |

`--name` 和 `--description` 至少传一个，否则 Validate 阶段报错。

## 返回值

**成功：**

```json
{
  "ok": true,
  "data": {
    "app": {
      "app_id": "app_4k5jepcbjmv6m",
      "name": "调研问卷 v2",
      "description": "...",
      "icon_url": "https://lf3-static.bytednsdoc.com/.../feisuda/avatar/5.svg",
      "created_at": "2026-05-18T10:00:00Z",
      "updated_at": "2026-05-18T10:05:00Z"
    }
  }
}
```

**失败：**

```json
{
  "ok": false,
  "error": { "type": "api_error", "message": "...", "hint": "..." }
}
```

## 字段语义

- 响应 `data.app` 含完整应用对象（所有字段），不只是被改的
- `created_at` / `updated_at` 都是 ISO 8601 UTC 时间字符串
- 失败时优先转述 `error.hint`

## 典型场景

### 场景 1：用户说"把应用 X 改名叫 Y"

```bash
lark-cli apps +update --app-id app_xxx --name "Y"
```

> 应用 `{app_id}` 已更新，新名字「{name}」。

### 场景 2：缺 `--app-id` 或没传可更新字段

Validate 直接拦截，提示用户加 flag。

### 场景 3：失败处理

转述 `error.hint` / `error.message`。

## 协同命令

| 场景 | 命令 |
|---|---|
| 找 app_id | 从用户提供的妙搭应用链接 `https://miaoda.feishu.cn/app/app_xxx` 的 `/app/` 后面提取，或让用户直接给 `app_xxx` 字符串（详见 `../SKILL.md`） |
| 创建新应用 | `apps +create` |

## 参考

- [lark-apps](../SKILL.md)
- [lark-shared](../../lark-shared/SKILL.md)
