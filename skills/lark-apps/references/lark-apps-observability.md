# apps observability

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)（认证 / 全局参数 / 安全）。

查询妙搭应用的线上运行观测和产品访问分析。所有 observability 命令只支持 `--environment online`；省略 `--environment` 时默认就是 online，传 dev 或其他环境是不支持的。不要使用旧的 `--env`，也不要使用短选项。

日志和 trace 的用户侧环境仍然是 online；但 OpenAPI 请求体里的后端 `app_env` 固定发送 `runtime`，因为线上应用的运行时日志和 trace 存储在 runtime 观测环境下。dry-run 输出会展示这个后端参数。

metric / analytics 的 `--environment` 只是 CLI 侧 online-only 校验：`+metric-list` 和 `+analytics-list` 不会向 OpenAPI body 发送 `env` 或 `app_env`。dry-run 里看不到环境字段是预期行为，不要补造参数。

时间过滤支持相对时间（如 `30s`、`5m`、`0.5h`、`2h`、`3d`、`1w`）、本地日期 / 时间和 RFC3339。

## 命令选择

- 日志检索：用 `+log-list` 搜索日志，用 `+log-get` 按 log ID 取单条日志。
- `+log-list` 不再支持 `--log-id`；已有 log ID 时直接用 `+log-get --log-id <log_id>`。
- 前端 ERROR 日志详情：`+log-get` 可能补充 `source_stack`；没有独立的 source-stack 命令。
- Trace 检索：用 `+trace-list` 搜索 trace，用 `+trace-get` 按 trace ID 取详情。
- 运行时指标：请求数、错误、延迟、CPU、memory 用 `+metric-list`。
- 产品分析：PV、UV、访问量这类业务访问分析用 `+analytics-list`，不要放到 runtime metric 里混查。
- `+analytics-list` 按最新 OpenAPI 发送 `metric_types`、纳秒时间戳和 `need_pack_lack_point=false`；`group_by` 暂不支持。
- 用户询问“最近一小时接口请求量、错误量、延迟、接口慢/报错多”时，这是平台运行时监控，不是本地项目文件。先用 `apps +list --keyword` 找 `app_id`，再查 `+metric-list`。

## 示例

```bash
lark-cli apps +log-list --app-id <app_id> --level error --keyword timeout --since 0.5h
lark-cli apps +log-get --app-id <app_id> --log-id <log_id>
lark-cli apps +trace-list --app-id <app_id> --trace-id <trace_id>
lark-cli apps +trace-get --app-id <app_id> --trace-id <trace_id>
lark-cli apps +metric-list --app-id <app_id> --metric requests --series total --since 1d
lark-cli apps +metric-list --app-id <app_id> --metric requests --since 1h
lark-cli apps +metric-list --app-id <app_id> --metric latency --since 1h
lark-cli apps +metric-list --app-id <app_id> --metric latency --series p99 --since 1d
lark-cli apps +metric-list --app-id <app_id> --metric cpu --since 1h
lark-cli apps +metric-list --app-id <app_id> --metric memory --since 1h
lark-cli apps +analytics-list --app-id <app_id> --analytics users --series active-users --granularity day
lark-cli apps +analytics-list --app-id <app_id> --analytics page-view --granularity day
```

## 使用边界

- 如果用户问“接口慢、报错多、CPU/内存高”，优先走 `+metric-list`。
- `+metric-list --metric requests` 不传 `--series` 会同时返回请求总量 total 和错误量 error；`--metric latency` 不传 `--series` 会同时返回 p50 和 p99。只想看单条曲线时再传 `--series total|error|p50|p99`。
- 按接口收窄范围时使用 `--api <path-or-name>`；当前没有 `group-by` 参数，不要臆造。
- `+metric-list` 未显式传 `--down-sample` 时会按时间范围自动选择粒度：短范围用 `1m`，中等范围用 `1h`，长范围用 `1d`；显式传入时尊重用户指定。
- 如果用户问“页面访问量、PV、UV、活跃用户”，优先走 `+analytics-list`。
- 如果用户已有 `trace_id` 或 `log_id`，直接用对应 get 命令；不知道 ID 时先 list。
