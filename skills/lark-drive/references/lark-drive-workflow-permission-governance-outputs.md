# 权限治理输出模板

本文只提供 `permission_governance` workflow 的用户可见输出模板。默认先给简短摘要；只有用户要求完整表格、需要写入确认，或结果大到需要结构化展示时才读取本文。

## 目录

- `输出策略`
- `Semantic Rendering`
- `定位与治理动作`
- `单目标公开性判断`
- `多目标明确列表诊断`
- `审计摘要`
- `容器安全诊断报告摘要`
- `可操作风险清单`
- `治理选择交互`
- `权限设置清单`
- `访问复核清单`
- `整改 dry-run`
- `批量权限申请确认`
- `owner 转移确认`
- `确认请求`
- `最终摘要`

## 输出策略

- 单目标默认输出审计摘要。
- 多目标明确列表默认输出逐目标诊断摘要；不要因为目标数大于 1 就套用容器递归发现报告。
- 用户可见结论默认跟随用户当前语言。用户用中文提问时输出中文，用户用英文提问时输出英文；混合语言时跟随主要语言。
- 单目标公开性判断默认输出业务表达，不直接展示 `link_share_entity`、`external_access_entity`、`external_access` 等底层字段名；只有用户要求 raw evidence、排障，或完整清单 / artifact 场景才展示底层字段。
- 中文用户可见输出中，`permission_public` / `public permission` 默认译为“文档公共访问和协作权限设置”；可在摘要里简称“公共访问与协作设置”。它在官方语义中包含链接分享、对外分享、协作者管理、复制内容、创建副本、打印、下载和评论；具体可判断字段以当前 CLI schema 和实际响应为准。只有命令名、schema 字段、raw evidence、排障信息和完整 artifact 字段名保留英文原文。
- 容器目标默认输出安全诊断报告摘要：一句话结论、覆盖情况、风险分级、优先处理对象、建议下一步和剩余限制。
- 容器目标不要把风险按数量机械排序；外部公开、允许对外分享、缺失密级标签优先于复制 / 下载 / 评论这类依赖策略的候选项。
- 用户没有提供明确 policy 时，使用“候选风险 / 待复核 / 待策略确认”，不要写“违规 / 已泄露 / 已外部访问”。
- 容器安全诊断里不要把 `external_access=true` / `external_access_entity=open` 简写成“高风险”或“外部泄露”；用户可见说法应为“允许对外分享，需 owner 复核；这不等于已经存在外部协作者”。
- 风险对象展示按规模渐进披露：1-10 个全部展示；11-30 个展示全部高优先级待复核对象，中 / 低优先级只做分组摘要；31-100 个按高优先级待复核分组展示 Top 5 和数量；100+ 个只展示分组统计和 Top 样例。
- 当摘要未展示全部风险对象时，必须明确“完整清单包含 <count> 条”，并提供生成 Markdown / CSV / 飞书文档风险清单或整改 dry-run 的下一步。
- 只要发现需要处理的对象，最终回复必须给出可执行下一步 CTA。不能因为默认只读，就只报告风险后结束。
- 完整风险清单是后续治理选择的输入；Markdown / CSV / 飞书文档报告必须使用同一套字段和稳定 `risk_id`。
- 写入前必须使用确认模板；权限申请、文档公共访问和协作权限设置修改、owner 转移、密级标签更新分别确认。
- 最终回复必须包含已完成事项、验证结果和剩余限制；异步权限申请审批不能表述为已完成授权。

## Semantic Rendering

面向用户的主结论优先渲染 `per_target_permission_assessment` 中的语义状态，并使用用户当前语言；底层字段名只在 raw evidence、排障或完整清单中保留。下表给出字段值到业务表达的标准映射；其他语言应表达同等业务含义。

