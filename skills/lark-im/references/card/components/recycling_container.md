# 循环容器（搭建工具专属，无 JSON tag）

批量渲染同版式不同数据的列表（如商品列表、推荐列表）。**仅支持在飞书卡片搭建工具中可视化构建，不支持手写卡片 JSON 代码实现**——因此没有 `tag` 字段可直接编排。

## 使用方式

1. 在[卡片搭建工具](https://open.feishu.cn/cardkit)中添加循环容器组件，绑定一个对象数组变量。
2. 在容器内添加任意展示/交互/分栏组件，并将其字段绑定到对象数组的子变量。
3. 发布卡片模板后，发送时通过 `template_variable` 传入实际数据数组，数组每个元素对应一条循环项。

## 发送示例（模板 + 变量赋值）

```json
{
  "type": "template",
  "data": {
    "template_id": "AAqi6xJ8rabcd",
    "template_version_name": "1.0.0",
    "template_variable": {
      "looping": [
        { "title": "**和风陶韵**", "description": "...", "image": { "img_key": "img_v3_xxx" } },
        { "title": "**匠心之作**", "description": "...", "image": { "img_key": "img_v3_yyy" } }
      ]
    }
  }
}
```

将以上 JSON 压缩转义后作为 `messages.create` 的 `content`，`msg_type` 为 `interactive`。

## 嵌套 / 易错点

- 不支持再嵌套循环容器（对象数组变量不支持嵌套对象数组类型）。
- 数组元素个数即渲染条数，可直接控制列表长度。
- 若循环容器内嵌表单容器的交互组件（如 input），交互组件的 `name`（表单项标识）必须绑定到不重复的子变量，否则预览/发送报错。
