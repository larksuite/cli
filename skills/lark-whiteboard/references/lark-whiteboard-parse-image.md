# whiteboard +parse-image

> **前置条件:** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。ParseImage shortcut 支持 `--as <identity>` 和 `--as bot`，按目标画板权限选择并保持同一身份。

提交一张本地图片，由服务端自动解析成画板内容并写入目标画板。该能力走 Engine 后台任务: CLI 只提交图片和查询任务状态，不下载 SVG，不调用本地 `whiteboard-cli` 做图片解析。

## 适用场景

- 用户明确要把图片内容解析成可写入飞书画板的内容。
- 已经有目标画板 token。
- 输入是一张本地图片文件。

## 不适用场景

- 用户只想把图片作为图片节点插入画板。
- 用户只想导出或拿到 SVG。
- 用户要编辑已有节点字段、删除节点或追加已编译 OpenAPI 节点；分别使用 `+node-update`、`+node-delete`、`+node-create`。
- 用户要在文档正文里创建或移动画板 block；交给 `lark-doc`。

## parse-image 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--whiteboard-token` | 是 | 目标画板 token。 |
| `--image` / `-i` | 是 | 本地图片路径，支持 PNG、JPG、JPEG、GIF、WEBP。 |
| `--overwrite` | 否 | 是否覆盖现有画板内容。默认 false，即追加。 |
| `--mode` | 否 | Canvas Agent 模式，可选 `mini`、`flash`、`agentic`、`agentic_max`；不传时服务端默认 `flash`。 |
| `--idempotent-token` | 否 | 幂等 token。未传时 CLI 自动生成。 |

## parse-image 示例

先预览请求:

```bash
lark-cli whiteboard +parse-image \
  --whiteboard-token <whiteboard_token> \
  --image ./input.png \
  --as <identity> \
  --dry-run
```

提交任务:

```bash
lark-cli whiteboard +parse-image \
  --whiteboard-token <whiteboard_token> \
  --image ./input.png \
  --mode agentic_max \
  --as <identity>
```

覆盖写入时必须显式传 `--overwrite`:

```bash
lark-cli whiteboard +parse-image \
  --whiteboard-token <whiteboard_token> \
  --image ./input.png \
  --overwrite \
  --as <identity>
```

提交成功输出 `task_id`、`status` 和可恢复命令 `next_command`。`status=pending` 只表示任务已创建，不表示画板已经写入完成。

## parse-image-result

查询或等待 ParseImage 任务结果。

### 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--whiteboard-token` | 是 | 提交任务时的目标画板 token。 |
| `--task-id` | 是 | `+parse-image` 返回的 task id。 |
| `--wait` | 否 | 持续轮询直到成功或超时。默认 false。 |
| `--timeout` | 否 | `--wait` 的整体超时时间，默认 `20m`。 |
| `--interval` | 否 | `--wait` 的轮询间隔，默认 `10s`。 |

### 示例

单次查询:

```bash
lark-cli whiteboard +parse-image-result \
  --whiteboard-token <whiteboard_token> \
  --task-id <task_id> \
  --as <identity>
```

等待完成:

```bash
lark-cli whiteboard +parse-image-result \
  --whiteboard-token <whiteboard_token> \
  --task-id <task_id> \
  --wait \
  --as <identity>
```

成功时输出 `ids`、`extra` 和 `previous_revision`。如果还在处理中，输出 `pending` 或 `running`。失败时保持结构化错误，不返回 SVG、TOS URL、Canvas 原始错误或完整画板数据。

## Safety

- `+parse-image` 是 `write` 风险命令；`+parse-image-result` 是 `read` 风险命令。
- 不要把 `+parse-image` 的提交成功当成最终写入成功；必须用 result 或后续 `+export` 验证。
- 不要自动把追加失败改成覆盖写入。
- 不要使用旧 `image2svg` 设计；本 skill 不新增也不调用 `image2svg` shortcut。
- 不要手工处理中间 SVG/TOS URL；这些属于服务端内部实现。
- 等待超时不代表任务失败。保留 `task_id`，之后继续用 `+parse-image-result` 查询。
