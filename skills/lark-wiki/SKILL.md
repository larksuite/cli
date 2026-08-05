---
name: lark-wiki
version: 1.0.3
description: "飞书知识库：管理知识空间、空间成员和文档节点。创建和查询知识空间、查看和管理空间成员、管理节点层级结构、在知识库中组织文档和快捷方式。当用户需要在知识库中查找或创建文档、浏览知识空间结构、查看或管理空间成员、移动或复制节点时使用。当用户给出 doubao.com 的 /wiki/ URL/token 时，也应直接使用本 skill，不要因为域名不是飞书而回退到 WebFetch；路由依据是 URL 路径模式和 token，而不是域名。不负责：上传文件到知识库节点下（走 lark-drive）、编辑文档/表格/Base 内容（走 lark-doc / lark-sheets / lark-base）。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli wiki --help"
---

# wiki (v2)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

> **成员管理硬限制：**
> - 如果目标是“部门”，先判断身份，再决定是否继续。
> - `--as bot` 对应 `tenant_access_token`。官方限制：这种身份下不能使用部门 ID (`opendepartmentid`) 添加知识空间成员。
> - 遇到“部门 + --as bot”时，禁止先调用 `lark-cli wiki +member-add` 试错；直接说明该路径不可行。
> - 如果用户明确要求“以 bot 身份运行”，且目标是部门，必须停下说明 bot 路径无法完成，不要静默切到 `--as user`。

## 身份选择：优先使用 user 身份

知识空间和节点都是用户的个人资源，**策略上应优先显式使用 `--as user`**（CLI 的 `--as` 默认值为 `auto`，不带 `--as` 时常被解析成 `bot`，列出的是应用所属空间而非用户的）。仅当用户明确要求“应用 / bot 视角”时才用 `--as bot`（仍受上面的成员管理硬限制约束）。

## 快速决策

- 用户要**按特定主题 / 关键词 / 内容线索查找资料并收集到知识库节点或新建知识库节点下**，必须先阅读 [`../lark-drive/references/lark-drive-workflow.md`](../lark-drive/references/lark-drive-workflow.md)，再按其中 `Workflow Registry` 进入 [`topic_move_collector`](../lark-drive/references/lark-drive-workflow-topic-move-collector.md) workflow。该 workflow 使用 Drive 全量搜索召回，再按 Wiki 目标解析、确认和移动；不要只用 Wiki 节点列表做局部遍历。
- 用户要**整理 / 盘点 / 归类 / 重构知识库、个人文档库、文档库目录或 Wiki 节点结构**，或要生成整理方案、目标目录树、移动计划时，不要只使用 Wiki 节点 API。必须先阅读 [`../lark-drive/references/lark-drive-workflow.md`](../lark-drive/references/lark-drive-workflow.md)，再按其中 `Workflow Registry` 进入 [`knowledge_organize`](../lark-drive/references/lark-drive-workflow-knowledge-organize.md) workflow；该 workflow 负责 Drive / Wiki / 个人文档库的统一入口解析、资源盘点、分类计划、写前确认和结果验证。
- 用户要把**已有 Wiki 节点移出知识库，放到 Drive 文件夹或“我的空间”根目录**：使用 `wiki +move-to-drive`，不要使用 `wiki +move` 或 `drive +move`。这是会改变节点归属和权限继承的写操作，执行前确认源节点与目标位置。
- 用户给的是知识库 URL（`.../wiki/<token>`），且后续要查成员/加成员/删成员：先调用 `lark-cli wiki spaces get_node --params '{"token":"<wiki_token>"}'` 获取 `space_id`，后续成员接口统一使用 `space_id`。
- 用户要**删除**知识空间（`wiki +delete-space`）但只给了名称或 URL：**不能**把名称 / URL 原样传给 `--space-id`，必须先解析出真实 `space_id`。解析方式：
  - URL（`.../wiki/<token>`）：`lark-cli wiki spaces get_node --params '{"token":"<wiki_token>"}' --format json`，读 `data.node.space_id`。
  - 只知名称：`lark-cli wiki spaces list --format json`，边翻页边收集 items 并按 `name` 精确匹配；**一旦任一页累计到至少 1 条精确匹配就停止翻页**。只有当翻完所有页（`has_more=false`）仍无精确匹配时，才对已收集的全量 items 做宽松匹配（`name` trim 空格、大小写不敏感、子串包含）。
  - **关键安全约束**：无论精确还是模糊，**无论命中 1 条还是多条，发起删除前都必须把候选（`name` + `space_id` + `description` + `space_type`）列给用户，由用户明确选定一个 `space_id` 再执行**。不要因为"只命中一条"就自动执行删除。
  - 命中 0 条：停下来问用户是名称拼错了还是调用方无权限；**不要**自行改名字重试。
  - 用户明确选定后再执行 `lark-cli wiki +delete-space --space-id <ID> --yes`（高风险写操作，必须显式 `--yes`）。
  - 反例：不要把 wiki URL / 名称直接当 `--space-id`（如 `--space-id "https://.../wiki/<wiki_token>"`）；务必先用 `wiki spaces get_node` 解析出 `data.node.space_id` 再传。
