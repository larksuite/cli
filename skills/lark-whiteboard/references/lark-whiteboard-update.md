# whiteboard +update

> **前置条件：** 若本操作链尚未读取 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)，先读取并确定 `<identity>` 为 `user` 或 `bot`；否则复用已确定身份。同一次读取、请求预览、写入和验证必须保持同一身份。

`+update` 向画板**创建**输入内容，是否清空旧内容由 `--overwrite` 决定：

- `initialize`：向已确认的空白画板写入第一批内容，不使用 `--overwrite`。
- `append`：向非空画板新增内容并保留旧内容，不使用 `--overwrite`。
- `replace`：用户明确接受丢弃非空画板全部旧状态后，使用 `--overwrite` 写入完整最终 artifact。

它不是节点级 patch 或 delete 的执行器；对应操作分别使用 [`+node-update`](./lark-whiteboard-node-update.md) 和 [`+node-delete`](./lark-whiteboard-node-delete.md)。完整路由、能力边界、确认和写后读回规则见 [`lark-whiteboard-workflow.md`](./lark-whiteboard-workflow.md)。

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--whiteboard-token` | 是 | 画板 token，需要拥有编辑权限。 |
| `--idempotent-token` | 否 | 幂等 token，最少 10 个字符；同一次逻辑写入的 dry-run、真实执行和重试复用同一个值。 |
| `--overwrite` | 否 | 写入前删除全部现有内容；只有 replace 可以使用。 |
| `--source` | 是 | 输入内容，支持 `@path` 文件或 `-` stdin。 |
| `--input_format` | 否 | `raw`、`plantuml`、`mermaid` 或 `svg`，默认 `raw`。 |

`--dry-run` 是全局参数，只执行 CLI 当前具备的本地校验并打印请求形状，不调用画板 OpenAPI。它可以发现 raw payload 的本地解析错误，但不能验证 PlantUML、Mermaid 或 SVG 的服务端解析与渲染结果。

## initialize（空白画板）

Workflow 必须先用 raw 确认 blank。先对最终 artifact 生成本地请求预览，真实写入前立即再次读取；状态变为 nonempty 或 unknown 时停止。

```bash
lark-cli whiteboard +update \
  --whiteboard-token <board_token> \
  --source @./diagram.mmd \
  --input_format mermaid \
  --idempotent-token <10+字符唯一串> \
  --dry-run \
  --as <identity>

lark-cli whiteboard +update \
  --whiteboard-token <board_token> \
  --source @./diagram.mmd \
  --input_format mermaid \
  --idempotent-token <同一个幂等串> \
  --as <identity>
```

写后通过 raw 或 preview 验证。当前接口没有 revision/CAS 条件，空白检查是 best-effort，不能声称原子保证。

## append（非空画板）

append 不传 `--overwrite`。本命令适合直接写入 Mermaid / PlantUML / SVG source 的普通追加；已编译的 OpenAPI `nodes[]` 优先使用 [`+node-create`](./lark-whiteboard-node-create.md)。两条路径都不得修改、删除或复用既有节点 ID。

如果非空画板的请求包含“在 X 旁边/右侧/下方/某列/某阶段”等相对位置要求，source append 通常不够安全，因为它不能在写前给出最终节点 bbox。此时应优先生成可定位的 OpenAPI `nodes[]` 并走 `+node-create`；无法生成时进入 workflow 的能力边界。不得为了满足位置要求把 append 改成 `--overwrite`。

```bash
lark-cli whiteboard +update \
  --whiteboard-token <board_token> \
  --source @./addition.svg \
  --input_format svg \
  --idempotent-token <10+字符唯一串> \
  --dry-run \
  --as <identity>

lark-cli whiteboard +update \
  --whiteboard-token <board_token> \
  --source @./addition.svg \
  --input_format svg \
  --idempotent-token <同一个幂等串> \
  --as <identity>
