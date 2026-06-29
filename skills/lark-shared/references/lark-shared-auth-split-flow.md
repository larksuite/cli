# Agent 代理发起认证（split-flow）

当你作为 AI agent 需要帮用户完成 user 身份认证时，优先使用 split-flow，避免在同一轮对话中阻塞等待用户授权。

```bash
# 发起授权（立即返回 device_code 和 verification_url）
lark-cli auth login --scope "calendar:calendar:readonly" --no-wait --json
```

拿到 `verification_url` 后，将它原样作为本轮最终消息发给用户，并结束本轮 / 交还控制权。**不要**在同一轮中展示 URL 后立刻执行 `--device-code` 阻塞轮询；在不透传中间输出的 agent harness 里，这会导致用户永远看不到 URL。

## 第一步：发起授权（当前轮）

1. 执行 `lark-cli auth login --scope "xxx" --no-wait --json`（必须加 `--no-wait --json`）。
2. 从 JSON 输出提取 `verification_url` 和 `device_code`。
3. 生成二维码：`lark-cli auth qrcode <verification_url> --output "xxx"`。
4. 将 URL 和二维码展示给用户（先 URL，后二维码）。
5. **结束本轮前明确告知用户**："请完成授权后，回来告诉我已授权完成，我会帮你完成后续步骤"。

## 第二步：完成授权（后续轮）

1. 等待用户回复"已完成授权"。
2. **由你（AI agent）亲自执行**：`lark-cli auth login --device-code <device_code>`。
3. 此命令会轮询授权状态并完成登录；返回成功即结束。

## 关键规则

- **你必须亲自执行 `--device-code` 命令**，不要指示用户自行执行。
- **不要在同一轮中展示 URL 后立刻执行 `--device-code`**，这会导致用户看不到 URL。
- **禁止缓存 `verification_url` / `device_code`**：每次需要授权时都重新执行 `lark-cli auth login --no-wait --json` 生成新链接，不要将授权链接和 device code 存入上下文供后续复用。

## 授权范围

- auth login 必须指定范围（`--domain <domain>` 或 `--scope "<scope>"`）；推荐 `--scope`，符合最小权限原则。
- 多次 login 的 scope 会累积（增量授权）。
- 可用 `--exclude "<scope>"` 排除特定 scope、`--recommend` 只请求推荐（可自动批准）的 scope。
