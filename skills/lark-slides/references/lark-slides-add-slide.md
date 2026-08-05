# slides +add-slide（向已有演示文稿追加/插入单页）

向已有演示文稿添加**一页**。这是两步创建流程的第二步：先 `+create` 建空壳，再逐页 `+add-slide`；也用于给已有 PPT 追加新页。

`--presentation` 接受 token / `/slides/` URL / `/wiki/` URL（wiki 自动解析），`--slide` 直接收 XML（支持 `@file` 和 stdin，复杂 XML 走文件可绕开 shell 转义），`<img src="@./local.png">` 占位符自动上传并替换成 `file_token`。

**CRITICAL — 提交前必须先跑版式 lint**：把待提交的 `<slide>` XML 存成本地文件，运行 [`scripts/xml_text_overlap_lint.py`](../scripts/xml_text_overlap_lint.py)，`summary.error_count` 必须为 0。

## 命令

```bash
# 追加到末尾（XML 直接作为参数）
lark-cli slides +add-slide --as user \
  --presentation "$PID" \
  --slide '<slide xmlns="https://www.larkoffice.com/sml/2.0"><data></data></slide>'

# XML 从文件读（推荐：避免 shell 转义和长参数截断）
lark-cli slides +add-slide --as user \
  --presentation "$PID" \
  --slide @page3.xml

# XML 从 stdin 读
cat page3.xml | lark-cli slides +add-slide --as user --presentation "$PID" --slide -

# 插到某页之前
lark-cli slides +add-slide --as user \
  --presentation "$PID" \
  --slide @cover.xml \
  --before-slide-id "$SID"

# wiki 链接（CLI 自动 wiki.spaces.get_node 解析，并校验 obj_type=slides）
lark-cli slides +add-slide --as user \
  --presentation "https://xxx.feishu.cn/wiki/wikcnXXXXXX" \
  --slide @page3.xml

# 预览请求，不实际写入
lark-cli slides +add-slide --presentation "$PID" --slide @page3.xml --dry-run
```

## 参数

| 参数 | 必需 | 说明 |
|------|------|------|
| `--presentation` | 是 | `xml_presentation_id`、`/slides/` URL 或 `/wiki/` URL |
| `--slide` | 是 | 一个完整的 `<slide>...</slide>` 文档；支持字面量、`@file`、stdin `-` |
| `--before-slide-id` | 否 | 插到该 `slide_id` 之前；**不传就是追加到末尾** |
| `--revision-id` | 否 | 演示文稿版本号，默认 `-1`（最新）；传具体版本号做乐观锁 |
| `--dry-run` | 否 | 打印将要发起的请求（含图片上传步骤），不写入 |

`@file` 路径**必须在 CWD 内**（如 `@./plan/page3.xml`）；绝对路径和 `../` 会被拒绝并报 `unsafe file path`。

## 本地图片：`@路径` 占位符

XML 里写 `<img src="@./chart.png" .../>`，CLI 会：先把每个不重复的本地文件上传到这份演示文稿（`parent_type=slide_file`），再把 `src` 替换成返回的 `file_token`，最后才提交页面。

占位符路径按**执行命令时的 CWD** 解析，跟 `--slide @file` 所在目录无关；`@./assets/x.png` 找的是 `$PWD/assets/x.png`。

```bash
lark-cli slides +add-slide --as user \
  --presentation "$PID" \
  --slide '<slide xmlns="https://www.larkoffice.com/sml/2.0"><data><img src="@./chart.png" topLeftX="100" topLeftY="100" width="320" height="180"/></data></slide>'
```

- 文件不存在、不是普通文件、超过 20 MB，都在**调用任何接口之前**报错，不会留下半成品。
- 去重只在**单次调用内**生效：多页共用同一张图时，逐页循环会把它每页重传一次。这种图先用 [`+media-upload`](lark-slides-media-upload.md) 传一次，把 `file_token` 写进各页的 `src`。

## 成功输出

```json
{
  "xml_presentation_id": "slides_example_presentation_id",
  "slide_id": "slide_example_id",
  "revision_id": 42,
  "before_slide_id": "slide_example_target_id",
  "images_uploaded": 1,
  "issues": "[issue=unsupported_attr tag=<strong> attr=style]"
}
```

| 字段 | 说明 |
|------|------|
| `slide_id` | 新创建页面的唯一标识 |
| `issues` | 字符串，**只在服务端丢弃过内容时才出现**：页面创建成功，但括号里列出的标签/属性没写进去。出现就必须 `+screenshot` 复核，别当纯警告忽略；干净提交时这个字段不返回 |

## 常见错误

| 现象 | 原因 | 解决 |
|------|------|------|
| `--slide is not a single complete <slide> document` | 传了 `<presentation>` 整份 XML，或多个 `<slide>` 拼在一起 | 一次只传一页，根元素必须是 `<slide>` |
| `--slide cannot be empty` | `@file` 指向空文件，或 stdin 没内容 | 检查文件内容 |
| 3350001 | XML 结构/转义有问题；**或 `--before-slide-id` 不是有效 `slide_id`** | 优先改用 `--slide @file` 绕开 shell 转义；插页失败先 `+xml-get` 回读确认 `slide_id`；再按 [troubleshooting.md](troubleshooting.md) 排查 |
| 1061004 / 403 | 当前身份对这份 PPT 没有编辑权限 | 检查是否拥有 `slides:presentation:update` 或 `slides:presentation:write_only` scope；wiki 链接另需 `wiki:node:read`，`@` 占位符另需 `docs:document.media:upload`；`--as bot` 还要求该 bot 对目标 PPT 有编辑权限 |
