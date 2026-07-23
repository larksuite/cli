# slides +screenshot

## 用途

获取幻灯片页面截图并保存为本地图片文件。默认用于已存在 PPT 页面截图；传入 `--content` 时用于直接渲染单个 `<slide>` XML 片段预览。本 shortcut 会在 CLI 进程内解码并写入文件，stdout 只返回文件路径、大小、页面 ID 等元信息，避免把图片 Base64 输出给模型。

注意：该截图能力受应用白名单限制，绝大多数应用不可用。截图失败时不要引导用户申请 `slides:presentation:screenshot` 权限；记录错误后降级到 XML 读回、结构 lint、文本重叠检查等非截图检查路径。

## 命令

```bash
lark-cli slides +screenshot --as user \
  --presentation '<xml_presentation_id 或 slides/wiki URL>' \
  --slide-id 'SLIDE_ID'
```

渲染本地 XML 内容：

```bash
lark-cli slides +screenshot --as user \
  --content @slide.xml
```

## 参数

| 参数 | 必需 | 说明 |
|------|------|------|
| `--presentation` | list 模式必需 | `xml_presentation_id`、`/slides/` URL，或解析后为 slides 的 `/wiki/` URL；只标识演示文稿，不会默认截图全部页面。传 `--content` 时不能使用 |
| `--slide-id` | list 模式标准入参 | 页面 short ID；截图、修复和 review 状态均以它关联；多页截图时重复传入；一次最多 10 页。先从创建响应或 `slides +xml-get` 取得当前 `slide_ids` |
| `--slide-number` | 用户只提供“第 N 页”或旧 deck 暂未取得 `slide_id` 时使用；成功定位后必须取得对应 `slide_id`，后续不再用页号关联截图或 review 状态。不能省略 `--slide-id` 和 `--slide-number` 两者 |
| `--content` | render 模式必需 | 要直接渲染的 `<slide>` XML 片段；支持直接传值、`@file`、`-` stdin。传入后不能同时传 `--slide-id` / `--slide-number` |
| `--output-dir` | 否 | 输出目录，默认 `.lark-slides/screenshots`；必须是当前目录内的相对路径。截图可能返回多张图片，使用目录而不是 `--output` 文件路径 |
| `--output-name` | 否 | 仅 render 模式（`--content`）的输出文件名 stem；未指定时优先用返回的 `slide_id`，否则用 `rendered-slide`。若目标文件已存在，会自动追加递增后缀避免覆盖 |

## 示例

### 单页截图

```bash
lark-cli slides +screenshot --as user \
  --presentation slides_example_presentation_id \
  --slide-id 'SLIDE_ID'
```

### 按 `slide_id` 截图与创建后视觉 review（推荐）

视觉 review 以当前回读得到的 `slide_ids` 为页清单。单页传一个 `--slide-id`；多页可重复传入，单次最多 10 页，超过时按批次串行执行。

首次新建且之后没有增删页、整页替换或重排时，可复用创建响应中的 `slide_ids`。发生上述页面集合变化后，必须先回读并刷新清单；不能用页码或旧响应中的页列表绑定 review 状态。

```bash
lark-cli slides +screenshot --as user \
  --presentation 'YOUR_PRESENTATION_ID' \
  --slide-id 'SLIDE_ID_1' \
  --slide-id 'SLIDE_ID_2' \
  --output-dir .lark-slides/review/<deck-or-task-id>/screenshots
```

随后必须用具备图像查看能力的工具打开每个返回的 `path`，逐页记录 `pass/fix`。截图落盘、批量请求成功或只查看关键页，都不等于已完成视觉 review。

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
2. 已存在 PPT 页面截图时，不传 `--content`，用 `--presentation` + `--slide-id`。
3. 本地 XML 预览时，传 `--content @file` 或 `--content -`，内容应为单个 `<slide>` XML 片段；此时不要传 `--presentation` / `--slide-id` / `--slide-number`。
4. `slide_id` 是页面 short ID，也是截图、修复和 review 状态的唯一关联键；页码仅作为用户可读的瞬时展示信息。
5. list 模式一次最多传 10 个 `--slide-id`；更多页面请分批截图，每页仍要独立记录 review 结论。
6. list 模式默认文件名包含 presentation ID、页码和/或 slide ID；文件已存在时自动追加 `_2`、`_3` 等后缀，避免覆盖旧截图。
7. 截图来自服务端渲染结果，适合创建/替换后验证页面是否为空白、破图或布局明显异常；与 `validation-checklist.md` 的逐页 rubric 一起使用。
8. 如果因用户只给页号而使用 `--slide-number`，截图后立即回读或从响应取得 `slide_id`，后续改用 `--slide-id`；如果收到频率限制，停止扩大发送并在短暂退避后逐批重试。截图 API 白名单失败时记录原始错误，继续完成 XML 静态检查，并把视觉状态标为 `not_verified`。