```

写后验证旧内容仍在、新内容出现，且新内容没有明显覆盖旧内容。存在相对位置要求时，还要验证新增节点 bbox 与目标 anchor bbox 的关系。追加失败或位置不符合预期时停止，不得自动整板覆盖。

DSL 产物需要先转换为 OpenAPI `nodes[]`，然后交给 `+node-create`：

```bash
npx -y @larksuite/whiteboard-cli@^0.2.13 -i ./diagram.json --to openapi --format json -o ./compiled-nodes.json

lark-cli whiteboard +node-create \
  --whiteboard-token <board_token> \
  --source @./compiled-nodes.json \
  --idempotent-token <10+字符唯一串> \
  --dry-run \
  --as <identity>
```

本地请求预览生成后，复用同一个 `compiled-nodes.json`、幂等 token 和身份执行 `+node-create`。

## replace（非空画板）

只有 replace 可以使用 `+update --overwrite`。执行前必须：

1. 用户明确要求或接受按 replace 准备完整 artifact。
2. 完整最终 artifact 已完成适用的 preview/check。
3. 对最终 artifact 执行 `--dry-run`，生成不发请求的本地请求预览。
4. 展示最终结果、将丢失的旧节点、ID、层级、引用和交互语义，以及精确覆盖请求。
5. 在上述证据都确定后取得 final overwrite approval。

```bash
lark-cli whiteboard +update \
  --whiteboard-token <board_token> \
  --source @./final.svg \
  --input_format svg \
  --idempotent-token <10+字符唯一串> \
  --overwrite \
  --dry-run \
  --as <identity>

lark-cli whiteboard +update \
  --whiteboard-token <board_token> \
  --source @./final.svg \
  --input_format svg \
  --idempotent-token <同一个幂等串> \
  --overwrite \
  --as <identity>
```

写后立即导出 preview 或 raw 验证。

## 输入格式边界

- `raw` 是 OpenAPI 原生**创建节点**格式。服务端会重新分配节点 ID；已编译 `nodes[]` 优先用 `+node-create`，不要编辑导出的 raw 并回传来声称 patch/delete。
- `mermaid` / `plantuml` / `svg` 由远端转换，可用于 initialize、普通 append 或 replace。完整源码不会自动决定写入语义；当用户要求保留原图并在指定位置新增时，源码写入不能升级为 replace。
- `whiteboard-cli` 当前不能本地渲染或编译 PlantUML；`--dry-run` 也不请求服务端，因此真实写入是第一次服务端解析。执行前必须披露该限制；用户要求写前视觉或服务端解析证明时停止。完整规则见 [已有输入与格式约束](./lark-whiteboard-workflow.md#已有输入与格式约束)。
- 对单一 Mermaid/PlantUML 图做源码 round-trip 实际是 replace，详见 [Source Round-Trip](./lark-whiteboard-workflow.md#source-round-trip)。
- 复杂图表的生成、渲染与检查见 [渲染 & 写入画板](./lark-whiteboard-workflow.md#渲染--写入画板)。

## Safety

- 所有写入先用 `--dry-run` 生成本地请求预览，并让检查、请求预览和真实执行复用同一 artifact。
- 同一次逻辑写入只生成一个幂等 token，重试不得重新生成。
- initialize/append 不传 `--overwrite`；replace 之外不得传 `--overwrite`。
- source append 无法满足指定位置时停止或改用可定位的 OpenAPI `nodes[]`；不得用 `--overwrite` 作为位置兜底。
- patch/delete 分别使用 `+node-update` / `+node-delete`；不得用 raw 创建或 replace 伪装成功。
- append 失败时不得自动回退到 SVG Edit、清空画板或 replace。
- patch 失败时不得自动回退到 `+node-create` 遮罩、SVG Edit、raw create 或 replace。
- source 写入超时、无响应或结果不明时，先读回当前画板判断是否已经生效；不得盲目重试并重新生成幂等 token。
- 写后通过 raw 或 preview 读回，不只根据退出码声称成功。
