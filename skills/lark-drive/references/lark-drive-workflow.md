# lark-drive Workflow 总框架

本文是 `lark-drive` workflow 总框架的运行协议和注册表。它面向 AI Agent 执行，只负责路由已纳入本总框架的 workflow。

`Workflow Registry` 是本总框架的唯一注册来源。未命中 registry 的请求必须按“未注册 workflow 处理”执行，不要按已有 workflow 类推扩展。

## 必读上下文

执行本总框架内的 workflow 前，必须先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

下游 reference 只能按需逐步加载。不要因为命中本总框架，就预加载所有 workflow 文件或相关 skill。

## 能力边界

`lark-drive` workflow 总框架以 `lark-drive` 作为 Drive / Docs / Wiki 资产编排的总入口。其他领域 skill 只有在已纳入本总框架的 workflow 明确需要时，才作为辅助能力加载。

| Layer | Owns | Must Not Own |
|-------|------|--------------|
| `lark-drive/SKILL.md` | 用户意图到具体 workflow entry 的短路由 | 长流程逻辑、未注册场景 |
| `lark-drive-workflow.md` | 共享运行协议、Artifact Contract、Workflow Registry、加载规则 | 非运行时背景说明、宽泛路线图、场景专项执行细节 |
| Registered workflow file | 场景范围、状态机、Command Map、确认门槛、验证规则 | 其他场景、隐藏写入、未被 CLI/API 支持的能力声明 |

## 执行协议

每个已纳入本总框架的 workflow 必须遵循同一条执行骨架：

```text
route -> scope -> read -> assess/plan -> confirm -> execute -> verify -> done
```

运行规则：

1. 在读取或写入资产前，先把用户意图解析到唯一一个已纳入本总框架的 workflow。
2. 在昂贵读取或写入规划前，先解析并确认 `target_scope`。
3. 事实必须来自可执行 CLI 命令或被引用 skill；不要只凭目录结构推断治理结论。
4. 无法执行的检查必须记录到 `unsupported_checks`，不能静默省略。
5. 写入前必须产出计划。每一次写入都需要用户对准确范围和 command family 显式确认。
6. CLI/API 支持验证时，写入后必须用 fresh read 验证。
7. 结束时进入 `done`，返回已完成事项、验证结果和剩余限制。不要把尚未完成的外部审批描述成已完成。

## Artifact Contract

每个已纳入本总框架的 workflow 必须维护以下内部字段：

| Field | Meaning |
|-------|---------|
| `workflow_id` | 本总框架注册的 workflow 名称，例如 `permission_governance` |
| `current_state` | 当前 workflow 状态 |
| `target_scope` | 已确认的目标范围和用户原始输入 |
| `identity` | 当前身份和执行视角，通常为 `user` |
| `facts` | 从 CLI 读取或引用 skill 获取的证据 |
| `plan_items` | 候选动作；每项包含 command family、target、risk、verification method |
| `unsupported_checks` | 因 CLI/API 覆盖、目标类型、认证或范围限制而无法执行的检查 |
| `partial` | 结果是否不完整，以及不完整原因 |
| `execution_results` | 已确认写入的执行结果 |
| `verification_results` | fresh read 验证结果，或明确的异步审批限制 |

用户可见输出默认使用简洁 chat summary。只有在用户要求、结果过大不适合聊天展示，或当前 workflow 明确要求共享产物时，才创建本地文件或飞书文档。

## Workflow Entry Contract

每个已纳入本总框架的 workflow entry file 必须让 Agent 能直接判断和执行：

- 何时进入该 workflow，以及哪些需求不属于该 workflow；
- 如何映射到共享执行骨架的 state machine；
- 当前 state 需要按需加载哪些 reference；
- 哪些 command family 可用，以及读写风险边界；
- 写入前如何确认，写入后如何验证；
- 最终回复必须包含哪些字段，或使用哪些 output templates。

