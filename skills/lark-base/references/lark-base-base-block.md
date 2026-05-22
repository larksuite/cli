# base +base-block-*

管理 Base 容器里的一级资源入口。Base block 是 Base 侧边栏/容器管理的条目，包括 folder、table、docx、dashboard、workflow。

> 注意：`base-block` 和 `dashboard-block` 不是同一个概念。`base-block` 是 Base 容器条目；`dashboard-block` 是仪表盘内部的图表/组件。

## 命令选择

| 目标 | 命令 | 关键参数 |
|------|------|----------|
| 列出 Base 容器条目 | `+base-block-list` | `--base-token`，可选 `--parent-id` |
| 创建 folder/table/docx/dashboard/workflow 条目 | `+base-block-create` | `--base-token`、`--type`、`--name`，可选 `--parent-id` |
| 移动条目到根或文件夹、调整同级位置 | `+base-block-move` | `--base-token`、`--block-id`，可选 `--parent-id`、`--before-id`、`--after-id` |
| 重命名条目 | `+base-block-rename` | `--base-token`、`--block-id`、`--name` |
| 删除条目 | `+base-block-delete` | `--base-token`、`--block-id`、`--yes` |

## 通用约束

- `--block-id` 是 Base 容器里的 block id，不是 docx token，也不是 dashboard 内部 chart/widget id。
- `--parent-id` 是目标 folder 的 base block id；创建和移动时不传表示根层级；list 时不传表示列出全部。
- 当前 CLI 不暴露分页参数。Base block 总数上限由后端控制，默认一次返回完整列表。
- 移动时 `--before-id` 和 `--after-id` 互斥。
- 当前不支持递归删除文件夹。删除非空 folder 时，先移动或删除其子项。
- 创建出的 docx/dashboard/workflow/table 的具体内容，需要继续用对应模块命令操作。

## 示例

```bash
lark-cli base +base-block-list --base-token app_xxx
lark-cli base +base-block-create --base-token app_xxx --type folder --name "项目资料"
lark-cli base +base-block-create --base-token app_xxx --type docx --name "需求文档" --parent-id blk_folder
lark-cli base +base-block-move --base-token app_xxx --block-id blk_docx --parent-id blk_folder --after-id blk_table
lark-cli base +base-block-rename --base-token app_xxx --block-id blk_docx --name "新的名称"
lark-cli base +base-block-delete --base-token app_xxx --block-id blk_docx --yes
```
