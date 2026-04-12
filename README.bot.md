# lark-cli - Claude Code Bot Edition

> **原项目**: [larksuite/cli](https://github.com/larksuite/cli) - 飞书官方 CLI 工具
> 
> **Fork 目的**: 扩展飞书 Bot 功能，集成 Claude Code，实现"飞书 → Claude Code"的智能助手

---

## 🎯 项目目标

实现一个飞书 Bot，用户可以通过飞书消息调用 Claude Code 完成开发任务：

- 💬 **自然对话**: 在飞书中与 Claude Code 对话
- 🔄 **多轮对话**: 保持会话上下文
- 🛠️ **命令模式**: 支持 `/run`, `/deploy`, `/status` 等快捷命令
- 📁 **文件操作**: 上传文件、下载生成结果
- 👥 **多用户**: 支持多用户独立会话

---

## ✨ 新增功能

### `lark-cli bot` 子命令

```bash
# 启动 Bot
lark-cli bot start [--config] [--daemon]

# 查看状态
lark-cli bot status

# 停止 Bot
lark-cli bot stop
```

### 核心特性

- ✅ **会话管理**: 每个聊天独立的 session_id 持久化
- ✅ **Claude Code 集成**: 调用 `claude -p --resume` 进行对话
- ✅ **命令路由**: 支持斜杠命令和自然语言
- ✅ **生产级部署**: 支持 pm2/systemd 守护进程
- ✅ **配置管理**: YAML 配置文件支持

---

## 🚀 快速开始

### 1. 安装依赖

```bash
# 需要 lark-cli (Go 1.23+)
go install github.com/richardiitse/cli@latest

# 需要 Claude Code CLI
npm install -g @anthropic-ai/claude-code
```

### 2. 配置飞书应用

```bash
# 初始化 lark-cli 配置
echo "YOUR_APP_SECRET" | lark-cli config init --app-id "cli_xxx" --app-secret-stdin
```

### 3. 启动 Bot

```bash
# 基础启动
lark-cli bot start

# 使用配置文件
lark-cli bot start --config ~/.lark-cli/bot-config.yaml

# 后台运行
lark-cli bot start --daemon
```

### 4. 在飞书中使用

```
你: 帮我写一个 Python 函数计算斐波那契数列
Bot: [Claude Code 生成的代码和解释]

你: 这个函数有 bug，帮我修复
Bot: [Claude Code 分析并修复 bug]

你: /run tests
Bot: [执行测试并返回结果]
```

---

## 📋 配置文件

```yaml
# ~/.lark-cli/bot-config.yaml
claude:
  work_dir: ~/projects              # Claude Code 工作目录
  system_prompt: "你是一个智能助手"
  max_sessions: 100                 # 最大会话数
  session_ttl: 24h                  # 会话过期时间

lark:
  app_id: cli_xxx                   # 飞书应用 ID
  app_secret: xxx                   # 飞书应用密钥

features:
  enable_commands: true             # 启用命令模式
  enable_file_ops: true             # 启用文件操作
  allowed_users:                    # 允许的用户列表
    - ou_xxx
    - ou_yyy

logging:
  level: info
  format: json
  output: /var/log/lark-bot.log
```

---

## 🏗️ 架构设计

```
飞书用户消息
    ↓
lark-cli event +subscribe (WebSocket 长连接)
    ↓
bot/handler.go (消息处理器)
    ↓
bot/router.go (命令路由)
    ↓
bot/claude.go (Claude Code 集成)
    ↓
bot/session.go (会话管理)
    ↓
lark-cli im +messages-send (回复飞书)
```

### 核心模块

| 模块 | 文件 | 功能 |
|------|------|------|
| **命令入口** | `cmd/bot/` | bot 子命令定义 |
| **消息处理** | `shortcuts/bot/handler.go` | 消息事件处理 |
| **会话管理** | `shortcuts/bot/session.go` | session_id 持久化 |
| **Claude 集成** | `shortcuts/bot/claude.go` | 调用 Claude Code |
| **命令路由** | `shortcuts/bot/router.go` | 斜杠命令路由 |
| **配置管理** | `internal/bot/config.go` | 配置文件解析 |

---

## 🔧 开发计划

### Phase 1: 核心 Bot 框架 ✅ 进行中
- [x] 创建 `cmd/bot/` 目录结构
- [ ] 实现 `bot start` 子命令
- [ ] 集成 `event +subscribe`
- [ ] 实现基础消息处理循环

### Phase 2: Claude Code 集成
- [ ] 实现 `shortcuts/bot/claude.go`
- [ ] 调用 `claude -p --resume`
- [ ] 解析 JSON 输出
- [ ] 错误处理和重试

### Phase 3: 命令路由和扩展
- [ ] 支持斜杠命令
- [ ] 命令白名单机制
- [ ] Shortcut 框架集成

### Phase 4: 测试和优化
- [ ] 单元测试
- [ ] 集成测试
- [ ] 性能优化

---

## 📚 文档

- [Bot 集成方案](docs/bot-integration-plan.md) - 完整的技术方案设计
- [lark-cli 原始文档](README.md) - 上游项目文档
- [Claude Code 文档](https://docs.anthropic.com/claude-code) - Claude Code CLI 文档

---

## 🤝 贡献

这个 fork 专注于 Claude Code Bot 功能的开发。欢迎：

1. **Bug 报告**: 提交 Issue 描述问题
2. **功能建议**: 提交 Issue 说明需求
3. **代码贡献**: 提交 Pull Request

### 开发流程

```bash
# 1. 克隆仓库
git clone https://github.com/richardiitse/cli.git
cd cli

# 2. 创建功能分支
git checkout -b feature/your-feature

# 3. 开发和测试
go build ./...
./lark-cli bot start

# 4. 提交更改
git add .
git commit -m "feat: add your feature"

# 5. 推送到 fork
git push fork feature/your-feature
```

---

## 📄 许可证

MIT License (继承自 larksuite/cli)

---

## 🔗 相关链接

- **上游仓库**: https://github.com/larksuite/cli
- **Fork 仓库**: https://github.com/richardiitse/cli
- **开发分支**: feature/claude-code-bot
- **Claude Code**: https://docs.anthropic.com/claude-code
- **飞书开放平台**: https://open.feishu.cn/

---

## 📮 联系方式

- **GitHub**: [@richardiitse](https://github.com/richardiitse)
- **项目**: 飞书 Bot + Claude Code 集成

---

**当前版本**: 0.1.0-alpha (开发中)  
**最后更新**: 2026-04-10