字段来源边界：下表同时覆盖官方 OpenAPI 语义和当前 / 未来 CLI schema。只有实际响应或当前 schema 返回的字段和值，才可渲染为确定状态；当前 installed CLI 未返回的字段（例如 `copy_entity`、`manage_collaborator_entity`、`external_access_entity`）或未出现的枚举值，只能在 raw response / schema 实际出现时使用，缺失时必须按 unknown / unsupported 处理，不要臆造。

| Raw field / value | Semantic State | 中文说法 | English phrasing |
|-------------------|----------------|----------|------------------|
| `link_share_entity=anyone_readable` | `link_access=public_readable` | 互联网上获得链接的任何人可阅读 | Anyone on the internet with the link can read |
| `link_share_entity=anyone_editable` | `link_access=public_editable` | 互联网上获得链接的任何人可编辑 | Anyone on the internet with the link can edit |
| `link_share_entity=partner_tenant_readable` | `link_access=partner_readable` | 关联组织内知道链接可读 | People in partner tenants with the link can read |
| `link_share_entity=partner_tenant_editable` | `link_access=partner_editable` | 关联组织内知道链接可编辑 | People in partner tenants with the link can edit |
| `link_share_entity=tenant_readable` | `link_access=tenant_readable` | 公司内知道链接可读 | People in the tenant with the link can read |
| `link_share_entity=tenant_editable` | `link_access=tenant_editable` | 公司内知道链接可编辑 | People in the tenant with the link can edit |
| link sharing empty / disabled | `link_access=closed` | 未开启链接分享 | Link sharing is disabled |
| `external_access_entity=open` or `external_access=true` | `external_sharing=open` | 允许分享到组织外；不等于已经存在外部协作者 | External sharing is open; this does not mean external collaborators already exist |
| `external_access_entity=allow_share_partner_tenant` | `external_sharing=partner_only` | 仅允许分享到关联组织 | Sharing is allowed only with partner tenants |
| `external_access_entity=closed` or `external_access=false` | `external_sharing=closed` | 当前不允许分享到组织外 | External sharing is disabled |
| `invite_external=true` | `external_invitation=enabled` | 当前允许邀请外部用户 | Inviting external users is enabled |
| `invite_external=false` | `external_invitation=disabled` | 当前不允许邀请外部用户 | Inviting external users is disabled |
| `share_entity=anyone` | `collaborator_org_scope=all_viewers_or_editors` | 所有可阅读或可编辑者可查看、添加、移除协作者 | All viewers or editors can view, add, and remove collaborators |
| `share_entity=same_tenant` | `collaborator_org_scope=tenant_viewers_or_editors` | 组织内可阅读或可编辑者可查看、添加、移除协作者 | Tenant viewers or editors can view, add, and remove collaborators |
| `manage_collaborator_entity=collaborator_can_view` | `collaborator_permission_scope=viewer` | 拥有可阅读权限的协作者可查看、添加、移除协作者 | Collaborators with view permission can view, add, and remove collaborators |
| `manage_collaborator_entity=collaborator_can_edit` | `collaborator_permission_scope=editor` | 拥有可编辑权限的协作者可查看、添加、移除协作者 | Collaborators with edit permission can view, add, and remove collaborators |
| `manage_collaborator_entity=collaborator_full_access` | `collaborator_permission_scope=full_access` | 拥有可管理权限的协作者可查看、添加、移除协作者 | Collaborators with full-access permission can view, add, and remove collaborators |
| `copy_entity=anyone_can_view` | `copy_scope=viewer` | 拥有可阅读权限的用户可复制内容 | Users with view permission can copy content |
| `copy_entity=anyone_can_edit` | `copy_scope=editor` | 拥有可编辑权限的用户可复制内容 | Users with edit permission can copy content |
| `copy_entity=only_full_access` | `copy_scope=full_access` | 仅拥有可管理权限的协作者可复制内容 | Only collaborators with full-access permission can copy content |
| `security_entity=anyone_can_view` | `security_scope=viewer` | 拥有可阅读权限的用户可创建副本、打印、下载 | Users with view permission can create copies, print, and download |
| `security_entity=anyone_can_edit` | `security_scope=editor` | 拥有可编辑权限的用户可创建副本、打印、下载 | Users with edit permission can create copies, print, and download |
| `security_entity=only_full_access` | `security_scope=full_access` | 仅拥有可管理权限的用户可创建副本、打印、下载 | Only users with full-access permission can create copies, print, and download |
| `comment_entity=anyone_can_view` | `comment_scope=viewer` | 拥有可阅读权限的用户可评论 | Users with view permission can comment |
| `comment_entity=anyone_can_edit` | `comment_scope=editor` | 拥有可编辑权限的用户可评论 | Users with edit permission can comment |
| `lock_switch=true` | `lock_state=locked_not_inheriting` | 已限制权限，不再继承父级页面权限 | The node is locked and no longer inherits parent-page permissions |
| `lock_switch=false` | `lock_state=not_locked_or_inheriting` | 未限制权限，可能继承父级页面权限 | The node is not locked and may inherit parent-page permissions |
| field absent / unsupported | `<state>=unknown` | 当前 schema 未返回，无法判断 | The current schema did not return this field, so it is unknown |
| `check_scope=current_public_permission_only` | `check_scope=current_public_permission_only` | 本次判断的是当前文档公共访问和协作权限设置，不是协作者名单或历史权限变更审计 | This check covers current public access and collaboration settings, not collaborator-list or historical permission-change auditing |
| `sec_label_name` missing | `sec_label=missing` | 缺少密级标签 | Security label is missing |

