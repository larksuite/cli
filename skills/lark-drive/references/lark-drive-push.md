
# drive +push

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

把本地目录**单向**镜像到飞书云空间的某个文件夹（本地 → Drive）。命令递归列出 `--folder-token` 下的远端清单，遍历 `--local-dir` 的所有常规文件，按相对路径在 Drive 上新建、覆盖或跳过；可选地（`--delete-remote --yes`）删除云端"本地没有"的 `type=file`。

输出按"动作"分类：

| 字段 | 含义 |
|------|------|
| `summary.uploaded` | 成功新建或覆盖的文件数 |
| `summary.skipped` | 按 `--if-exists=skip` 跳过的文件数 |
| `summary.failed` | 上传 / 覆盖 / 建目录 / 删除失败的条目数 |
| `summary.deleted_remote` | 启用 `--delete-remote --yes` 时删除的云端文件数 |
| `items[]` | 每个条目的明细（`rel_path` / `file_token` / `action` / 覆盖时的 `version` / `size_bytes` / 失败时的 `error`） |

`items[].action` 取值：`uploaded` / `overwritten` / `skipped` / `folder_created` / `deleted_remote` / `failed` / `delete_failed`。

> 本地目录（包括空目录）会被镜像到 Drive；新建的子目录会以 `action: "folder_created"` 出现在 `items[]` 里，但**不计入** `summary.uploaded`（该字段只数文件）。已存在的远端目录复用其 token，不会重复 `create_folder`，也不会出现在 `items[]` 里。

## 命令

```bash
# 基础用法 —— 把本地 ./repo 镜像到云端 fldcXXX
lark-cli drive +push --local-dir ./repo --folder-token fldcnxxxxxxxxx

# 已存在的远端文件保持不动
lark-cli drive +push --local-dir ./repo --folder-token fldcnxxxxxxxxx \
  --if-exists skip

# 镜像同步：上传 / 覆盖 + 删除本地不存在的远端文件
# （--delete-remote 必须搭配 --yes，否则会被 Validate 直接拒绝）
lark-cli drive +push --local-dir ./repo --folder-token fldcnxxxxxxxxx \
  --delete-remote --yes
```

## 参数

| 标志 | 必填 | 类型 | 说明 |
|------|------|------|------|
| `--local-dir` | 是 | path | 本地根目录（**必须是 cwd 的相对路径**；绝对路径或逃出 cwd 的相对路径会被 CLI 直接拒绝） |
| `--folder-token` | 是 | string | 目标 Drive 文件夹 token |
| `--if-exists` | 否 | enum | 远端文件已存在时的策略：`overwrite`（默认）/ `skip` |
| `--delete-remote` | 否 | bool | 删除云端本地不存在的文件，实现镜像同步语义；**必须配合 `--yes`** |
| `--yes` | 否 | bool | 确认 `--delete-remote`；不传时该破坏性操作在 Validate 阶段被拒绝 |

## 上传与目录复刻范围

- **只上传 / 覆盖 / 删除 Drive `type=file`**。在线文档（`docx` / `sheet` / `bitable` / `mindnote` / `slides`）和快捷方式（`shortcut`）即使在同一 rel_path 下出现，也不会被覆盖或删除 —— 它们没有等价的本地二进制。
- **本地目录结构整体被镜像**：所有子目录（含**空目录**）会按需在 Drive 上 `create_folder`；同名远端目录复用其 token，不重建。空目录不计入 `summary.uploaded`，但会在 `items[]` 里以 `folder_created` 形式留痕。
- 已存在的远端文件按 `--if-exists` 决定 `overwrite` 还是 `skip`，没有第三种选择 —— 想做 `keep-both` 这类的请自行改名再 push。

## 覆盖语义

`--if-exists=overwrite` 走 `POST /open-apis/drive/v1/files/upload_all`，并在 form 中带上现有文件的 `file_token`，由后端原地更新内容并返回新版本号。`items[].version` 字段会回填该版本号。

> **协议注意**：`upload_all` 接受 `file_token` 字段、并在响应里返回 `version` 是设计文档（Drive 同步盘）规定的协议；此后端尚在灰度发布。命令实现按协议来，但若所选 tenant 还没开通这一字段，覆盖会因"无 version 返回"而报结构化 `api_error`：调用方可以暂时改用 `--if-exists=skip` 直到后端到位。新建上传不依赖该字段，不受影响。

大文件（>20MB）会自动切到三段式 `upload_prepare` / `upload_part` / `upload_finish`；该路径下 `version` 暂未在响应中返回，覆盖结果中 `items[].version` 会留空，但 `file_token` 与 `action: overwritten` 仍会正确产出。

## --delete-remote 的安全行为

`--delete-remote` 是命令里**唯一的破坏性 flag**，会按"远端有但本地没有"逐个 `DELETE /open-apis/drive/v1/files/<token>?type=file` 清理云端副本。设计上把它跟 `--yes` 强绑定：

