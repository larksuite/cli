# base +base-create

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

创建一个新的 Base；可选指定父文件夹和时区。

## 推荐命令

```bash
lark-cli base +base-create \
  --name "New Base"

lark-cli base +base-create \
  --name "项目管理" \
  --folder-token fld_xxx \
  --time-zone Asia/Shanghai
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--name <name>` | 是 | 新 Base 名称 |
| `--folder-token <token>` | 否 | 目标文件夹 token |
| `--time-zone <tz>` | 否 | 时区，如 `Asia/Shanghai` |

## API 入参详情

**HTTP 方法和路径：**

```
POST /open-apis/base/v3/bases
```

## 返回重点

- 返回 `base`。
- CLI 会额外标记 `created: true`。
- 回复结果时，必须主动返回新 Base 的可访问链接：
  - 优先使用返回结果中的 `base.url`
  - 同时返回新 Base 的 token；字段名以实际返回为准，常见为 `base_token` 或 `app_token`
  - 如果本次返回没有 `url`，至少返回新 Base 的名称和 token

> [!IMPORTANT]
> 如果 Base 是**以应用身份（bot）创建**的，shortcut 会在创建成功后自动尝试为当前 CLI 用户添加该 Base 的 `full_access`（管理员）权限，并在输出中附带 `permission_grant` 字段。
>
> `permission_grant.status` 语义如下：
> - `granted`：当前 CLI 用户已获得该 Base 的管理员权限
> - `skipped`：Base 已创建成功，但没有可授权的当前 CLI 用户，或创建结果缺少可授权 token
> - `failed`：Base 已创建成功，但自动授权失败；结果中会包含失败原因，用户可稍后重试授权，或继续使用应用身份（bot）处理该 Base
>
> 回复创建结果时，除 `base token` 和可访问链接外，还必须明确告知用户 `permission_grant` 的结果。
>
> **仍然不要擅自执行 owner 转移。** 如果用户需要把 owner 转给自己，必须单独确认。

## 工作流

> [!CAUTION]
> 这是**写入操作** — 执行前必须向用户确认。

1. 先确认 Base 名称。
2. `--folder-token`、`--time-zone` 都是可选项；用户没要求时不要为此额外追问。
3. 创建成功后，整理并返回：Base 名称、token，以及响应中已有的可访问链接。
4. 创建成功时，只需说明：新 Base 里会自带 1 张默认空表，表内会预置 5-10 行空记录。

## 默认表删除决策规则

### 触发条件

- 当用户后续已经明显把工作重心转到默认表之外时，必须询问默认表是否需要删除。
- 可接受的触发信号只有这两类：
  - 用户新增了其他表。
  - 用户在非默认表里新增了字段、视图或记录。

### 禁止事项

- 不要在刚创建 Base 后立刻追问是否删除。
- 不要把“用户接下来可能不用它”当成删除信号。

### 删除前置条件

- 只有同时满足以下条件，才可以进入删除流程：
  - 已满足上面的触发条件。
  - 用户已明确表示要删除默认表。
  - `+table-list` 已确认当前 Base 内的真实表列表，并能明确识别删除目标。
- 缺少任一条件时，都不要执行删除。
- 删除目标确认后，才继续走 `+table-delete`。

## 注意事项

- 即使用户已同意删除，也不要直接假设默认表的 `table_id` 或名称；先列出表，再基于真实返回结果删除。

## 参考

- [lark-base-workspace.md](lark-base-workspace.md) — base / workspace 索引页
- [lark-base-base-copy.md](lark-base-base-copy.md) — 复制 Base