## 定位与治理动作

风险对象必须能让用户直接定位和处理：

- 摘要中的每个优先处理对象必须包含 `risk_id`、`path/title`、`URL`、`type`、owner、sec_label、风险原因、关键证据和建议动作。
- 完整清单、访问复核清单、整改 dry-run 和写入确认都必须包含 URL。缺少 URL 时，展示 token / node_token，并说明 URL 未能获取。
- 同名文档、shortcut 或副本必须用 path + URL 区分；不要只输出 title。
- 完整风险清单中的每条记录必须有稳定 `risk_id`，格式为 `PG-001`、`PG-002`。`risk_id` 在同一次诊断和后续 dry-run / 确认 / 验证中保持不变。
- 即使摘要只展示 Top 样例，也必须给样例分配稳定 `risk_id`；不能输出无法选择的标题列表。
- 建议动作必须和风险类型绑定：互联网公开链接优先建议关闭链接分享或收紧为组织内；允许对外分享优先建议 owner 复核或关闭对外分享；缺少密级标签优先建议补齐密级；复制 / 下载 / 评论范围只在用户 policy 明确时建议收紧。
- 写入动作只能作为下一步选项或确认请求出现。不要在诊断摘要里暗示已经执行缩权。

## 单目标公开性判断

当 `intent=public_exposure_check` 且 `target_scope=single_resource` 时，使用此模板。默认渲染 `target_count=1` 的 `per_target_permission_assessment`，跟随用户当前语言，不直接展示底层字段名；用户要求 raw evidence 时，再追加字段证据。

中文模板：

```text
结论：<不是对外公开 / 存在互联网公开链接 / 允许对外分享>。

目标：<title>
URL：<url-or-token-if-url-unavailable>
类型：<type>

当前链接访问范围：<render link_access>
对外分享：<render external_sharing>
外部邀请：<render external_invitation or omit if unknown because field is absent>
协作者管理（组织维度）：<render collaborator_org_scope>
协作者管理（权限维度）：<render collaborator_permission_scope or omit if unknown because field is absent>
复制内容：<render copy_scope or omit if unknown because field is absent>
创建副本 / 打印 / 下载：<render security_scope>
评论：<render comment_scope>
Wiki 继承限制：<render lock_state or omit if unknown because field is absent>

检查边界：<render check_scope>
```

English template:

