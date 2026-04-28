
# drive +pull

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

把飞书云空间的某个文件夹**单向**镜像到本地目录（Drive → 本地）。命令递归列出 `--folder-token` 下所有 `type=file` 的文件，逐一下载到 `--local-dir` 对应的相对路径，子文件夹自动复刻为本地目录。

输出按"动作"分类：

| 字段 | 含义 |
|------|------|
| `summary.downloaded` | 成功下载的文件数 |
| `summary.skipped` | 按 `--if-exists=skip` 跳过的文件数 |
| `summary.failed` | 下载或写盘失败的文件数 |
| `summary.deleted_local` | 启用 `--delete-local --yes` 时删除的本地文件数 |
| `items[]` | 每个文件的明细（`rel_path` / `file_token` / `action` / 失败时的 `error`） |

## 命令

```bash
# 基础用法 —— 把云端 fldcXXX 镜像到 ./repo
lark-cli drive +pull --local-dir ./repo --folder-token fldcnxxxxxxxxx

# 已存在的本地文件保持不动
lark-cli drive +pull --local-dir ./repo --folder-token fldcnxxxxxxxxx \
  --if-exists skip

# 镜像同步：下载新文件 + 删除云端没有的本地文件
# （--delete-local 必须搭配 --yes，否则会被 Validate 直接拒绝）
lark-cli drive +pull --local-dir ./repo --folder-token fldcnxxxxxxxxx \
  --delete-local --yes
```

## 参数

| 标志 | 必填 | 类型 | 说明 |
|------|------|------|------|
| `--local-dir` | 是 | path | 本地根目录（**必须是 cwd 的相对路径**；绝对路径或逃出 cwd 的相对路径会被 CLI 直接拒绝） |
| `--folder-token` | 是 | string | 源 Drive 文件夹 token |
| `--if-exists` | 否 | enum | 本地文件已存在时的策略：`overwrite`（默认）/ `skip` |
| `--delete-local` | 否 | bool | 删除本地云端不存在的文件，实现镜像同步语义；**必须配合 `--yes`** |
| `--yes` | 否 | bool | 确认 `--delete-local`；不传时该破坏性操作在 Validate 阶段被拒绝 |

## 比较与下载范围

- **只下载 Drive `type=file` 的二进制文件**。在线文档（`docx` / `sheet` / `bitable` / `mindnote` / `slides`）和快捷方式（`shortcut`）会被跳过 —— 它们没有等价的本地二进制可写盘，否则会变成产生噪声的"假"下载。
- 子文件夹会递归遍历；rel_path 形如 `sub1/sub2/file.txt`，本地缺失的父目录会被自动创建。
- 已存在的本地文件按 `--if-exists` 决定 `overwrite` 还是 `skip`，没有第三种选择 —— 想做 `keep-both` 这类的请自己改名再 pull。

## --delete-local 的安全行为

`--delete-local` 是命令里**唯一的破坏性 flag**，会按"本地有但云端没有"清理本地副本。设计上把它跟 `--yes` 强绑定：

- `--delete-local`（无 `--yes`）→ Validate 直接报错：`--delete-local requires --yes`，没有任何下载、列表请求或删除发生。
- `--delete-local --yes` → 正常执行：先把云端文件下载到本地，再扫一遍 `--local-dir` 下所有常规文件，把不在云端清单里的逐个 `os.Remove`。
- 不传 `--delete-local` → `summary.deleted_local` 永远是 0；命令对本地"多余"文件视而不见。

第 6 章里把 `+pull --delete-local` 标了 `high-risk-write`，CLI 这边的实现等价于"未传 `--yes` 时拒绝执行"，符合该约束的精神。

## 输出 schema

```json
{
  "summary": {
    "downloaded": 0,
    "skipped": 0,
    "failed": 0,
    "deleted_local": 0
  },
  "items": [
    {"rel_path": "...", "file_token": "...", "action": "downloaded"},
    {"rel_path": "...", "file_token": "...", "action": "skipped"},
    {"rel_path": "...", "file_token": "...", "action": "failed", "error": "..."},
    {"rel_path": "...", "action": "deleted_local"},
    {"rel_path": "...", "action": "delete_failed", "error": "..."}
  ]
}
```

`rel_path` 始终用 `/` 作为分隔符（跨平台一致）。删除条目（`deleted_local` / `delete_failed`）没有 `file_token`，因为该文件本来就只在本地。

## 性能注意

- 下载流量 ≈ 云端待下载文件的总字节数。pull 是**全量**写盘 —— 跟 `+status` 不一样，不会跳过"内容相同"的文件（status 是按 hash 比较，pull 是按 `--if-exists`），所以一次跑可能很重。
- 想避免重跑全量，可以先 `+status` 找出 `new_remote` 和 `modified`，再只对这些文件单独 `+download`。
- 大文件会用 SDK 的流式下载（不会把整个 body 读进内存），但本地磁盘空间需要够。

## 所需 scope

| 操作 | scope |
|------|-------|
| 列出文件夹 / 子目录 | `drive:drive.metadata:readonly` |
| 下载文件 | `drive:file:download` |

如果当前 token 缺这些 scope，命令会直接报 `missing_scope` 并提示重新登录。`drive:drive` 在部分企业被策略禁用，所以 +pull 故意只声明上面这两个细粒度 scope。

## 范围限制

`--local-dir` 只接受 cwd 内的相对路径。如果用户想 pull 到 cwd 之外的目录，**不要 agent 自己 `cd` 绕过**；告诉用户切换 agent 工作目录到合适的祖先后重试，或者把目标软链接到 cwd 内。CLI 会在路径越界时直接报 `unsafe file path`。

## 参考

- [lark-drive](../SKILL.md) —— 云空间全部命令
- [lark-shared](../../lark-shared/SKILL.md) —— 认证和全局参数
- [lark-drive-status](lark-drive-status.md) —— 下载前先看差异
- [lark-drive-download](lark-drive-download.md) —— 单文件按需拉取
