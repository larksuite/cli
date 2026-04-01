
# drive +export-status

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

查询导出任务结果。适用于 `drive +export` 返回了 `ticket`，但尚未完成下载的场景。

## 命令

```bash
lark-cli drive +export-status \
  --token "<SOURCE_TOKEN>" \
  --ticket "<TICKET>"
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--token` | 是 | 创建导出任务时使用的源文档 token |
| `--ticket` | 是 | `drive +export` 返回的任务 ticket |

## 输出重点

- `ready=true`：任务已完成，可直接读 `file_token`
- `failed=true`：任务失败，查看 `job_status` / `job_error_msg`
- `file_token`：导出产物 token，供 `drive +export-download` 使用
- `file_name`：导出文件名
- `file_extension`：导出格式

## 推荐续跑

```bash
# 先查状态
lark-cli drive +export-status \
  --token "<SOURCE_TOKEN>" \
  --ticket "<TICKET>"

# ready=true 后再下载
lark-cli drive +export-download \
  --file-token "<EXPORTED_FILE_TOKEN>" \
  --output-dir ./exports
```

## 参考

- [lark-drive](../SKILL.md) -- 云空间全部命令
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
