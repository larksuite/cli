
# sheets +media-upload（上传浮动图片素材）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +media-upload`。

把本地图片上传到指定电子表格的素材空间，返回 `file_token`，该 token 可以作为 [`+create-float-image`](lark-sheets-create-float-image.md) 的 `--float-image-token` 使用。

> [!CAUTION]
> 这是**写入操作**（创建素材）—— 执行前必须确认用户意图。可以先用 `--dry-run` 预览。

## 说明

- 内部调用 `drive/v1/medias/upload_all`，`parent_type` 锁定为 `sheet_image`，`parent_node` 取自 `--url` / `--spreadsheet-token`。
- 文件大小通过 `FileIO.Stat` 自动读取，无需手动算（跨平台一致）。
- `>20MB` 自动切换到分片上传（`upload_prepare` → `upload_part` → `upload_finish`），无需额外参数。

## 命令

```bash
# 小文件（<=20MB）
lark-cli sheets +media-upload --url "https://example.larksuite.com/sheets/shtxxxxxxxx" \
  --file ./image.png

# 也支持 --spreadsheet-token
lark-cli sheets +media-upload --spreadsheet-token "shtxxxxxxxx" --file ./image.png
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 否 | 电子表格 URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 否 | 表格 token |
| `--file` | 是 | 本地图片路径，**必须是相对当前工作目录的相对路径**（见下方「注意事项」）；>20MB 自动分片 |
| `--dry-run` | 否 | 仅打印请求计划，不执行 |

## 输出

```json
{
  "file_token": "boxcnXXXX",
  "file_name": "image.png",
  "size": 358934,
  "spreadsheet_token": "shtxxxxxxxx"
}
```

## 典型用法：上传 + 插入

```bash
# 1. 上传
TOKEN=$(lark-cli sheets +media-upload --url "<url>" --file ./image.png --jq '.data.file_token')

# 2. 插入浮动图片
lark-cli sheets +create-float-image --url "<url>" --sheet-id "<sheetId>" \
  --float-image-token "$TOKEN" --range "<sheetId>!A1:A1" --width 300 --height 200
```

## 注意事项

- **`--file` 只接受当前工作目录（CWD）下的相对路径**。CLI 的 `SafeInputPath` 会拒绝绝对路径以及逃出 CWD 的路径（`..` 展开后超出 CWD 也会拒）。
  - ❌ 错误：`--file /Users/alice/Desktop/image.png`
  - ❌ 错误：`--file ~/Desktop/image.png`（shell 会展开为绝对路径）
  - ✅ 正确：`cp /Users/alice/Desktop/image.png ./image.png && lark-cli sheets +media-upload --file ./image.png ...`
  - 典型报错：`unsafe file path: --file must be a relative path within the current directory`。
- 所需权限：`docs:document.media:upload`（与 docs/slides/base 的媒体上传共用同一 scope）。
- 返回的 `file_token` **只能**用于浮动图片；走 `drive +upload` 拿到的 token 格式不兼容，会报 `Wrong Float Image Token`。

## 参考

- [lark-sheets-create-float-image](lark-sheets-create-float-image.md) — 用返回的 token 创建浮动图片
- [lark-sheets-get-float-image](lark-sheets-get-float-image.md) — 读取浮动图片元数据
