# 查询妙记及其产物

围绕目标妙记执行查询：先取得唯一 `minute_token`，再按用户目标查询基础信息、AI 产物、逐字稿、原始媒体或关联的智能纪要。已有会议上下文或 `meeting_id` 时，先从会议链路取得 `minute_token`，不要重复搜索妙记。

## 定位妙记

- 已有 `minute_token` 时直接使用。
- 妙记 URL 的路径最后一段是 `minute_token`；去掉 query 参数。
- 没有 token 时，用标题/关键词、所有者、参与者或时间范围执行搜索：

  ```bash
  lark-cli minutes +search --query <query> --start <start> --end <end> --as user
  ```

- `minutes +search` 支持用户身份和应用身份。默认使用用户身份；只有用户明确要求应用视角或上下文已经是应用身份时才使用 `--as bot`。
- `me` 只适用于用户身份；应用身份没有“当前用户”，必须传明确的 `ou_` open_id。应用身份的 token 或 scope 问题不能通过 `auth login` 修复。
- “我参与的妙记”默认是“我拥有”与“我作为参与者”两次查询的并集，具体过滤语义见 [`lark-minutes-search`](../references/lark-minutes-search.md)。
- 根据 `has_more` 和 `page_token` 翻页。用户未明确要求全量时，累计结果超过 50 条且仍有更多结果再确认是否继续；用户明确要求“全部、所有、统计、排序”时直接获取全部分页，并按结果中的 `token` 去重后再返回或统计。
- 多个候选时展示标题、时间、所有者、URL 和 token，让用户选择，不擅自挑选。

只需要搜索结果时，返回命中项后停止。

一旦用某个身份搜索或解析出 `minute_token`，后续妙记详情、产物读取、媒体下载、权限申请及关联 Note / Doc 查询都必须显式沿用同一个 `--as`。不要依赖 profile 默认身份，也不要为绕过资源权限切换身份。

## 查询基础信息

用户只要标题、时长、封面、所有者或 URL 时，使用基础信息命令：

```bash
lark-cli minutes minutes get --params '{"minute_token":"<minute_token>"}' --as <source_identity>
```

基础信息已经满足目标时，不继续读取 AI 产物或逐字稿。命令参数不足时运行 `lark-cli minutes minutes get --help`。

## 获取 AI 产物和逐字稿

使用 `minutes +detail`，只传用户需要的产物 flag：

```bash
lark-cli minutes +detail --minute-tokens <minute_token> --summary --todo --chapter --keyword --transcript --as <source_identity>
```

- 可选 `--summary`、`--todo`、`--chapter`、`--keyword`、`--transcript`。
- 不传产物 flag 只返回基础信息和可能存在的顶层 `note_id`。
- 用户只要现成总结、待办或章节时，返回对应 AI 产物。
- 用户要求提炼、重新总结、分析或复盘时，只读取 Transcript 并基于原始发言独立分析；禁止照搬 `--summary`。

产物 flags、返回字段和本地输出见 [`lark-minutes-detail`](../references/lark-minutes-detail.md)。

## 下载原始音视频

用户需要原始媒体文件或下载链接时，使用 `minutes +download`。同一妙记的下载产物统一归拢到 `./minutes/<minute_token>/`，除非用户指定其他安全相对路径。

媒体类型、路径、链接有效期和权限见 [`lark-minutes-download`](../references/lark-minutes-download.md)。

## 获取关联的智能纪要

从 `minutes +detail` 顶层读取 `note_id`，直接执行 `note +detail`：

```bash
lark-cli note +detail --note-id <note_id> --as <source_identity>
```

- 不要把 `minute_token` 当作 `note_id`，也不要绕回 VC。
- 顶层没有 `note_id` 表示该妙记没有关联 Note，到此停止。
- 取得 `note_doc_token`、`verbatim_doc_token` 或 `shared_doc_tokens` 后，按智能纪要和 Doc 的规则继续。

## 处理无权限结果

没有查看权限时，说明需要妙记所有者授权；不要自动执行 `minutes +apply-permission`。只有用户明确要求申请查看或编辑权限时，才进入编辑妙记场景发起申请，并沿用触发无权错误时的身份。详见 [`lark-minutes-apply-permission`](../references/lark-minutes-apply-permission.md)。
