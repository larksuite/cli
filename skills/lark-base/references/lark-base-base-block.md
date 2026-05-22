# base +base-block-*

管理 Base 容器里的一级资源入口。Base block 是 Base 侧边栏/容器管理的条目，包括 folder、table、docx、dashboard、workflow。

> 注意：`base-block` 和 `dashboard-block` 不是同一个概念。`base-block` 是 Base 容器条目；`dashboard-block` 是仪表盘内部的图表/组件。

## 命令选择

| 目标 | 命令 | 参考 |
|------|------|------|
| 列出 Base 容器条目 | `+base-block-list` | [lark-base-base-block-list.md](lark-base-base-block-list.md) |
| 创建 folder/table/docx/dashboard/workflow 条目 | `+base-block-create` | [lark-base-base-block-create.md](lark-base-base-block-create.md) |
| 移动条目到根或文件夹、调整同级位置 | `+base-block-move` | [lark-base-base-block-move.md](lark-base-base-block-move.md) |
| 重命名条目 | `+base-block-rename` | [lark-base-base-block-rename.md](lark-base-base-block-rename.md) |
| 删除条目 | `+base-block-delete` | [lark-base-base-block-delete.md](lark-base-base-block-delete.md) |

## 通用约束

- `--block-id` 是 Base 容器里的 block id，不是 docx token，也不是 dashboard 内部 chart/widget id。
- `--parent-id` 是目标 folder 的 base block id；不传表示根层级。
- 当前 CLI 不暴露分页参数。Base block 总数上限由后端控制，默认一次返回完整列表。
- 当前不支持递归删除文件夹。删除非空 folder 时，先移动或删除其子项。
- 创建出的 docx/dashboard/workflow/table 的具体内容，需要继续用对应模块命令操作。
