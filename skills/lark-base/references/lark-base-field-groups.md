# 字段编组（Field group）

同一 Table 的各 View 共享编组归属。聚合展示时尽力遵循 `visible_fields`
顺序；接口返回的数组顺序不代表展示顺序，编组接口本身不提供排序操作。

## 读取

```bash
lark-cli base +field-group-list \
  --base-token <base_token> --table-id <table_id> --as user
```

响应 `data`：

```jsonc
{
  "groups": [ // 全部现有编组，无分页；无编组时为 []，未编组字段不在此列表中
    {
      "type": "group",
      "id": "fg_example",
      "name": "联系信息",
      "description": "**联系信息**", // 无描述时省略
      "children": [ // 读取可包含已有嵌套 group 节点
        {"type": "field", "id": "fld_email"},
        {"type": "field", "id": "fld_phone"}
      ]
    }
  ]
}
```

未编组字段 = `+field-list` 的全部字段 − 编组中的成员。

## 写入：六种操作

```bash
lark-cli base +field-group-manage \
  --base-token <base_token> --table-id <table_id> --json @operations.json --as user
```

```jsonc
// 按任务选取、组合操作；提交 --json 时使用去掉注释的标准 JSON。
{
  "operations": [ // 1–100 项，顺序执行；校验失败拒绝整批提交
    {
      "operation": "add",
      "name": "联系方式", // 组名在表内唯一；省略则自动命名，组 ID 由服务端生成
      "description": "**联系信息**：[说明](https://example.com)", // 可选 Markdown
      "children": [ // 非空、字段引用唯一；当前写入只支持 field 节点
        {"type": "field", "id": "邮箱"}, // 字段 ID 或精确名称，优先匹配 ID
        {"type": "field", "name": "电话"} // name 别名仅在缺少 id 时生效
      ]
    },
    {
      "operation": "rename",
      "id": "联系方式", // 编组 ID 或精确名称；同批可引用刚创建的命名组
      "name": "客户联系" // 改名后，后项使用新名称或原 ID；名称区分大小写、保留空格
    },
    {
      "operation": "set_description",
      "id": "客户联系",
      "description": "" // 替换 Markdown 描述；空字符串清空
    },
    {
      "operation": "add_children",
      "id": "客户联系",
      "children": [{"type": "field", "id": "备用邮箱"}]
      // 一个字段最多属于一个组；add / add_children 按最新加入操作迁移归属。
      // 已有成员保持原状；来源组清空后自动删除。
    },
    {
      "operation": "remove_children",
      "id": "客户联系",
      "children": [{"type": "field", "id": "电话"}] // 移为未编组字段；非成员保持原状
    },
    {
      "operation": "delete",
      "id": "客户联系" // 解除编组，字段和记录数据保留
    }
  ]
}
```

返回 `data.results` 与请求同序，每项保留操作参数并返回真实编组 `id`；
`children` 保留规范化后的输入引用，名称引用仍可能是名称。

## 系统限制

- 写入仅支持顶层单层编组；加入子编组或直接编辑嵌套子组返回
  `FIELD_GROUP_NESTING_NOT_SUPPORTED`。请求严格校验各操作的属性。
- 每项操作后立即清理空组，后项引用的组需仍然存在。两组交换唯一成员并
  保留原 ID 时，第一步会清空来源组，这种重组当前不支持。
- 实际生成的服务端操作也限 100 项，迁移可能产生额外操作；超限时按业务
  边界拆批，每批独立提交。编组数和成员数另受目标环境动态配额约束。
