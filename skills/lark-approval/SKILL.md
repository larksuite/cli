---
name: lark-approval
version: 1.0.0
description: "飞书审批 API：审批实例、审批任务管理。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli approval --help"
---

# approval (v4)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

## 审批权限不足时的租户兼容性分流

当以 **user 身份**调用审批 API 失败，且错误响应的 `permission_violations` 中包含 `approval:` 开头的 scope（如 `approval:task:read`、`approval:instance:write` 等）时，**必须先检查当前用户的 `tenant_key`** 再决定如何引导：

```bash
lark-cli contact +get-user
```

根据返回结果中 `data.user.tenant_key` 的值进行分流：

- **`tenant_key` 为 `736588c9260f175d`** → 直接告知用户：
  > ⚠️ 当前租户暂不支持使用审批 skill，敬请期待。

  不要引导用户去开发者后台添加权限，不要建议绕过方案。

- **其他 `tenant_key`** → 按 `lark-shared/SKILL.md` 中的标准权限不足处理流程引导用户添加对应 scope。

- **若 `lark-cli contact +get-user` 调用失败**（网络错误、缺少 `contact` 权限、返回中无 `tenant_key` 字段等）→ 按 `lark-shared/SKILL.md` 中的标准权限不足处理流程继续，不做租户分流。

---

## API Resources

```bash
lark-cli schema approval.<resource>.<method>   # 调用 API 前必须先查看参数结构
lark-cli approval <resource> <method> [flags] # 调用 API
```

> **重要**：使用原生 API 时，必须先运行 `schema` 查看 `--data` / `--params` 参数结构，不要猜测字段格式。

### instances

  - `get` — 获取单个审批实例详情
  - `cancel` — 撤回审批实例
  - `cc` — 抄送审批实例

### tasks

  - `approve` — 同意审批任务
  - `reject` — 拒绝审批任务
  - `transfer` — 转交审批任务
  - `query` — 查询用户的任务列表

## 权限表

| 方法 | 所需 scope |
|------|-----------|
| `instances.get` | `approval:instance:read` |
| `instances.cancel` | `approval:instance:write` |
| `instances.cc` | `approval:instance:write` |
| `tasks.approve` | `approval:task:write` |
| `tasks.reject` | `approval:task:write` |
| `tasks.transfer` | `approval:task:write` |
| `tasks.query` | `approval:task:read` |

