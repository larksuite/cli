# lark-slides +replace-slide-xml

## 用途

整页替换已有 slide。CLI 会固定执行：

1. 如有 `@path` 图片占位符，先上传素材
2. 在旧页前创建新页
3. 删除旧页

适合“改一页 XML 但不想手工做 create + delete 编排”的场景。

## 命令

```bash
lark-cli slides +replace-slide-xml --as user \
  --presentation "slides_xxx" \
  --slide-id "slide_old" \
  --content @./slides/replacement.xml \
  --asset-root ./assets
```

## 关键参数

| 参数 | 说明 |
|------|------|
| `--presentation` | `xml_presentation_id`、`/slides/` URL，或 wiki URL |
| `--slide-id` | 要被替换的旧 `slide_id` |
| `--content` | 新的 slide XML，支持 `@file` / stdin |
| `--asset-root` | 新 XML 中 `@path` 图片占位符的素材根目录 |

## 返回值

成功时返回：

- `presentation_id`
- `replaced_slide_id`
- `replacement_slide_id`
- `create_revision_id`
- `delete_revision_id`
- `images_uploaded`

## 注意事项

1. 这是整页替换，不是元素级 patch
2. 新页 `slide_id` 一定会变化
3. 如果“创建新页成功但删除旧页失败”，错误信息会带上新的 `slide_id`，便于人工恢复
