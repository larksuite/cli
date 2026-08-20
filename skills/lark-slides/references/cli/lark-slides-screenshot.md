# slides +screenshot

## 用途

获取幻灯片页面截图并保存为本地图片文件。默认用于已存在 PPT 页面截图；传入 `--content` 时用于直接渲染单个 `<slide>` XML 片段预览。本 shortcut 会在 CLI 进程内解码并写入文件，stdout 只返回文件路径、大小、页面 ID 等元信息，避免把图片 Base64 输出给模型。

截图失败则降级到 XML 读回、结构 lint等非截图检查路径。

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

## 截图全部页面

枚举全部页面的 `slide_id` 或页码，按每批最多 10 页分组并串行调用 `slides +screenshot`，复用同一个 `--output-dir`；记录失败批次，已完成批次不重复执行。

## 参数

| 参数 | 必需 | 说明 |
|------|------|------|
| `--presentation` | list / overview 模式必需 | `xml_presentation_id`、`/slides/` URL，或解析后为 slides 的 `/wiki/` URL。传 `--content` 时不能使用 |
| `--slide-id` | list 模式与 `--slide-number` 二选一 | 页面 short ID；不能与 `--slide-number` 同时使用；多页截图时重复传入，或用逗号分隔一次传多个（如 `--slide-id slide_1,slide_2`）；一次最多 10 个 ID |
| `--slide-number` | list 模式与 `--slide-id` 二选一 | 页面页号；不能与 `--slide-id` 同时使用；多页截图时重复传入，或用逗号分隔一次传多个（如 `--slide-number 1,2,3`）；一次最多 10 个页码 |
| `--content` | render 模式必需 | 要直接渲染的 `<slide>` XML 片段；支持直接传值、`@file`、`-` stdin。传入后不能同时传 `--slide-id` / `--slide-number` |
| `--output` | 否 | 单张截图或 overview 图片的期望相对输出路径，可不写扩展名。普通单张截图的显式扩展名支持 `.png`、`.jpg`、`.jpeg`；overview 的显式扩展名只能是 `.png`。只能选择一页或使用 `--overview`，不能与 `--output-dir` / `--output-name` 同时使用；最终路径以返回的 `output` 为准 |
| `--output-dir` | 否 | 输出目录，默认 `.lark-slides/screenshots`；必须是当前目录内的相对路径 |
| `--output-name` | 否 | 仅用于 `--content` render 模式设置输出文件名 stem。普通页面截图传入该参数会返回 `validation/invalid_argument`（`param: --output-name`）并提示改用 `--output` |
| `--overview` | 否 | 输出当前演示文稿的一张索引总览 PNG。每个 overview 页最多包含 20 页幻灯片，按固定 4 列排列；不与 `--slide-id`、`--slide-number`、`--content`、`--region` 同时使用 |
| `--overview-page` | 否，默认 `1` | 从 1 开始的 overview 页号，仅与 `--overview` 同时使用。返回值中的 `has_next`、`next_overview_page`、`has_previous`、`previous_overview_page` 表示分页状态 |
| `--region` | 否 | 对一张已存在幻灯片的服务端截图按 `x,y,width,height` 像素矩形裁剪。`x`、`y` 为非负整数，`width`、`height` 为正整数，矩形必须在源截图范围内；需要且仅能使用一个 `--slide-id` 或 `--slide-number`，不与 `--content`、`--overview` 同时使用 |

## 示例

### 单页截图并固定路径

```bash
lark-cli slides +screenshot --as user \
  --presentation slides_example_presentation_id \
  --slide-number 1 \
  --output .lark-slides/screenshots/example-deck-task/page-01
```

按 `slide_id` 选择单页时同样使用 `--output`：

```bash
lark-cli slides +screenshot --as user \
  --presentation slides_example_presentation_id \
  --slide-id slide_example_id \
  --output .lark-slides/screenshots/example-deck-task/page-01
```

