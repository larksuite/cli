# Drive 文件复制

用于复制在线文档、创建副本、另存为副本，或把文件复制到用户指定的 Drive 文件夹。

## 直接路径

1. 定位源文件：
   - 已给 docx/sheet/bitable/slides/file URL 时，直接从 URL 路径取源 `file_token`，并按路径确定 `type`。
   - 已给完整标题时，用 `lark-cli drive +search --query "<标题或合法短查询>" --as user --format json` 定位候选；`--query` 最长 30 个字符，过长标题先用稳定核心标题查询。只有候选提供完整、未截断的标题且能与用户给定标题精确匹配到唯一源文件时才继续；`title_highlighted` 可能被截断，不能单独证明完整标题相等。缺少完整标题或候选仍不唯一时，停止并请用户提供 URL/token；不要取首项、改用全量 `drive files list`，或使用搜索结果之外的隐藏 token。
   - `drive +search` 结果里的 `token` 是源 `file_token`；`result_meta.doc_types` 或同等类型字段给出源类型，复制时使用小写类型值，如 `DOCX` -> `docx`。
2. 复制源文件：
   - `--file-token` 传源文件 token。
   - `--data` JSON 至少包含 `folder_token`、`name`、`type`。
   - 用户指定目标文件夹 URL/token 时，`folder_token` 传该文件夹 token。
   - 用户没有指定目标文件夹时，传 `folder_token:""`，表示复制到当前调用用户的 Drive 根目录；不要为了猜父目录执行 `drive files list`。
3. 使用复制结果：
   - 响应中的 `file.token` 是新副本 token，后续读取或正文编辑必须使用这个 token。
   - 不要再按副本标题搜索来找目标文档。

## 命令形态

```bash
lark-cli drive files copy \
  --as user \
  --file-token <SOURCE_FILE_TOKEN> \
  --data '{"folder_token":"","name":"<COPY_NAME>","type":"docx"}' \
  --format json
```

把示例中的空 `folder_token` 换成用户明确指定的目标文件夹 token；不要换成源文件 token 或 wiki token。

## 边界

- 普通 docx/sheet/bitable/slides/file 复制不需要先跑 `schema drive.files.copy`、`drive +inspect`、`drive metas batch_query` 或 `drive files list`。
- wiki URL/token 不是底层 file token；只有来源是 wiki 时才先用 `drive +inspect` 解包出底层 `token` 和 `type`。
- `data.type` 必须与源文件实际类型一致；类型不确定时优先用搜索结果字段或 URL 路径，不要猜。
- 如果返回 `confirmation_required`，按 `lark-shared` 高风险审批协议确认后，在同一 copy 命令追加 `--yes` 重试。
