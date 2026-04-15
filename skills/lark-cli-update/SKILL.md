---
name: lark-cli-update
version: 1.0.0
description: "升级 lark-cli 本体与全部 skill。当用户需要检查新版本、升级 lark-cli 命令行工具、刷新 skills、或修复损坏安装时使用。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli --version"
---

# lark-cli-update (v1)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含通用约定。**

本 skill 是 stateless runbook：每次调用都从零探测，不读写任何本地状态文件。步骤天然幂等，中断后重跑即可恢复。

## Intent Mapping

本 skill 使用**语意匹配**而不是固定字符串匹配。AI 根据用户意图对照下表选择执行路径。

| 用户意图（举例） | 执行路径 |
|------------------|---------|
| "升级 lark-cli" / "update to latest" / "装最新版" | 完整 4 步流程 |
| "看看有没有新版本" / "check for updates" / "当前版本多少" | Step 1+2，停在版本对比 |
| "只更新 skill" / "refresh skills" / "skill 旧了" | 跳 Step 3 binary 升级，只跑 `skills add` |
| "重装" / "强制升级" / "lark-cli 坏了" | 跳 Step 2 版本检查，直接 Step 3 |

**不确定时选保守路径**（check-only），向用户确认后再升级。

## Step 1 — Detect install method

执行以下 bash 命令并按规则归类：

```bash
LARK_CLI_PATH=$(which lark-cli 2>/dev/null || true)
NPM_BIN_DIR="$(dirname "$(npm root -g 2>/dev/null)")/../bin" 2>/dev/null || echo ""
```

| 信号 | 判定 |
|------|------|
| `LARK_CLI_PATH` 为空 | `not_installed` — 跳到错误处理：报"未检测到 lark-cli"，给出 README 的初装命令，终止 |
| `LARK_CLI_PATH` 在 `npm root -g` 的 `../bin` 目录中（即 `$LARK_CLI_PATH == $NPM_BIN_DIR/lark-cli`） | `npm` |
| 路径可以追溯到含 `.git` + `Makefile`（其中有 `install:` target）的目录（例如 `$GOPATH/bin/lark-cli` 对应某个 clone 出来的 lark-cli repo） | `source` |
| 其它路径 | `unknown` — 询问用户安装方式，不要瞎猜 |

将判定结果记在对话中，供后续步骤引用。

## Step 2 — Version check

**如果用户意图是 `force` 路径，跳过本步直接进 Step 3。**

当前版本（所有模式共用）：

```bash
CURRENT_VERSION=$(lark-cli --version 2>&1 | head -1)
```

最新版本：

| Install method | 命令 |
|----------------|------|
| `npm` | `LATEST_VERSION=$(npm view @larksuite/cli version 2>&1)` |
| `source` | 询问用户 repo 路径（记为 `$REPO`），然后 `git -C "$REPO" fetch origin && LATEST_COMMITS_AHEAD=$(git -C "$REPO" rev-list --count HEAD..@{u})` |

**比较：**

- 若 `CURRENT_VERSION == LATEST_VERSION`（或 source 模式下 `LATEST_COMMITS_AHEAD == 0`）且用户意图**不是** `force` / `skills-only`：
  - 输出 "already latest" JSON（见 Output Contract 中的 no-op 形式），stderr 提示"如需同时刷新 skills，告知我"
  - 终止流程
- 若用户意图是 `check-only`：无论版本是否相同，输出对比结果后终止
- 否则进入 Step 3

## Step 3 — Execute upgrade

**破坏性操作。执行前 MUST 把即将运行的命令完整打印到 stderr，让用户看得到。**

| Install method | Binary 升级 | Skills 刷新 |
|----------------|-------------|-------------|
| `npm` | `npm install -g @larksuite/cli@latest` | `npx skills add larksuite/cli -y -g --force` |
| `source` | 打印 `cd <repo> && git pull && make install`；等待用户显式确认（回复 "yes" / "go" / "继续"）后执行 | `npx skills add larksuite/cli -y -g --force` |

**意图分支：**

- `skills-only`：跳过"Binary 升级"列，只跑"Skills 刷新"列
- `binary-only`：只跑"Binary 升级"列
- 默认：两列都跑

