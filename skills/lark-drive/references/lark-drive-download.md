
# drive +download

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

从飞书云空间（云盘/云存储）下载文件到本地。下载对象是 Drive **文件**（上传的 PDF/zip/图片/音视频等文件），以及支持 Wiki URL / Wiki token。

## 命令

```bash
# 下载到指定路径
lark-cli drive +download --file-token boxbc_xxx --output ./report.pdf

# 只提供 token，默认保存到当前目录
lark-cli drive +download --file-token boxbc_xxx

# 直接传 URL，CLI 自动解析类型和 token
lark-cli drive +download --url "https://example.feishu.cn/file/<FILE_TOKEN>" --output ./report.pdf

# Wiki URL 也可直接传，CLI 会先解析到底层 obj_token/obj_type（obj_type 必须是 file）
lark-cli drive +download --url "https://example.feishu.cn/wiki/<WIKI_NODE_TOKEN>" --output ./report.pdf

# 只有裸 Wiki node token 时，显式传 --wiki-token，让 CLI 先解析底层文件
lark-cli drive +download --wiki-token "<WIKI_NODE_TOKEN>" --output ./report.pdf

# 中断后从显式输出路径旁的 partial 文件继续
lark-cli drive +download --file-token boxbc_xxx --output ./report.pdf --continue
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--file-token` | 条件必填 | Drive 文件 token；与 `--url` / `--wiki-token` 三选一 |
| `--url` | 条件必填 | 飞书文件 URL 或 Wiki URL；CLI 自动解析类型和 token |
| `--wiki-token` | 条件必填 | 裸 Wiki node token；CLI 先解析到底层 Drive 文件 |
| `--output` | 否 | 本地输出路径；不传时默认保存到当前目录 |
| `--overwrite` | 否 | 覆盖已存在的输出文件；不传时目标已存在会报错 |
| `--continue` | 否 | 从 `<output>.partial` 继续中断的下载；必须同时指定 `--output` |

## URL 解析

从飞书文件 URL 提取 token：

```
https://xxx.feishu.cn/drive/file/boxbc_xxx
                                  ^^^^^^^^^
                                  file_token
```

Wiki URL / 裸 Wiki node token 会先解析到底层文档，解析后会在输出里附带 `wiki_token` 和 `wiki_node`（含底层 `obj_token`/`obj_type`）。

## 分片与文件一致性

普通下载与 `+pull` 在修改时间可用时采用 128 MiB 顺序 Range 请求，并在保存提交前复查 `latest_modify_time`。发现变化时返回 `network/representation_changed`，不发布本次下载结果。

`latest_modify_time` 只有秒级精度，同秒内覆盖可能漏检；它不等价于强 ETag 或固定版本标识。将来端点提供强 validator 或可读取的固定版本时，应改用该证据。

指定 `--output` 也会尝试读取修改时间，但普通下载不新增必需 scope。修改时间缺失或查询失败时回退到单流下载；大文件会在 stderr 提示长连接仍可能失败。

## 关键约束

- Wiki 节点解析后的 `obj_type` 必须是 `file`；不确定 token 类型时，先用 `lark-cli drive +inspect --url <TOKEN> --type wiki` 检查。
- 普通下载直接通过当前 FileIO backend 保存到 `--output`；不会读取或清理同名 `.partial` 文件。
- `--continue` 需要 backend 同时支持 partial 的追加、checkpoint 读写/删除和最终提交；不支持时命令会在发起下载前失败。
- `--continue` 只接受显式 `--output`。中断时会保留 `<output>.partial` 和 `<output>.partial.meta` checkpoint，下一次会校验服务环境、底层文件 token、总大小与 `latest_modify_time`，偏移以 partial 的实际大小为准；成功提交后清理 checkpoint。
- `--continue` 必须能读取修改时间，包括首次运行。查询失败或修改时间缺失时保留断点并失败；网络超时和限流遵循原错误的重试提示，权限问题需先补齐元数据权限。
- 恢复前发现来源、大小或修改时间不匹配时丢弃旧断点并重新下载。旧版仅含 ETag 的 checkpoint 也会重新下载。提交前复查失败时保留断点，确认修改时间变化时删除已失效的 partial 与 checkpoint。
- 同一组 `<output>.partial` 和 checkpoint 应由一个 `--continue` 进程顺序使用；不要同时启动多个相同输出路径的继续下载。

## 排障

- 如果返回 `permission_denied`，或最终下载返回 `HTTP 403`，按错误 `hint` 使用 `lark-cli drive +preview --file-token <FILE_TOKEN> --type source_file --output <path>` 获取预览产物。
- 如果返回限流错误，停止立即重试，稍后按指数退避重试。
- 如果目标（或 Wiki 解析出的底层文档）是 `docx` / `sheet` / `bitable` / `slides` 等在线文档，`+download` 无法直接下载，会返回 typed validation error；改用 [lark-drive-export](lark-drive-export.md) 渲染成 pdf / xlsx / pptx / markdown 等格式。

## 参考

- [lark-drive](../SKILL.md) -- 云空间（云盘/云存储）全部命令
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
