# --debug 标志实现计划

> **对于代理工作者：** 使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务执行此计划。步骤使用复选框 (`- [ ]`) 语法跟踪。

**目标：** 为 lark-cli 添加全局 `--debug` 标志，启用详细的调试日志输出到 stderr。

**架构：** 通过在 GlobalOptions 中添加布尔标志，在 bootstrap 期间解析，然后在 Factory 中使用 DebugEnabled 字段存储。任何有权访问 Factory 的命令都可以调用 `f.Debugf()` 方法输出调试信息。

**技术栈：** Go 1.21+, Cobra 框架, pflag 标志解析库

**测试计划来源：** `docs/superpowers/specs/2026-04-15-debug-flag-test-plan.md`

---

## 文件结构

### 需要修改的文件：
1. `cmd/global_flags.go` — 添加 `--debug` 标志定义
2. `cmd/bootstrap.go` — 在 bootstrap 期间捕获 Debug 值
3. `cmd/root.go` — 将全局选项连接到 Factory
4. `internal/cmdutil/factory.go` — 添加 DebugEnabled 字段和 Debugf() 方法
5. `internal/cmdutil/factory_default.go` — 创建 Factory 时初始化 DebugEnabled

### 需要创建的测试文件：
1. `cmd/global_flags_test.go` — 测试标志解析
2. `internal/cmdutil/factory_debug_test.go` — 测试 Debugf() 行为
3. `tests_e2e/cmd/2026_04_15_debug_flag_test.go` — E2E 测试（由 dev-e2e-testcase-writer 生成）

---

## 任务分解

### 任务 1：修改 global_flags.go 添加 Debug 字段

**文件：**
- Modify: `cmd/global_flags.go:10-17`

- [ ] **步骤 1：读取当前代码并理解结构**

```go
// 当前内容：
type GlobalOptions struct {
    Profile string
}

func RegisterGlobalFlags(fs *pflag.FlagSet, opts *GlobalOptions) {
    fs.StringVar(&opts.Profile, "profile", "", "use a specific profile")
}
```

- [ ] **步骤 2：修改 GlobalOptions 添加 Debug 字段**

在 `cmd/global_flags.go` 中修改 `GlobalOptions` 结构体，添加 `Debug` 字段：

```go
type GlobalOptions struct {
    Profile string
    Debug   bool
}
```

- [ ] **步骤 3：修改 RegisterGlobalFlags 注册 debug 标志**

在 `RegisterGlobalFlags` 函数中添加布尔标志注册：

```go
func RegisterGlobalFlags(fs *pflag.FlagSet, opts *GlobalOptions) {
    fs.StringVar(&opts.Profile, "profile", "", "use a specific profile")
    fs.BoolVar(&opts.Debug, "debug", false, "enable debug logging")
}
```

- [ ] **步骤 4：提交更改**

```bash
git add cmd/global_flags.go
git commit -m "feat: add debug field to GlobalOptions"
```

---

### 任务 2：修改 bootstrap.go 捕获 Debug 值

**文件：**
- Modify: `cmd/bootstrap.go:29`

- [ ] **步骤 1：修改 BootstrapInvocationContext 返回 Debug 值**

修改 `BootstrapInvocationContext` 函数，返回包含 Debug 值的扩展上下文。首先，需要扩展 `InvocationContext` 结构体（在 `internal/cmdutil/factory.go` 中），但由于我们不能修改那个包的导出类型而不影响其他代码，我们改为在 `cmd/root.go` 中直接处理全局选项。

实际上，不需要修改 bootstrap.go。我们在 root.go 的 Execute 函数中直接处理全局选项。

- [ ] **步骤 2：验证 bootstrap.go 当前行为**

验证 bootstrap.go 正确解析全局选项。由于 RegisterGlobalFlags 现在包含 debug 标志，bootstrap 会自动解析它。无需修改 bootstrap.go。

---

### 任务 3：修改 Factory 添加 DebugEnabled 字段和 Debugf 方法

**文件：**
- Modify: `internal/cmdutil/factory.go:32-46`

