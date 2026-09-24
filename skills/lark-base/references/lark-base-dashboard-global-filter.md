# 仪表盘全局筛选

全局筛选配置保存在 Dashboard 容器上。先确定真实的 `base_token` 和
`dashboard_id`；仪表盘 ID 从 `+dashboard-list` 获取，不使用内部图表组件的
`block_id`。单个组件的 `data_config.filter` 和 Table View 的筛选有各自的
配置入口，不能用修改它们代替设置仪表盘的 `global_filters`。

## 入口与前置条件

目标环境必须已部署全局筛选 HTTP 路由，并允许当前身份调用。
优先使用目标 CLI 中已发布的类型化命令，以 `lark-cli base --help` 和实际
命令的 `--help` 确认入口；服务元数据尚未收录时，使用本页的 `lark-cli api`。
不要猜测新的 `+dashboard-*` 命令，也不要向 `+dashboard-update` 添加未声明的参数。

GET 和 PATCH 共用 `/global_filter` 路径，JSON 字段名为复数
`global_filters`。请求体直接提交 JSON 对象，不包装成 `Payload` 字符串，
也不提交网关注入的用户、租户或应用信息。

## 读取配置

```bash
lark-cli api GET \
  '/open-apis/base/v3/bases/<base_token>/dashboards/<dashboard_id>/global_filter' \
  --as user
```

GET 不需要请求体或专用 query 参数。成功时 CLI 的 `data` 包含：

```json
{
  "dashboard_id": "<dashboard_id>",
  "global_filters": []
}
```

只有请求成功、返回的 `dashboard_id` 与目标一致，且 `global_filters`
明确为数组，才能据此判断当前配置。`[]` 表示没有全局筛选；读取失败、缺字段
或返回 `null` 都不能当作空配置。

若 GET 提示 `Global filters cannot be represented completely with the current schema and access permissions`，
表示当前筛选无法完整导出。此时无法安全备份原配置，不要通过 PATCH 或清空来
试探修复；先检查字段访问权限或在仪表盘中调整配置。联调写入可在同一 Base
使用临时仪表盘，验证后清理测试资源。

## 设置配置

将本次期望保存的 `global_filters` 数组放进 JSON 对象，一次提交。
局部修改前先读取当前配置，保留其他需要保留的条目；不要依赖 PATCH 按表名
自动合并数组，也不要自行将 View 的 tuple 筛选转换成此接口的条件。

下面是数值筛选的请求示例；表名、字段名和字段类型必须与目标 Base 一致。
数组元素的详细业务校验由服务端处理。

```json
{
  "global_filters": [
    {
      "table_name": "订单",
      "filter": {
        "conjunction": "and",
        "conditions": [
          {"field_name": "金额", "operator": "isGreater", "value": 1000}
        ]
      }
    }
  ]
}
```

将请求对象保存为 `filters.json` 后执行：

```bash
lark-cli api PATCH \
  '/open-apis/base/v3/bases/<base_token>/dashboards/<dashboard_id>/global_filter' \
  --data @filters.json \
  --as user
```

文件内容是上面的请求对象，不是完整的 CLI `ok/data` 输出。
也可使用 `--data '<JSON 对象>'` 或 `--data -` 从 stdin 读取。

服务端要求顶层只包含 `global_filters`，且其值为非 null 数组：空 body、
`{}`、`null`、非数组及额外的 `name` / `theme` 等顶层字段都会被拒绝。
通用 `api` 的 dry-run 只验证请求构造，不能代替服务端的业务校验。

## 清空配置

明确要清空全局筛选时，提交空数组：

```bash
lark-cli api PATCH \
  '/open-apis/base/v3/bases/<base_token>/dashboards/<dashboard_id>/global_filter' \
  --data '{"global_filters":[]}' \
  --as user
```

不要用 `null`、省略字段或省略请求体表达清空。

## 结果与验收

GET 和 PATCH 都通过 CLI 标准 `ok/data` 结构返回 `dashboard_id` 和
`global_filters`。PATCH 返回服务端处理后的配置，筛选值可能经过归一化，
例如数字 `1000` 返回为 `["1000"]`；展示或继续编辑时以返回配置为准，
不要把提交值回显为服务端结果。

需要验收持久化状态时，再用 GET 核对目标 ID 和筛选配置；验证清空时要求
明确返回 `[]`。用户要求图表按筛选变化时，再对目标数据组件执行
`+dashboard-block-get-data`，验证预期命中与排除的数据，不能仅凭写成功
宣称图表结果已符合要求。

调用失败时按 CLI 返回的业务错误、提示和 log ID 处理。权限失败按
`lark-shared` 修复当前身份的 scope 或资源权限；路径未部署与资源不存在
需要分别核对。失败响应不能作为“已设置”或“已清空”的证据。
