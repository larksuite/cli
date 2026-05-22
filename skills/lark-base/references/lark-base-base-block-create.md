# base +base-block-create

在 Base 容器里创建一个 block 条目。支持 `folder`、`table`、`docx`、`dashboard`、`workflow`。

## 推荐命令

```bash
lark-cli base +base-block-create \
  --base-token app_xxx \
  --type folder \
  --name "项目资料"

lark-cli base +base-block-create \
  --base-token app_xxx \
  --type docx \
  --name "需求文档" \
  --parent-id blk_folder
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token |
| `--type <type>` | 是 | `folder` / `table` / `docx` / `dashboard` / `workflow` |
| `--name <name>` | 是 | 新 block 名称 |
| `--parent-id <block_id>` | 否 | 父 folder 的 base block id；不传时创建在根层级 |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 返回重点

- 返回 `block` 和 `created=true`。
- `block.id` 是 Base 容器里的 block id。
- 如果创建 docx，返回中可能包含 `docx_token`，后续 docx 内容操作使用该 token。

## 坑点

- `--name` 必填，不能依赖默认名称。
- 创建的是 Base 容器里的入口及对应资源；资源内容仍需用 table/docx/dashboard/workflow 对应命令继续操作。
- 创建 workflow/dashboard 时，初始配置可能为空，后续需要再用对应模块补全。

## 参考

- [lark-base-base-block.md](lark-base-base-block.md) — base block 总览
