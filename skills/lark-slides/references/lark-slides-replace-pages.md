# slides +replace-pages（多页整页重建）

批量替换已有演示文稿里的多个页面，保持原 `xml_presentation_id` 和原 Slides 链接不变。适合多页版式大改、坐标重排、整页视觉重建；单个文本框、图片或 shape 的局部编辑仍优先用 [`+replace-slide`](lark-slides-replace-slide.md)。

> 重要：这是多步编排，不是后端原子事务。CLI 对每页执行“先创建新页到旧页前，再删除旧页”；创建失败时旧页会保留。删除失败时可能出现新旧页同时存在，需要按返回结果继续处理。

## 命令

```bash
lark-cli slides +replace-pages \
  --as user \
  --presentation <slides_url_or_xml_presentation_id> \
  --pages @pages.json
```

## 参数

| 参数 | 必需 | 说明 |
|------|------|------|
| `--presentation` | 是 | `xml_presentation_id`、`/slides/` URL 或 `/wiki/` URL |
| `--pages` | 是 | JSON 数组，每项包含 `slide_id` 和 `content`；支持 literal、`@file`、stdin `-` |
| `--dry-run` | 否 | 基于 `slide_id` 输入输出替换计划，不执行 create/delete |
| `--continue-on-error` | 否 | 默认失败即停；开启后继续处理后续页，并在结果中标记失败项 |
| `--validate-only` | 否 | 只校验输入并生成替换计划，不执行 Slides get/create/delete |

## pages.json

```json
[
  {
    "slide_id": "slide_short_id_1",
    "content": "<slide xmlns=\"http://www.larkoffice.com/sml/2.0\"><data></data></slide>"
  },
  {
    "slide_id": "slide_short_id_2",
    "content": "<slide xmlns=\"http://www.larkoffice.com/sml/2.0\"><data></data></slide>"
  }
]
```

规则：

- 每项必须提供 `slide_id`；不支持 `slide_number`。
- `content` 必须是完整 `<slide>...</slide>` XML。
- 同一批次不能重复 `slide_id`。
- CLI 不会回读整份 presentation；如果 `slide_id` 已失效，create/delete 阶段会返回对应错误。

## Dry Run

```bash
lark-cli slides +replace-pages --as user \
  --presentation "$PID" \
  --pages @pages.json \
  --dry-run
```

输出包含 `xml_presentation_id`、`pages_count`、`plan`，以及每页的 `old_slide_id`、`insert_before_slide_id` 和动作 `create_before_then_delete_old`。Dry-run 只基于输入的 `slide_id` 构造计划，不会调用 `xml_presentations.get`，也不会执行 create/delete。

## 成功输出

```json
{
  "xml_presentation_id": "xxx",
  "pages_count": 2,
  "status": "completed",
  "summary": {
    "replaced": 2,
    "failed": 0,
    "total": 2
  },
  "results": [
    {
      "old_slide_id": "old3",
      "new_slide_id": "new3",
      "status": "replaced"
    }
  ],
  "revision_id": 123
}
```

如果使用 `--continue-on-error` 且任一页面失败，CLI 会继续处理后续页，但最终以 partial failure 非零退出；stdout 仍保留完整 `results`，顶层 `ok` 为 `false`，`status` 为 `partial_failure`。

`status` 可能为：

- `replaced`：新页创建成功，旧页删除成功。
- `create_failed`：新页创建失败，旧页保留。
- `delete_failed`：新页已创建，但旧页删除失败。

## 使用建议

1. 大幅改写前先 `xml_presentations.get` 保存当前 XML，并记录要替换页面的 `slide_id`。
2. 生成只含 `slide_id` 的 `pages.json` 后先跑 `--dry-run` 或 `--validate-only`。
3. 默认不要开 `--continue-on-error`，除非能接受部分页面已替换。
4. 替换后再回读全文 XML 并截图检查，确认页序、视觉和文本没有破损。
