# 画板创作/编辑工作流

本 Workflow 是 `lark-whiteboard` 唯一的远端写入编排入口。`SKILL.md` 只选择原子操作，`routes/*.md` 只生成和检查本地产物，Shortcut reference 只描述单个命令契约。

## 范围守卫

先判断用户要操作的是文档结构还是同一画布：

| Scope | Owner | Rule |
|---|---|---|
| 从已有文档链接解析 whiteboard token | `lark-whiteboard` 的只读准备阶段 | 可读取文档并提取已有 token，不修改文档 block。 |
| 新建、插入、移动或删除文档中的画板 block | `lark-doc` | 切换到 `lark-doc`，完成 block 操作后再交回 token。 |
| 查看或修改某个既有画板内部内容 | `lark-whiteboard` | 进入后续路由。 |
| “下方/旁边/后面”的 anchor 可能同时指文档 block 和画布节点 | 先澄清 | 未明确 scope 前不得写文档或画板。 |

不能只根据相对位置词一律澄清。“在节点 A 下方”已有明确 canvas anchor；只有 anchor 属于文档还是画布无法判断时才澄清。

## 请求原子化

first-match 前先把复合请求拆成有序原子操作。例如：

```text
“把标题改成 Q3，删除右上角的废弃节点，再导出图片”
->
1. patch 标题节点
2. delete 废弃节点
3. read/export 最终预览
```

- 保留用户明确表达的顺序。
- 后续操作依赖前序结果时，先验证前序结果，再重新读取当前 board state。
- 任一操作失败时停止依赖它的后续写操作，报告已完成、失败和未执行部分；不要声称事务回滚。
- 不为减少命令数把 append、patch 或 delete 自动折叠成 replace。

## 写前事实

任何写操作都先读取实际画板状态：

先按 `lark-shared` 和目标资源权限确定 `<identity>` 为 `user` 或 `bot`，并在状态读取、请求预览、真实写入和写后验证中保持一致。

```bash
lark-cli whiteboard +export \
  --whiteboard-token <board_token> \
  --output-type raw \
  --as <identity>
```

默认 JSON envelope 中 `ok === true`，并且满足以下任一条件时，画板才是 `blank`：

- `data.nodes` 是空数组。
- `data.msg === "whiteboard is empty"`。

`data.nodes` 是非空数组时为 `nonempty`。命令失败、响应格式异常或缺少上述合法空白证据时为 `unknown`，必须停止写入。用户声称“这是空白画板”只能作为线索。

## 主操作与当前能力

| Operation | 当前执行器 | 边界 |
|---|---|---|
| read/export | `+export` | 按 preview、svg、source、raw 选择输出。 |
| initialize | `+update`，不带 `--overwrite` | 仅限已确认 blank 的画板。 |
| append | OpenAPI `nodes[]` 用 `+node-create`；Mermaid / PlantUML / SVG source 只能用于不要求锚点放置的普通追加，且不带 `--overwrite` | 只创建新内容；不修改或删除既有节点。 |
| patch | `+node-update` | 必须已唯一定位 node id，并准备只包含待修改字段的 OpenAPI payload。 |
| delete | `+node-delete` | 必须通过 raw 确认 node id，dry-run 后取得删除批准，真实执行传 `--yes`。 |
| replace | `+update --overwrite` | 丢弃非空画板的全部旧状态，必须经过覆盖确认。 |
| parse-image | `+parse-image` / `+parse-image-result` | 服务端把一张本地图片解析成画板内容并自动写入目标画板；不返回 SVG，不走本地 `whiteboard-cli`。 |

`+update --input_format raw` 仍调用创建节点接口：输入 ID 只用于本批次关系标识，服务端会重新分配 ID。它不能更新或删除现有节点；对应操作必须使用 `+node-update` 和 `+node-delete`。

## ParseImage Workflow

ParseImage 只用于“把一张本地图片里的结构解析成目标画板内容”。它不是图片节点上传，也不是旧 `image2svg` 原型。

适用条件：

- 用户提供本地图片路径，或上下文中已有可访问的本地图片文件。
- 用户目标是让服务端自动解析图片内容并写入某个已有画板。
- 已定位 `whiteboard-token`。

不适用条件：

- 只想把图片作为图片/素材插入画板。
- 只想得到 SVG、TOS URL 或中间解析产物。
- 想用本地 `whiteboard-cli` 从图片生成节点；当前本路径不做本地图片解析。

执行步骤：

