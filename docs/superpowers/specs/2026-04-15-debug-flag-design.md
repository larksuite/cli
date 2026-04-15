# 设计文档：全局 --debug 标志

**任务：** 为 lark-cli 添加全局 `--debug` 标志，启用详细的调试日志输出

**日期：** 2026-04-15

---

## 需求概述

用户需要能够通过 `--debug` 全局标志运行任何 lark-cli 命令，以启用详细的调试日志输出到 stderr。这将帮助用户诊断问题和理解命令执行流程。

**使用示例：**
```bash
lark-cli --debug +calendar agenda
lark-cli --debug --profile myprofile drive files list
```

---

## 架构设计

### 全局标志解析

**文件：** `cmd/global_flags.go`

在 `GlobalOptions` 结构体中添加 `Debug` 布尔字段，并在 `RegisterGlobalFlags` 函数中注册标志：

```go
type GlobalOptions struct {
    Profile string
    Debug   bool
}

func RegisterGlobalFlags(fs *pflag.FlagSet, opts *GlobalOptions) {
    fs.StringVar(&opts.Profile, "profile", "", "use a specific profile")
    fs.BoolVar(&opts.Debug, "debug", false, "enable debug logging")
}
```

### Factory 扩展

**文件：** `internal/cmdutil/factory.go`

在 `Factory` 结构体中添加 `DebugEnabled` 字段，该字段在命令初始化时从 `GlobalOptions.Debug` 设置。

### 调试输出辅助函数

在 `internal/cmdutil/factory.go` 或 `internal/cmdutil/iostreams.go` 中添加简单的调试输出函数：

```go
func (f *Factory) Debugf(format string, args ...interface{}) {
    if f == nil || !f.DebugEnabled || f.IOStreams == nil || f.IOStreams.ErrOut == nil {
        return
    }
    msg := fmt.Sprintf("[DEBUG] "+format, args...)
    fmt.Fprintln(f.IOStreams.ErrOut, msg)
}
```

这样任何有权访问 Factory 的命令都可以调用 `f.Debugf(...)` 来输出调试信息到 stderr。

### 数据流

```text
1. 用户运行：lark-cli --debug +calendar agenda
                           ↓
2. Cobra 解析 --debug 标志到 GlobalOptions.Debug = true
                           ↓
3. cmd/root.go 创建 Factory，设置 f.DebugEnabled = globals.Debug
                           ↓
4. 命令执行时可调用 f.Debugf("message")
                           ↓
5. 如果 DebugEnabled 为 true，消息输出到 stderr；否则不输出
```

---

## 修改范围

### 1. cmd/global_flags.go
- 在 `GlobalOptions` 添加 `Debug bool` 字段
- 在 `RegisterGlobalFlags` 添加布尔标志注册

### 2. internal/cmdutil/factory.go
- 在 `Factory` 结构体添加 `DebugEnabled bool` 字段
- 添加 `Debugf()` 方法

### 3. cmd/root.go
- 在 `Execute()` 函数中，将 `globals.Debug` 赋值给 `f.DebugEnabled`

### 4. 现有命令（可选）
- 如果需要，可在命令中添加 `f.Debugf()` 调用以输出有用的调试信息
- 这不是强制要求，但可以帮助用户诊断问题

---

## 设计决策

**为什么使用 Factory 中的 DebugEnabled？**
- Factory 已经被传递到整个命令层次结构
- 遵循现有的依赖注入模式
- 易于测试（可以在测试中模拟 DebugEnabled）
- 避免全局状态

**为什么输出到 stderr？**
- 调试信息不是命令的主要输出
- 分离调试日志和命令输出，使脚本处理变得容易
- 允许用户使用 `>` 和 `2>` 分别重定向输出

**为什么使用 [DEBUG] 前缀？**
- 清楚地标识调试消息
- 易于在输出中识别
- 便于脚本过滤（例如 `grep -F "[DEBUG]"`）

---

## 测试策略

见 `2026-04-15-debug-flag-test-plan.md`

---

## 实现步骤（高级）

1. 修改 `cmd/global_flags.go` 添加调试标志
2. 修改 `internal/cmdutil/factory.go` 添加 DebugEnabled 和 Debugf()
3. 修改 `cmd/root.go` 将全局选项连接到 Factory
4. 编写单元测试和 E2E 测试
5. 验收测试（e2e-tester）

---

## 向后兼容性

该功能是完全向后兼容的：
- 默认情况下 `--debug` 为 false（未指定时）
- 现有命令不需要任何更改
- 现有脚本不受影响

---

## 后续改进（不在本次范围内）

- 在各个命令中添加更多 `f.Debugf()` 调用
- 支持多个调试级别（`--debug=verbose` 等）
- 支持调试日志到文件
- 支持环境变量启用调试
