# lark-slides +compose

## 用途

从一组 slide XML 文件创建新的飞书幻灯片演示文稿。适合多页、复杂 XML、带本地图片占位符的场景。

## 命令

```bash
lark-cli slides +compose --as user --title "发布会" --slides-dir ./slides --asset-root ./assets
```

或使用 manifest：

```bash
lark-cli slides +compose --as user --manifest @compose.json --asset-root ./assets
```

## 关键参数

| 参数 | 说明 |
|------|------|
| `--title` | 演示文稿标题；未传时回退到 manifest.title 或 `Untitled` |
| `--slides-dir` | 存放 `.xml` slide 文件的目录，按文件名字典序逐页创建 |
| `--manifest` | compose manifest JSON，可通过 `@file` 或 stdin 传入 |
| `--asset-root` | 本地图片素材根目录；slide XML 中的 `@path` 会相对该目录解析 |

## manifest 结构

```json
{
  "title": "iPhone 17 Pro Max 发布会",
  "slides": [
    { "file": "01-cover.xml" },
    { "file": "02-hero.xml" },
    { "content": "<slide xmlns=\"http://www.larkoffice.com/sml/2.0\"><data>...</data></slide>" }
  ]
}
```

每个 slide 必须二选一：

- `file`：相对 `--slides-dir` 或当前工作目录的 XML 文件
- `content`：内联 slide XML

## 行为

1. 创建空白 PPT
2. 扫描所有 slide XML 中的 `<img src="@...">`
3. 自动上传本地图片到新 PPT
4. 用 `file_token` 替换 XML 中的 `@path`
5. 逐页调用 `xml_presentation.slide.create`

## 返回值

成功时返回：

- `xml_presentation_id`
- `title`
- `revision_id`
- `images_uploaded`
- `slide_ids`
- `slides_added`
- `url`

## 注意事项

1. `--slides-dir` 只做单层目录扫描，不递归
2. `--asset-root` 只是降低素材路径切换成本，路径仍必须位于当前工作目录内
3. 复杂 XML 仍建议创建后用 `xml_presentations.get` 回读校验
4. 如果本地有 Python 3，可再配合 `python3 skills/lark-slides/scripts/layout-lint.py --input presentation.xml` 做布局检查