- [ ] **步骤 1：在 Factory 结构体添加 DebugEnabled 字段**

在 `internal/cmdutil/factory.go` 的 `Factory` 结构体中添加字段：

```go
type Factory struct {
    Config     func() (*core.CliConfig, error)
    HttpClient func() (*http.Client, error)
    LarkClient func() (*lark.Client, error)
    IOStreams  *IOStreams

    Invocation           InvocationContext
    Keychain             keychain.KeychainAccess
    IdentityAutoDetected bool
    ResolvedIdentity     core.Identity
    DebugEnabled         bool  // 新增字段

    Credential *credential.CredentialProvider
    FileIOProvider fileio.Provider
}
```

- [ ] **步骤 2：在 Factory 中添加 Debugf 方法**

在 `factory.go` 文件的末尾（在 `NewAPIClientWithConfig` 方法之后）添加 `Debugf` 方法：

```go
// Debugf writes debug output to stderr if debug mode is enabled.
// Each debug message is prefixed with [DEBUG] to distinguish it from regular output.
func (f *Factory) Debugf(format string, args ...interface{}) {
    if f == nil || !f.DebugEnabled || f.IOStreams == nil {
        return
    }
    msg := fmt.Sprintf("[DEBUG] "+format, args...)
    fmt.Fprintln(f.IOStreams.ErrOut, msg)
}
```

- [ ] **步骤 3：添加必要的导入**

在 `factory.go` 的导入部分中，确保已导入 `fmt`（应该已经导入了，但验证一下）。

- [ ] **步骤 4：提交更改**

```bash
git add internal/cmdutil/factory.go
git commit -m "feat: add DebugEnabled field and Debugf method to Factory"
```

---

### 任务 4：修改 root.go 连接全局选项到 Factory

**文件：**
- Modify: `cmd/root.go:92-100`

- [ ] **步骤 1：理解当前的 Execute 函数**

查看 `cmd/root.go` 的 `Execute` 函数，找到创建 Factory 的位置（大约第 92-100 行）。

当前代码：
```go
func Execute() int {
    inv, err := BootstrapInvocationContext(os.Args[1:])
    if err != nil {
        fmt.Fprintln(os.Stderr, "Error:", err)
        return 1
    }
    f := cmdutil.NewDefault(inv)

    globals := &GlobalOptions{Profile: inv.Profile}
    // ... 后续代码
}
```

- [ ] **步骤 2：修改 Execute 函数传递 Debug 值**

需要修改 Execute 函数以捕获和传递 debug 标志。首先，重新解析全局选项以获取 Debug 值：

```go
func Execute() int {
    inv, err := BootstrapInvocationContext(os.Args[1:])
    if err != nil {
        fmt.Fprintln(os.Stderr, "Error:", err)
        return 1
    }
    f := cmdutil.NewDefault(inv)

    // 解析全局选项以获取 debug 标志
    globals := &GlobalOptions{}
    globalFlags := &cobra.Command{}
    RegisterGlobalFlags(globalFlags.PersistentFlags(), globals)
    
    // 手动解析全局标志
    fs := pflag.NewFlagSet("global", pflag.ContinueOnError)
    fs.ParseErrorsAllowlist.UnknownFlags = true
    fs.SetOutput(io.Discard)
    RegisterGlobalFlags(fs, globals)
    fs.Parse(os.Args[1:])
    
    // 将 debug 值设置到 Factory
    f.DebugEnabled = globals.Debug
    
    // ... 后续代码
}
```

实际上，这会导致重复解析。更好的方法是修改 BootstrapInvocationContext 返回完整的全局选项。但为了最小化改动，我们可以简单地在 Execute 中再次解析（pflag 允许这样做）。

让我重新思考：最简单的方法是在 Execute 函数中简单地再解析一次，因为 pflag 足够智能可以处理这个。

- [ ] **步骤 3：正确的修改方式**

修改 `cmd/root.go` 的 `Execute` 函数。找到以下行：

```go
globals := &GlobalOptions{Profile: inv.Profile}
```

