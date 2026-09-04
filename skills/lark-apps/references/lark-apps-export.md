# apps +export

`+export` 把妙搭应用的源码打成 zip 下载到本地。运行时命令事实以 `lark-cli apps +export --help` 为准。

## 何时用

只要一份源码快照的场景：读代码、审计、归档、做静态分析、把源码喂给别的工具。

**跨应用是它相对 `+init` 的核心价值**：创意应用的分享链接（`/page/<token>`）指向别人的应用，你对那个仓库没有权限，`git clone` 走不通；`+export` 只要求你对该应用有下载权限。

## 不要用它的时候

要继续开发就用 `+init`，不要用 `+export` 再手动 `git init`。两者产出不同：

| | `+export` | `+init` |
|---|---|---|
| 产出 | 一个 zip | 完整 git 工作区 |
| Git 凭证 | 不配 | 配好，可 push |
| 本地环境变量 | 不拉 | 拉 `.env.local` |
| 前提 | 对应用有下载权限 | 对**仓库**有权限 |

用 `+export` 拿到的目录没有 git 历史、没有远端、没有凭证，改完发不回去。

## 导出的是「最后一次提交」，不是沙箱当前状态

服务端对远端仓库跑 `git archive`，从不读沙箱文件系统。用户在沙箱里改了文件但没触发 checkpoint 或发布，**那些改动不在归档里**。

这是设计如此，不是缺陷。若导出结果看起来"少了刚写的代码"，先确认改动是否已提交，而不是重试导出。

## 命令骨架

- `--app-id` 与 `--meta-token` **恰传其一**：前者是自己的应用，后者是分享链接里的 token。
  服务端两者共用同一个 path 段、按 `app_` 前缀自行判别（与 `+get` 同契约），所以 flag 只是语义标注，
  不会因为"放错 flag"而失败。
  - 两者都只收**裸标识符**。拿到的是整条链接（`.../app/<app_id>` 或 `.../page/<token>`）时，
    只传最后一段——整条 URL 传进来会被本地拦下并提示，不会变成一个看起来像"应用不存在"的 404。
- `--checkpoint-id` 可选，**正整数**，指定导出某个检查点；省略取默认分支最新提交（不要显式传 `0`）。
- `--output` 可选，相对当前目录；省略时用服务端给的文件名（通常是 `<app_id>.zip`）。

## 示例

```bash
lark-cli apps +export --app-id app_xxx --output ./src.zip
lark-cli apps +export --app-id app_xxx                      # 存成 ./app_xxx.zip
lark-cli apps +export --meta-token <share-token>            # 别人分享给你的应用
lark-cli apps +export --app-id app_xxx --checkpoint-id 42
lark-cli apps +export --app-id app_xxx --dry-run
```

## 输出契约

- 成功时 stdout 是 JSON envelope，含 `output`（落盘的绝对路径）与 `size_bytes`；传了 `--app-id` 时还会回显 `app_id`。
- 归档以流式写盘，不会整包驻留内存，大仓库也安全。
- 失败时不会留下半个文件。

## 错误处理

| 情况 | 怎么办 |
|---|---|
| 提示代码不在 git（422） | 该应用类型（存量静态 HTML）的产物存在文件存储里，不在 git。改用 `+file-list` / `+file-download`，重试无用 |
| 权限不足（403） | 你需要该应用的下载权限。**持有分享 token 不等于有权限** |
| 应用不存在（404） | 用 `+list --keyword <name>` 核对 app_id |
| 归档过大（413） | 超出导出体积上限，改用 `+git-credential-init` + 原生 git clone |
| 参数报错 | `--app-id` 与 `--meta-token` 只能给一个，且必须给一个；两者都要裸标识符（不是整条链接），`--checkpoint-id` 要正整数 |
