# drive +create-folder（创建云空间/云盘/云存储文件夹）

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

在飞书云空间（云盘/云存储）中创建一个新文件夹。该 shortcut 对原生 `drive files create_folder` 做了一层更适合日常使用的封装：`--folder-token` 可省略，此时会在调用者根目录创建。

## 命令

```bash
# 在根目录创建文件夹
lark-cli drive +create-folder \
  --name "周报归档"

# 在指定父文件夹下创建子文件夹
lark-cli drive +create-folder \
  --folder-token <PARENT_FOLDER_TOKEN> \
  --name "2026-W16"

# 预览底层调用
lark-cli drive +create-folder \
  --folder-token <PARENT_FOLDER_TOKEN> \
  --name "分析资料" \
  --dry-run
```

## 返回值

成功后会返回一个 JSON 对象，常见字段包括：

- `folder_token`：新建文件夹 token，可直接用于后续 `drive +move`、`drive +upload` 等命令
- `url`：新建文件夹链接（如果接口返回）
- `name`：文件夹名称
- `parent_folder_token`：父文件夹 token；为空字符串表示创建在根目录

> [!IMPORTANT]
> **不要擅自执行 owner 转移。** 如果用户需要把 owner 转给自己，必须单独确认。

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--name` | 是 | 文件夹名称，不能为空，最长 256 字节 |
| `--folder-token` | 否 | 父文件夹 token；省略时表示在调用者根目录创建 |

## 行为说明

- **根目录创建**：不传 `--folder-token` 时，shortcut 会向 API 显式传空字符串 `folder_token=""`，让后端按“根目录”语义创建
- **原生 API 仍可用**：如果用户明确要求按底层 API 字段调用，仍可继续使用 `lark-cli drive files create_folder`

## 推荐场景

- 用户说“在云空间（云盘/云存储）新建一个文件夹 / 目录”时，优先使用 `drive +create-folder`
- 用户给了父文件夹链接或 token，需要在其下继续分层建目录时，传 `--folder-token`
- 如果后续还要上传文件、移动文件、建子目录，优先复用返回值里的 `folder_token`

> [!CAUTION]
> `drive +create-folder` 是**写入操作**，执行前必须确认用户意图。

## 参考

- [lark-drive](../SKILL.md) -- 云空间（云盘/云存储）全部命令
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
