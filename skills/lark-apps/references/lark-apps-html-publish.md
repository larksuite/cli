# apps +html-publish

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)。

把本地的 HTML 文件或目录部署为可访问的妙搭应用，响应返回应用的访问链接 `url`。

## 命令

```bash
# 发布整个目录
lark-cli apps +html-publish --app-id app_xxx --path ./dist/

# 发布单个 HTML 文件
lark-cli apps +html-publish --app-id app_xxx --path ./index.html

# 预演（打印文件清单 + SHA256 + 目标 endpoint，不发请求）
lark-cli apps +html-publish --app-id app_xxx --path ./dist --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--app-id <id>` | ✅ | 应用 ID（从 `apps +create` 或 `apps +list` 拿） |
| `--path <path>` | ✅ | 本地文件或目录路径；目录会递归打包成 tar.gz。**必须含 `index.html`**：目录形态时根目录下，单文件形态时文件名必须就是 `index.html`（妙搭统一以 `index.html` 作为应用入口） |

## 返回值

**成功：**

```json
{
  "ok": true,
  "data": {
    "url": "https://miaoda.feishu.cn/app/app_4k5jepcbjmv6m"
  }
}
```

**业务失败（如构建失败、应用不存在）：**

```json
{
  "ok": false,
  "error": {
    "type": "api_error",
    "code": "api_error",
    "message": "html-publish failed (code=90001): build failed: dependency conflict",
    "hint": "构建失败：用 `lark-cli apps +html-publish --path <path> --dry-run` 检查打包文件清单"
  }
}
```

**基础设施失败（网络 / HTTP 5xx）：**

```json
{
  "ok": false,
  "error": { "type": "infra_error", "message": "...", "hint": "" }
}
```

**Validate 失败（本地校验，如缺 --app-id）：**

```json
{
  "ok": false,
  "error": { "type": "validation", "message": "--app-id is required" }
}
```

## 字段语义

| 字段 / 组合 | 含义 |
|---|---|
| `data.url` 存在且无 `error` | 发布成功，URL 可访问 |
| `error.type=api_error` | 业务失败（构建失败、应用不存在等），按 `hint` 引导用户修复 |
| `error.type=infra_error` | 网络 / 服务端 5xx，告诉用户稍后重试 |
| `error.type=validation` | 本地参数错，提示用户修 flag |
| `error.hint` 非空 | **优先转述给用户**，比 `error.message` 更可操作 |

## 典型场景

### 场景 1：用户说"把这个目录发布到妙搭"

```bash
lark-cli apps +html-publish --app-id app_xxx --path ./dist
```

成功后：

> 应用发布成功！访问 `{url}` 查看。

可选追加：

> 如需让其他人访问，可以用 `apps +access-scope-set` 设置可用范围。

### 场景 2：用户没有 app_id

```bash
APP=$(lark-cli apps +create --name "..." -q '.data.app_id' | tr -d '"')
lark-cli apps +html-publish --app-id "$APP" --path ./dist
```

### 场景 3：构建失败（code=90001）

转述 hint：

> 构建失败，建议用 `lark-cli apps +html-publish --path ./dist --dry-run` 看一下打包文件清单是否完整。

### 场景 4：应用不存在（code=90002）

> hint："应用不存在或无权访问；用 `lark-cli apps +list` 确认 app_id"

转述给用户。

### 场景 5：网络 / 服务端失败（infra_error）

> 服务暂时不可用，建议稍后重试。

## 提示

- `--path` 可以传相对路径
- 目录打包成 tar.gz 时**不做过滤**（`.git` / `node_modules` 等会一并打包），让用户传干净的产物目录（如 `./dist`）
- **不要**原样把 envelope JSON 转述给用户

## 协同命令

| 场景 | 命令 |
|---|---|
| 创建新应用 | `apps +create` |
| 设置可用范围 | `apps +access-scope-set` |

## 参考

- [lark-apps](../SKILL.md)
- [lark-shared](../../lark-shared/SKILL.md)