```text
Conclusion: <Not publicly accessible on the internet / A public internet link is enabled / External sharing is enabled>.

Target: <title>
URL: <url-or-token-if-url-unavailable>
Type: <type>

Current link access: <render link_access>
External sharing: <render external_sharing>
External invitations: <render external_invitation or omit if unknown because field is absent>
Collaborator management by tenant: <render collaborator_org_scope>
Collaborator management by permission: <render collaborator_permission_scope or omit if unknown because field is absent>
Copy content: <render copy_scope or omit if unknown because field is absent>
Create copies / print / download: <render security_scope>
Comments: <render comment_scope>
Wiki inheritance lock: <render lock_state or omit if unknown because field is absent>

Check boundary: <render check_scope>
```

Raw evidence, only when requested:

```text
Evidence fields:
- link_share_entity=<value>
- external_access_entity=<value>
- external_access=<value>
- invite_external=<value>
- share_entity=<value>
- manage_collaborator_entity=<value>
- copy_entity=<value>
- security_entity=<value>
- comment_entity=<value>
- lock_switch=<value>
```

## 多目标明确列表诊断

当 `target_scope=explicit_list` 时，使用此模板。该场景不执行容器递归发现；对用户提供的每个 URL / token 逐个生成 `per_target_permission_assessment`，再按风险分组聚合。权限语义和单目标、容器诊断完全复用，不新增判断模型。

```text
已完成只读权限诊断，没有做任何权限修改。

一句话结论：<N> 个目标中，<risk_count> 个存在待复核权限风险；<internet_public_count> 个存在互联网公开链接候选，<external_access_count> 个允许对外分享，<unknown_count> 个无法完整判断。

覆盖情况：
- 用户提供目标：<input_target_count>；成功解析：<resolved_count>
- 成功读取文档公共访问和协作权限设置：<permission_checked_count>；读取失败 / 不支持 / 无权限：<failed_or_unsupported_count>

逐目标结果（1-10 个目标默认全部展示；超过 10 个时按 `摘要清单展开规则` 展示，并提示生成完整风险清单）：

- <risk_id-or-item_id> <path-or-title> (<type>)
  URL: <url-or-token-if-url-unavailable>
  结论：<not_public / public_link_enabled / external_sharing_enabled / policy_review / unknown>
  关键权限：<render link_access>; <render external_sharing>; <render security_scope>; <render comment_scope>
  密级：<sec_label_name-or-missing-or-unknown>
  待复核原因：<risk reason or none>
  建议动作：<recommended action or no action>

分组摘要：
- 互联网公开链接候选：<count>；允许对外分享：<count>；公司内链接可访问 / 可编辑：<count>
- 复制 / 下载 / 打印 / 评论待策略确认：<count>；无法判断：<count and reason summary>

建议下一步：
- 处理明确的 <risk_id>，先生成只读 dry-run。
- 生成完整风险清单 artifact，后续可按 `risk_id`、风险分组、URL 或 `selected=true` 选择治理范围；只看权限设置时改用 `权限设置清单`。
```

## 摘要清单展开规则

容器安全诊断的摘要必须兼顾可读性和可治理性。不要用固定 Top N 代替可处理清单。

| 风险对象数 | 摘要默认展示 | 必须提供的下一步 |
|------------|--------------|------------------|
| `0` | 只展示覆盖情况、未覆盖能力和剩余限制 | 如需更细审计，可生成权限设置清单 |
| `1-10` | 展示全部风险对象 | 可直接按 `risk_id` 生成 dry-run 或写入确认 |
| `11-30` | 展示全部高优先级待复核对象；中 / 低优先级做分组摘要 | 生成完整风险清单 artifact，或按风险分组生成 dry-run |
| `31-100` | 每个高优先级待复核分组展示 Top 5，附未展示数量 | 生成 Markdown / CSV / 飞书文档完整风险清单 |
| `100+` | 只展示分组统计、Top 样例和覆盖限制，不内联长表 | 强烈建议生成结构化风险清单后再选择治理范围 |