- 用户要在知识库中创建新节点，优先使用 `lark-cli wiki +node-create`。
- 用户要列出 Wiki 节点：先用 `wiki +space-list --as user` 拿数字 `space_id`，再用 `wiki +node-list --space-id <space_id>`。不要把 wiki URL、node token、doc token、名称直接当 `--space-id`。钻子节点时 `--parent-node-token` 必须是 wiki node token；如果用户给的是 docx/sheet/base URL，先用 `wiki +node-get --node-token <url>` 解析出 `node_token`。
- `wiki +node-list` 命中 `invalid_parameters`、`not_found`、`permission_denied` 时，不要重复调用同一参数；按 hint 修 `space_id` / `parent_node_token` / 权限。只有 `rate_limit` 才做退避重试。
- 用户说“给知识库添加成员/管理员”：先把目标解析成“用户 / 群 / 部门 / 应用”四类之一，再决定 `--member-type`，不要先调 `wiki +member-add` 再根据报错反推类型。
- 用户说“部门 + bot”：这是已知不支持路径。不要继续尝试 `wiki +member-add --as bot`；直接提示必须改成 `--as user`，或明确告知当前要求无法完成。
- 用户说“用户 / 群 / 应用 + 添加成员”：先解析对应 ID，再执行 `wiki +member-add`。
- 用户说“查看 / 列出空间成员”：用 `wiki +member-list`；该 shortcut 默认只取一页，多成员场景显式加 `--page-all`。
- 用户说“移除 / 删除空间成员”：用 `wiki +member-remove`，必须传齐原始授予时的 `--member-type` 和 `--member-role`（不知道就先 `wiki +member-list` 查一下）。

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli wiki +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut | 说明 |
|----------|------|
| [`+move`](references/lark-wiki-move.md) | Move a wiki node, or move a Drive document into Wiki |
| [`+move-to-drive`](references/lark-wiki-move-to-drive.md) | Move a wiki node to a Drive folder and poll the async task |
| [`+node-create`](references/lark-wiki-node-create.md) | Create a wiki node with automatic space resolution |
| [`+delete-space`](references/lark-wiki-delete-space.md) | Delete a wiki space, polling the async delete task when needed |
| [`+space-list`](references/lark-wiki-space-list.md) | List all wiki spaces accessible to the caller |
| [`+space-create`](references/lark-wiki-space-create.md) | Create a wiki space (user identity only) |
| [`+node-list`](references/lark-wiki-node-list.md) | List wiki nodes in a space or under a parent node (supports pagination) |
| [`+node-copy`](references/lark-wiki-node-copy.md) | Copy a wiki node to a target space or parent node |
| [`+node-get`](references/lark-wiki-node-get.md) | Get a wiki node's details by node_token / obj_token / Lark URL |
| [`+node-delete`](references/lark-wiki-node-delete.md) | Delete a wiki node, polling the async delete task when needed |
| [`+member-add`](references/lark-wiki-member-add.md) | Add a member to a wiki space |
| [`+member-remove`](references/lark-wiki-member-remove.md) | Remove a member from a wiki space |
| [`+member-list`](references/lark-wiki-member-list.md) | List members of a wiki space (supports pagination) |

