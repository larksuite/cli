> **成员管理硬限制：**
> - 如果目标是“部门”，先判断身份，再决定是否继续。
> - `--as bot` 对应 `tenant_access_token`。官方限制：这种身份下不能使用部门 ID (`opendepartmentid`) 添加知识空间成员。
> - 遇到“部门 + --as bot”时，禁止先调用 `lark-cli wiki +member-add` 试错；直接说明该路径不可行。
> - 如果用户明确要求“以 bot 身份运行”，且目标是部门，必须停下说明 bot 路径无法完成，不要静默切到 `--as user`。

## 快速决策

- 用户要**整理 / 盘点 / 归类 / 重构知识库、个人文档库、文档库目录或 Wiki 节点结构**，或要生成整理方案、目标目录树、移动计划时，不要只使用 Wiki 节点 API。必须先阅读 [`../lark-drive/references/lark-drive-workflow-knowledge-organize.md`](../lark-drive/references/lark-drive-workflow-knowledge-organize.md)，该 workflow 负责 Drive / Wiki / 个人文档库的统一入口解析、资源盘点、分类计划、写前确认和结果验证。
- 用户要把**已有 Wiki 节点移出知识库，放到 Drive 文件夹或“我的空间”根目录**：使用 `wiki +move-to-drive`，不要使用 `wiki +move` 或 `drive +move`。执行前确认源节点与目标位置。
- 用户给的是知识库 URL（`.../wiki/<token>`），且后续要查成员/加成员/删成员：先调用 `lark-cli wiki spaces get_node --params '{"token":"<wiki_token>"}'` 获取 `space_id`，后续成员接口统一使用 `space_id`。
- 用户要**删除**知识空间（`wiki +delete-space`）但只给了名称或 URL：**不能**把名称 / URL 原样传给 `--space-id`，必须先解析出真实 `space_id`。解析方式：
  - URL（`.../wiki/<token>`）：`lark-cli wiki spaces get_node --params '{"token":"<wiki_token>"}' --format json`，读 `data.node.space_id`。
  - 只知名称：`lark-cli wiki spaces list --format json`，边翻页边收集 items 并按 `name` 精确匹配；**一旦任一页累计到至少 1 条精确匹配就停止翻页**。只有当翻完所有页（`has_more=false`）仍无精确匹配时，才对已收集的全量 items 做宽松匹配（`name` trim 空格、大小写不敏感、子串包含）。
  - **关键安全约束**：无论精确还是模糊，**无论命中 1 条还是多条，发起删除前都必须把候选（`name` + `space_id` + `description` + `space_type`）列给用户，由用户明确选定一个 `space_id` 再执行**。不要因为"只命中一条"就自动执行删除。
  - 命中 0 条：停下来问用户是名称拼错了还是调用方无权限；**不要**自行改名字重试。
  - 用户明确选定后再执行 `lark-cli wiki +delete-space --space-id <ID> --yes`（高风险写操作，必须显式 `--yes`）。
  - 反例：不要把 wiki URL / 名称直接当 `--space-id`（如 `--space-id "https://.../wiki/<wiki_token>"`）；务必先用 `wiki spaces get_node` 解析出 `data.node.space_id` 再传。
- 用户要在知识库中创建新节点，优先使用 `lark-cli wiki +node-create`。
- 用户说“给知识库添加成员/管理员”：先把目标解析成“用户 / 群 / 部门 / 应用”四类之一，再决定 `--member-type`，不要先调 `wiki +member-add` 再根据报错反推类型。
- 用户说“部门 + bot”：这是已知不支持路径。不要继续尝试 `wiki +member-add --as bot`；直接提示必须改成 `--as user`，或明确告知当前要求无法完成。
- 用户说“用户 / 群 / 应用 + 添加成员”：先解析对应 ID，再执行 `wiki +member-add`。
- 用户说“查看 / 列出空间成员”：用 `wiki +member-list`；该 shortcut 默认只取一页，多成员场景显式加 `--page-all`。
- 用户说“移除 / 删除空间成员”：用 `wiki +member-remove`，必须传齐原始授予时的 `--member-type` 和 `--member-role`（不知道就先 `wiki +member-list` 查一下）。

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
- 如果用户明确说的是 Drive 文件夹、云空间根目录、`我的空间`，再按源对象分流：源对象是 Wiki 节点时用 `wiki +move-to-drive`，源对象已在 Drive 时用 `drive +move`
