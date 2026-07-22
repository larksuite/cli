# Troubleshooting

本文件覆盖 lark-slides 的通用创建前自检、XML 排障和常见失败处理。命令专属问题优先看对应 reference，例如 `+replace-slide`、`+media-upload`、`xml_presentation.slide.create`。

## XML Preflight

在真正创建或替换前，至少检查：

- 特殊字符已转义：正文和标题里的 `&`、`<`、`>` 不能裸写；属性值里的裸 `&` 也必须写成 `&amp;`。
- 属性引号安全：XML 属性、shell 引号、JSON 字符串包装之间没有互相打断。
- 结构合法：`<slide>` 下只放 `<style>`、`<data>`、`<note>`，文本都在 `<content>` 内。
- 图片路径正确：`<img src="@...">` 只在 `+create --slides` 的支持链路中使用；直接调用 `xml_presentation.slide.create` 必须先拿到 `file_token`。

## Failure Order

遇到 `invalid param`、某一页创建失败、页面空白或布局错乱时，按顺序处理：

1. 记录 `xml_presentation_id`，不要假设失败代表什么都没创建。
2. 用 `slides +xml-get` 回读，确认是否已有部分页面写入。
3. 检查失败页是否含未转义字符：`Q&A -> Q&amp;A`，文本 `<` / `>` 写成 `&lt;` / `&gt;`，属性 URL `a=1&b=2 -> a=1&amp;b=2`。
4. 检查标签闭合、属性引号、`<content>` 结构，以及 `<slide>` 直接子元素。
5. 页面空白、溢出、重叠或越界时，按 [validation-checklist.md](validation-checklist.md) 运行 `xml_text_overlap_lint.py`；先修复所有 `error`，再对 `warning` 指向的页面和元素做截图复核。
6. 如果使用 `--slides '[...]'`，怀疑 shell 截断时直接切到两步创建：先 `slides +create`，再用 `xml_presentation.slide.create` 逐页添加。
7. 局部问题用 `+replace-slide` 块级修正；整页结构要改时再用 `slide.delete` 旧页 + `slide.create` 新页。

## Symptom Fixes

| 看到的问题 | 处理方式 |
|-----------|----------|
| 文字被截断 / 看不全 | 增大 shape 的 `width` 或 `height`，或减少文本量 |
| 元素重叠 | 调整 `topLeftX` / `topLeftY`，拉开间距 |
| 页面大面积空白 | 回读确认内容是否写入；若内容存在，再缩小间距或增加主体元素 |
| 文字和背景色太接近 | 深色背景用浅色文字，浅色背景用深色文字 |
| 表格列宽不合理 | 调整 `colgroup` 中 `col` 的 `width` 值 |
| 图表没有显示 | 检查 `chartPlotArea` 和 `chartData` 是否都包含，`dim1` / `dim2` 数据数量是否匹配 |
| 图片被裁掉一部分 | `<img>` 的 `width` / `height` 是裁剪后尺寸；要整图显示就让 `width:height` 对齐原图比例 |
| 图片不显示 / `<img src>` 仍是 `@path` | `@` 占位符只在 `+create --slides` 中替换；直接调 `xml_presentation.slide.create` 必须先用 `+media-upload` 拿 `file_token` |
| 新插入的 `<img>` 挡住原有元素 | `slide.get` 读原页，对照已有块坐标挑空白位置；空间不够就在同一批 `--parts` 里先移动/缩小现有块再插图 |
| 渐变背景变成白色 | 渐变必须用 `rgba()` 格式 + 百分比停靠点，如 `linear-gradient(135deg,rgba(30,60,114,1) 0%,rgba(59,130,246,1) 100%)` |
| 整体风格不统一 | 封面页和结尾页用同一背景，内容页保持一致的配色和字号体系 |

## Common Errors

| 错误码 / 信号 | 含义 | 解决方案 |
|--------------|------|----------|
| 400 XML 格式错误 | XML 语法错误 | 检查标签闭合、属性引号、特殊字符转义 |
| 400 请求包装错误 | `--data` 未按 schema 包装 | 检查是否传入 `xml_presentation.content` 或 `slide.content` |
| 创建成功但页面空白 / 内容缺失 / 布局错乱 | 常见于 `--slides '[...]'` 的 shell 转义或长参数传递问题 | 改用两步创建，并在创建后立即读取 XML 验证 |
| 403 权限不足 | scope 或文档权限不匹配 | 确认 scope 和文档权限；无权限时根据错误响应引导用户解决 |
| 404 演示文稿不存在 | `xml_presentation_id` 不正确或无权限 | 检查 token；wiki URL 需先解析真实 `obj_token` |
| 404 幻灯片不存在 | `slide_id` 不正确 | 重新读取 presentation 或 slide，确认最新 ID |
| 400 无法删除唯一幻灯片 | 演示文稿至少保留一页 | 先创建新页，再删除旧页 |
| 1061002 媒体上传 params error | slides 媒体上传参数不符合约定 | 用 `slides +media-upload`，不要手拼原生 `medias/upload_all`；slides 唯一可用 `parent_type` 是 `slide_file` |
| 1061004 forbidden | 当前用户对演示文稿无编辑权限 | 确认当前用户对目标 PPT 有编辑权限 |
| 3350001 | XML 非 well-formed、XML 结构不符合服务端要求，或 replace 片段问题 | 优先检查未转义字符；replace 场景再看 `block_id` 和 `<content/>` |
| 3350002 | `revision_id` 大于当前版本 | 用 `-1` 取当前版本，或重新用 `slides +xml-get` 取最新 `revision_id` |
| validation: unsafe file path | `--file` 给了绝对路径或上层路径 | `--file` 必须是 CWD 内相对路径；先 `cd` 到素材目录再执行 |

## Command-Specific References

- 图片上传、`@path` 占位符、`file_token`：见 [lark-slides-media-upload.md](lark-slides-media-upload.md) 和 [lark-slides-create.md](lark-slides-create.md)。
- 块级替换、`block_id`、3350001 replace 细节：见 [lark-slides-replace-slide.md](lark-slides-replace-slide.md)。
- 原生 `slide.create` 包装、`before_slide_id` 和 jq 模板：见 [lark-slides-xml-presentation-slide-create.md](lark-slides-xml-presentation-slide-create.md)。
