# 完整操作示例

本文档提供与 CLI schema 一致的调用示例，XML 内容均遵循 [slides_xml_schema_definition.xml](slides_xml_schema_definition.xml)。

> **重要**：创建 PPT 请优先使用 `slides +create`；实际页面内容请使用 `xml_presentation.slide.create` 逐页添加。

## 目录

- [示例 1: 使用 Shortcut 创建空白演示文稿](#示例-1-使用-shortcut-创建空白演示文稿)
- [示例 2: 创建后添加第一页](#示例-2-创建后添加第一页)
- [示例 3: 读取 XML 内容](#示例-3-读取-xml-内容)
- [示例 4: 在指定页面前插入新幻灯片](#示例-4-在指定页面前插入新幻灯片)
- [示例 5: 删除幻灯片](#示例-5-删除幻灯片)
- [示例 6: 从文件读取 XML 后添加页面](#示例-6-从文件读取-xml-后添加页面)
- [示例 7: +replace-slide + block_insert 给已有页加图](#示例-7-replace-slide--block_insert-给已有页加图)
- [示例 8: +replace-slide + block_replace 替换一个块](#示例-8-replace-slide--block_replace-替换一个块)

## 示例 1: 使用 Shortcut 创建空白演示文稿

```bash
lark-cli slides +create --title "项目汇报"
```

预期返回结构：

```json
{
  "data": {
    "xml_presentation_id": "slides_example_presentation_id",
    "title": "项目汇报",
    "revision_id": 1
  }
}
```

## 示例 2: 创建后添加第一页

```bash
PRESENTATION_ID=$(lark-cli slides +create --title "季度复盘" | jq -r '.data.xml_presentation_id')

lark-cli slides xml_presentation.slide create --as user --params "{\"xml_presentation_id\":\"$PRESENTATION_ID\"}" --data '{
  "slide": {
    "content": "<slide xmlns=\"http://www.larkoffice.com/sml/2.0\"><style><fill><fillColor color=\"rgb(245, 245, 245)\"/></fill></style><data><shape type=\"text\" topLeftX=\"80\" topLeftY=\"72\" width=\"760\" height=\"90\"><content textType=\"title\"><p>2024 Q3 季度复盘</p></content></shape><shape type=\"text\" topLeftX=\"80\" topLeftY=\"190\" width=\"520\" height=\"220\"><content textType=\"body\"><p>关键结论</p><ul><li><p>收入增长 30%</p></li><li><p>重点项目全部上线</p></li><li><p>用户满意度持续提升</p></li></ul></content></shape><shape type=\"rect\" topLeftX=\"660\" topLeftY=\"180\" width=\"180\" height=\"140\"><fill><fillColor color=\"rgba(100, 149, 237, 0.25)\"/></fill><border color=\"rgb(100, 149, 237)\" width=\"2\"/></shape></data><note><content textType=\"body\"><p>讲述时先给结论，再补充数据。</p></content></note></slide>"
  }
}'
```

## 示例 3: 读取 XML 内容

```bash
lark-cli slides xml_presentations get --as user --params '{
  "xml_presentation_id": "slides_example_presentation_id"
}'
```

提取 XML 内容：

```bash
lark-cli slides xml_presentations get --as user --params '{
  "xml_presentation_id": "slides_example_presentation_id"
}' | jq -r '.data.xml_presentation.content'
```

预期返回结构：

```json
{
  "code": 0,
  "data": {
    "xml_presentation": {
      "presentation_id": "slides_example_presentation_id",
      "revision_id": 3,
      "content": "<presentation xmlns=\"http://www.larkoffice.com/sml/2.0\" height=\"540\" width=\"960\">...</presentation>"
    }
  },
  "msg": "success"
}
```

## 示例 4: 在指定页面前插入新幻灯片

```bash
lark-cli slides xml_presentation.slide create --as user --params '{
  "xml_presentation_id": "slides_example_presentation_id"
}' --data '{
  "slide": {
    "content": "<slide xmlns=\"http://www.larkoffice.com/sml/2.0\"><data><shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"800\" height=\"120\"><content textType=\"title\"><p>新增页面</p></content></shape><shape type=\"text\" topLeftX=\"80\" topLeftY=\"200\" width=\"800\" height=\"180\"><content textType=\"body\"><p>这是新增页面的正文。</p></content></shape></data></slide>"
  },
  "before_slide_id": "sld_before_target"
}'
```

预期返回结构：

```json
{
  "code": 0,
  "data": {
    "slide_id": "slide_example_id",
    "revision_id": 100
  },
  "msg": "success"
}
```

## 示例 5: 删除幻灯片

```bash
lark-cli slides xml_presentation.slide delete --as user --params '{
  "xml_presentation_id": "slides_example_presentation_id",
  "slide_id": "slide_example_id"
}'
```

预期返回结构：

```json
{
  "code": 0,
  "data": {
    "revision_id": 101
  },
  "msg": "success"
}
```

## 示例 6: 从文件读取 XML 后添加页面

先准备 `slide.xml`：

```xml
<slide xmlns="http://www.larkoffice.com/sml/2.0">
  <data>
    <shape type="text" topLeftX="80" topLeftY="80" width="800" height="120">
      <content textType="title">
        <p>从文件加载</p>
      </content>
    </shape>
  </data>
</slide>
```

先创建演示文稿：

```bash
PRESENTATION_ID=$(lark-cli slides +create --title "从文件添加页面" | jq -r '.data.xml_presentation_id')
```

再用 `jq` 组装请求体，从文件添加页面：

```bash
lark-cli slides xml_presentation.slide create --as user \
  --params "{\"xml_presentation_id\":\"$PRESENTATION_ID\"}" \
  --data "$(jq -n --arg content "$(cat slide.xml)" '{slide:{content:$content}}')"
```

## 示例 7: +replace-slide + block_insert 给已有页加图

只想在已有页上加一张图、不动其他元素——走 shortcut `+replace-slide`，`block_insert` 追加到页末（或用 `insert_before_block_id` 指定位置）。

```bash
PID="slides_example_presentation_id"
SID="slide_example_id"

# 1. 上传图片拿 file_token
TOKEN=$(lark-cli slides +media-upload --file ./pic.png --presentation "$PID" --as user \
  | jq -r '.data.file_token')

# 2. block_insert 到页面末尾（省略 insert_before_block_id）
# 注：<img .../> 是自闭合标签，CLI 不会展开（只有 <shape/> 会被补 <content/>）
lark-cli slides +replace-slide --as user \
  --presentation "$PID" --slide-id "$SID" \
  --parts "$(jq -n --arg token "$TOKEN" \
    '[{action:"block_insert",insertion:("<img src=\""+$token+"\" topLeftX=\"500\" topLeftY=\"100\" width=\"200\" height=\"150\"/>")}]')"
```

预期返回：

```json
{
  "ok": true,
  "data": {
    "xml_presentation_id": "slides_example_presentation_id",
    "slide_id": "slide_example_id",
    "parts_count": 1,
    "revision_id": 102
  }
}
```

## 示例 8: +replace-slide + block_replace 替换一个块

已知某块的 3 位 short element ID（从 `slide.get` 返回 XML 里读），整块换掉。`replacement` 根元素的 `id` 会由 CLI 自动注入为 `block_id`，无需手写；若写了 `<shape/>` 自闭合形式，CLI 也会自动补 `<content/>`。

```bash
lark-cli slides +replace-slide --as user \
  --presentation slides_example_presentation_id \
  --slide-id slide_example_id \
  --parts '[
    {
      "action": "block_replace",
      "block_id": "bab",
      "replacement": "<shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"800\" height=\"120\"><content textType=\"title\"><p>新标题</p></content></shape>"
    }
  ]'
# CLI 实际发送的 replacement 根元素会带 id="bab"，即使手写时省略了
```

失败时（3350001 错误，CLI 在 error 字段中给出 hint）：

```json
{
  "ok": false,
  "error": {
    "type": "api_error",
    "code": 3350001,
    "message": "API error: [3350001] invalid param",
    "hint": "common causes: (1) block_id not found in current slide ..."
  }
}
```

整批作为原子事务，任一 part 失败则整批不生效；按 `failed_part_index` 定位修正后重发。

## 常见处理技巧

### 获取最新 revision_id

```bash
lark-cli slides xml_presentations get --as user --params '{
  "xml_presentation_id": "slides_example_presentation_id"
}' | jq '.data.xml_presentation.revision_id'
```

### 批量插入多页

```bash
#!/bin/bash

PRESENTATION_ID="slides_example_presentation_id"

slides=(
  '<slide xmlns="http://www.larkoffice.com/sml/2.0"><data><shape type="text" topLeftX="80" topLeftY="80" width="800" height="120"><content textType="title"><p>页面 1</p></content></shape></data></slide>'
  '<slide xmlns="http://www.larkoffice.com/sml/2.0"><data><shape type="text" topLeftX="80" topLeftY="80" width="800" height="120"><content textType="title"><p>页面 2</p></content></shape></data></slide>'
)

for slide_xml in "${slides[@]}"; do
  payload=$(jq -n --arg content "$slide_xml" '{slide:{content:$content}}')
  lark-cli slides xml_presentation.slide create --as user --params "{\"xml_presentation_id\":\"$PRESENTATION_ID\"}" --data "$payload"
done
```

### 本地校验 XML 基本语法

```bash
xmllint --noout presentation.xml
```

### 真实示例

- [slides_demo.xml](slides_demo.xml) 提供了更完整的页面示例，包含 `theme`、渐变填充、图片、图标和备注内容。