修改为：

```go
// 解析全局选项（包括 debug 标志）
globals := &GlobalOptions{Profile: inv.Profile}
globalFS := pflag.NewFlagSet("globals", pflag.ContinueOnError)
globalFS.ParseErrorsAllowlist.UnknownFlags = true
globalFS.SetOutput(io.Discard)
RegisterGlobalFlags(globalFS, globals)
globalFS.Parse(os.Args[1:])

// 将 debug 标志设置到 Factory
f.DebugEnabled = globals.Debug
```

或者，更简洁的方法：使用现有的 bootstrap 逻辑。让我修改 BootstrapInvocationContext 返回完整的 GlobalOptions。

实际上，最小改动是直接在 Execute 中进行额外的解析。让我这样做：

```go
func Execute() int {
    inv, err := BootstrapInvocationContext(os.Args[1:])
    if err != nil {
        fmt.Fprintln(os.Stderr, "Error:", err)
        return 1
    }
    f := cmdutil.NewDefault(inv)

    // 解析全局标志以获取 debug 设置
    var debugFlag bool
    debugFS := pflag.NewFlagSet("debug", pflag.ContinueOnError)
    debugFS.ParseErrorsAllowlist.UnknownFlags = true
    debugFS.SetOutput(io.Discard)
    debugFS.BoolVar(&debugFlag, "debug", false, "")
    debugFS.Parse(os.Args[1:])
    f.DebugEnabled = debugFlag

    globals := &GlobalOptions{Profile: inv.Profile}
    // ... 后续代码
```

这个方法虽然简洁，但有点重复。更优雅的做法是扩展 InvocationContext。让我检查是否可以这样做而不破坏其他代码。

为了保持改动最小，我将直接在 root.go 的 Execute 函数中再做一次全局标志解析。这是合理的，因为 bootstrap.go 只提取必要的参数给 credential 初始化使用。

实际实现：

```go
func Execute() int {
    inv, err := BootstrapInvocationContext(os.Args[1:])
    if err != nil {
        fmt.Fprintln(os.Stderr, "Error:", err)
        return 1
    }
    f := cmdutil.NewDefault(inv)

    // 解析全局标志，特别是 --debug
    globals := &GlobalOptions{Profile: inv.Profile}
    {
        fs := pflag.NewFlagSet("global", pflag.ContinueOnError)
        fs.ParseErrorsAllowlist.UnknownFlags = true
        fs.SetOutput(io.Discard)
        RegisterGlobalFlags(fs, globals)
        fs.Parse(os.Args[1:])
        f.DebugEnabled = globals.Debug
    }

    rootCmd := &cobra.Command{
        // ... 现有代码
    }
    // ... 后续代码
}
```

- [ ] **步骤 4：验证修改不会破坏现有功能**

确保添加的代码不会影响 Profile 的处理（它应该已经在 bootstrap 中处理过了）。

- [ ] **步骤 5：提交更改**

```bash
git add cmd/root.go
git commit -m "feat: pass debug flag from global options to Factory"
```

---

### 任务 5：创建 Factory 调试功能单元测试

**文件：**
- Create: `internal/cmdutil/factory_debug_test.go`

- [ ] **步骤 1：创建测试文件**

创建新文件 `internal/cmdutil/factory_debug_test.go` 包含 Factory 的 Debugf 方法测试：

