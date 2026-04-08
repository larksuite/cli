---
name: lark-wiki
version: 1.0.0
description: "飞书知识库：管理知识空间和文档节点。创建和查询知识空间、管理节点层级结构、在知识库中组织文档和快捷方式。当用户需要在知识库中查找或创建文档、浏览知识空间结构、移动或复制节点时使用。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli wiki --help"
---

# wiki (v2)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

## 核心概念：Wiki 节点是壳，不是内容

`https://x.larkoffice.com/wiki/<TOKEN>` 形式的链接是一个**包装节点**，它本身没有内容。真正的内容（doc / bitable / sheet / file）藏在节点指向的 `obj_token` 里，并由 `obj_type` 决定该用哪一类 API 去读取。

**这是飞书企业知识场景里出错率最高的一个坑** —— 直接拿 wiki token 当 doc/base/sheet token 用，必然 `param invalid`。任何"读 wiki 链接内容"的任务，第一步都必须是 wiki node 解析。

### 标准解析流程：用 `+resolve-node`

```bash
lark-cli wiki +resolve-node --token "https://x.larkoffice.com/wiki/EzY8wvj5RiLtfIkw4UPcTdKinRe"
# 或者只传 token
lark-cli wiki +resolve-node --token "EzY8wvj5RiLtfIkw4UPcTdKinRe"
```

返回干净的扁平结构 `{node_token, obj_token, obj_type, title, space_id}`。把 `obj_token` + `obj_type` 喂给下一步：

| `obj_type` | 下一步命令 |
|---|---|
| `docx` / `doc` | `lark-cli docs +fetch --doc <obj_token>` |
| `bitable` | `lark-cli base +table-list --base-token <obj_token>` |
| `sheet` | `lark-cli sheets +read --sheet <obj_token>` |
| `slides` / `file` / `mindnote` | `lark-cli drive ...` |

详见 [`references/lark-wiki-resolve-node.md`](references/lark-wiki-resolve-node.md)。

**反模式**：不要再写 `lark-cli api GET /open-apis/wiki/v2/spaces/get_node --params '{"token":"...","obj_type":"wiki"}'` 这种绕道写法 —— 那只是 `+resolve-node` 出现之前的临时方案。

## Shortcuts（推荐优先使用）

| Shortcut | 说明 |
|---|---|
| [`+resolve-node`](references/lark-wiki-resolve-node.md) | 把 wiki 节点 URL/token 解析为底层 obj_token + obj_type + title。任何要读取 wiki 链接内容的任务都必须先用它。 |

## API Resources

```bash
lark-cli schema wiki.<resource>.<method>   # 调用 API 前必须先查看参数结构
lark-cli wiki <resource> <method> [flags] # 调用 API
```

> **重要**：使用原生 API 时，必须先运行 `schema` 查看 `--data` / `--params` 参数结构，不要猜测字段格式。

### spaces

- `get` — 获取知识空间信息
- `get_node` — 获取知识空间节点信息
- `list` — 获取知识空间列表

### nodes

- `copy` — 创建知识空间节点副本
- `create` — 创建知识空间节点
- `list` — 获取知识空间子节点列表

## 权限表

| 方法 | 所需 scope |
|------|-----------|
| `spaces.get` | `wiki:space:read` |
| `spaces.get_node` | `wiki:node:read` |
| `spaces.list` | `wiki:space:retrieve` |
| `nodes.copy` | `wiki:node:copy` |
| `nodes.create` | `wiki:node:create` |
| `nodes.list` | `wiki:node:retrieve` |
