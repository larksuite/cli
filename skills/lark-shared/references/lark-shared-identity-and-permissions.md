# 身份与权限

基本心智模型——`--as` 代表谁操作、`--as bot` 碰用户资源可能静默返空——见 SKILL.md 正文准则。本文补充：身份怎么获得、授权分几层、权限不足时怎么恢复。

## 获取方式与授权层级

- **user 身份**（`--as user`）：用户通过 `lark-cli auth login` 授权获得。要能访问，需**两层都满足**——后台开通对应 scope + 用户 auth login 授权。
- **bot 身份**（`--as bot`）：自动，只需 appId + appSecret；只需后台开通 scope，无需 auth login。

输出里的 `[identity: bot/user]` 是当前身份。

## bot 碰用户资源的失败形态

因命令而异：有的静默返回空结果（如查日程落到 bot 自己的空日历），有的明确报"未登录 / 越权"。**无论哪种，都别把 bot 的结果当成用户的真实数据。**

## 权限 / scope 不足恢复

错误响应中的关键字段：

- 缺失的 scope：`permission_violations`（原始 API 错误块，元素形如 `{subject: "<scope>"}`）或 `missing_scopes`（CLI 结构化错误，已抽好的 scope 字符串数组）。
- `console_url`：飞书开发者后台的权限配置链接。
- `hint`：建议的修复命令。

按身份分流：

- **Bot 身份**：把 `console_url` 提供给用户（按正文准则配二维码转发），引导去后台开通 scope。**禁止**对 bot 执行 `auth login`，也不要因为 user 报错就降级到 bot 重试。
- **User 身份**：补授权用 `lark-cli auth login --scope "<missing_scope>"`（推荐，最小权限）或 `--domain <domain>`；必须指定其一，多次 login 的 scope 会累积（增量授权）。作为 agent 代发起时走 split-flow，见 [`lark-shared-auth-split-flow.md`](lark-shared-auth-split-flow.md)。
