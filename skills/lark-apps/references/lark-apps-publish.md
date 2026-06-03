# apps +publish

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

为指定妙搭应用创建一个发布单（release），触发应用发布流水线。

## 命令

```bash
# 使用服务端默认分支发布
lark-cli apps +publish --app-id app_xxx

# 指定发布分支
lark-cli apps +publish --app-id app_xxx --branch main

# 预演（打印意图请求，不执行，当前可用）
lark-cli apps +publish --app-id app_xxx --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--app-id <id>` | ✅ | 应用 ID（如 `app_xxx`）|
| `--branch <branch>` | ❌ | 发布分支；不传则服务端使用默认分支 |

## 返回值

**成功：**

```json
{
  "ok": true,
  "data": {
    "release_id": "release_yyy",
    "status": "publishing"
  }
}
```

`release_id` 即发布ID；后续查状态 / 错误日志时把该值传给 `--release-id`。`status` 为字符串，初始值为 `publishing`，最终值为 `finished` 或 `failed`。

**Validate 失败（如缺 --app-id）：**

```json
{
  "ok": false,
  "error": { "type": "validation", "message": "--app-id is required" }
}
```

## --dry-run 示例输出

```json
{
  "ok": true,
  "data": {
    "dry_run": true,
    "api": [
      {
        "method": "POST",
        "url": "/open-apis/spark/v1/apps/app_xxx/releases",
        "body": { "branch": "main" }
      }
    ]
  }
}
```

## 字段语义

| 字段 | 含义 |
|---|---|
| `data.release_id` | 发布ID，用于 `+publish-status` / `+publish-error-log` 的 `--release-id` |
| `data.status` | 发布状态字符串：`publishing`（进行中）/ `finished`（成功）/ `failed`（失败）|
| `error.type=validation` | 本地参数错，修正 flag 后重试 |

## 典型场景

### 场景 1：触发发布并轮询状态

```bash
lark-cli apps +publish --app-id app_xxx
# 拿到 release_id 后查状态：
lark-cli apps +publish-status --app-id app_xxx --release-id release_yyy
```

## 协同命令

| 场景 | 命令 |
|---|---|
| 查发布历史 | `apps +publish-history` |
| 查单个发布状态 | `apps +publish-status` |
| 查发布错误日志 | `apps +publish-error-log` |

## 参考

- [lark-apps](../SKILL.md)
- [lark-shared](../../lark-shared/SKILL.md)