每个纳入本总框架的 workflow 默认从一个独立 reference 文件开始。只有当写入、回滚或验证流程复杂到影响可读性时，才继续拆 phase 文件。

## Risk / Structure Gate

每个纳入本总框架的 workflow 都必须同时声明 `Risk Level` 和 `Structure Level`。风险等级决定安全门槛；结构等级决定文件拆分。高风险写入不等于必须拆 phase。

Risk Level：

| Level | Meaning | Runtime Requirement |
|-------|---------|---------------------|
| `R0` | read-only：只读发现、分析、报告 | 记录事实来源、`unsupported_checks` 和 `partial` 原因 |
| `R1` | low-risk write：创建草稿、生成临时产物等低风险写入 | 写前说明范围，写后返回结果链接或标识 |
| `R2` | high-risk write：权限变更、批量移动、标签修改等高风险写入 | 写前计划、准确 diff、用户显式确认、fresh read 验证 |
| `R3` | destructive / recovery-sensitive write：删除、自动归档、双向同步、rollback cleanup | 恢复边界、执行日志、分批策略、失败停止条件和单独确认 |

Structure Level：

| Level | File Shape | When To Use |
|-------|------------|-------------|
| `S1` | compact entry only | 只读、轻量审计、简单计划，无复杂写入 |
| `S2` | entry + optional `commands` / `outputs` / `artifacts` references | 有命令样例、输出模板、少量高风险写入，但状态链可集中表达 |
| `S3` | entry + phase files + optional shared references | 多阶段写入、复杂验证、恢复 / rollback、长任务或分批执行 |

升级规则：

1. 新 workflow 默认从 `S1` 开始。
2. Entry file 超过约 300 行时，优先拆 `commands`、`outputs` 或 `artifacts` reference。
3. 只有执行、验证、恢复或 rollback 状态链复杂到影响可读性时，才升级到 `S3` phase files。
4. 垂直业务包优先作为已有 workflow 的 recipe / policy / template，不默认新增独立 workflow。
5. 已有样板：`permission_governance` 是 `R2/S2`；已发布的独立 `knowledge_organize` 是 `R2-R3/S3`，当前不作为本总框架 registry entry。

## 加载与拆分边界

- 每个纳入本总框架的场景默认只保留一个紧凑 workflow entry file。
- 不为未注册或未来场景创建占位 reference / registry entry。
- 只有 workflow 已经具备可执行规则时，才允许作为本总框架 workflow 出现在 `SKILL.md` 并加入 `Workflow Registry`。
- 多文件 phase 拆分只用于执行、回滚或验证流程复杂到影响可读性的 `S3` 场景。

## Workflow Registry

| Workflow | Status | Risk | Structure | Entry File | Trigger |
|----------|--------|------|-----------|------------|---------|
| `permission_governance` | Registered | `R2` | `S2` | [`lark-drive-workflow-permission-governance.md`](lark-drive-workflow-permission-governance.md) | 权限审计、公开链接/外部访问、复制/下载/评论/分享设置、权限申请、owner 转移 / 批量 owner 转移、密级标签调整 |

## Workflow Loading

当用户意图匹配到本总框架已注册 workflow 时：

1. 先读取本总框架文件。
2. 只读取 `Workflow Registry` 中命中的 entry file。
3. 按该 workflow 的 progressive load map 继续加载额外 reference。
4. 除非用户改变意图，或当前 workflow 明确路由到其他 workflow，否则不要读取其他 workflow 文件。

## 未注册 workflow 处理

`Workflow Registry` 是本总框架的唯一注册来源。用户请求未列入 registry 的 workflow 或组合型治理场景时：

1. 明确说明该需求暂无纳入本总框架的 `lark-drive` workflow。
2. 只在不新增本总框架 workflow 行为的前提下，将请求收窄为现有 skill / CLI 可执行的原子操作。
3. 不要类比本总框架任何已注册 workflow 新增 state machine、artifact shape、风险分类、写入行为或验证结论。