- `--delete-remote`（无 `--yes`）→ Validate 直接报错：`--delete-remote requires --yes`，不会发起任何列表 / 上传 / 删除请求。
- `--delete-remote --yes` → 正常执行：先把本地文件 push 上去，再扫一遍远端 `type=file` 列表，把不在本地清单里的逐个删除。
- 不传 `--delete-remote` → `summary.deleted_remote` 永远是 0；命令对远端"多余"文件视而不见。
- 在线文档（docx / sheet / bitable / ...）和快捷方式即使本地完全没有同名文件，也**不会**进入删除候选，因为它们从来不进 `summary.uploaded` 的对齐域。

第 6 章里把 `+push --delete-remote` 标了 `high-risk-write`，CLI 这边的实现等价于"未传 `--yes` 时拒绝执行"，符合该约束的精神。

## 输出 schema

```json
{
  "summary": {
    "uploaded": 0,
    "skipped": 0,
    "failed": 0,
    "deleted_remote": 0
  },
  "items": [
    {"rel_path": "...", "file_token": "...", "action": "folder_created"},
    {"rel_path": "...", "file_token": "...", "action": "uploaded",       "size_bytes": 0},
    {"rel_path": "...", "file_token": "...", "action": "overwritten",    "version": "...", "size_bytes": 0},
    {"rel_path": "...", "file_token": "...", "action": "skipped",        "size_bytes": 0},
    {"rel_path": "...",                       "action": "failed",        "size_bytes": 0, "error": "..."},
    {"rel_path": "...", "file_token": "...", "action": "deleted_remote"},
    {"rel_path": "...", "file_token": "...", "action": "delete_failed",  "error": "..."}
  ]
}
```

`rel_path` 始终用 `/` 作为分隔符（跨平台一致）。

## 性能注意

- 上传流量 ≈ 本地待上传文件的总字节数。push 是**全量**上传 —— 跟 `+status` 不一样，不会按 hash 跳过"内容相同"的文件（status 是按 hash 比较，push 是按 `--if-exists`），所以一次跑可能很重。
- 想避免重跑全量，可以先 `+status` 找出 `new_local` 和 `modified`，再只对这些文件单独上传 / 覆盖。
- 大文件会用三段式分片上传（不会把整个 body 读进内存），但本地磁盘和上行带宽需要够。

## 所需 scope

| 操作 | scope | 是否在命令上预声明 |
|------|-------|-------------------|
| 列出文件夹 / 子目录 | `drive:drive.metadata:readonly` | ✅ 预声明 |
| 上传 / 覆盖文件 | `drive:file:upload` | ✅ 预声明 |
| 新建子目录（`create_folder`） | `space:folder:create` | ✅ 预声明 |
| 删除文件（仅 `--delete-remote --yes`） | `space:document:delete` | ❌ 不预声明 |

`drive:drive` 在部分企业被策略禁用，所以 +push 故意只声明上面这几条细粒度 scope。

> **关于 `space:document:delete`：** 框架的 scope 预检（`runner.go: checkShortcutScopes`）会在 `Validate` 和 `--dry-run` 之前就把命令上声明的 scope 全检查一遍；如果把删除 scope 也预声明，**普通上传或 dry-run** 都会因为没授权删除权限而被拦下来。所以这一项故意从命令的默认 Scopes 里**摘掉**，只在用户实际带 `--delete-remote --yes` 时才会撞上 —— 此时 runtime DELETE 调用会从后端拿到 `missing_scope` 结构化错误，提示去补 scope 后重跑。**用户在准备做镜像同步前，自己先用 `lark-cli auth login --scope space:document:delete` 一并授权**，可以避免半同步状态：先上传成功、再删除时报错。

## 范围限制

`--local-dir` 只接受 cwd 内的相对路径。CLI 会先 `EvalSymlinks` 整条路径，再判断它是否仍落在 cwd 内 —— **指向 cwd 外的符号链接也会被拒**，"在 cwd 内放一条软链指向外面" 这条捷径走不通，会直接撞上 `unsafe file path`。

如果用户想 push cwd 之外的目录，**不要 agent 自己 `cd` 绕过**。可以选：让用户在外部把 agent 工作目录切换到目标的祖先后重启会话；或者把目标整体物理移动 / 拷贝到 cwd 内（不是软链）；或者直接放弃这次同步，改用别的方式。

## 参考

- [lark-drive](../SKILL.md) —— 云空间全部命令
- [lark-shared](../../lark-shared/SKILL.md) —— 认证和全局参数
- [lark-drive-status](lark-drive-status.md) —— 上传前先看差异（避免全量回写）
- [lark-drive-pull](lark-drive-pull.md) —— Drive → 本地的对称命令
- [lark-drive-upload](lark-drive-upload.md) —— 单文件按需上传
