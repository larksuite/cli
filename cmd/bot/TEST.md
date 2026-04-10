# Bot 命令测试指南

## 静态代码验证 ✅

### 代码结构验证

| 检查项 | 状态 | 说明 |
|-------|------|------|
| **包声明** | ✅ | 所有文件正确声明 `package bot` |
| **导入** | ✅ | 导入 `cmdutil`, `cobra` 符合规范 |
| **函数签名** | ✅ | `NewCmdBot()` 与其他命令模式一致 |
| **命令注册** | ✅ | root.go 正确导入和注册 bot 命令 |
| **子命令** | ✅ | start/status/stop 三个子命令正确添加 |

### 代码质量验证

| 检查项 | 状态 | 说明 |
|-------|------|------|
| **版权声明** | ✅ | 所有文件包含 MIT 许可证头 |
| **命名规范** | ✅ | 遵循 Go 命名约定 |
| **注释** | ✅ | 公开函数有文档注释 |
| **错误处理** | ✅ | 使用 error 返回值 |

---

## 动态功能测试（需要 Go 环境）

### 前置条件

```bash
# 1. 安装 Go 1.23+
brew install go

# 2. 设置环境变量
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin
export PATH=$PATH:/usr/local/go/bin

# 3. 验证安装
go version
```

### 编译测试

```bash
# 进入项目目录
cd /Users/rongchuanxie/Documents/52VisionWorld/projects/cli

# 编译
go build -o /tmp/lark-cli ./cmd/root.go

# 验证二进制文件
/tmp/lark-cli --version
```

### 功能测试

#### 测试 1: 命令帮助

```bash
# 查看 bot 命令帮助
/tmp/lark-cli bot --help

# 预期输出：
# Claude Code Bot: integrate Lark with Claude Code for AI-powered conversations
#
# Usage:
#   lark-cli bot [command]
#
# Available Commands:
#   start    启动 Claude Code Bot
#   status   查看 Bot 运行状态
#   stop     停止运行中的 Bot
```

#### 测试 2: start 子命令

```bash
# 查看 start 帮助
/tmp/lark-cli bot start --help

# 预期输出：
# 启动 Claude Code Bot
# 启动飞书 Bot，监听消息并路由给 Claude Code 处理
#
# Flags:
#       --config string   配置文件路径
#       --daemon          后台运行模式
# -h, --help            help for start
```

#### 测试 3: status 子命令

```bash
# 查看 status 帮助
/tmp/lark-cli bot status --help

# 预期输出：
# 查看 Bot 运行状态
# 查看 Claude Code Bot 的运行状态、会话数、消息处理统计等
```

#### 测试 4: stop 子命令

```bash
# 查看 stop 帮助
/tmp/lark-cli bot stop --help

# 预期输出：
# 停止运行中的 Bot
# 优雅地停止 Claude Code Bot，保存会话状态
```

#### 测试 5: 实际启动（临时实现）

```bash
# 启动 Bot（会显示"功能正在开发中"）
/tmp/lark-cli bot start

# 预期输出：
# === Claude Code Bot 启动中 ===
# {"status":"not_implemented","message":"Bot 功能正在开发中，敬请期待"...}
#
# 按 Ctrl+C 停止 Bot
# === Bot 已停止 ===
```

---

## 当前实现状态

### ✅ 已实现

- [x] 命令框架结构
- [x] cobra 命令注册
- [x] 子命令定义（start/status/stop）
- [x] 帮助文档
- [x] 基础输出格式

### ⏳ 待实现（TODO 标记）

- [ ] 实际的 Bot 启动逻辑
- [ ] event +subscribe 集成
- [ ] session 管理
- [ ] Claude Code 集成
- [ ] 命令路由
- [ ] 配置文件解析
- [ ] daemon 模式
- [ ] PID 文件管理

---

## 验证清单

### 代码完整性 ✅

- [x] `cmd/bot/bot.go` - 主命令入口
- [x] `cmd/bot/start.go` - 启动子命令
- [x] `cmd/bot/status.go` - 状态子命令
- [x] `cmd/bot/stop.go` - 停止子命令
- [x] `cmd/root.go` - 命令注册

### Git 提交 ✅

- [x] 本地提交: `0c7ba35`
- [x] 推送到 GitHub: https://github.com/richardiitse/cli
- [x] 分支: feature/claude-code-bot

---

## 下一步

如果编译测试通过，可以继续：

1. **Phase 1 继续**: 集成 event +subscribe
2. **Phase 2**: 实现核心模块（session, claude, router）
3. **Phase 3**: 命令路由和扩展

---

**测试日期**: 2026-04-10
**测试状态**: 静态验证通过，动态测试需要 Go 环境
