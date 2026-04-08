# wiki +resolve-node

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

把一个飞书知识库节点（wiki node）解析为它真正指向的对象：`obj_token` + `obj_type`（docx / bitable / sheet / file / ...）+ `title`。

## 为什么需要这个命令

飞书的 wiki 链接 `https://x.larkoffice.com/wiki/<TOKEN>` 看起来像一个文档 URL，但它实际上只是一个**包装节点**。真正的内容存在节点指向的 `obj_token` 里，且类型可能是任何东西：

- 看着像 wiki，背后其实是 docx → 用 `docs +fetch`
- 看着像 wiki，背后其实是 bitable → 用 `base +table-list` / `+record-list`
- 看着像 wiki，背后其实是 sheet → 用 `sheets +read`

如果直接拿 wiki 链接里的 token 当 doc/base/sheet token 用，会得到 `param invalid` 错误（如 `code 800004006`）。**必须先解析。**

## 命令

```bash
# 直接传 wiki URL（最常用）
lark-cli wiki +resolve-node --token "https://bytedance.larkoffice.com/wiki/EzY8wvj5RiLtfIkw4UPcTdKinRe"

# 传 bare token 也行
lark-cli wiki +resolve-node --token "EzY8wvj5RiLtfIkw4UPcTdKinRe"

# 输出 pretty 表格
lark-cli wiki +resolve-node --token "..." --format pretty

# 用 jq 直接提取最常用的字段
lark-cli wiki +resolve-node --token "..." --format json --jq '.data | {obj_token, obj_type, title}'

# 预览不执行
lark-cli wiki +resolve-node --token "..." --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--token` | 是 | wiki 节点 URL（自动从 `/wiki/XXX` 中提取 token）或 bare token |
| `--format` | 否 | 输出格式：json（默认）/ pretty / table / ndjson / csv |
| `--as` | 否 | 身份：user / bot（默认 user） |
| `--dry-run` | 否 | 只打印请求，不执行 |

## 输出

返回一个扁平结构（不嵌套在 `node` 里，方便直接 jq）：

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "node_token":  "EzY8wvj5RiLtfIkw4UPcTdKinRe",
    "obj_token":   "Q6Peb7nsqatsQIsIaYScetYNnPd",
    "obj_type":    "bitable",
    "title":       "华东区效率先锋大赛案例汇总",
    "space_id":    "7280057719443996675",
    "node_type":   "origin",
    "creator":     "ou_xxx",
    "has_child":   false
  }
}
```

字段说明：

| 字段 | 用途 |
|---|---|
| `obj_token` | 真正的内容 token，必须用它去调下一步的 doc/base/sheet API |
| `obj_type` | 决定下一步用哪个 skill：`docx` / `doc` / `bitable` / `sheet` / `file` / `slides` / `mindnote` |
| `node_token` | 原始 wiki 节点 token（即输入 URL 里的 token） |
| `title` | 文档标题，用于人类可读输出和确认是否找对了文档 |
| `space_id` | 所属知识空间，调 `wiki nodes list` 浏览同空间其他节点时需要 |

## 标准下游流程

```bash
# 第一步：解析
RESULT=$(lark-cli wiki +resolve-node --token "https://x.larkoffice.com/wiki/wikXXX" --format json)
OBJ_TOKEN=$(echo "$RESULT" | jq -r '.data.obj_token')
OBJ_TYPE=$(echo "$RESULT" | jq -r '.data.obj_type')

# 第二步：根据 obj_type 走对应 skill
case "$OBJ_TYPE" in
  docx|doc)
    lark-cli docs +fetch --doc "$OBJ_TOKEN"
    ;;
  bitable)
    lark-cli base +table-list --base-token "$OBJ_TOKEN"
    ;;
  sheet)
    lark-cli sheets +read --sheet "$OBJ_TOKEN"
    ;;
  *)
    echo "obj_type=$OBJ_TYPE — see lark-drive or lark-whiteboard skill"
    ;;
esac
```

LLM agent 不需要写脚本，直接按顺序调两次 lark-cli，把第一次的 `obj_token` / `obj_type` 喂给第二次的命令即可。

## 历史问题

在 `+resolve-node` 这个 shortcut 出现之前，agent 常见的两种错误：

1. **直接拿 wiki token 当 doc/base/sheet token 用** → `param invalid` (code 800004006)
2. **绕道写一长串 raw API 调用**：
   ```bash
   lark-cli api GET /open-apis/wiki/v2/spaces/get_node \
     --params '{"token":"...","obj_type":"wiki"}'
   ```
   能用，但是对 LLM 友好度差（要知道 OpenAPI 路径、要构造嵌套 JSON params、返回结果也是嵌套在 `data.node` 下）。

新 shortcut 把这两个坑都填了：URL 自动解析、扁平输出、一行命令。

## 权限

| 操作 | 所需 scope |
|---|---|
| `+resolve-node` | `wiki:wiki:readonly`（或更上级的 `wiki:node:read`） |

## 决策规则

- **任何来源是 wiki URL 的"读内容"任务，第一步都是 `+resolve-node`。**
- 如果用户给的是 doc/sheet/bitable 的直接 URL（不带 `/wiki/`），则**不需要**先 resolve，直接走对应 skill。
- 如果 `+resolve-node` 返回 `obj_type` 是你不熟悉的类型，先去对应 skill 看一下处理方式，不要瞎猜。
- 解析失败（404 或 permission denied）通常意味着 wiki 节点不存在、被删除、或当前身份没有访问权限 —— 不是 token 格式问题。