**Source 模式安全约束：**

- **禁止**自动 `git stash` / `git reset`。若 `git status` 显示本地未提交改动，立即停手，把 diff 报给用户
- **禁止**在未询问路径时假设 repo 位置
- `make install` 的目标路径可能需要 sudo；若无写权限，原样透传错误并加 hint

**npm 模式失败处理：**

- `npm install -g` 报权限错 → 透传 stderr，加 hint "可能需要 sudo 或修复 npm prefix"
- 网络失败 → 透传 stderr，不自动重试

## Step 4 — Verify

```bash
AFTER_VERSION=$(lark-cli --version 2>&1 | head -1)
```

- 若 `AFTER_VERSION` 执行失败 → 报"升级完成但 binary 无法执行"，给出 before/after 路径，建议回滚（npm 模式：`npm install -g @larksuite/cli@<before>`）
- 成功则构造 Output Contract 中定义的 JSON 结构，写到 stdout

Skills 刷新的验证：`ls $(npm root -g)/@larksuite/cli/skills 2>/dev/null | wc -l`，期望 >= 22。失败不阻塞 binary 升级成功的上报，但在 JSON 的 `skills.action` 标为 `failed` 并附 reason。

## Output Contract

遵循 lark-cli 约定：stdout 是结构化数据，stderr 是进度 / 提示 / 即将执行的命令预览。

### 成功（升级发生）

```json
{
  "install_method": "npm",
  "binary": {"before": "1.2.3", "after": "1.3.0", "action": "upgraded"},
  "skills": {"action": "refreshed", "count": 22}
}
```

### No-op（已是最新 / check-only / 部分跳过）

```json
{
  "install_method": "npm",
  "binary": {"before": "1.3.0", "after": "1.3.0", "action": "skipped", "reason": "already-latest"},
  "skills": {"action": "skipped", "reason": "binary-only-intent"}
}
```

### 字段说明

| 字段 | 值 |
|------|----|
| `install_method` | `npm` / `source` / `unknown` |
| `binary.action` | `upgraded` / `skipped` / `failed` |
| `binary.reason` | 当 `action != upgraded` 时必填 |
| `skills.action` | `refreshed` / `skipped` / `failed` |
| `skills.count` | 刷新后实际文件数（仅 `refreshed` 时有意义） |

## Error Handling

| 场景 | 处理 |
|------|------|
| `which lark-cli` 为空 | 报"lark-cli 未安装"；给 README 的**初装**命令（不是 upgrade）；退出 |
| Source 模式找不到 repo 路径 | 询问用户，不要猜测 |
| `npm install -g` 权限错 | 透传 npm stderr；hint "retry with sudo or fix npm prefix" |
| Source `git pull` 有本地改动 | 报告 diff；拒绝继续；**不**自动 stash |
| 升级后 `lark-cli --version` 失败 | 报 before/after 路径；建议回滚；标记 `binary.action = failed` |
| Skills 刷新失败 | binary 升级结果保留；`skills.action = failed` 附 reason |

**共同原则：** 透传底层工具的 stderr，不吞错。AI 看到原始错误文本才能做下一步判断。

## Idempotency & Resumption

所有步骤幂等：

- `npm install -g pkg@latest` 重复调用安全
- `npx skills add ... --force` 覆盖式写入
- `git pull` + `make install` 重复调用安全（前提：无本地未提交改动）
- 版本检查是纯读操作

**中断后如何恢复：**

用户 Ctrl-C 打断后，重跑本 skill 即可。AI **不得**依赖任何本地缓存 / 状态文件。每次调用都从 Step 1 重新探测——状态可能已经变了（部分安装 / 路径改变 / 权限变化）。

**不要**在本 skill 中引入 lockfile、checkpoint 或"resume from step N"逻辑。正确性优先于花哨恢复。

## Security

- 每条破坏性命令执行前 MUST 写到 stderr 让用户预览
- Source 模式的 `git pull` / `make install` 必须经用户显式确认才跑；**禁止**自动 mutate git 工作区
- 不引入新的网络端点；所有网络调用（`npm install`、`npm view`、`git fetch`）都是已有安装路径中已经存在的
- 不读写凭证 / 令牌；lark-cli 的 credential 存储保持不变