高优先级待复核对象包括：互联网公开链接、允许对外分享、允许对外分享且缺少 / 低于 policy 密级标签、公司内可编辑链接。协作者管理范围较宽默认归入中优先级待复核；只有用户 policy 明确要求严格协作者管理时才提升优先级。复制 / 下载 / 打印、评论范围在用户未提供明确 policy 时归入“待策略确认”，不要挤占高优先级清单。

摘要中的每个待复核对象必须包含 `risk_id`、path/title、URL、type、owner、sec_label、风险原因、关键证据和建议动作。对同一底层文档的多个 Wiki 入口或 shortcut，必须用 URL 区分；如果建议合并治理，在建议动作里说明它们指向同一底层对象。

## 审计摘要

```text
目标：<title> (<type>)
URL：<url-or-token-if-url-unavailable>
结论：<合规 / 待确认风险 / 无法完整判断>
证据：
- link_share_entity=<value>
- external_access_entity=<value>
- external_access=<value>
- invite_external=<value>
- share_entity=<value>
- manage_collaborator_entity=<value>
- copy_entity=<value>
- security_entity=<value>
- comment_entity=<value>
- lock_switch=<value>
- sec_label_name=<value-or-missing>
限制：<unsupported_checks or none>
建议动作：<read-only next step or proposed remediation>
```

## 容器安全诊断报告摘要

```text
已完成只读安全诊断，没有做任何权限修改。

一句话结论：<未发现互联网公开链接 / 存在互联网公开链接候选风险>；<external_access_count> 个文档允许对外分享，<missing_label_count> 个文档缺少密级标签。建议优先复核 <top_priority_group_or_paths>。

覆盖情况：
- 当前身份可见目标：<visible_count>
- 已成功检查文档公共访问和协作权限设置：<permission_checked_count>
- 读取失败 / 已删除 / 无权限：<failed_count>
- 未覆盖能力：<collaborator_list / inheritance / audit_log / view_records / none>

风险分级：
- 高优先级待复核：<internet_public_count> 个互联网公开链接候选；<external_access_count> 个允许对外分享；其中 <external_without_label_count> 个同时缺少密级标签。
- 中优先级待复核：<tenant_link_count> 个公司内知道链接可访问 / 可编辑；<wide_share_count> 个协作者管理范围较宽。
- 待策略确认：<security_count> 个复制 / 下载 / 打印范围待复核；<comment_count> 个评论范围待复核。
- 无法判断：<unsupported_or_unverified_summary>。

分级含义：
- 互联网公开链接：获得链接的任何人可能访问，最高优先级。
- 允许对外分享：外部分享能力已开启，需 owner 复核；不等于已经存在外部协作者。
- 公司内链接可访问：不是对外公开，但组织内扩散范围较宽。
- 复制 / 下载 / 打印 / 评论：是否需要收紧取决于业务 policy 和文档密级。

高优先级待复核清单：
> 按 `摘要清单展开规则` 展示。每个对象必须包含 `risk_id` 和 URL；缺少 URL 时展示 token / node_token 和原因。若没有高优先级对象，只展示中优先级或待策略确认分组摘要。

- <risk_id> <path-or-title> (<type>)
  URL: <url-or-token-if-url-unavailable>
  Owner: <owner-or-unknown>
  密级：<sec_label_name-or-missing-or-unknown>
  待复核原因：<why high priority>
  证据：<short user-language evidence, e.g. 对外分享=已开启；链接分享=未开启互联网公开链接>
  建议动作：<recommended action>

未完全展开：
- 完整风险清单包含 <risk_manifest_count> 条；本摘要已展示 <shown_count> 条，未展示 <hidden_count> 条。
- 未展示分组：<risk_group=count summary or none>

建议下一步：
- 生成完整风险清单 artifact，包含 `risk_id`、URL、owner、密级、证据字段、建议动作和 `selected` 列。
- 基于 risk_id、风险分组、owner、路径、URL 或 artifact 中 `selected=true` 的行生成只读整改 dry-run。
- 只针对最高优先级目标进入写入确认流程，例如关闭互联网公开链接或收紧对外分享；写入前仍需二次确认。
- 按 owner / 密级生成复核清单。
- 继续读取访问记录，判断低活跃高暴露。

剩余限制：
- <do not claim collaborator-list verification if unsupported>
- <external_access_entity=open or external_access=true only means sharing outside is allowed, not that external collaborators exist>
- <missing view_records / DLP / AI index status / audit log limitations>
```

