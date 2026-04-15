
# sheets +create-float-image（创建浮动图片）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +create-float-image`。

在工作表中创建浮动图片。

> [!CAUTION]
> 这是**写入操作** —— 执行前必须确认用户意图。可以先用 `--dry-run` 预览。

## 前置步骤：获取 float_image_token

`--float-image-token` 由 [`sheets +media-upload`](lark-sheets-media-upload.md) 产出（内部走 `drive/v1/medias/upload_all`，`>20MB` 自动切到分片上传，详见 [`lark-sheets-media-upload.md`](lark-sheets-media-upload.md)）：

```bash
# 1. 上传图片，自动计算大小、自动分片
lark-cli sheets +media-upload --url "<url>" --file ./image.png
# 响应: {"file_token":"boxcnXXXX","file_name":"image.png","size":123456,"spreadsheet_token":"<token>"}

# 2. 用返回的 file_token 作为 --float-image-token
lark-cli sheets +create-float-image --url "<url>" --sheet-id "<sheetId>" \
  --float-image-token "boxcnXXXX" --range "<sheetId>!A1:A1"
```

> **常见错误**：
> - 用 `drive +upload` 的 token → 报 `Wrong Float Image Token`（走的是不同的上传接口，token 格式不兼容；必须用 `sheets +media-upload`）

## 命令

```bash
lark-cli sheets +create-float-image --url "https://example.larksuite.com/sheets/shtxxxxxxxx" \
  --sheet-id "<sheetId>" --float-image-token "boxcnXXXX" \
  --range "<sheetId>!A1:A1" --width 200 --height 150

# 指定自定义 ID 和偏移
lark-cli sheets +create-float-image --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --float-image-token "boxcnXXXX" \
  --range "<sheetId>!B2:B2" --width 300 --height 200 \
  --offset-x 10 --offset-y 20 --float-image-id "myImg12345"
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 否 | 电子表格 URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 否 | 表格 token |
| `--sheet-id` | 是 | 工作表 ID |
| `--float-image-token` | 是 | 图片 token（通过上方「前置步骤」的素材上传接口获取，不能用 `drive +upload` 的 token） |
| `--range` | 是 | 锚定单元格，必须是单格（如 `sheetId!A1:A1`）。CLI 会校验前缀必须等于 `--sheet-id` |
| `--width` | 否 | 图片宽度（像素，`>=20`；不传则使用图片原始宽度） |
| `--height` | 否 | 图片高度（像素，`>=20`；不传则使用图片原始高度） |
| `--offset-x` | 否 | 图片**左上角**到**锚定单元格左上角**的横向距离（向右为正，像素）；`>=0` 且**小于锚定单元格的宽度**（超限由服务端拒绝） |
| `--offset-y` | 否 | 图片**左上角**到**锚定单元格左上角**的纵向距离（向下为正，像素）；`>=0` 且**小于锚定单元格的高度**（超限由服务端拒绝） |
| `--float-image-id` | 否 | 自定义 10 位字母数字 ID（不传则自动生成） |
| `--dry-run` | 否 | 仅打印参数，不执行请求 |

## 输出

JSON，包含 `float_image`（float_image_id, float_image_token, range, width, height, offset_x, offset_y）。**只返回元数据，不含图片字节**，如需查看图片内容见下方「读取图片内容」。

## 读取图片内容

本接口及 `+get-float-image` / `+list-float-images` 均只返回 `float_image_token`。要读取图片字节，用该 token 调 `docs +media-preview`：

```bash
lark-cli docs +media-preview --token "<float_image_token>" --output ./image.png
```

`user` / `bot` 身份都可用，前提是调用方对该 spreadsheet 具备读权限。

## 常见错误

- `1310246 Wrong Float Image Value`：width/height/offset 参数不合法，CLI 会自动在 hint 中指向 `--width / --height / --offset-x / --offset-y`。典型成因：
  - `--width` / `--height` 小于 20；
  - `--offset-x` 大于等于锚定单元格宽度（或 `--offset-y` 大于等于单元格高度）；
  - 传了负值。

## 参考

- [lark-sheets-update-float-image](lark-sheets-update-float-image.md)
- [lark-sheets-get-float-image](lark-sheets-get-float-image.md)
- [lark-sheets-list-float-images](lark-sheets-list-float-images.md)
- [lark-sheets-delete-float-image](lark-sheets-delete-float-image.md)
