# 转交妙搭应用所有者

把一个妙搭应用的所有者转交给另一个人。该操作会改变应用资产归属，属于 `high-risk-write`。必须由当前所有者本人以 `--as user` 发起；建议先用 `--dry-run` 预览。真正执行时，只有用户明确确认目标应用、新所有者、原所有者权限、通知和资源位置选项后，才可带 `--yes` 运行；不要在未获用户明确同意时静默追加 `--yes`。

## 1. 取得应用对应的 Drive token 和 type

转交接口使用的不是 `app_id` / `app_type`，而是应用详情里的 `meta_token` / `meta_doc_type`。先以当前所有者的用户身份读取：

```bash
lark-cli apps +get --app-id <app_id> --as user \
  --jq '{app_id:.data.app.app_id,name:.data.app.name,meta_token:.data.app.meta_token,meta_doc_type:.data.app.meta_doc_type}'
```

核对返回的应用名称和 `app_id`，再原样记录 `meta_token` 与 `meta_doc_type`。`meta_doc_type` 可能是 `apps` 或 `miaoda_apps`；不要把它替换成 `app_type`，也不要把 `miaoda_apps` 猜改为 `apps`。任一字段缺失或为空时停止，不执行转交。

## 2. 明确新所有者

转交请求体用 `member_id` 和匹配的 `member_type` 指定新所有者。优先使用明确的用户 open_id：

```json
{"member_id":"ou_xxx","member_type":"openid"}
```

也可以使用准确的飞书邮箱：

```json
{"member_id":"new-owner@example.com","member_type":"email"}
```

如果只有姓名，先执行 `lark-cli contact +search-user --query '<姓名或邮箱>' --as user`，让用户在重名候选中确认后再取 `open_id`；不要按姓名猜 `member_id`。当前命令 schema 也接受 `userid`，但只有调用方已经提供并确认了该类型的精确用户 ID 时才可使用。本文不使用面向应用身份的 `appid`，因为目标是转交给另一个人。

## 3. 建议用 dry-run 预览完整请求

建议先用 `--dry-run` 预览完整请求。用 `--params` 原样传入 `+get` 返回的 token/type，避免把存量应用返回的 `meta_doc_type=miaoda_apps` 手工改写。当前 `transfer_owner` schema 的 typed `--type` 枚举只声明了 `apps`；`--params` 可以原样构造 `miaoda_apps` 请求，但 dry-run 只证明请求构造正确，不代表服务端保证支持该存量类型。以下示例保留原所有者的 `full_access` 权限、保持资源位置不变，并通知新所有者：

```bash
lark-cli drive permission.members transfer_owner \
  --params '{"token":"<meta_token>","type":"<meta_doc_type>","need_notification":true,"remove_old_owner":false,"old_owner_perm":"full_access","stay_put":true}' \
  --data '{"member_id":"<new_owner_open_id>","member_type":"openid"}' \
  --as user --dry-run --format json
```

本妙搭转交流程必须使用当前所有者的 `--as user` 身份发起：转交需要当前用户有所有者权限，且 `need_notification` 只有 user access token 调用时生效。底层命令虽然也暴露 bot 身份，但本流程不使用它绕过权限。

真正执行前，向用户展示并逐项核对：

- 目标应用的名称和 `app_id`，以及请求中的 token/type。
- 新所有者的可辨识身份、`member_id` 和 `member_type`。
- `remove_old_owner=false` 时，原所有者保留 `old_owner_perm` 指定的权限；上例明确保留 `full_access`。
- `remove_old_owner=true` 时，原所有者权限会被删除，`old_owner_perm` 不再生效。只有用户明确要求移除原所有者权限时才使用该值。
- `need_notification=true` 会通知新所有者；设为 `false` 则不通知。不要依赖服务端默认值，命令中显式写出用户确认的选择。
- `stay_put` 只在资源位于个人文件夹时生效：`true` 保持原位置，`false` 会把它移动到新所有者空间。不要依赖默认的 `false`，应用转交默认显式使用 `true`；只有用户明确要求移动资源时才改为 `false`。

## 4. 确认后执行并核验

用户明确确认上述影响且目标无误后，使用已经确认的 `--params` 和 `--data`，带 `--yes` 真正执行。不要在未获用户明确同意时静默追加 `--yes`：

```bash
lark-cli drive permission.members transfer_owner \
  --params '{"token":"<meta_token>","type":"<meta_doc_type>","need_notification":true,"remove_old_owner":false,"old_owner_perm":"full_access","stay_put":true}' \
  --data '{"member_id":"<new_owner_open_id>","member_type":"openid"}' \
  --as user --yes --format json
```

命令成功后 fresh read metadata，核对 `data.metas[0].owner_id` 等于新所有者的 open_id：

```bash
lark-cli drive metas batch_query \
  --data '{"request_docs":[{"doc_token":"<meta_token>","doc_type":"<meta_doc_type>"}],"with_url":true}' \
  --as user --format json
```

当 `remove_old_owner=false` 时，原所有者仍可执行该读取；当原所有者已被移除或读取失败时，由新所有者使用自己的 user profile 执行。若需要核验原所有者保留的权限，再由仍有权限的一方读取直接成员列表，确认原所有者 `member_id` 对应的 `perm`：

```bash
lark-cli drive +member-list --token '<meta_token>' --type '<meta_doc_type>' --as user --format json
```

若当前身份无法读取 metadata 或成员列表，把对应核验结论标为 partial，不要用 `apps +get` 可访问性代替 owner 或权限级别证据。

## 失败处理

- `+get` 失败或 `meta_token` / `meta_doc_type` 缺失：停止，不用 `app_id`、页面 token 或 `app_type` 猜测替代。
- 缺少 `docs:permission.member:transfer` 等 scope：按错误里的 `error.hint` 完成对应用户授权后，仅重试原命令；不要切换 bot 身份。
- 返回无权限或不是所有者：确认当前 `--as user` 登录人确为应用所有者。不是所有者时由当前所有者发起，不能用协作者命令、应用角色或可见范围设置绕过。
- `meta_doc_type=miaoda_apps` 或其他服务端业务错误：当前 schema 不承诺支持该存量 type；保留 `+get` 返回的原始值并转述 `error.hint`，不要改写为 `apps` 反复尝试。需要时改由当前所有者在妙搭网页端完成转交。
- 网络超时或结果未知：先用上述 `drive metas batch_query` fresh read `owner_id`；当前身份读不到时，由新所有者使用自己的 user profile 读取。仍无法确认则报告结果未知，不要直接重复提交，以免造成二次转交。
