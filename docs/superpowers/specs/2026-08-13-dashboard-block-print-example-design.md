# Base 仪表盘组件模板输出设计

## 目标

为 `lark-cli base +dashboard-block-create` 增加纯本地的
`--print-example <type>` 模式，让 AI 在创建仪表盘组件前按目标类型获取一份最小、可编辑、可直接传给 `--data-config` 的 JSON 模板。

该能力只提供配置示例，不查询 Base、不访问网络，也不创建组件。

## 接口

```bash
lark-cli base +dashboard-block-create --print-example column
```

模板模式不要求 `--base-token`、`--dashboard-id`、`--name`、`--type` 或
`--data-config`。成功时 stdout 只输出目标类型的 `data_config` JSON，并以
状态码 0 结束。

未知类型返回 typed validation error，错误参数为 `--print-example`，并列出
全部可用的 canonical 类型。类型严格匹配，不自动纠错或转换别名。

第一版不支持 `all`，避免把所有模板加载进 AI 上下文。

## 实现边界

实现沿用 Sheets `+chart-create --print-example <type>` 的现有模式：

1. 在 Base shortcut 包内维护按 block type 索引的最小 JSON 模板。
2. 为 `+dashboard-block-create` 增加 `--print-example` flag。
3. 命中模板模式时，在认证、必填参数检查和 API 调用前打印模板并结束。
4. 正常创建路径保持不变。

本次不修改现有 `--type` 创建校验、不增加后端接口、不新增独立 Shortcut，
也不改变其他 Dashboard 命令。

## 模板范围

模板覆盖当前文档列出的组件类型：

```text
area, bar, column, combo, funnel, line, pie, radar, ring,
scatter, statistics, text, wordCloud
```

模板使用占位表名和字段名，保持最小合法结构。复杂业务说明（筛选规则、
漏斗累计口径等）继续由 `dashboard-block-data-config.md` 承担，模板输出不复制
长篇指导。

## 错误处理

若请求不存在的模板，例如 `--print-example colum`：

- 返回 `validation / invalid_argument`；
- `param` 为 `--print-example`；
- 消息包含输入值和排序后的可用类型；
- 不认证、不访问网络、不进入真实创建路径。

## 验证

新增邻近测试固定以下契约：

1. 合法类型无需定位参数即可输出模板并跳过 API。
2. 未知类型返回带 `--print-example` 参数的 typed validation error。
3. 每份模板都是合法 JSON。
4. 每份模板都通过现有 `normalizeDataConfig` 和
   `validateBlockDataConfig` 校验，防止模板与当前 CLI 创建契约漂移。
5. 正常 `+dashboard-block-create` 行为保持不变。

按仓库要求执行技能格式检查和相关质量门；不扩大到无关功能或历史校验问题。