## 可操作风险清单

完整风险清单用于让用户选择后续治理范围。Markdown / CSV / 飞书文档报告都必须包含以下字段；如果某种格式无法完整展示嵌套证据，使用短文本摘要，保留 `risk_id` 和 URL。

```text
范围：<explicit_list / wiki_space / wiki_node / drive_folder> <name-or-id>
生成时间：<timestamp>
用途：用户可按 risk_id、priority、risk_group、owner、path、URL 或 selected=true 选择治理对象。

| risk_id | priority | Path | URL | Type | Owner | sec_label | risk_group | evidence | recommended_action | current_setting | target_setting | selected | decision | status | skip_reason |
|---------|----------|------|-----|------|-------|-----------|------------|----------|--------------------|-----------------|----------------|----------|----------|--------|-------------|
| PG-001 | P1 | <path> | <url-or-token> | <type> | <owner-or-unknown> | <sec-label-or-missing> | <risk_group> | <short evidence> | <recommended-action> | <field=value> | <field=value-or-owner-review> | false | undecided | pending | <none-or-reason> |
```

字段规则：

- `risk_id` 按 priority、risk_group、normalized path、URL、canonical token / node_token 稳定排序生成；URL 缺失时必须使用 token / node_token 作为 tie-breaker。同名、同路径、shortcut 或多个 Wiki 入口不能只靠 path 生成编号；同一次诊断中不得重复。
- `priority` 使用 `P0`、`P1`、`P2`、`PolicyReview`、`Unknown`；面向用户展示时可译为“最高优先级 / 高优先级待复核 / 中优先级待复核 / 待策略确认 / 无法判断”。
- `selected` 默认 `false`；用户可在 CSV / 飞书文档表格中改为 `true`，或在聊天中直接说 “处理 PG-001、PG-003”。
- `decision` 表示用户决策：`undecided`、`keep`、`dry_run`、`confirm_write`、`skip`。
- `status` 表示执行状态：`pending`、`dry_run_ready`、`confirmed`、`executed`、`verified`、`failed`、`skipped`。
- `target_setting` 是建议目标状态，不代表已执行；没有明确 policy 时只能写 owner review / policy review。

## 治理选择交互

用户基于完整风险清单继续治理时，Agent 必须先解析选择范围，再生成只读 dry-run：

```text
可接受的用户选择：
- 处理 PG-001、PG-003、PG-008，把互联网公开链接关闭。
- 先处理所有 risk_group=internet_public_link，不处理 external_access_only。
- 把 CSV / 飞书文档里 selected=true 的行生成整改 dry-run。
- PG-003 先跳过，只处理 PG-001。

Agent 必须回复：
- 已选择对象数：<count>
- 选择来源：<risk_id list / risk_group / selected=true / URL / path>
- 将执行的下一步：生成 dry-run；不执行写入
- 需要跳过或重新确认的对象：<missing risk_id / unsupported / changed_since_report / no manage_public>
```

如果用户选择来自旧报告或外部 artifact，生成 dry-run 前必须对所选目标重新读取当前权限。当前设置和报告快照不一致时，标记为 `changed_since_report`，不要直接沿用旧字段执行。

## 权限设置清单

```text
范围：<explicit_list / wiki_space / wiki_node / drive_folder> <name-or-id>

| Path | URL | Type | link_share_entity | external_access_entity / external_access | invite_external | share_entity | manage_collaborator_entity | copy_entity | security_entity | comment_entity | lock_switch | sec_label_name | 建议动作 | 限制 |
|------|-----|------|-------------------|------------------------------------------|-----------------|--------------|----------------------------|-------------|-----------------|----------------|-------------|----------------|----------|------|
| <path> | <url-or-token> | <type> | <value> | <value> | <value-or-unknown> | <value> | <value-or-unknown> | <value-or-unknown> | <value> | <value> | <value-or-unknown> | <value-or-missing> | <recommended-action> | <unsupported-or-none> |
```

