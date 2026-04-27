
# sheets +delete-float-image（删除浮动图片）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +delete-float-image`。

删除工作表中的浮动图片。

> [!CAUTION]
> 这是**删除操作** —— 执行前必须确认用户意图。可以先用 `--dry-run` 预览。

## 命令

```bash
lark-cli sheets +delete-float-image --url "https://example.larksuite.com/sheets/shtxxxxxxxx" \
  --sheet-id "<sheetId>" --float-image-id "fi12345678"
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 否 | 电子表格 URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 否 | 表格 token |
| `--sheet-id` | 是 | 工作表 ID |
| `--float-image-id` | 是 | 浮动图片 ID |
| `--dry-run` | 否 | 仅打印参数，不执行请求 |

## 输出

JSON，包含 `code`（0=成功）和 `msg`。

## 参考

- [lark-sheets-create-float-image](lark-sheets-create-float-image.md)
- [lark-sheets-list-float-images](lark-sheets-list-float-images.md)