```go
// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
    "bytes"
    "testing"
)

// TestDebugfWhenEnabled verifies Debugf outputs to stderr when DebugEnabled is true.
func TestDebugfWhenEnabled(t *testing.T) {
    buf := &bytes.Buffer{}
    f := &Factory{
        DebugEnabled: true,
        IOStreams: &IOStreams{
            ErrOut: buf,
        },
    }

    f.Debugf("test message %d", 42)

    output := buf.String()
    if !contains(output, "[DEBUG]") {
        t.Errorf("expected [DEBUG] prefix in output, got: %s", output)
    }
    if !contains(output, "test message 42") {
        t.Errorf("expected formatted message in output, got: %s", output)
    }
}

// TestDebugfWhenDisabled verifies Debugf outputs nothing when DebugEnabled is false.
func TestDebugfWhenDisabled(t *testing.T) {
    buf := &bytes.Buffer{}
    f := &Factory{
        DebugEnabled: false,
        IOStreams: &IOStreams{
            ErrOut: buf,
        },
    }

    f.Debugf("test message %d", 42)

    if buf.Len() > 0 {
        t.Errorf("expected no output when debug disabled, got: %s", buf.String())
    }
}

// TestDebugfWithNilIOStreams verifies Debugf doesn't panic when IOStreams is nil.
func TestDebugfWithNilIOStreams(t *testing.T) {
    f := &Factory{
        DebugEnabled: true,
        IOStreams:    nil,
    }

    // Should not panic
    f.Debugf("test message")
}

// TestDebugfWithNilFactory verifies Debugf doesn't panic when called on nil Factory.
func TestDebugfWithNilFactory(t *testing.T) {
    var f *Factory

    // Should not panic
    f.Debugf("test message")
}

// TestDebugfFormat verifies Debugf correctly formats the message.
func TestDebugfFormat(t *testing.T) {
    buf := &bytes.Buffer{}
    f := &Factory{
        DebugEnabled: true,
        IOStreams: &IOStreams{
            ErrOut: buf,
        },
    }

    f.Debugf("value: %s, number: %d", "test", 123)

    output := buf.String()
    expected := "[DEBUG] value: test, number: 123"
    if output != expected+"\n" {
        t.Errorf("expected %q, got %q", expected+"\n", output)
    }
}

// Helper function
func contains(s, substr string) bool {
    for i := 0; i <= len(s)-len(substr); i++ {
        if s[i:i+len(substr)] == substr {
            return true
        }
    }
    return false
}
```

- [ ] **步骤 2：运行测试确保通过**

```bash
go test ./internal/cmdutil -run TestDebugf -v
```

期望：所有测试通过

- [ ] **步骤 3：提交测试文件**

```bash
git add internal/cmdutil/factory_debug_test.go
git commit -m "test: add Debugf method tests"
```

---

### 任务 6：创建全局标志解析单元测试

**文件：**
- Create: `cmd/global_flags_test.go`

- [ ] **步骤 1：创建测试文件**

创建新文件 `cmd/global_flags_test.go`：

```go
// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
    "testing"

    "github.com/spf13/pflag"
)

// TestDebugFlagDefault verifies --debug flag defaults to false.
func TestDebugFlagDefault(t *testing.T) {
    opts := &GlobalOptions{}
    fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
    RegisterGlobalFlags(fs, opts)

    if err := fs.Parse([]string{}); err != nil {
        t.Fatalf("parse failed: %v", err)
    }

    if opts.Debug != false {
        t.Errorf("expected Debug=false, got %v", opts.Debug)
    }
}

// TestDebugFlagParsedTrue verifies --debug flag is parsed as true.
func TestDebugFlagParsedTrue(t *testing.T) {
    opts := &GlobalOptions{}
    fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
    RegisterGlobalFlags(fs, opts)

    if err := fs.Parse([]string{"--debug"}); err != nil {
        t.Fatalf("parse failed: %v", err)
    }

    if opts.Debug != true {
        t.Errorf("expected Debug=true, got %v", opts.Debug)
    }
}

// TestDebugFlagWithProfile verifies --debug works together with --profile.
func TestDebugFlagWithProfile(t *testing.T) {
    opts := &GlobalOptions{}
    fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
    RegisterGlobalFlags(fs, opts)

    if err := fs.Parse([]string{"--debug", "--profile", "myprofile"}); err != nil {
        t.Fatalf("parse failed: %v", err)
    }

    if opts.Debug != true {
        t.Errorf("expected Debug=true, got %v", opts.Debug)
    }
    if opts.Profile != "myprofile" {
        t.Errorf("expected Profile=myprofile, got %v", opts.Profile)
    }
}

// TestDebugFlagReversedOrder verifies flags work in any order.
func TestDebugFlagReversedOrder(t *testing.T) {
    opts := &GlobalOptions{}
    fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
    RegisterGlobalFlags(fs, opts)

    if err := fs.Parse([]string{"--profile", "myprofile", "--debug"}); err != nil {
        t.Fatalf("parse failed: %v", err)
    }

    if opts.Debug != true {
        t.Errorf("expected Debug=true, got %v", opts.Debug)
    }
    if opts.Profile != "myprofile" {
        t.Errorf("expected Profile=myprofile, got %v", opts.Profile)
    }
}
```