1. 按 `lark-shared` 选择身份；首版 `+parse-image` 只使用 `--as user`。
2. 若目标画板可能已有内容，默认 `overwrite=false` 追加。只有用户明确要求覆盖/替换，才传 `--overwrite`。
3. 如需指定 Canvas Agent 模式，传 `--mode mini|flash|agentic|agentic_max`；不传时服务端默认 `flash`。
4. 先 dry-run:

```bash
lark-cli whiteboard +parse-image \
  --whiteboard-token <board_token> \
  --image <local_image_path> \
  --as user \
  --dry-run
```

5. 真实提交:

```bash
lark-cli whiteboard +parse-image \
  --whiteboard-token <board_token> \
  --image <local_image_path> \
  --as user
```

6. 提交成功只表示服务端任务已创建，不表示画板已经写入完成。记录返回的 `task_id` 和 `next_command`。
7. 用户明确要求等待、评测要求同轮闭环，或需要最终写入证据时，调用:

```bash
lark-cli whiteboard +parse-image-result \
  --whiteboard-token <board_token> \
  --task-id <task_id> \
  --wait \
  --as user
```

8. 成功后如需证明画板内容，继续用 `+export --output-type raw` 或 preview 验证。

禁止事项：

- 不使用 `image2svg` shortcut；本仓不新增该 shortcut。
- 不下载 SVG 或 TOS URL 交给 agent 手工处理。
- 不用 `whiteboard-cli` 作为 ParseImage 的本地替代路径。
- 不把 submit 的 `pending` 结果报告为画板写入完成。
- `+parse-image-result` 只是查询/等待，不触发写入；后台写入由 Engine worker 自动完成。

## 创作 Workflow

### Step 1：获取 board_token

| 用户给了什么 | 怎么处理 |
|---|---|
| whiteboard token（`wbcnXXX`） | 直接使用。 |
| 文档 URL，文档中已有画板 | 只读获取文档内容，从 `<whiteboard token="..."/>` 提取 token。 |
| 文档 URL，需要新建画板 | 切换到 `lark-doc` 创建空白画板 block；本 Skill 不直接修改文档结构。 |

`lark-doc` 创建 block 后必须把 token 和 canvas goal 交回本 Skill。

### Step 2：确认 initialize

用 raw 确认 board state。只有 `blank` 才能进入 initialize；`nonempty` 时重新判断用户要 append 还是 replace，`unknown` 时停止。

### Step 3：生成与检查产物