## 成员添加流程

- 调用 `lark-cli wiki +member-add` 前，先把自然语言里的“人 / 群 / 部门 / 应用”解析成正确的 `--member-id`，不要猜格式。
- 用户场景默认优先 `--member-type=openid`：用 `lark-cli contact +search-user --query "<姓名/邮箱/手机号>" --format json` 获取 `open_id`。
- 群组场景使用 `--member-type=openchat`：用 `lark-cli im +chat-search --query "<群名关键词>" --format json` 获取 `chat_id`。
- 应用场景使用 `--member-type=appid`：`--member-id` 传应用 ID，格式通常为 `cli_xxx`。
- `userid` / `unionid` 只在下游明确要求时才使用；先拿到 `open_id`，再调用 `lark-cli api GET /open-apis/contact/v3/users/<open_id> --params '{"user_id_type":"open_id"}' --format json` 读取 `user_id` / `union_id`。
- 部门场景使用 `--member-type=opendepartmentid`：当前 CLI 没有 shortcut，需调用 `lark-cli api POST /open-apis/contact/v3/departments/search --as user --params '{"department_id_type":"open_department_id"}' --data '{"query":"<部门名>"}'` 获取 `open_department_id`。
- 只有在目标类型和身份都已确认可行后，才调用 `lark-cli wiki +member-add`。对于部门场景，这意味着必须是 `--as user`。

## 目标语义约束

- `我的文档库` / `My Document Library` / `我的知识库` / `个人知识库` / `my_library` 都应视为 **Wiki personal library**，不是 Drive 根目录
- 处理这类目标时，先解析 `my_library` 对应的真实 `space_id`，再执行 `wiki +move`、`wiki +node-create` 或其他 Wiki 写操作
- 不要因为缺少显式 `space_id` 就退化成 `drive +move`
- 如果用户明确说的是 Drive 文件夹、云空间（云盘/云存储）根目录、`我的空间`，再按源对象分流：源对象是 Wiki 节点时用 `wiki +move-to-drive`，源对象已在 Drive 时用 `drive +move`

## API Resources

```bash
lark-cli schema wiki.<resource>.<method>   # 调用原生 API 前必须先查看 --data / --params 参数结构，不要猜测字段格式
lark-cli wiki <resource> <method> [flags]  # 调用 API
```

### spaces

- `create` — 创建知识空间
- `get` — 获取知识空间信息
- `get_node` — 获取知识空间节点信息
- `list` — 获取知识空间列表

### members

- `create` — 添加知识空间成员
- `delete` — 删除知识空间成员
- `list` — 获取知识空间成员列表

### nodes

- `copy` — 创建知识空间节点副本
- `create` — 创建知识空间节点
- `list` — 获取知识空间子节点列表

## 不在本 skill 范围

- 上传 / 下载文件到知识库节点下 → [`lark-drive`](../lark-drive/SKILL.md)（`drive +upload --wiki-token`）
- 编辑文档正文内容 → [`lark-doc`](../lark-doc/SKILL.md)
- 表格 / 多维表格数据操作 → [`lark-sheets`](../lark-sheets/SKILL.md) / [`lark-base`](../lark-base/SKILL.md)
- 按名称搜索文档 / Wiki / 表格文件、评论与权限管理 → [`lark-drive`](../lark-drive/SKILL.md)
