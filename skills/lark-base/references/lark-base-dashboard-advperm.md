# 仪表盘跟随访问者权限统计开关

仪表盘跟随访问者权限统计开关使用 `is_adv_perm_enable`，控制该仪表盘是否
跟随访问者权限统计。操作前先确认目标
`base_token` 和 `dashboard_id`，仪表盘 ID 从 `+dashboard-list` 获取，使用
Base 内的仪表盘 block ID。

## 选择操作对象

| 用户目标 | 操作对象 |
|---|---|
| 开启或关闭整个 Base 的高级权限 | `+advperm-enable` / `+advperm-disable`，状态从 `+base-get` 的 `data.base.is_advanced` 读取 |
| 查询或设置仪表盘跟随访问者权限统计开关 | 本页的 `is_adv_perm_enable` 接口 |
| 修改某个角色对仪表盘的阅读权限 | `+role-update` 的 `dashboard_rule_map` |
| 修改仪表盘的分享设置 | `+dashboard-share-get` / `+dashboard-share-update` |

## 接口可用性

这是新接口，目标环境必须已部署对应 HTTP 路由并授予调用身份所需权限。
优先使用目标 CLI 中已发布的类型化命令，先从 `lark-cli base --help` 和
`lark-cli schema` 确认实际命令名、scope 和支持身份。

当前服务元数据尚未收录时，可使用下方已知路径通过 `lark-cli api` 联调。
不要猜测不存在的 `+dashboard-advperm-*` 命令，也不要把 dry-run 成功当作
接口已经部署或权限已经生效的证明。

## 读取开关

```bash
lark-cli api GET \
  '/open-apis/base/v3/bases/<base_token>/dashboards/<dashboard_id>/adv_perm' \
  --as user
```

成功时 CLI 返回 `ok: true`，开关位于 `data.is_adv_perm_enable`。`false`
是明确的关闭状态。失败、缺少该字段或字段类型错误均不能当作 `false`。

## 设置开关

确认用户希望修改的目标仪表盘及开关状态后提交一次 PATCH。开启时使用
JSON 布尔值 `true`，关闭时显式使用 `false`；请求字段必填，不能省略，也
不能使用字符串 `"false"` 或 `null`。

```bash
# 开启仪表盘跟随访问者权限统计开关。
lark-cli api PATCH \
  '/open-apis/base/v3/bases/<base_token>/dashboards/<dashboard_id>/adv_perm' \
  --data '{"is_adv_perm_enable":true}' \
  --as user

# 关闭仪表盘跟随访问者权限统计开关，false 必须保留在 body 中。
lark-cli api PATCH \
  '/open-apis/base/v3/bases/<base_token>/dashboards/<dashboard_id>/adv_perm' \
  --data '{"is_adv_perm_enable":false}' \
  --as user
```

需要检查请求构造时可追加 `--dry-run`。它不会调用业务接口。

设置成功的业务数据为空对象 `{}`，CLI 返回 `ok: true, data: {}` 即表示
请求成功；接口没有返回当前开关值。仅当用户要求核验或结果不确定时，再
执行一次 GET，不能把提交值冒充为回读结果。

## 失败处理

- 所属 Base 高级权限未开启：通过 `+base-get` 确认前置条件；只有用户已经
  授权修改 Base 总开关时，才执行 `+advperm-enable`。
- 当前身份无权操作：按 `lark-shared` 检查 scope 和资源权限，保留当前身份。
- 租户权益不足：说明服务端返回的权益限制，不反复重试开关。
- 资源不存在：重新核对所属 Base 和仪表盘 block ID，不使用仪表盘内部组件 ID。

CLI 失败输出中的业务错误码及提示用于判断下一步。GET 失败不代表开关
关闭，PATCH 失败也不代表状态已发生改变。
