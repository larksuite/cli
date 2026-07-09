# Wiki token routing

Wiki URL 中的 `/wiki/<token>` 是节点 token，不一定是底层文档、表格、Base、文件或幻灯片的对象 token。需要对底层对象做读取、评论、导出、下载、表内操作等动作时，先解包，再按底层类型路由。

## 推荐方式

优先使用 `lark-drive` 的 `drive +inspect`：

```bash
lark-cli drive +inspect --url 'https://xxx.feishu.cn/wiki/<wiki_token>'
```

输出中的 `type` 是底层对象类型，`token` 是后续命令应使用的 canonical token。`wiki_node` 字段保留节点侧信息，如 `space_id`、`node_token`、`obj_token`、`obj_type`。

## 手动方式

如果不能使用 shortcut，再调用 Wiki 节点接口：

```bash
lark-cli wiki spaces get_node --params '{"token":"<wiki_token>"}'
```

从返回值中读取：

| 字段 | 含义 |
|------|------|
| `node.obj_type` | 底层对象类型，如 `docx`、`doc`、`sheet`、`bitable`、`slides`、`file`、`mindnote` |
| `node.obj_token` | 底层对象 token，用于对应业务 skill 或原生 API |
| `node.node_token` / `token` | Wiki 节点 token，用于 Wiki 节点层级操作 |
| `node.space_id` | 所属知识空间 |

## 路由

| `obj_type` | 后续操作 |
|------------|----------|
| `docx` / `doc` | 文档内容走 `lark-doc`；评论、权限、导出等云空间能力走 `lark-drive` |
| `sheet` | 表内数据走 `lark-sheets`；评论、权限、导出等云空间能力走 `lark-drive` |
| `bitable` | 表内数据走 `lark-base`；评论、权限、导出等云空间能力走 `lark-drive` |
| `slides` | 幻灯片内容编辑走 `lark-slides`；评论、权限、导出等云空间能力走 `lark-drive` |
| `file` | 普通文件上传、下载、评论、权限等走 `lark-drive` |
| `mindnote` | 思维笔记的移动、删除、快捷方式、权限、安全标签等云空间能力走 `lark-drive`；知识库节点层级操作走 `lark-wiki` |
| wiki 节点层级 / 空间成员 | 走 `lark-wiki`，不要把底层对象 token 当节点 token |