## 访问复核清单

```text
范围：<wiki_space / wiki_node / drive_folder / explicit_list> <name-or-id>
复核对象数：<count>

| Owner | Path | URL | Type | 密级 | 风险标签 | 当前权限摘要 | 最近访问证据 | 建议动作 |
|-------|------|-----|------|------|----------|--------------|--------------|----------|
| <owner-or-unknown> | <path> | <url-or-token> | <type> | <sec-label-or-missing> | <labels> | <link/external/share/security/comment> | <uv/pv/last_view_or_unknown> | <keep / tighten / owner review / unsupported> |

限制：<unsupported_checks / discovery_blockers / none>
```

## 整改 dry-run

```text
将生成整改计划，不执行写入：
- 范围：<scope>
- 选择来源：<risk_id list / risk_group / selected=true artifact / URL list>
- 候选目标数：<count>
- 计划执行命令：<command family>
- 重新读取：已对所选目标重新读取当前权限；changed_since_report=<count>
- 字段变更：
  - <risk_id> <path> (<url-or-token>): <field> <old> -> <new>
- 跳过项：<unsupported / no manage_public / unsupported type / missing policy>
- 验证方式：执行后重新读取 <元数据 / 文档公共访问和协作权限设置>
- 有限回滚范围：<文档公共访问和协作权限设置快照字段 / 不适用>

请确认是否进入写入确认。
```

## 批量权限申请确认

```text
将逐个发起 <view / edit> 权限申请：
- 候选目标数：<count>
- 命令类型：drive +apply-permission
- 风险：write；每个请求都会通知 owner
- 执行方式：按候选列表顺序逐个调用，失败项会单独记录

候选示例：
- <risk_id> <title> (<type>, <url-or-token>)：<reason>

请确认是否对上述候选目标发起权限申请。
```

## owner 转移确认

```text
将逐个转移 owner：
- 候选目标数：<count>
- 命令类型：drive permission.members transfer_owner
- 风险：high-risk-write；会改变文档 owner，可能影响原 owner 权限和文档所在位置
- 新 owner 映射：<same_new_owner / per_target_new_owner>
- 全局新 owner：<member_id> (<member_type>)；仅当所有候选目标的新 owner 相同时展示，否则省略
- 通知新 owner：<need_notification>
- 原 owner 权限：<remove_old_owner=true / old_owner_perm>
- 个人空间位置：<stay_put>
- 执行方式：按候选列表顺序逐个调用，失败项会单独记录
- 验证方式：执行后重新读取 metadata owner；metadata 不支持的类型标记为 partial
- 回滚边界：不做自动回滚；如需恢复 owner，必须另起一次反向 owner 转移确认

候选示例：
- <risk_id> <title> (<type>, <url-or-token>)：当前 owner=<owner-or-unknown> -> 新 owner=<member_id> (<member_type>)

请确认是否对上述候选目标转移 owner。
```

## 确认请求

```text
将执行 <operation>：
- 目标：<risk_id> <title> (<type>, <url-or-token>)
- 命令类型：<command family>
- 风险：<risk_level>
- 字段变更：
  - <field>: <old> -> <new>
- 验证方式：执行后重新读取 <元数据 / 文档公共访问和协作权限设置>
- 有限回滚材料：<文档公共访问和协作权限设置快照 / 不适用>

请确认是否执行。
```

## 最终摘要

```text
已完成：<read checks / writes>
验证：<fresh read result or async permission-request approval note>
清单状态：<risk_id status updates / not applicable>
回滚材料：<文档公共访问和协作权限设置快照 / 不适用>
剩余限制：<unsupported_checks / partial facts / approvals>
```