- [ ] **步骤 2：运行测试确保通过**

```bash
go test ./cmd -run TestDebugFlag -v
```

期望：所有测试通过

- [ ] **步骤 3：提交测试文件**

```bash
git add cmd/global_flags_test.go
git commit -m "test: add debug flag parsing tests"
```

---

### 任务 7：运行现有测试确保未破坏任何功能

**文件：** （不修改）

- [ ] **步骤 1：运行全部单元测试**

```bash
make test
```

或

```bash
go test ./... -v
```

期望：所有现有测试通过

如果有失败，仔细阅读失败信息，修复代码。

- [ ] **步骤 2：运行代码验证（如果项目有 make validate）**

```bash
make validate
```

期望：所有检查通过

- [ ] **步骤 3：如果有集成测试，运行它们**

```bash
go test ./tests_integration/... -v 2>/dev/null || echo "No integration tests"
```

---

### 任务 8（最终）：E2E 验收验证

这个任务不是通过编写代码实现的，而是运行验收检查以验证完整的实现。

- [ ] **步骤 1：运行 make validate**

```bash
make validate
```

期望：所有检查通过（构建、vet、单元测试、集成测试、安全测试、约定检查）

- [ ] **步骤 2：运行 E2E 测试**

E2E 测试代码在第 3 阶段由 dev-e2e-testcase-writer 编写。现在针对完成的实现运行它们：

```bash
go test ./tests_e2e/cmd/... -count=1 -timeout=3m -v
```

期望：所有测试通过（绿色）

如果任何测试失败：
- 读取失败输出
- 修复失败的代码（不是测试——测试反映规范）
- 重新运行仅失败的测试：`go test ./tests_e2e/cmd/... -run TestXxx`
- 最多重试 3 轮，如果仍失败则上报给人工

- [ ] **步骤 3：手动验证 --debug 标志工作正常**

```bash
# 测试带 --debug 的命令输出调试信息
./lark-cli --debug api GET /open-apis/contact/v3/users 2>&1 | grep -q "\[DEBUG\]" && echo "PASS: debug flag works" || echo "FAIL: no debug output"

# 测试不带 --debug 的命令不输出调试信息
./lark-cli api GET /open-apis/contact/v3/users 2>&1 | grep -q "\[DEBUG\]" && echo "FAIL: debug output found when not enabled" || echo "PASS: no debug output when disabled"
```

- [ ] **步骤 4：汇总结果给人工确认**

准备以下内容：
- 变更摘要（修改的文件、增删行数）
- make validate 结果
- E2E 测试结果（`go test` 输出）
- PR 描述草稿

**停止。等待人工批准后再创建 PR。**

---

## 验收标准检查清单

在提交给人工之前，自检以下内容：

✅ 修改了 `cmd/global_flags.go` — 添加了 Debug 字段和标志注册
✅ 修改了 `cmd/root.go` — 在 Execute 中连接 debug 值到 Factory
✅ 修改了 `internal/cmdutil/factory.go` — 添加了 DebugEnabled 字段和 Debugf 方法
✅ 创建了 `cmd/global_flags_test.go` — 覆盖标志解析场景
✅ 创建了 `internal/cmdutil/factory_debug_test.go` — 覆盖 Debugf 输出行为
✅ 所有单元测试通过
✅ make validate 通过
✅ E2E 测试已由 dev-e2e-testcase-writer 生成并通过
✅ 代码向后兼容（--debug 默认为 false）
