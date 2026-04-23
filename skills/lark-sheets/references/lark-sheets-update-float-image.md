
# sheets +update-float-image（更新浮动图片）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +update-float-image`。

更新浮动图片的位置、大小和偏移量。

> [!CAUTION]
> 这是**写入操作** —— 执行前必须确认用户意图。可以先用 `--dry-run` 预览。

## 命令

```bash
lark-cli sheets +update-float-image --url "https://example.larksuite.com/sheets/shtxxxxxxxx" \
  --sheet-id "<sheetId>" --float-image-id "fi12345678" \
  --width 400 --height 300 --offset-y 20
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 否 | 电子表格 URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 否 | 表格 token |
| `--sheet-id` | 是 | 工作表 ID |
| `--float-image-id` | 是 | 浮动图片 ID |
| `--range` | 否 | 新锚定单元格，必须是单格（如 `sheetId!B2:B2`）。CLI 会校验前缀必须等于 `--sheet-id` |
| `--width` | 否 | 图片宽度（像素，`>=20`） |
| `--height` | 否 | 图片高度（像素，`>=20`） |
| `--offset-x` | 否 | 图片**左上角**到**锚定单元格左上角**的横向距离（向右为正，像素）；`>=0` 且**小于锚定单元格的宽度**（超限由服务端拒绝） |
| `--offset-y` | 否 | 图片**左上角**到**锚定单元格左上角**的纵向距离（向下为正，像素）；`>=0` 且**小于锚定单元格的高度**（超限由服务端拒绝） |
| `--dry-run` | 否 | 仅打印参数，不执行请求 |

> 必须至少传入 `--range` / `--width` / `--height` / `--offset-x` / `--offset-y` 其中之一；只传 ID 会被 CLI 拦截，避免 PATCH 空对象导致的无操作或服务端错误。

## 输出

JSON，包含更新后的 `float_image` 对象。**只返回元数据，不含图片字节**，如需查看图片内容用 `float_image_token` 调 `docs +media-preview`（见 [`lark-sheets-create-float-image.md`](lark-sheets-create-float-image.md) 的「读取图片内容」小节）。

## 常见错误

- `1310246 Wrong Float Image Value`：width/height/offset 参数不合法，CLI 会自动在 hint 中指向 `--width / --height / --offset-x / --offset-y`。典型成因：
  - `--width` / `--height` 小于 20；
  - `--offset-x` 大于等于锚定单元格宽度（或 `--offset-y` 大于等于单元格高度）；
  - 传了负值。

## 参考

- [lark-sheets-create-float-image](lark-sheets-create-float-image.md)
- [lark-sheets-get-float-image](lark-sheets-get-float-image.md)
