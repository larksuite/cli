# slides +xml-get（读取全文 XML）

读取已有演示文稿的完整 XML。适合创建后验收、编辑前备份、获取 `slide_id` / `revision_id`，以及排查空白页、破图、文本溢出等问题。相比直接调用底层 `xml_presentations.get`，本 shortcut 会自动解析 Slides URL / Wiki URL，并可把大段 XML 保存到本地文件，避免终端输出被截断。

## 命令

```bash
lark-cli slides +xml-get \
  --as user \
  --presentation <slides_url_or_xml_presentation_id> \
  --output .lark-slides/plan/<deck-id>/readback.xml
```

## 参数

| 参数 | 必需 | 说明 |
|------|------|------|
| `--presentation` | 是 | `xml_presentation_id`、`/slides/` URL 或 `/wiki/` URL |
| `--output` | 否 | 本地 XML 保存路径，必须是当前工作目录内的相对路径，不能传绝对路径。传入时 XML 内容保存到文件，stdout 只返回保存后的绝对路径、大小等简短元信息；省略时 XML 原文直接输出到 stdout |
| `--revision-id` | 否 | 读取指定版本；默认 `-1`，表示最新版本 |
| `--remove-attr-id` | 否 | 移除返回 XML 中的 `id` 属性；适合只读检查，不适合精确块级编辑 |
| `--dry-run` | 否 | 预览将调用的 API 和输出方式，不读取真实 XML |

## 输出到文件

推荐普通工作流都传 `--output`，尤其是中大型 PPT。`--output` 必须是当前工作目录内的相对路径，例如 `.lark-slides/plan/$PID/readback.xml`，不要传 `/tmp/readback.xml` 这类绝对路径。XML 会写入本地文件，stdout 只保留元信息，便于后续脚本读取。

```bash
lark-cli slides +xml-get --as user \
  --presentation "$PID" \
  --output .lark-slides/plan/$PID/readback.xml
```

成功输出中的 `data` 类似：

```json
{
  "xml_presentation_id": "slides_example_presentation_id",
  "path": "/abs/path/.lark-slides/plan/slides_example_presentation_id/readback.xml",
  "size": 123456,
  "content_saved": true,
  "revision_id": 12
}
```

其中 `path` 是 CLI 解析后的绝对路径。

如果传入 `--remove-attr-id`，返回元信息中会包含 `"remove_attr_id": true`。

## 输出到终端

省略 `--output` 时，CLI 会把 XML 原文直接写到 stdout，不包 JSON envelope。这个模式只适合小型演示文稿或临时调试；大文件仍建议保存到文件，避免终端截断。

```bash
lark-cli slides +xml-get --as user \
  --presentation "$PID"
```

需要临时过滤内容时，可以用管道处理 stdout：

```bash
lark-cli slides +xml-get --as user \
  --presentation "$PID" \
  | rg '<slide '
```

## 读取指定版本

```bash
lark-cli slides +xml-get --as user \
  --presentation "$PID" \
  --revision-id 10 \
  --output .lark-slides/plan/$PID/readback-r10.xml
```

## 移除 XML id 属性后读取

```bash
lark-cli slides +xml-get --as user \
  --presentation "$PID" \
  --remove-attr-id \
  --output .lark-slides/plan/$PID/readback-no-id.xml
```

注意：`--remove-attr-id` 是只读检查用的便利选项。后续如果要用 `+replace-slide` 做块级编辑，仍应保留原始 `id` / short id 信息。

## 使用建议

1. 创建或大幅改写后，用 `slides +xml-get --output` 回读全文 XML，再按 `validation-checklist.md` 做页数、关键元素和静态检查。
2. 编辑已有 PPT 前，先保存一份 readback XML，记录 `xml_presentation_id`、`slide_id`、`revision_id`。
3. Wiki URL 可直接传给 `--presentation`，CLI 会先解析出真实 Slides token。
4. 普通工作流优先保存文件；只有小型演示文稿或临时调试时才省略 `--output`。

## 相关命令

- [slides +screenshot](lark-slides-screenshot.md) - 获取页面截图做视觉验证
- [slides +replace-slide](lark-slides-replace-slide.md) - 局部替换或插入页面元素
- [slides +replace-pages](lark-slides-replace-pages.md) - 多页整页重建
- [xml_presentations get](lark-slides-xml-presentations-get.md) - 底层原生 API 参考
