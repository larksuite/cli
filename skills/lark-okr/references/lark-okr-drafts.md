# OKR 草稿工作流

> **前置条件：** 先阅读 [`lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和高风险操作确认规则。

Objective、KR、对齐关系和量化指标都可能返回 `is_draft`。`true` 表示本次读取返回草稿态内容，`false` 表示返回发布态内容。草稿以整个 Objective 聚合为单位发布或撤销。

**`apply-to-draft=true` 会强制使用草稿，不要求事先已有草稿。** 编辑 O、KR、指标、对齐或 KR 权重时，若所属 Objective 尚无草稿，会先为整个 Objective 创建草稿再写入；新建 O 则直接创建未发布的草稿 O。`prefer-draft=true` 只选择读取视图，不会创建草稿。

## 操作决策

1. 写操作前，用 `+cycle-detail --prefer-draft=true` 获取当前内容，以所属 Objective 的 `is_draft` 判断有无草稿。只有当前身份可编辑对应的 OKR 时，`true` 才表示已有草稿，`false` 才可判断无草稿。
   - 以 user 身份访问时，仅有归属于当前登陆用户的 OKR 才可读取到草稿，他人的 OKR 总是返回发布态。
2. 用户明确要求“不介入”“无需确认”“不要二次确认”或直接完成发布态变更时，选择发布态流程：所有支持草稿的写操作都显式传 `--apply-to-draft=false`。
3. 其他场景选择草稿流程：用 `--apply-to-draft=true` 编辑草稿，展示草稿结果并停止，等待用户确认。
4. 只在用户看过当前草稿后明确要求发布时，才调用 `+publish-draft`。除非有明确指示（如“发布已有的草稿”），否则不应当在无用户确认的情况下发布草稿。
5. 在操作发布态流程中遇到已有草稿时，不得自动发布或撤销该草稿。继续用 `--apply-to-draft=false` 操作发布态，保留已有草稿，并告知用户该草稿以后发布时可能覆盖本次发布态修改。

```bash
# 草稿优先读取；有草稿时返回草稿内容
lark-cli okr +cycle-detail \
  --cycle-id 7000000000000000001 \
  --prefer-draft=true \
  --as user

# 编辑草稿
lark-cli okr +patch \
  --level objective \
  --target-id 7000000000000000002 \
  --content '{"text":"更新后的目标"}' \
  --apply-to-draft=true \
  --as user

# 用户看过当前草稿并明确确认发布后，发布整个 Objective 草稿
lark-cli okr +publish-draft \
  --objective-id 7000000000000000002 \
  --as user

# 用户明确要求不介入或无需确认时，直接修改发布态
lark-cli okr +patch \
  --level objective \
  --target-id 7000000000000000002 \
  --content '{"text":"直接更新发布态"}' \
  --apply-to-draft=false \
  --as user
```

## 发布与撤销

只有用户看过当前草稿并明确确认发布后，才发布整个 Objective 草稿：

```bash
lark-cli okr +publish-draft \
  --objective-id 7000000000000000002 \
  --as user
```

撤销整个 Objective 草稿是 `high-risk-write` 操作，不支持只撤销单个 KR。确认后显式传 `--yes`：

```bash
lark-cli okr +drop-draft \
  --objective-id 7000000000000000002 \
  --yes \
  --as user
```

## 参数适用范围

- `--prefer-draft`：读取选择。`+cycle-detail` 默认 `true`，但命令中应显式传值。
- `--apply-to-draft`：写入选择。`+create`、`+batch-create`、`+patch`、`+indicator-update` 和 KR `+weight` 默认 `true`，但命令中应显式传值。
- 对齐原生命令通过 JSON 传参：读取 `--params` 中的 `prefer_draft`，创建 `--data` 中的 `apply_to_draft`，删除 `--params` 中的 `apply_to_draft`，见 [对齐工作流](lark-okr-alignments.md)。
- `+reorder` 不暴露草稿参数：它固定按草稿优先视图读取，并对 KR 草稿排序。
- Objective 权重在草稿态和发布态间共用；`+weight --level objective` 不按 `--apply-to-draft` 分离存储。
- `+patch --score` 不支持草稿，必须使用 `--apply-to-draft=false`；草稿读取中不支持的字段可能不返回。
- 评论、文字进展、进展记录、进展图片和分类操作发布态，不使用草稿参数。

## 权限边界

草稿读取只对 OKR 所有者本人的用户身份（`--as user`）和具备相应应用数据权限的 bot 身份（`--as bot`）生效；他人的用户身份不能读取草稿内容。无法读取草稿时，不要据此断言目标不存在，也不要直接绕过草稿修改发布态。

## 参考

- [`okr +cycle-detail`](lark-okr-cycle-detail.md)
- [`okr +create`](lark-okr-create.md)
- [`okr +batch-create`](lark-okr-batch-create.md)
- [`okr +patch`](lark-okr-patch.md)
- [`okr +indicator-update`](lark-okr-indicator-update.md)
- [`okr +weight`](lark-okr-weight.md)
