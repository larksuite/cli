# XML 扩展块补充说明

本文件用于补充说明 block XML 扩展能力。常用标签和通用规则见 [`lark-doc-xml.md`](lark-doc-xml.md)；后续新增其他 block 说明时可继续追加到本文件。

## OKR block

OKR block 可用 XML 格式完整表达。创建前先参考 [`lark-okr`](../../lark-okr/SKILL.md) 确认可用周期；创建时只写 root-only `<okr cycle-id="..."/>` 挂载已有 OKR，不构造 Objective/KR/Progress 子树。

获取时，XML 结构示例如下：

```xml
<okr cycle-id="" cycle-name="CYCLE_NAME" user-name="USER_NAME">
  <okr-objective objective-id="OBJECTIVE_ID" status="normal" percent="80" score="75">
    <p>O 描述</p>
    <okr-progress>
      <p>O 进展</p>
      <checkbox done="true">事项</checkbox>
      <ul><li>列表项内可包含 <a href="https://example.com">链接</a></li></ul>
    </okr-progress>
    <okr-key-result key-result-id="KEY_RESULT_ID" status="risk" percent="60" score="80">
      <p>KR 描述</p>
      <okr-progress>
        <p>KR 进展</p>
      </okr-progress>
    </okr-key-result>
  </okr-objective>
</okr>
```

- `cycle-id` 仅用于创建时挂载已有当前周期 OKR；`cycle-name`、`user-name` 只读。
- `objective-id`、`key-result-id` 为只读业务 ID，更新已有 OKR 时保持不变。
- `okr-objective` / `okr-key-result`
  - 可更新 `status`、`percent`、`score`；`percent` / `score` 取值 0-100，`status` 取值 `unset`/`normal`/`risk`/`extended`。
  - 不可更新 objective 和 key-result 内容描述。
- `okr-progress` 承载进展内容，支持更新。直接子节点支持 `<p>`、`<checkbox>`、`<grid>`、`<img>`、`<source>`、`<ol>`、`<ul>`、`<h1>` 到 `<h9>`。
