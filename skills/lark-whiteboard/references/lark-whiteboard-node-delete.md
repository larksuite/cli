# whiteboard +node-delete

> **前置条件:** 若本操作链尚未读取 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)，先读取并确定 `<identity>`；否则复用已确定身份。命令同时支持 `user` 和 `bot`，并在读取、请求预览、写入和验证中保持一致。

按 node id 删除已有节点。这是高风险写操作，只能删除已经确认的目标节点。

## 适用场景

- 已经知道 `whiteboard-token`，且所选身份拥有画板编辑权限。
- 已经确认要删除的 node id。
- 需要删除已有画板中的局部节点。

## 不适用场景

- 不知道或无法唯一确认目标 node id。
- 只是想隐藏、移动或更新节点。
- 需要整体替换画板。

## 定位节点

先导出 raw 节点结构:

```bash
lark-cli whiteboard +export \
  --whiteboard-token <whiteboard_token> \
  --output-type raw \
  --as <identity>
```

从返回的 `data.nodes[].id` 读取目标 node id。不要删除从 ambient context 或最近消息猜测出来的节点。

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--whiteboard-token` | 是 | 画板 token。 |
| `--node-ids` | 是 | 要删除的 node id，多个 id 用英文逗号分隔。 |
| `--idempotent-token` | 否 | 幂等 token，最少 10 个字符。重试同一次逻辑删除时复用同一个值。 |
| `--yes` | 真实执行需要 | 高风险写操作确认。先 dry-run 并核对目标，取得删除批准后再传。 |

## 示例

```bash
lark-cli whiteboard +node-delete \
  --whiteboard-token <whiteboard_token> \
  --node-ids <node_id_1>,<node_id_2> \
  --idempotent-token <10+字符唯一串> \
  --as <identity> \
  --dry-run

lark-cli whiteboard +node-delete \
  --whiteboard-token <whiteboard_token> \
  --node-ids <node_id_1>,<node_id_2> \
  --idempotent-token <同一个幂等串> \
  --as <identity> \
  --yes
```

## 输出

```json
{
  "data": {
    "ids": "o2:5,o2:6",
    "count": 2
  }
}
```

## Safety

- 删除前必须用 `+export --output-type raw` 确认 node id。
- 先运行 `--dry-run`，检查 method 是 `DELETE`、URL 是 `/nodes/batch_delete`、body 是 `{"ids":[...]}`。dry-run 只生成本地请求预览，不请求画板 OpenAPI。
- 向用户展示精确 node id 和删除请求；只有确认目标并取得删除批准后才传 `--yes`。
- 请求预览、真实执行和重试复用同一 node id 列表、幂等 token 和身份。
- 写后用 raw 或 preview 读回，确认目标节点已删除且未选中内容未被意外删除。
- 不要因为用户说“删掉这个”就删除最近消息里的节点；缺少 node id 时先导出 raw 或要求定位依据。
