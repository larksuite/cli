# minutes +todo

> **路由**：本命令操作**妙记内的 AI 待办**，不是飞书任务（Task）。用户说「在妙记里新建待办」时**必须**用本命令，**禁止**走 `lark-cli task` / `tasklists list` / `task +create`。详见 [lark-minutes/SKILL.md](../SKILL.md) 第 6 节。


对妙记中的待办做新增 / 更新 / 删除（单条或批量）。写操作。

本 skill 对应 shortcut：`lark-cli minutes +todo`（调用 `POST /open-apis/minutes/v1/minutes/{minute_token}/todo`）。

## 典型触发表达

- "给这条妙记加一条/多条待办"
- "把某条待办改成……"
- "标记某条待办为已完成 / 取消完成"
- "删除某条待办"

## 命令

**单条模式**：`--operation` + 对应字段。  
**批量模式**：`--todos` JSON 数组（与单条 flags 互斥），一次请求可混合 `add` / `update` / `delete`。

```bash
# 单条：新增
lark-cli minutes +todo --minute-token obcnxxxxxxxxxxxxxxxxxxxx --operation add --todo "跟进预算审批" --is-done=false --as user

# 批量：一次新增两条
lark-cli minutes +todo --minute-token obcnxxxxxxxxxxxxxxxxxxxx --as user --todos '[
  {"operation":"add","content":"晚上好1","is_done":true},
  {"operation":"add","content":"晚上好2","is_done":false}
]'

# 批量：混合增删改
lark-cli minutes +todo --minute-token obcnxxxxxxxxxxxxxxxxxxxx --as user --todos '[
  {"operation":"add","content":"新待办","is_done":false},
  {"operation":"update","todo_id":"1234567890","content":"已更新","is_done":true},
  {"operation":"delete","todo_id":"9876543210"}
]'

# 从文件读取
lark-cli minutes +todo --minute-token obcnxxxxxxxxxxxxxxxxxxxx --as user --todos @todos.json

# 单条：更新 / 删除
lark-cli minutes +todo --minute-token obcnxxxxxxxxxxxxxxxxxxxx --operation update --todo-id 1234567890 --todo "整理会议纪要" --is-done --as user
lark-cli minutes +todo --minute-token obcnxxxxxxxxxxxxxxxxxxxx --operation delete --todo-id 1234567890 --as user

# 预览
lark-cli minutes +todo --minute-token obcnxxxxxxxxxxxxxxxxxxxx --operation add --todo "新待办" --is-done --dry-run --as user
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--minute-token <token>` | 是 | 妙记 Token |
| `--operation <op>` | 单条模式 | `add` / `update` / `delete`；与 `--todos` 互斥 |
| `--todo <text>` | 单条 add/update | 待办纯文本 |
| `--is-done` | 单条 add/update | `--is-done` = true，`--is-done=false` = false |
| `--todo-id <id>` | 单条 update/delete | 已有待办 id |
| `--todos <json>` | 批量模式 | JSON 数组，支持 `@file` / `@-`；与单条 flags 互斥 |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 单条模式

| `--operation` | 必填参数 | 禁止参数 |
|---------------|----------|----------|
| `add` | `--todo` + `--is-done` | `--todo-id` |
| `update` | `--todo-id` + `--todo` + `--is-done` | — |
| `delete` | `--todo-id` | `--todo`、`--is-done` |

## 批量模式：`--todos`

每条元素字段与 API `todo_items[]` 一致：

| JSON 字段 | add | update | delete |
|-----------|-----|--------|--------|
| `operation` | 必填 | 必填 | 必填 |
| `content` | 必填 | 必填 | 禁止 |
| `is_done` | 必填 | 必填 | 禁止 |
| `todo_id` | 禁止 | 必填 | 必填 |

示例 `todos.json`：

```json
[
  {"operation": "add", "content": "晚上好1", "is_done": true},
  {"operation": "add", "content": "晚上好2", "is_done": false}
]
```

数组顺序会原样写入请求体；端上展示顺序仍可能受完成状态分组影响。

## 核心约束

### 1. 先读后写，待办 id 如何获取

更新 / 删除前先用 `lark-cli minutes +detail --minute-tokens <token> --todo` 读取当前待办。返回的每条待办带 `todo_id` 字段。

> 待办 id 仅用于程序内部定位，不必展示给用户。

### 2. 待办内容为纯文本

`content` **不是 Markdown**，请直接传入待办描述文字。

### 3. 所需权限

| 身份 | 所需 scope |
|------|-----------|
| user | `minutes:minutes:update` |

## 输出结果

```json
{
  "minute_token": "obcnxxxxxxxxxxxxxxxxxxxx",
  "count": 2,
  "updated": true
}
```

单条模式额外包含 `"operation": "add"`。

## 常见错误与排查

| 错误现象 | 解决方案 |
|---------|---------|
| 未指定操作 | 单条模式传 `--operation`，或批量传 `--todos` |
| `--todos` 与单条 flags 冲突 | 二选一 |
| `todos[i]` 校验失败 | 检查该条 `operation` 与字段组合 |
| `error.type` = `no_edit_permission` | **妙记资源无编辑权**：向妙记所有者申请该妙记的编辑/协作权限；**不要**走 `auth login --scope` |
| 缺少 OAuth scope（`permission_violations` 含 `minutes:minutes:update`） | `lark-cli auth login --scope "minutes:minutes:update"` |

## 参考

- [lark-minutes](../SKILL.md)
- [minutes +summary](lark-minutes-summary.md)
- [minutes +detail](lark-minutes-detail.md)