### 多页截图

一次不要超过 10 页；如需更多页面，分批调用。可以重复传参，也可以用逗号分隔一次传多个：

```bash
lark-cli slides +screenshot --as user \
  --presentation slides_example_presentation_id \
  --slide-number 1 \
  --slide-number 2 \
  --output-dir .lark-slides/screenshots/example-deck-task
```

### 渲染 XML 预览

```bash
lark-cli slides +screenshot --as user \
  --content @.lark-slides/out/demo/slide.xml \
  --output .lark-slides/screenshots/example-deck-task/preview
```

## 返回值

返回 JSON 不包含 Base64 图片内容：

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "xml_presentation_id": "slides_example_presentation_id",
    "output": "/abs/path/.lark-slides/screenshots/example-deck-task/page-01.jpg",
    "screenshots": [
      {
        "slide_id": "slide_example_id",
        "slide_number": 1,
        "format": "jpeg",
        "path": "/abs/path/.lark-slides/screenshots/example-deck-task/page-01.jpg",
        "size": 12345,
        "image_size": {"width": 1920, "height": 1080}
      }
    ]
  }
}
```

常规截图在本地可解析图像尺寸时，`screenshots[]` 包含 `image_size.width` 和 `image_size.height`。

`--overview` 的 `data.overview` 包含：

- `path`、`format`、`size`、`image_size`：合成 PNG 的本地路径、格式、字节数和像素尺寸。
- `columns`（固定为 `4`）、`total_slides`、`overview_page`、`page_size`、`slide_range`：网格和分页信息。
- `has_previous`、`previous_overview_page`、`has_next`、`next_overview_page`：存在时表示相邻 overview 页。
- `slides[]`：当前 overview 页内每张幻灯片的 `index`、`label`、`slide_id`、`slide_number`、`row`、`column`、`tile` 和 `thumbnail`。后两者为 `{x,y,width,height}` 像素矩形。

`--region` 的 `data.region` 包含 `requested_pixel_rect`、`source_image_size`、`output_image_size` 和 `format`（固定为 `png`）。裁剪后的图片仍通过 `data.output` 和 `data.screenshots[0]` 返回。

## 注意事项

1. 优先使用 `slides +screenshot` 保存本地图片，不要把图片 Base64 打到 stdout。
2. 已存在 PPT 页面截图时，不传 `--content`，用 `--presentation` + `--slide-id` 或 `--slide-number`。
3. 本地 XML 预览时，传 `--content @file` 或 `--content -`，内容应为单个 `<slide>` XML 片段；此时不要传 `--presentation` / `--slide-id` / `--slide-number`。
4. `slide_id` 是页面 short ID，页码请用 `--slide-number`。
5. list 模式下 `--slide-id` 与 `--slide-number` 必须二选一；同一类型 selector 一次最多传 10 个，更多页面请分批截图。
6. 单张使用 `--output`，多张使用 `--output-dir`，由 CLI 按页面信息生成文件名。新建或大幅改写 Deck 时，截图目录复用 planning 阶段的 `<deck-or-task-id>`；已有 Deck 没有 task ID 时，使用 presentation ID 作为目录名。
7. CLI 不转换图片格式，也不要求模型预判服务端格式。未写扩展名时自动追加真实扩展名；请求扩展名与真实格式不一致时保留目录和名称、修正扩展名，例如请求 `slide3.png` 而服务端返回 JPEG 时实际保存为 `slide3.jpg`。
8. 发生扩展名修正或同名避让时会返回原始 `requested_output`、实际绝对路径 `output` 和 `output_adjusted: true`；后续必须使用 `output` / `screenshots[].path`，不要继续猜测请求路径。
9. list 模式默认文件名包含 presentation ID、页码和/或 slide ID。
10. 截图来自服务端渲染结果，适合创建/替换后验证页面是否为空白、破图或布局明显异常。
