# whiteboard +node-create

> **前置条件:** 若本操作链尚未读取 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)，先读取并确定 `<identity>`；否则复用已确定身份。命令同时支持 `user` 和 `bot`，并在读取、请求预览、写入和验证中保持一致。

向已有画板追加 OpenAPI 节点。它适合局部新增已编译好的节点，不适合从零创作复杂图表。

## 适用场景

- 已经知道 `whiteboard-token`，且所选身份拥有画板编辑权限。
- 已经有可追加的 OpenAPI `nodes[]`。
- 需要向已有画板追加节点，而不是覆盖整图。

## 不适用场景

- 从零创作复杂图表，或需要自动布局、批量排版、复杂连线计算。
- 只有 DSL / Mermaid / SVG，还没有转换成 OpenAPI 节点。
- 只想在文档正文里插入或移动画板 block，这属于 `lark-doc`。
- 需要修改或删除已有节点；分别使用 `+node-update` 或 `+node-delete`。
- 需要遮住旧文字、覆盖旧标题、隐藏误放节点或用新增说明节点代替编辑已有节点；这些都是 patch 失败后的视觉补救，不属于 append。

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--whiteboard-token` | 是 | 画板 token。 |
| `--source` | 是 | JSON，必须包含非空 `nodes` 数组。支持 `@path` 文件读取或 `-` stdin。 |
| `--idempotent-token` | 否 | 幂等 token，最少 10 个字符。重试同一次逻辑新增时复用同一个值。 |

## 输入

`nodes[]` 必须是飞书 OpenAPI 画板节点，不是 whiteboard-cli DSL。不要把 `{"type":"shape","shape":...}` 这类 DSL 节点直接传给本命令。

推荐先用 `npx -y @larksuite/whiteboard-cli@^0.2.13 --to openapi --format json` 生成 OpenAPI 结果，再整理成 `{ "nodes": [...] }`。

如果上游产物是 `CliResponse` envelope（例如顶层包含 `code` / `data.result.nodes`），先抽取 `data.result.nodes` 并整理成顶层 `{ "nodes": [...] }`；不要把 envelope 当作 `nodes[]` payload 直接传给本命令。

```json
{
  "nodes": [
    {
      "id": "tmpNode",
      "type": "composite_shape",
      "x": 0,
      "y": 0,
      "width": 260,
      "height": 45,
      "text": {
        "text": "hello",
        "font_weight": "regular",
        "font_size": 14,
        "horizontal_align": "center",
        "vertical_align": "mid"
      },
      "style": {
        "border_color": "#3370ff",
        "border_width": "narrow",
        "border_style": "solid",
        "fill_color": "#e8f3ff"
      },
      "composite_shape": {
        "type": "round_rect"
      }
    }
  ]
}
```

## 示例

```bash
lark-cli whiteboard +node-create \
  --whiteboard-token <whiteboard_token> \
  --source @./nodes.json \
  --idempotent-token <10+字符唯一串> \
  --as <identity> \
  --dry-run

lark-cli whiteboard +node-create \
  --whiteboard-token <whiteboard_token> \
  --source @./nodes.json \
  --idempotent-token <同一个幂等串> \
  --as <identity>
```

## 输出

JSON 输出使用 `data.ids`，多个 id 用逗号拼接:

```json
{
  "data": {
    "ids": "o2:5"
  }
}
```

## Safety

- 写入前先用 `--dry-run` 检查 method、URL、params 和 body。dry-run 只执行本地校验并打印请求预览，不请求画板 OpenAPI。
- 对手写节点尤其要先 dry-run；它只能验证请求结构，不能证明节点语义一定可插入。
- 请求预览、真实执行和重试复用同一 `nodes.json`、幂等 token 和身份。
- 写后用 `+export --output-type raw` 或 preview 读回，确认旧内容仍在、新节点已出现，且新节点没有明显覆盖旧内容。
- 如果新增内容有“在 X 旁边/右侧/下方/某列/某阶段”等 anchor 要求，写前记录 anchor bbox 和预计新增 bbox；写后确认新增节点仍在目标区域。
- 复杂图表继续走 `whiteboard-cli -> Workflow` 路径，不要把 `+node-create` 当作默认创作入口。
