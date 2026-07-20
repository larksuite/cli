# 飞书思维笔记（Mindnote）

> **前置条件：** 先阅读 [`../SKILL.md`](../SKILL.md) 和 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和路由规则。

当用户要操作思维笔记时，入口属于 `lark-doc`，但实际执行命令使用 `lark-cli mindnotes nodes list/create`，不是 `docs +...`。

> [!IMPORTANT]
> 当前这条链路只支持**读取已有思维笔记**，以及在**已有思维笔记**里读取节点、创建子节点。
> `mindnotes nodes create` 是新增/更新节点命令，**不是**新建一个新的思维笔记。
> 如果用户要**新建思维笔记**，不要走本链路，改走 [lark-doc-whiteboard](lark-doc-whiteboard.md)。

## 获取 `mindnote_id`

`--mindnote-id` 传 **Mindnote 文档 token**，不是节点 ID。`lark-cli mindnotes` 只负责读取和写入思维笔记内部节点。

```bash
# 用户给了 Mindnote URL，或给了可能包着 Mindnote 的 Wiki URL
lark-cli drive +inspect --url "<mindnote_or_wiki_url>"
```

处理规则：

- 普通 Mindnote URL：`drive +inspect` 返回的 Mindnote token 可作为 `--mindnote-id`。
- Wiki URL：不要把 `/wiki/` 路径里的 wiki token 当作 `--mindnote-id`；必须先 `drive +inspect` 解包，确认底层类型是 `mindnote` 后再使用返回的真实 token。直接把 wiki token 传给 `mindnotes nodes list` 通常会返回 `3410003 resource not found`。

## 命令

```bash
# 先看命令帮助
lark-cli mindnotes nodes list --help
lark-cli mindnotes nodes create --help

# 读取节点列表
lark-cli mindnotes nodes list --mindnote-id "<mindnote_token>"

# 创建子节点
lark-cli mindnotes nodes create \
  --mindnote-id "<mindnote_token>" \
  --data '{"client_token":"<client_token>","nodes":[{"parent_id":"node_parent123","texts":[{"element_type":"text","text":{"content":"子节点内容"}}],"highlight":"yellow","finish":false}]}'

# 更新已有节点
lark-cli mindnotes nodes create \
  --mindnote-id "<mindnote_token>" \
  --data '{"client_token":"<client_token>","nodes":[{"node_id":"node_existing123","texts":[{"element_type":"text","text":{"content":"更新后的节点内容"}}],"highlight":"blue","finish":true}]}'
```

## 参数

### `mindnotes nodes list`

| 参数 | 必填 | 说明 |
|------|------|------|
| `--mindnote-id` | 是 | 思维笔记 token / 唯一标识 |

返回重点：`data.nodes` 中常见字段有 `node_id`、`parent_id`、`texts`、`notes`、`images`、`finish`、`highlight`。

### `mindnotes nodes create`

命令参数：

| 参数 | 必填 | 说明 |
|------|------|------|
| `--mindnote-id` | 是 | 思维笔记 token / 唯一标识 |
| `--data` | 是 | JSON 请求体 |

请求体字段：

| 字段 | 必填 | 说明 |
|------|------|------|
| `client_token` | 否 | 幂等 token，建议写操作传入；推荐使用时间戳或 UUID |
| `nodes` | 是 | 待创建或更新的节点数组 |
| `nodes[].node_id` | 否 | 节点 ID；传入已有 `node_id` 时表示更新对应节点 |
| `nodes[].parent_id` | 否 | 父节点 ID；创建子节点时传入 |
| `nodes[].texts` | 否 | 节点正文富文本数组 |
| `nodes[].notes` | 否 | 节点备注富文本数组 |
| `nodes[].images` | 否 | 节点图片列表 |
| `nodes[].highlight` | 否 | `red` / `yellow` / `pink` / `blue` / `cyan` / `olive` / `grey` |
| `nodes[].finish` | 否 | 节点完成状态 |

富文本字段 `texts` / `notes` 是元素数组。最常见的是：

```json
[{"element_type":"text","text":{"content":"节点内容"}}]
```

### 节点图片（`nodes[].images`）

`nodes[].images` 接收的是**图片 token**，不是本地文件路径，也不是 URL。

```bash
# 先上传图片，拿到 token
lark-cli docs +media-upload --file ./image.png --parent-type mindnote_image --parent-node <mindnote_token>

# 再把 token 写进节点
lark-cli mindnotes nodes create \
  --mindnote-id "<mindnote_token>" \
  --data '{"client_token":"<client_token>","nodes":[{"node_id":"node_existing123","images":[{"token":"canonical_token"}]}]}'
```

参数说明：

| 参数 | 必填 | 说明 |
|------|------|------|
| `--file` | 是 | 本地图片路径 |
| `--parent-type` | 是 | 上传目标类型；图片使用 `mindnote_image` |
| `--parent-node` | 是 | 传 Mindnote 的 token |
| `nodes[].images[].token` | 是 | 上传后返回的图片 token |

## 推荐工作流

1. 先判断用户目标是不是“新建一个思维笔记”。
2. 如果是新建思维笔记，切到 [lark-doc-whiteboard](lark-doc-whiteboard.md)。
3. 如果是操作已有思维笔记，先按上方「获取 `mindnote_id`」确认已拿到 Mindnote 文档 token。
4. 确认目标类型是 **Mindnote** 后，把真实 Mindnote token 作为 `--mindnote-id`。
5. 先执行 `mindnotes nodes list`，确认目标 `parent_id`。
6. 新增子节点时，在 `nodes[]` 里传 `parent_id`；更新已有节点时，在 `nodes[]` 里传已有 `node_id`。
7. 再执行 `mindnotes nodes create`。
8. 写操作优先带 `client_token`，推荐使用时间戳或 UUID，避免重试时重复创建或重复更新。

> [!CAUTION]
> `mindnotes nodes create` 是写操作。创建时确认插入位置，更新时确认 `node_id` 指向的就是目标节点。

## 参考

- [lark-doc-fetch](lark-doc-fetch.md) — 获取文档内容
- [lark-doc-whiteboard](lark-doc-whiteboard.md) — 新建思维笔记走画板链路
- [lark-drive](../../lark-drive/SKILL.md) — 解析 Mindnote / Wiki 等云空间资源
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
