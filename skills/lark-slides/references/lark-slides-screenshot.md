# slides +screenshot

## 用途

获取幻灯片页面截图并保存为本地图片文件。默认用于已存在 PPT 页面截图；传入 `--content` 时用于直接渲染单个 `<slide>` XML 片段预览。本 shortcut 会在 CLI 进程内解码并写入文件，stdout 只返回文件路径、大小、页面 ID 等元信息，避免把图片 Base64 输出给模型。

注意：该截图能力受应用白名单限制，绝大多数应用不可用。若截图失败，记录错误即可；不要引导用户申请 `slides:presentation:screenshot` 权限。后续按 `validation-checklist.md` 走非截图验证，不要声称已完成截图验收。

## 命令

```bash
lark-cli slides +screenshot --as user \
  --presentation '<xml_presentation_id 或 slides/wiki URL>' \
  --slide-number 1
```

渲染本地 XML 内容：

```bash
lark-cli slides +screenshot --as user \
  --content @slide.xml
```

## 参数

| 参数 | 必需 | 说明 |
|------|------|------|
| `--presentation` | list 模式必需 | `xml_presentation_id`、`/slides/` URL，或解析后为 slides 的 `/wiki/` URL。传 `--content` 时不能使用 |
| `--slide-id` | list 模式至少提供 `--slide-id` / `--slide-number` 之一 | 页面 short ID；多页截图时重复传入；一次最多 10 页（`--slide-id` + `--slide-number` 合计小于等于 10） |
| `--slide-number` | list 模式至少提供 `--slide-id` / `--slide-number` 之一 | 页面页号；多页截图时重复传入；一次最多 10 页（`--slide-id` + `--slide-number` 合计小于等于 10） |
| `--content` | render 模式必需 | 要直接渲染的 `<slide>` XML 片段；支持直接传值、`@file`、`-` stdin。传入后不能同时传 `--slide-id` / `--slide-number` |
| `--output-dir` | 否 | 输出目录，默认 `.lark-slides/screenshots`；必须是当前目录内的相对路径 |
| `--output-name` | 否 | render 模式的输出文件名 stem；未指定时优先用返回的 `slide_id`，否则用 `rendered-slide`。若目标文件已存在，会自动追加递增后缀避免覆盖 |

## 示例

### 单页截图

```bash
lark-cli slides +screenshot --as user \
  --presentation slides_example_presentation_id \
  --slide-number 1
```

### 多页截图

一次不要超过 10 页；如需更多页面，分批调用。

```bash
lark-cli slides +screenshot --as user \
  --presentation slides_example_presentation_id \
  --slide-number 1 \
  --slide-number 2 \
  --output-dir .lark-slides/screenshots/demo
```

### 渲染 XML 预览

```bash
lark-cli slides +screenshot --as user \
  --content @.lark-slides/out/demo/slide.xml \
  --output-name preview
```

## 返回值

返回 JSON 不包含 Base64 图片内容：

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "xml_presentation_id": "slides_example_presentation_id",
    "output_dir": ".lark-slides/screenshots",
    "screenshots": [
      {
        "slide_id": "slide_example_id",
        "slide_number": 1,
        "format": "png",
        "path": "/abs/path/.lark-slides/screenshots/slides_example_presentation_id_p001_slide_example_id.png",
        "size": 12345
      }
    ]
  }
}
```

## 注意事项

1. 优先使用 `slides +screenshot` 保存本地图片，不要把图片 Base64 打到 stdout。
2. 已存在 PPT 页面截图时，不传 `--content`，用 `--presentation` + `--slide-id` 或 `--slide-number`。
3. 本地 XML 预览时，传 `--content @file` 或 `--content -`，内容应为单个 `<slide>` XML 片段；此时不要传 `--presentation` / `--slide-id` / `--slide-number`。
4. `slide_id` 是页面 short ID，页码请用 `--slide-number`。
5. list 模式一次最多传 10 页（`--slide-id` + `--slide-number` 合计小于等于 10）；更多页面请分批截图。
6. list 模式默认文件名包含 presentation ID、页码和/或 slide ID；文件已存在时自动追加 `_2`、`_3` 等后缀，避免覆盖旧截图。
7. 截图来自服务端渲染结果，适合创建/替换后验证页面是否为空白、破图或布局明显异常。
