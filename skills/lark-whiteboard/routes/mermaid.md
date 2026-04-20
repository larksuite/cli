# Mermaid 路径

适用于：思维导图、时序图、类图、饼图、甘特图。

## Workflow

```
Step 1: 读取知识
  - 读 scenes/mermaid.md — Mermaid 语法和使用方式

Step 2: 生成 Mermaid
  - 按 mermaid.md 的语法编写 .mmd 文件
  - 只输出纯 Mermaid 语法文本

Step 3: 渲染验证 & 写入画板 & 交付
  1. 创建产物目录 ./diagrams/YYYY-MM-DDTHHMMSS/
  2. 保存为 diagram.mmd
  3. 渲染（仅用于预览验证，PNG 不是最终产物）：
       npx -y @larksuite/whiteboard-cli@^0.2.0 -i diagram.mmd -o diagram.png
  4. 审查 PNG，有问题修改后重新渲染（最多 2 轮）
  5. 写入画板：用 whiteboard-cli 将 diagram.mmd 转换为 OpenAPI 格式并 pipe 给 +update：
       npx -y @larksuite/whiteboard-cli@^0.2.0 -i diagram.mmd --to openapi --format json \
         | lark-cli whiteboard +update --whiteboard-token <board_token> \
             --source - --input_format raw --idempotent-token <时间戳+标识> --yes --as user
       → 完整 dry-run / 确认流程见 SKILL.md [§ 写入画板](../SKILL.md#写入画板)
  6. 交付：向用户报告 board_token 写入成功
```