进入[渲染 & 写入画板](#渲染--写入画板)，按 route 生成 artifact 和检查证据。

### Step 4：写入与验证

使用 `+update`，不得传 `--overwrite`。先 dry-run，真实写入前立即再次确认 blank，再复用同一 artifact 和幂等 token 执行。写后导出 raw 或 preview 验证。

当前接口没有 revision/CAS 条件，空白检查是 best-effort，不能声称原子保证。

## 编辑 Workflow

### append

append 只创建新内容并保留现有节点：

1. raw 确认目标为 `nonempty`。
2. 确认请求不要求修改、删除或复用既有节点 ID。
3. 生成并检查独立 artifact，然后按 artifact contract 选择唯一执行器：
   - 已编译 OpenAPI `nodes[]`（包括 DSL 转换结果）用 `+node-create`。
   - Mermaid / PlantUML / SVG source 只有在普通追加、不要求相对既有节点放置时，才能用不带 `--overwrite` 的 `+update`。
4. 用户说“在 X 旁边/附近/下方/上方/某列/某阶段/某泳道/某象限”时，默认这是 canvas 空间约束，而不是普通追加。必须先用 raw 定位 anchor 节点或同组节点 bbox，再用可验证的 OpenAPI payload 放置新增节点，并在写后验证新增 bbox 仍在目标区域。
5. 如果相对放置、无碰撞证明或跨新旧节点连接缺少可验证的最终 OpenAPI payload，进入[能力边界](#能力边界)。不得改用 source `+update` 或 `--overwrite` 来绕过定位能力。
6. 对最终 artifact 执行所选命令的 dry-run；不要传 `--overwrite`。
7. 复用同一 artifact、幂等 token 和身份执行，再用 raw 或 preview 验证既有内容仍在、新内容出现，且新增内容没有明显覆盖旧内容。存在 anchor 要求时，同时验证新增节点 bbox 与 anchor bbox 的关系。

Mermaid、PlantUML 和 SVG 可直接使用对应 `--input_format`。DSL 必须先由 `whiteboard-cli` 转成 OpenAPI `nodes[]`。导出的现有 raw 不得作为 append 输入；否则会复制节点，而不是修改节点。

### patch

已有节点的字段级修改使用 `+node-update`:

1. 用 raw 唯一定位每个目标 node id，并读取同一节点的 type；目标不唯一时停止并要求定位依据。
2. 构造 `{ "nodes": [...] }`，每个 item 至少包含 raw 中的 `id` 和 `type`，其余只表达这次要改的内容。省略其他字段表示不修改，不得用默认值填充，也不要根据 id 猜 type。可以把目标 raw node 或原 `text` 子对象作为输入起点；`+node-update` 按当前 whiteboard node OpenAPI 结构保留正式字段，并剥离 response envelope 和非 node 噪声。不要在 prompt 中手工枚举可写字段。
3. 对最终 payload 执行 `+node-update --dry-run`，检查 `batch_update` URL、params 和 body。dry-run body 才是实际会发给服务端的字段集合；如果目标改动没有出现在 dry-run body 中，说明 payload 结构或 CLI 投影有问题，不要继续真实写入。
4. 复用同一 payload、幂等 token 和身份真实执行。
5. 用 raw 读回所有目标 node id，验证显式修改字段已经变化，未请求字段不能因默认值被覆盖。如服务端提示未完整完成，不得只依据部分成功响应声称整批成功。文本替换还必须导出 preview，确认文字可读、层级正确，并保留原容器的颜色和强调关系；服务端把颜色字段归一化为 theme code 时，以 preview 可读性为完成依据。
6. 真实执行失败时，先读取 `error.field_violations[]` 的字段路径和原因，再看 hint 与 log_id；只围绕该字段和同一修改意图做一次可验证调整。没有字段详情或调整后仍失败就停止并报告能力边界；不得自动改用 `+node-create` 遮罩旧节点、SVG Edit、raw create 或 `+update --overwrite`，也不要在 minimal/full/raw/envelope 之间多轮盲试。

对单一 Mermaid/PlantUML 代码图的源码重写仍属于 [Source Round-Trip](#source-round-trip)，它实际是 replace，不得用 `+node-update` 伪装。无法生成安全的节点级 payload 时进入[能力边界](#能力边界)，不自动转 replace。

### delete

已有节点的精确删除使用 `+node-delete`:

1. 用 raw 确认精确 node id；不能根据 ambient context 或最近消息猜测。
2. 对最终 id 列表执行 `+node-delete --dry-run`，检查 `DELETE /nodes/batch_delete` 和 body。
3. 展示精确 node id 与删除请求，取得这次删除的批准。
4. 复用同一 id 列表、幂等 token 和身份，传 `--yes` 真实执行。
5. 用 raw 或 preview 读回，确认目标已删除且未选中内容未被意外删除。

“删除全部节点”在用户语义上仍是 delete，不得因最终为空就自动改写为 `+update --overwrite`。

### replace

replace 用完整最终 artifact 丢弃非空画板的全部旧状态：

1. raw 确认当前为 `nonempty`；preview 只用于视觉核对和损失展示，不能替代 board state 证据。
2. 生成完整最终 artifact，并完成当前格式可用的 preview/check。
3. 对同一 artifact 执行 `+update --overwrite --dry-run`，生成不发请求的本地请求预览。
4. 展示最终结果、会丢失的旧节点、ID、层级、连接、资源及交互语义，以及精确覆盖请求，再取得 final overwrite approval。
5. 批准后立即用 raw 复查 board state；目标、状态、artifact 或身份变化时重新 preflight 和确认。
6. 复用同一 artifact、幂等 token 和身份执行 `+update --overwrite`。
7. 写后立即导出 raw 或 preview 验证。

## 渲染 & 写入画板

### 渲染路由

**先确定模型家族**：按训练来源选择 `Claude` / `Gemini` / `GPT` / `GLM` / `Doubao 或 Seed` / `Other`。模型家族只决定本地产物表达方式，不改变 mutation semantics，也不是 `--as user/bot` 的认证身份。

先处理用户显式格式约束：

- 用户明确要求 Mermaid、PlantUML、SVG、DSL 或 OpenAPI nodes 时，优先使用该 artifact 路径。
- 显式格式只约束本地产物或 source 类型，不能改变 mutation semantics。例如“用 SVG 修改已有画板”仍必须是 patch 或 replace，不能因为选择 SVG 就自动 append 或 overwrite。
- 显式格式不可满足当前 mutation 的安全边界时，进入[能力边界](#能力边界)，不要改走另一个写入语义。

没有显式格式约束时，按上到下匹配，命中即停：

| 图表类型 | 模型家族 | 路径 |
|---|---|---|
| 当前要生成/追加的内容包含 @用户提及或图片/配图 | 任何身份 | [`../routes/dsl.md`](../routes/dsl.md) |
| 思维导图、时序图、类图、饼图、甘特图 | 任何身份 | [`../routes/mermaid.md`](../routes/mermaid.md) |
| 鱼骨图、金字塔图、流程图 | `Doubao` / `Seed` | [`../routes/dsl.md`](../routes/dsl.md) |
| 其他图表 | `Claude` / `Gemini` / `GPT` / `GLM` / `Doubao` / `Seed` | [`../routes/svg.md`](../routes/svg.md) |
| 其他图表 | `Other` | [`../routes/dsl.md`](../routes/dsl.md) |

SVG route 在写入前发生语法崩溃、两轮仍有 `text-overflow` error，或 preview 严重错乱时，可以丢弃本地 SVG 并从零改走 DSL。该 fallback 只替换本地产物，不能改变 append/replace 语义或绕过写入门槛。

### Artifact Contract

route 只交回：

- source 文本或文件路径。
- 当前格式可用的 preview/check 结果。
- 人工视觉审查结论。
- 可选的 compiled OpenAPI nodes；若提供，后续 dry-run 与真实写入必须复用同一份 payload。

route 不读取目标画板、不选择 mutation semantics、不执行远端写入，也不自行报告远端成功。

交给 `+node-create` 的 artifact 必须是顶层 `{ "nodes": [...] }` 或该命令明确支持的等价输入。`whiteboard-cli --to openapi --format json` 的 `CliResponse` envelope 只有在被执行器明确支持时才能直接传入；否则必须先整理为 `{ "nodes": data.result.nodes }`。不得把 route 的本地成功响应 envelope 当成已可写入的节点 payload。

### 已有输入与格式约束

用户提供完整 source 或 OpenAPI nodes，只能跳过生成步骤，不能跳过 board state、preview/check、dry-run、确认或写后验证。

- 非空画板收到完整 Mermaid/PlantUML/SVG，但用户没有说明 append 还是 replace：必须先澄清 mutation intent。
- `raw` 只适合消费可信生成器的创建 payload；禁止手改导出的 raw 后用 `+update` 声称 patch。
- `whiteboard-cli` 当前不能本地渲染 PlantUML。`+update --dry-run` 也只生成本地请求预览，不验证服务端解析；真实写入是第一次服务端解析。执行前必须明确缺少写前几何 preview 和服务端解析证明，用户要求其中任一证明时停止。

### 原生语义约束

用户明确要求“原生表格 / 原生思维导图 / Mermaid 源码 / PlantUML 源码 / 可继续按源码维护”时，不能只用普通形状、文本和连线模拟后声称满足原生可编辑性。

- 如果输入本身是 Mermaid/PlantUML source，保留源码路径；对既有单一源码图的修改走 [Source Round-Trip](#source-round-trip)，并说明 replace 语义。
- 如果只能生成普通 OpenAPI nodes 或 SVG 来模拟表格、思维导图、Mermaid/PlantUML，必须把它标为视觉降级；用户没有接受降级时进入[能力边界](#能力边界)，不写入。
- 写后验证不能只看视觉接近；需要核对目标是否仍具备用户要求的原生结构或源码级维护入口。无法核对时报告能力边界。

### 写入策略

| Operation | Preservation | Confirmation |
|---|---|---|
| initialize | 只写已确认 blank 的画板，不覆盖 | 原始明确写入请求即可。 |
| append | 只创建新内容，不覆盖、不修改既有节点 | 原始明确追加请求即可；精确放置要求必须先通过能力边界。 |
| patch | 只更新已定位 node id 的显式字段 | 原始明确修改请求即可；目标或字段不明时先澄清。 |
| delete | 只删除已确认的 node id | dry-run 后展示精确目标，取得删除批准并传 `--yes`。 |
| replace | 丢弃非空画板完整旧状态 | 本地请求预览后展示最终 preview、损失和精确覆盖请求，再取得 final overwrite approval。 |

所有可执行写操作共享以下规则：

- `--dry-run` 只执行本地校验和请求预览，不调用画板 OpenAPI，也不证明服务端会接受或正确渲染 artifact。
- 先生成本地请求预览，再真实执行；请求预览不是对未说明破坏性效果的授权。
- 检查、请求预览和真实执行复用同一 artifact；同一次逻辑写入只生成一个幂等 token，重试必须复用。
- 确认后如果目标、artifact 或 board state 变化，重新 preflight；replace 需要重新确认。
- 写后用 raw 或 preview 读回，不只看命令退出码。
- append 失败时不得自动使用 SVG Edit、清空画板或 `+update --overwrite`。
- patch 失败时不得自动使用 `+node-create` 遮罩、SVG Edit、raw create 或 `+update --overwrite`。
- source append 不能因为无法表达“放到右侧/旁边/某列”就升级成 replace；没有明确覆盖授权时，`--overwrite` 是破坏性操作。
- source 写入超时、无响应或结果不明时，先用 raw/preview 读回判断是否已经生效；不得盲目重试并重新生成幂等 token。

具体参数和命令示例见 [`+update`](./lark-whiteboard-update.md)、[`+node-create`](./lark-whiteboard-node-create.md)、[`+node-update`](./lark-whiteboard-node-update.md) 和 [`+node-delete`](./lark-whiteboard-node-delete.md)。

## Source Round-Trip

`+export --output-type source` 不是修改已有画板的默认第一步。只有同时满足以下条件，才能提出 source round-trip：

1. 导出确认目标是单一 Mermaid 或 PlantUML 代码图。
2. 用户要修改的是该代码图本身，不是在旁边追加独立内容。
3. 没有额外非源码内容需要保留，或用户接受丢弃它们。
4. 编辑后源码完成当前格式可用的检查。

该路径实际是 replace，不是 patch。必须先说明整板替换语义并取得同意，再进入 replace。Mermaid 可先本地 render/check；PlantUML 遵守[已有输入与格式约束](#已有输入与格式约束)中的 preview 限制。

source replace 执行后如果结果不明，先读回当前画板并与目标 artifact 对比；不能直接再次写入同一整图或改用 append 补救。

## 能力边界

当前 Skill 不能完成以下请求：

- 无法唯一定位 node id，或无法构造安全 OpenAPI payload 的高层级结构修改。
- 在复杂非空画板的指定位置插入或移动图形，同时证明无碰撞并保留全部连接语义、层级、资源和交互属性。
- 非空画板中要求“新增到右侧/旁边/下方/某列”等相对位置，但当前只有 Mermaid/PlantUML/SVG source，无法生成带最终 bbox 的 OpenAPI payload。
- 在没有对应原生写入能力时，承诺生成原生表格、原生思维导图、Mermaid/PlantUML 源码级可维护节点；普通形状模拟只能作为用户明确接受的视觉降级。

遇到能力边界时保持原画板不变，并提供与用户目标相符的选项：

1. 交给 `lark-doc` 新建独立画板，再 initialize 新内容。
2. 原内容可完整重建且用户接受损失时，生成完整 artifact 后走 replace。
3. 不写入，等待具备所需高层级转换或联合几何检查能力后再处理。

禁止把 raw ID 复用、根节点 offset、SVG Edit、清空画板或整板覆盖作为自动兜底。

## SVG Edit

[`svg-edit`](../routes/svg-edit.md) 是视觉重建路径，只能服务 replace，不能服务 append、patch 或 delete，也不能承诺保留节点语义。

它有两个独立确认阶段：

1. semantic-loss route consent：开始本地编辑前说明 node ID、连接语义、层级、表格、mention、锁定和评论等会丢失。
2. final overwrite gate：完成 edited SVG、preview/check 和本地请求预览后，展示最终视觉结果与精确整板覆盖请求，再取得写入批准。

第一阶段不是远端写入授权。任一阶段拒绝都不得写入。

## lark-doc 交接

`lark-doc` 负责文档结构，`lark-whiteboard` 负责画布内容。本 Skill 不直接创建、移动或删除文档 block。

`lark-doc` 创建或插入画板 block 后，至少交回：

- `board_token`。
- 用户的 canvas goal。
- 用户提供的源码或结构化数据。
- mutation intent，例如 initialize、append 或 replace。
- placement requirement，例如 document-level 或 canvas-level。

如果 initialize 失败，返回空白 token、失败阶段和可重试信息，不自行删除 block，也不把空白画板误报为完成。
