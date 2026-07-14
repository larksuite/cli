# lark-slides xml_presentations get

## 用途

读取飞书幻灯片（PPT）演示文稿的完整 XML 内容信息。

## 底层原生命令形态

```bash
lark-cli slides xml_presentations get --as user --params '<json_params>'
```

## 参数说明

| 参数 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `--params` | JSON string | 是 | 路径参数与查询参数，结构以 schema 为准 |

### params JSON 结构

```json
{
  "xml_presentation_id": "slides_example_presentation_id",
  "revision_id": -1
}
```

| 字段 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `xml_presentation_id` | string | 是 | 演示文稿的唯一标识符 |
| `revision_id` | integer | 否 | 版本号，`-1` 表示最新版本 |

## 使用示例

### 基础示例

```bash
lark-cli slides xml_presentations get --as user \
  --params '{"xml_presentation_id":"slides_example_presentation_id","revision_id":-1}'
```

### 指定版本读取

```bash
lark-cli slides xml_presentations get --as user \
  --params '{"xml_presentation_id":"slides_example_presentation_id","revision_id":10}'
```

### 移除 XML id 属性后读取

```bash
lark-cli slides xml_presentations get --as user \
  --params '{"xml_presentation_id":"slides_example_presentation_id","revision_id":-1,"remove_attr_id":true}'
```

## 返回值

成功时返回演示文稿的完整信息：

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "xml_presentation": {
      "presentation_id": "slides_example_presentation_id",
      "revision_id": 1,
      "content": "<presentation xmlns=\"http://www.larkoffice.com/sml/2.0\" height=\"540\" width=\"960\">...</presentation>"
    }
  }
}
```

### 返回字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `data.xml_presentation.presentation_id` | string | 演示文稿唯一标识 |
| `data.xml_presentation.revision_id` | integer | 版本号 |
| `data.xml_presentation.content` | string | XML 格式的完整内容 |

## 常见错误

| 错误码 | 含义 | 解决方案 |
|--------|------|----------|
| 404 | 演示文稿不存在 | 检查 `xml_presentation_id` 是否正确 |
| 403 | 权限不足 | 检查是否拥有 `slides:presentation:read` scope，或是否有访问权限 |
| 400 | 参数格式错误 | 确保 `--params` 是合法的 JSON 字符串 |

## 注意事项

1. 直接调用底层 API 前，使用 `lark-cli schema slides.xml_presentations.get` 查看最新的参数结构
2. 返回的 XML 在 `data.xml_presentation.content` 字段中
3. 如果只需要部分信息，可以使用 `jq` 等工具过滤返回结果

## 相关命令

- [slides +create](lark-slides-create.md) - 创建 PPT / 添加幻灯片页面
- [xml_presentation.slide delete](lark-slides-xml-presentation-slide-delete.md) - 删除幻灯片页面
