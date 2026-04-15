# 测试计划：全局 --debug 标志

**功能：** 为 lark-cli 添加全局 `--debug` 标志，启用调试日志输出

**日期：** 2026-04-15

---

## 单元测试场景

- [ ] 场景：--debug 标志被正确解析为 true
  - 验证 GlobalOptions.Debug 在传入 `--debug` 时为 true
  - 测试文件：`cmd/global_flags_test.go` 或 `cmd/bootstrap_test.go`

- [ ] 场景：未指定 --debug 时默认为 false
  - 验证 GlobalOptions.Debug 在不传 `--debug` 时为 false

- [ ] 场景：Factory.Debugf() 在 DebugEnabled=true 时输出到 stderr
  - 验证调试消息出现在 IOStreams.ErrOut 中
  - 检查消息格式包含 `[DEBUG]` 前缀

- [ ] 场景：Factory.Debugf() 在 DebugEnabled=false 时不输出
  - 验证调试消息不出现在任何流中

- [ ] 场景：--debug 与其他全局标志兼容
  - 验证 `--debug --profile myprofile` 同时工作
  - 验证 `--profile myprofile --debug` 同时工作（顺序无关）

---

## E2E 测试场景

### 场景1：带 --debug 标志执行简单命令

- 设置：确保认证已配置
- 命令：`lark-cli --debug +calendar agenda`
- 断言：
  - 命令以 exit code 0 成功执行
  - stdout 包含有效的日程 JSON 或表格输出
  - stderr 可能包含调试信息（取决于实现）
- 清理：无

### 场景2：不带 --debug 标志执行相同命令

- 设置：同上
- 命令：`lark-cli +calendar agenda`
- 断言：
  - 命令成功执行
  - stdout 包含相同的输出
  - stderr 中没有 `[DEBUG]` 前缀的消息
- 清理：无

### 场景3：--debug 标志与 API 命令一起工作

- 设置：有效的认证配置
- 命令：`lark-cli --debug api GET /open-apis/contact/v3/users`
- 断言：
  - 返回 exit code 0
  - stdout 包含有效的 JSON API 响应
  - stderr 可能包含调试日志
- 清理：无

### 场景4：--debug 与 --profile 组合

- 设置：存在名为 "default" 的已配置 profile
- 命令：`lark-cli --debug --profile default +calendar agenda`
- 断言：
  - 命令使用指定的 profile 执行
  - 同时启用调试模式
  - exit code 0
- 清理：无

---

## 负面场景（错误处理）

### 错误场景1：--debug 放在命令后面（不是全局标志）

- 命令：`lark-cli +calendar --debug agenda`
- 断言：
  - `--debug` 被解释为 `agenda` 命令的参数
  - 不启用全局调试模式
  - 可能出现 "unknown flag" 错误或被忽略

### 错误场景2：--debug 与无效的命令组合

- 命令：`lark-cli --debug invalid-command`
- 断assert：
  - 返回非零 exit code
  - 显示 "unknown command" 错误

---

## e2e-tester 人工验收用例

### 用例1：全局 --debug 标志启用调试输出 (P0)

- 命令：`lark-cli --debug api GET /open-apis/contact/v3/users`
- 期望 stdout：有效的 JSON API 响应，包含用户信息
- 期望 stderr：可能包含调试信息或为空（取决于实现中是否有 Debugf 调用）
- 通过条件：
  - exit code 0
  - stdout 是有效的 JSON
  - 如果有调试信息，应包含 `[DEBUG]` 前缀

### 用例2：不使用 --debug 时没有调试输出 (P0)

- 命令：`lark-cli api GET /open-apis/contact/v3/users`
- 期望 stdout：有效的 JSON API 响应
- 期望 stderr：不包含 `[DEBUG]` 标记
- 通过条件：
  - exit code 0
  - 无调试前缀消息

### 用例3：--debug 与 --profile 组合使用 (P1)

- 命令：`lark-cli --debug --profile default api GET /open-apis/contact/v3/users`
- 期望 stdout：有效的 JSON API 响应
- 期望 stderr：可能包含调试信息
- 通过条件：
  - exit code 0
  - 正确识别并应用 profile
  - 调试模式被启用

### 用例4：短命令与 --debug (P1)

- 命令：`lark-cli --debug +calendar agenda`
- 期望 stdout：日程信息（JSON 或表格格式）
- 期望 stderr：可能包含调试日志
- 通过条件：
  - exit code 0
  - 返回正确的日程信息

---

## Skill 评测用例

不涉及 shortcut/skill/meta API 变更。

---

## 测试优先级

| 优先级 | 场景 |
|--------|------|
| P0 | 单元测试：标志解析（true/false） |
| P0 | 单元测试：Debugf 输出行为 |
| P0 | E2E：带 --debug 的命令成功执行 |
| P0 | E2E：不带 --debug 时无调试输出 |
| P1 | E2E：--debug 与其他标志兼容 |
| P1 | E2E：短命令与 --debug 工作 |
| P2 | 错误处理：--debug 放在错误位置 |

---

## 覆盖率目标

- 代码覆盖率：>= 80%（新增代码）
- 关键路径覆盖：100%（标志解析、Debugf 调用）
- E2E 场景覆盖：所有主要命令（api、calendar、drive 等）
