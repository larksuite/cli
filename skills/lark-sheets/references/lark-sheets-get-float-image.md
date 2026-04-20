
# sheets +get-float-image（获取浮动图片）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +get-float-image`。

获取单个浮动图片的详细信息。

## 命令

```bash
lark-cli sheets +get-float-image --url "https://example.larksuite.com/sheets/shtxxxxxxxx" \
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

JSON，包含 `float_image`（float_image_id, float_image_token, range, width, height, offset_x, offset_y）。**只返回元数据，不含图片字节**。

## 读取图片内容

本接口只返回 `float_image_token`。要读取图片字节，用 token 调 `docs +media-preview`：

```bash
lark-cli docs +media-preview --token "<float_image_token>" --output ./image.png
```

`user` / `bot` 身份都可用，前提是调用方对该 spreadsheet 具备读权限。

## 参考

- [lark-sheets-list-float-images](lark-sheets-list-float-images.md)
- [lark-sheets-create-float-image](lark-sheets-create-float-image.md)
