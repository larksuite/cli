# Agent 代理发起授权（split-flow）

帮用户完成 user 身份授权。背景：如果运行环境只把最终消息发给用户、不显示中间命令输出，阻塞式 `auth login` 会让用户永远看不到授权链接，所以把"发起"和"完成"拆到两轮。

## 第一步：发起（当前轮）

1. 执行 `lark-cli auth login --scope "<scope>" --no-wait --json`，从输出提取 `verification_url` 和 `device_code`。
2. 把 `verification_url` 按正文准则配二维码展示给用户（生成二维码、URL 在前、原样不改写）。
3. 明确告知用户"完成授权后回来告诉我"，然后交还控制权。**不要**在同一轮接着执行 `--device-code` 阻塞轮询——否则用户看不到链接。

## 第二步：完成（后续轮）

等用户回复已授权，**由你（agent）亲自执行** `lark-cli auth login --device-code <device_code>`（别让用户自己跑）。该命令轮询授权状态并完成登录，成功即结束。

## 规则

- **禁止缓存 `verification_url` / `device_code`**：每次授权都重新 `--no-wait` 发起拿新值，不要存旧值复用。
- **范围必须显式指定**：`--scope`（推荐，最小权限）或 `--domain`；多次 login 的 scope 累积（增量授权）。`--exclude` 排除特定 scope，`--recommend` 只请求可自动批准的 scope。
