# lark-slides +bulk-media-upload

## 用途

批量上传本地图片到指定演示文稿，并返回稳定的 `file_name -> file_token` map。

## 命令

```bash
lark-cli slides +bulk-media-upload --as user \
  --presentation "slides_xxx" \
  --dir ./assets
```

或重复传文件：

```bash
lark-cli slides +bulk-media-upload --as user \
  --presentation "slides_xxx" \
  --asset-root ./assets \
  --files cover.png \
  --files chip.jpg
```

## 关键参数

| 参数 | 说明 |
|------|------|
| `--presentation` | `xml_presentation_id`、`/slides/` URL，或能解析成 slides 的 wiki URL |
| `--files` | 本地图片路径，可重复 |
| `--dir` | 本地图片目录，非递归扫描常见图片后缀 |
| `--asset-root` | `--files` 的素材根目录 |

## 返回值

成功时返回：

- `presentation_id`
- `uploaded_count`
- `file_names`
- `file_tokens`
- `files`

## 注意事项

1. 相同文件名会被拒绝，因为返回结果按文件名建 map
2. 目录扫描只包含常见图片后缀：png/jpg/jpeg/webp/gif/bmp/svg
3. 单图仍受 20 MB 限制
