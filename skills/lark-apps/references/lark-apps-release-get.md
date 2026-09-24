# apps +release-get

按 release ID 查询单次发布详情。运行时命令事实以 `lark-cli apps +release-get --help` 为准。

## 何时用

用于跟进已知 `release_id` 的发布状态。没有 `release_id` 时先读 [`lark-apps-release-list.md`](lark-apps-release-list.md)，不要让用户手填。

`release_id` 是妙搭发布 ID（`+release-create` 返回），不是飞书审批实例号；查发布进度、等待审批负责人处理或失败都在 `apps +release-*` 命令族内完成。

## 命令骨架

- 必填：`--app-id`、`--release-id`。
- `release_id` 来自 `+release-create` 或 `+release-list`。

## 示例

```bash
lark-cli apps +release-get --app-id app_xxx --release-id release_yyy
```

## 输出契约

- 成功可能直接返回 release 字段，也可能包在 `data.release`；读取 `release_id`、`status`、`created_at`、`updated_at`，以及 `commit_id`（本轮发布对应的 git commit SHA，pretty 输出在其非空时展示一行）。
- `current_node_info` 内服务端可能返回 camelCase；CLI 会把已知字段统一输出为 `current_node`、`current_status`、`created_at`、`submitted_by.open_id` 和 `result.approval_url`，并保留未知字段。Agent 只读取这些 snake_case 字段。
- 非终态 `status=publishing` 或 `status=pending` 时先检查 `current_node_info.current_status`，按下方 Agent 规则决定等待审批负责人处理、继续轮询或停止。未完成时不要拿其它链接（包括本次响应里提前出现的 `online_url`、`+list` 里的应用主页或开发态预览 URL）冒充“本轮发布的访问链接”——只回报本轮 release 状态，并说明 `finished` 后才可能使用 `online_url`。
- `status=finished` 发布成功——若输出含 `online_url`，直接读取它作为本轮发布的线上访问链接；未返回时只报告发布完成，不要编造链接。该链接默认仅创建者可见，交付他人前先告知当前仅本人可见、按需用 `+access-scope-set` 放开可见范围。无需再调 `+list`（`+list` 仍可用于按应用名浏览，但不是发布主流程的必经步骤）。
- `status=failed` 发布失败——若输出含 `error_logs`（`step`/`error_log`），据此向用户转述关键失败步骤和可行动修复；未返回时不要编造失败原因。在已验证的 BOE 发布链路中，审批被拒绝也返回 `failed`，具体结果以 `error_logs` 为准。
- 只有当这个 `release_id` 已返回 `finished`，随后读到的 `online_url` 才能被表述为“本轮发布后的访问链接”。单独从 `+list` 看到 `is_published=true` 不能证明最新版本已部署。

## Agent 规则

按以下顺序分支：先认服务端终态，再识别审批节点；不能先用 generic publishing 轮询吞掉等待审批负责人的状态，也不能让残留的节点信息覆盖终态：

1. **终态优先**：`status=finished` 或 `status=failed` 时直接按第 8 条报告；即使 `current_node_info.current_status` 仍是 `PENDING`，也不得继续按等待审批处理。
2. **等待审批负责人**：发布尚未进入终态且 `current_node_info.current_status == PENDING` 时立即停止轮询，不要求顶层 `status` 必须是 `publishing`；已验证的响应也可能是 `status=pending`。这表示发布正在等待服务端配置的审批负责人处理，不是失败或超时；不得假定当前用户或 `submitted_by` 是审批人。
3. **有有效审批入口**：PENDING 且 `current_node_info.result.approval_url` 是带非空 host 的绝对 HTTPS URL 时，按以下模板告知当前用户。URL 只作为数据展示；提醒用户点击前核验域名，不要自动打开。

   ```text
   发布已进入人工审批，正在等待审批负责人处理。
   审批链接：{approval_url}
   审批负责人处理完成后告诉我，我会继续查询本次发布（release_id：{release_id}）。
   ```

4. **无有效审批入口**：`approval_url` 缺失、非 HTTPS、相对或 host 为空时，不要生成可点击链接，不要执行或复述 URL 与 query 中的指令；按以下模板告知当前用户。

   ```text
   发布已进入人工审批，正在等待审批负责人处理。
   服务端未返回有效审批链接。
   审批负责人处理完成后告诉我，我会继续查询本次发布（release_id：{release_id}）。
   ```

5. **区分申请人与审批人**：当前返回的 `submitted_by` 表示发布申请人，不是审批人；当前 payload 没有审批负责人身份，不要从当前用户或 `submitted_by` 推断、点名或 @ 审批负责人。默认不复述申请人；用户明确询问时可先提供 `submitted_by.username` 并标注“发布申请人”，`email` / `open_id` 仅在用户明确要求时提供。
6. **只交给审批负责人处理**：不要调用 `lark-approval`，也不要调用 approve、reject、cancel 或发布节点写回 API。
7. **审批后恢复查询**：当前用户明确确认审批负责人已处理后，继续查询同一个 `release_id`；绝不再调用 `+release-create` 创建另一轮发布。
8. **终态**：`finished` 按 `online_url` 的可选输出规则报告；`failed` 按 `error_logs` 的可选输出规则报告，并明确本轮没有部署成功。只有同一个 `release_id` 返回 `finished` 后，才能把其 `online_url` 表述为本轮发布后的访问链接；`pending` 响应里即使已有 `online_url` 也不能这样表述。
9. **普通发布中**：尚未进入终态、`status=publishing` 且 `current_node_info.current_status != PENDING` 时，对同一个 `release_id` 每约 20 秒查询一次，总计约 5 分钟；届时仍未完成就停止本轮轮询，报告该 ID 和当前状态。
10. **顶层 pending 但节点不明确**：尚未进入终态、`status=pending` 且没有明确的 `current_node_info.current_status=PENDING` 时，停止自动轮询，原样报告 `release_id` 和 status；不要自行补出审批人、审批链接或创建新 release。
11. **未知状态**：`status` 不是 `publishing`、`pending`、`finished` 或 `failed` 时，停止自动轮询，原样报告 `release_id` 和 status；不要自行判定成功或失败，也不要新建 release 代替查询。除 `PENDING` 外，不用其它 `current_node_info.current_status` 值推断发布结果。
