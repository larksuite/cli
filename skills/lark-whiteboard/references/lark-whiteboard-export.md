# whiteboard +export（导出画板）

> **前置条件：** 若本操作链尚未读取 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)，先读取并确定 `<identity>` 为 `user` 或 `bot`；否则复用已确定身份。以下命令在同一条操作链中保持同一身份。

导出画板内容，支持导出为预览图片、SVG 矢量图、提取 PlantUML/Mermaid 代码，或获取飞书 OpenAPI 原生画板节点格式。

## 参数

| 参数                   | 必填 | 说明                                                                     |
|----------------------|----|------------------------------------------------------------------------|
| `--whiteboard-token` | 是  | 画板 token，需要拥有画板的读权限                                                    |
| `--output-type`      | 是  | 输出格式：`preview`（预览图片）、`svg`（SVG 矢量图）、`source`（PlantUML/Mermaid 代码）、`raw`（OpenAPI 原生画板节点格式） |
| `--output`           | 否  | 输出路径。当 `--output-type preview` 时必填，推荐传入无后缀路径；当 `--output-type svg/source/raw` 时可选，不填则直接输出到终端 |
| `--overwrite`        | 否  | 覆盖已存在的文件，默认为 false                                                     |

这里的 `+export --overwrite` 只覆盖 `--output` 指向的本地输出文件，不改变远端画板。它与 [`+update --overwrite`](./lark-whiteboard-update.md#replace非空画板) 的整板替换语义完全不同。

## 输出格式

- `preview`：预览图片。用于查看当前画布视觉状态、布局和写后效果。推荐使用 `--output ./preview` 这类无后缀路径；CLI 会根据实际 `Content-Type` 补齐 `.png` 或 `.jpg`。
- `svg`：标准 SVG 矢量图。用于离线查看、视觉对比或本地视觉证据。它是纯视觉快照，不代表原生节点语义；思维导图层级、表格结构、连接器绑定等信息不能从 SVG 中可靠恢复。
- `source`：PlantUML/Mermaid 代码。仅限画板内有且仅有一个 PlantUML/Mermaid 图时，才可导出代码；否则返回值会说明不存在源码或存在多个源码节点。
- `raw`：飞书 OpenAPI 原生画板节点格式。用于读取 board state、定位 `data.nodes[].id`、核对节点字段和作为写入前事实依据。`raw` 导出本身不代表写入语义；不要把完整导出快照手改后交给 `+update raw` 声称局部修改，因为 `+update raw` 会创建新节点并重新分配 ID。

导出只负责读取和保存当前状态。后续 append、patch、delete 或 replace 的写入语义见 [Workflow](./lark-whiteboard-workflow.md)。

## 示例

### 示例 1：导出画板为预览图片

```bash
lark-cli whiteboard +export \
  --whiteboard-token "wbcnxxxxxxxx" \
  --output-type preview \
  --output ./preview \
  --as <identity>
```

### 示例 2：提取画板中的代码并直接输出

```bash
lark-cli whiteboard +export \
  --whiteboard-token "wbcnxxxxxxxx" \
  --output-type source \
  --as <identity>
```

### 示例 3：导出画板为 SVG 矢量图

```bash
lark-cli whiteboard +export \
  --whiteboard-token "wbcnxxxxxxxx" \
  --output-type svg \
  --output ./whiteboard.svg \
  --as <identity>
```

### 示例 4：导出画板原始节点结构到文件

```bash
lark-cli whiteboard +export \
  --whiteboard-token "wbcnxxxxxxxx" \
  --output-type raw \
  --output ./nodes.json \
  --overwrite \
  --as <identity>
```
