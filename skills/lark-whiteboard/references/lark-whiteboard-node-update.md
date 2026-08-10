# whiteboard +node-update

> **前置条件:** 若本操作链尚未读取 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)，先读取并确定 `<identity>`；否则复用已确定身份。命令同时支持 `user` 和 `bot`，并在读取、请求预览、写入和验证中保持一致。

按 node id 更新已有节点字段。CLI 输入是批量形态，执行层会发起一次 `batch_update` OpenAPI 请求。

## 适用场景

- 已经知道 `whiteboard-token`，且所选身份拥有画板编辑权限。
- 已经知道目标 node id。
- 需要局部修改文字、颜色、样式、位置等节点字段。

## 不适用场景

- 不知道或无法唯一确认目标 node id。
- 需要重绘复杂图表、重新布局整板，或在未知几何状态下承诺无碰撞。
- 想替换整个画板内容。

## 定位节点

先导出 raw 节点结构:

```bash
lark-cli whiteboard +export \
  --whiteboard-token <whiteboard_token> \
  --output-type raw \
  --as <identity>
```

从返回的 `data.nodes[]` 读取目标 node id 和 type。当前 gateway contract 要求每个 update item 同时包含原节点的 `id` 与 `type`；其余只保留待修改字段。省略其他字段表示不修改，不要用默认值填充未指定字段，也不要根据 id 猜 type。

文本节点是例外：只改文案时，不要只发送 `text: { "text": "新文案" }` 后假设样式会被保留。可以从 raw 中复制目标节点原有的 `text` 子对象作为起点，再替换其中的文案字段。CLI 会按当前 whiteboard node OpenAPI 结构保留正式字段；不要在 prompt 中手工枚举可写字段。raw 文本相等也不能证明 preview 中仍可读，所以文本替换后仍要看预览。

如果已经持有 `+export --output-type raw` 或 nodes OpenAPI 返回的完整 raw node，优先整理成顶层 `{ "nodes": [...] }`。`+node-update` 在发送 `batch_update` 前会剥离 response envelope、明显非 node 字段和无法映射到 whiteboard node OpenAPI 结构的噪声；正式 node 字段不应被误删。构造 payload 时仍应尽量只表达本次显式修改，避免把整个画板快照当成 patch；最终以 dry-run body 为真实发送预览。

兼容性只用于提高容错：如果上游误把未清洗的 raw/export 响应整体传给 `--source`，CLI 会尝试提取其中的 `data.nodes`。不要为了使用这个兼容能力主动构造 response envelope。

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--whiteboard-token` | 是 | 画板 token。 |
| `--source` | 是 | JSON，推荐包含顶层非空 `nodes` 数组，每个 node 必须包含 raw 中的 `id` 和 `type`。为容错也会识别未清洗 raw/export 响应中的 `data.nodes`。支持 `@path` 文件读取或 `-` stdin。 |
| `--idempotent-token` | 否 | 幂等 token，最少 10 个字符；非空时作为 `client_token` 随 `batch_update` 请求发送。 |

## 输入

CLI 输入保持批量形态:

```json
{
  "nodes": [
    {
      "id": "o2:5",
      "type": "composite_shape",
      "style": {
        "fill_color": "#F54A45"
      }
    }
  ]
}
```

文本替换示例，保留目标节点原有 `text` 样式，只替换文案:

```json
{
  "nodes": [
    {
      "id": "o2:5",
      "type": "composite_shape",
      "text": {
        "text": "新标题",
        "font_size": 18,
        "text_color": "#f1f5f9",
        "horizontal_align": "center",
        "vertical_align": "mid"
      }
    }
  ]
}
```

示例里的样式字段应来自目标节点 raw 或用户明确要求，不要凭空套默认值。写后必须依赖 preview 验证可读性，不能只依据 raw 文本判断成功。

执行时所有节点保持在同一请求中:

- `PUT /open-apis/board/v1/whiteboards/:whiteboard_id/nodes/batch_update`。
- body 为 `{"nodes": [...]}`，节点内的 `id` 会保留；response envelope 和非 node 噪声不会发送给 `batch_update`。
- 如果输入误带 response envelope，只会提取 `data.nodes`；`code/msg/ok` 等 envelope 字段不会发送给 `batch_update`。
- `--idempotent-token` 非空时，query 参数带 `client_token=<token>`。

## 示例

```bash
lark-cli whiteboard +node-update \
  --whiteboard-token <whiteboard_token> \
  --source @./node-updates.json \
  --idempotent-token <10+字符唯一串> \
  --as <identity> \
  --dry-run

lark-cli whiteboard +node-update \
  --whiteboard-token <whiteboard_token> \
  --source @./node-updates.json \
  --idempotent-token <同一个幂等串> \
  --as <identity>
```

## 输出

```json
{
  "data": {
    "ids": "o2:5",
    "count": 1
  }
}
```

## Safety

- 多节点更新前先使用 `--dry-run` 检查 `batch_update` method、URL、params 和 body。dry-run 只执行本地校验并打印请求预览，不请求画板 OpenAPI。
- 请求预览、真实执行和重试复用同一 `node-updates.json`、幂等 token 和身份。
- 用完整 raw node 写回时，以 dry-run body 为准确认哪些字段会真正发送。dry-run 只证明本地请求形状，不证明服务端接受这些字段。
- 文本替换写后必须导出 preview 检查可读性、层级和原容器样式；只看到 raw 文案变更不能声称完成。深色背景、强调卡片、标题和标签尤其要确认对比度没有被服务端主题色归一化破坏。
- `batch_update` 写前会统一校验，但结构性字段可能分阶段应用。如服务端提示未完整完成，必须读回所有请求 node id 确认状态。
- 真实执行失败时，先读取 `error.field_violations[]` 的字段路径和原因，再看 hint 与 log_id；只围绕该字段和同一修改意图做一次可验证调整。没有字段详情或调整后仍失败就停止并报告能力边界，不要在 minimal/full/raw/envelope 之间多轮盲试。
- 不要在节点更新失败时自动回退到 `+node-create` 遮罩、SVG Edit、raw create 或 `+update --overwrite`。只有用户另行明确要求替换整个画板时，才能进入 replace workflow。
