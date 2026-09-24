# 表单提交、通知、提交后动作与抽奖配置

这组命令管理已有表单的运行配置。操作前先确定 `base_token`、`table_id` 和 `form_id`，每组配置都先执行对应的 Get，再只更新用户要求的部分。

## 命令路由

| 配置 | 读取 | 更新 |
|---|---|---|
| 提交时间、次数、修改权限和语音输入 | `+form-submission-settings-get` | `+form-submission-settings-update` |
| 提交后通知和定时通知 | `+form-notifications-get` | `+form-notifications-update` |
| 提交结果页和跳转 | `+form-submit-actions-get` | `+form-submit-actions-update` |
| 抽奖 | `+form-lottery-get` | `+form-lottery-action` |

## 提交设置

`+form-submission-settings-update` 每次只能修改一个顶层配置组：

- `submission_period`：开启时必须同时提供开始时间、结束时间和 IANA 时区。先 Get，并原样使用返回的 Base 时区；不能改成其他时区。已有开始时间即使已经过去，更新其他字段时也可以原样带回，最终由服务端判断。
- `user_submit_limit`：次数范围为 1～999，并指定 `total`、`day`、`week` 或 `month`。
- `total_submit_limit`：总次数范围为 1～100000。
- `allow_modify_submission`：允许或禁止修改已提交记录。
- `ai_voice_input`：允许或禁止语音输入。

关闭某个限制时只传该组的启停参数，不要继续携带已停用组的时间、次数或周期。

## 通知和提交后动作

- 通知一次只更新 `on-submission` 或 `scheduled`。启用时提供 1～100 个 `receiver-open-id`；定时通知还要提供未来的 RFC3339 时间、与 Base 一致的 IANA 时区，以及 `no_repeat`、`day`、`week`、`month`、`year`、`workday` 之一。
- 提交后动作一次只更新 `result-page` 或 `redirect`。结果页启用时标题必填且最多 12 个字符；跳转启用时只接受 HTTP 或 HTTPS URL。
- 主动写入结果页描述时只使用 `text` 和 `url`，不要构造人员 `mention`。读取结果页或只修改标题时应保留已有描述，包括原有的 `mention`。
- `revision` 用于并发控制；Get 返回该值时，Update 应带回当前非负值。

## 抽奖

开启抽奖前，用户必须阅读并明确同意[《飞书问卷抽奖管理规范》](https://www.feishu.cn/hc/zh-CN/articles/047445276639-%E9%A3%9E%E4%B9%A6%E9%97%AE%E5%8D%B7%E6%8A%BD%E5%A5%96%E7%AE%A1%E7%90%86%E8%A7%84%E8%8C%83)，确认后才传 `--agree-lottery-terms`。

- `enable` 和 `update` 必须提交完整 `--config-json`；`update` 还必须带 Get 返回的非负 `version`。
- `probability` 是 0～10000 的整数；`awards` 必须包含 1～7 个奖品。
- 每个奖品都必须提供唯一的 `position`，合法值只有 `0`、`1`、`2`、`3`、`5`、`6`、`7`；数组顺序不决定九宫格位置。
- `enable` 禁止携带 `award_id`。`update` 保留已有奖品时带回 Get 返回的 `award_id`；新增奖品时省略。
- `disable` 不接受 `--config-json`。
- `relink-winning-table` 的 `--config-json` 可省略；提供时不能包含 `awards`。
- `icon_token` 和 `winning_table` 是只读信息，不要写回。
