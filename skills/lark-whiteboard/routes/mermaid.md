# Mermaid 路径

适用于：思维导图、时序图、类图、饼图、甘特图。

## Workflow

```text
Step 1: 读取知识
  - 读 scenes/mermaid.md，了解 Mermaid 语法和使用方式

Step 2: 生成 Mermaid
  - 创建 ./diagrams/YYYY-MM-DDTHHMMSS/
  - 把纯 Mermaid 语法保存为 diagram.mmd

Step 3: 渲染审查并交回 Workflow
  - 渲染 preview：npx -y @larksuite/whiteboard-cli@^0.2.13 -i diagram.mmd -o diagram.png
  - 审查 PNG，有问题修改后重新渲染，最多 2 轮
  - 检查：npx -y @larksuite/whiteboard-cli@^0.2.13 -i diagram.mmd --check
  - 按需生成 compiled nodes：npx -y @larksuite/whiteboard-cli@^0.2.13 -i diagram.mmd --to openapi --format json -o compiled-nodes.json
  - 不在 route 内读取目标画板、选择 mutation semantics、执行远端写入或报告远端成功
```

## Artifact Contract

本 route 交回 [`lark-whiteboard-workflow.md`](../references/lark-whiteboard-workflow.md#渲染--写入画板)：

- `diagram.mmd`：Mermaid source。
- `diagram.png`：本地 preview。
- 本地 `--check` 结果和人工视觉审查结论。
- 可选 `compiled-nodes.json`：同一 source 转换出的 OpenAPI 创建 payload。

Workflow 决定 initialize、append 或 replace，并负责本地请求预览、确认、远端写入和读回。
