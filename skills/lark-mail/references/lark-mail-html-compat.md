# 邮件 HTML 兼容白名单

优先使用飞书原生 HTML：段落、标题、列表、表格、链接、引用与 inline style。不要手拼 raw EML。

## 推荐标签

- 文本与结构：`p`、`div`、`span`、`br`、`hr`、`h1`-`h6`
- 强调：`b`、`strong`、`i`、`em`、`u`、`s`、`code`、`pre`
- 列表与引用：`ul`、`ol`、`li`、`blockquote`
- 表格：`table`、`thead`、`tbody`、`tr`、`th`、`td`
- 链接与图片：`a href`、`img src`

## 降级与拦截

- `font` 会降级为 `span`
- `center` 会降级为 `div style="text-align:center"`
- `script`、`style`、`iframe`、`object`、`embed`、`form`、`input`、`link`、`meta`、`base` 会被移除
- `onclick` 等事件属性、`javascript:` / `vbscript:` URL 会被移除

内嵌图片路径必须位于当前工作目录内，拒绝绝对路径和 `..` 越界路径。

