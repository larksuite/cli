# 飞书 Bot + Claude Code 集成方案

**项目**: lark-cli
**分支**: feature/claude-code-bot
**日期**: 2026-04-10
**作者**: richardiitse

---

## 核心需求

**目标**: "用飞书bot接入claude code,实现类似飞书bot接入openclaw类工具的功能，通过飞书发送指令给claude code完成任务"

**功能描述**:
- 用户在飞书中发送消息
- Bot 接收消息并路由给 Claude Code
- Claude Code 处理任务（代码生成、调试、文件操作等）
- Bot 将结果回复到飞书
- 支持多轮对话（会话保持）

---

## 方案对比

### 1. 纯 Shell 脚本方案

**实现**: `/tmp/lark-claude-bot.sh`

**优势**:
- ✅ 快速验证、简单
- ✅ 已有基础版本可用

**劣势**:
- ❌ 生产级差、维护困难
- ❌ 错误处理有限
- ❌ 不支持复杂消息类型（卡片、文件等）

**适用场景**: 快速原型、功能验证

---

### 2. lark-cli bot 子命令方案 ⭐⭐⭐⭐⭐ 推荐

**实现**: 在 lark-cli 中新增 `bot` 子命令

**架构设计**:

```
lark-cli bot start
    ↓
使用现有的 event +subscribe 订阅消息
    ↓
新的 bot/session.go 管理会话
    ↓
新的 bot/handler.go 处理消息
    ↓
调用外部 claude CLI（集成点）
    ↓
使用现有的 im +messages-send 回复
```

**目录结构**:

```
lark-cli/
├── cmd/bot/
│   ├── bot.go              # bot 子命令入口
│   └── start.go            # lark-cli bot start
├── shortcuts/bot/
│   ├── shortcuts.go        # Bot shortcuts
│   ├── handler.go          # 消息处理器
│   ├── session.go          # 会话管理
│   ├── claude.go           # Claude Code 集成
│   └── router.go           # 命令路由
└── internal/bot/
    ├── config.go           # Bot 配置
    └── registry.go         # 命令注册表
```

**优势**:
- ✅ 复用 lark-cli 完善的基础设施（事件订阅、消息发送、认证）
- ✅ 在 Go 代码中实现会话管理（比 shell 脚本更可靠）
- ✅ 利用 Shortcut 框架扩展命令
- ✅ 生产级部署（pm2/systemd 支持）
- ✅ 可以利用 lark-cli 的所有飞书功能（卡片、文件、日历等）

**劣势**:
- ⚠️ 需要开发时间（4-6 天）

**适用场景**: 生产部署、长期维护

---

### 3. 独立 Go 服务方案

**实现**: 创建独立的 Go 服务，直接调用飞书 API

**优势**:
- ✅ 完全独立、灵活

**劣势**:
- ❌ 重复造轮、维护成本高
- ❌ 需要重新实现飞书集成
- ❌ 无法利用 lark-cli 现有功能

**适用场景**: 不推荐

---

## 功能对比分析

### 当前 Shell 脚本方案

| 功能 | 实现方式 | 状态 |
|------|---------|------|
| 监听飞书消息 | `lark-cli event +subscribe` | ✅ 已实现 |
| 调用 Claude Code | `claude -p --resume` | ✅ 已实现 |
| 多轮对话 | session_id 文件持久化 | ✅ 已实现 |
| 并发处理 | 后台进程 + `</dev/null` | ✅ 已实现 |
| 错误处理 | fallback 消息 | ✅ 已实现 |
| 命令模式 | ❌ 未实现 | 🔜 待添加 |
| 文件操作 | ❌ 未实现 | 🔜 待添加 |
| 多用户隔离 | ❌ 未实现 | 🔜 待添加 |

### lark-cli 已有功能

| 功能 | 位置 | 状态 |
|------|------|------|
| **事件订阅** | `shortcuts/event/subscribe.go` | ✅ 完整 |
| **消息发送** | `shortcuts/im/im_messages_send.go` | ✅ 完整 |
| **Bot 认证** | `internal/credential/` | ✅ 完整 |
| **Shortcut 框架** | `shortcuts/` 全目录 | ✅ 灵活 |
| **会话管理** | ❌ | ❌ 缺失 |
| **消息路由** | ❌ | ❌ 缺失 |
| **外部工具集成** | ❌ | ❌ 缺失 |

---

## 差距分析

### Shell 脚本能做，lark-cli 缺少的：

1. **会话管理**
   - Shell: 简单的文件持久化（`/tmp/lark-bot-sessions/`）
   - lark-cli: ❌ 无此功能

2. **外部工具集成**
   - Shell: 直接调用 `claude` CLI
   - lark-cli: ❌ 无外部命令执行机制

3. **消息路由和分发**
   - Shell: 简单的 if-else 判断
   - lark-cli: ❌ 无路由框架

### lark-cli 能做，Shell 脚本缺少的：

1. **多种消息类型**
   - lark-cli: 富文本、卡片、图片、文件等
   - Shell: 仅文本消息

2. **完善的错误处理**
   - lark-cli: SDK 级别的错误处理和重试
   - Shell: 基础的错误捕获

3. **权限和身份管理**
   - lark-cli: Bot/User 双身份、Scope 管理
   - Shell: 仅 Bot 身份

---

## 推荐方案：lark-cli bot 子命令

### 核心差距只有 2 个

1. **会话管理**
2. **外部工具集成（Claude Code）**

### 其他功能 lark-cli 都有

- ✅ 事件订阅
- ✅ 消息发送
- ✅ 认证管理
- ✅ Shortcut 框架
- ✅ 错误处理

---

## 实施计划

### Phase 1: 核心 Bot 框架（2-3 天）

**任务**:
- [ ] 创建 `cmd/bot/` 目录结构
- [ ] 实现 `bot start` 子命令
- [ ] 集成现有的 `event +subscribe`
- [ ] 实现基础消息处理循环

**关键文件**:
```
cmd/bot/bot.go              # bot 子命令入口
cmd/bot/start.go            # bot start 实现
shortcuts/bot/handler.go     # 消息处理器
shortcuts/bot/session.go     # 会话管理
```

---

### Phase 2: Claude Code 集成（1 天）

**任务**:
- [ ] 实现 `shortcuts/bot/claude.go`
- [ ] 调用 `claude -p --resume` 命令
- [ ] 解析 JSON 输出（result + session_id）
- [ ] 错误处理和重试

**关键文件**:
```
shortcuts/bot/claude.go      # Claude Code 集成
```

---

### Phase 3: 命令路由和扩展（1-2 天）

**任务**:
- [ ] 实现 `shortcuts/bot/router.go`
- [ ] 支持斜杠命令（/run, /deploy, /status）
- [ ] 命令白名单机制
- [ ] 利用 Shortcut 框架注册新命令

**关键文件**:
```
shortcuts/bot/router.go      # 命令路由
shortcuts/bot/shortcuts.go   # Bot shortcuts
```

---

### Phase 4: 测试和优化（1 天）

**任务**:
- [ ] 单元测试
- [ ] 集成测试（真实飞书环境）
- [ ] 性能优化
- [ ] 文档完善

---

## 使用示例

### 启动 Bot

```bash
# 基础启动
lark-cli bot start

# 指定配置文件
lark-cli bot start --config ~/.lark-cli/bot-config.yaml

# 后台运行
lark-cli bot start --daemon

# 查看状态
lark-cli bot status

# 停止 Bot
lark-cli bot stop
```

### 配置文件

```yaml
# ~/.lark-cli/bot-config.yaml
claude:
  work_dir: ~/projects
  system_prompt: "你是一个智能助手，请用中文简洁地回答问题。"
  max_sessions: 100
  session_ttl: 24h

lark:
  app_id: cli_xxx
  app_secret: xxx

features:
  enable_commands: true
  enable_file_ops: true
  allowed_users:
    - ou_xxx
    - ou_yyy

logging:
  level: info
  format: json
  output: /var/log/lark-bot.log
```

### 飞书中使用

```
用户: 帮我写一个 Python 函数计算斐波那契数列
Bot: [Claude Code 生成的代码和解释]

用户: /run tests
Bot: [执行测试并返回结果]

用户: 这个函数有 bug，帮我修复
Bot: [Claude Code 分析并修复 bug]
```

---

## 关键代码模块

### 1. Session 管理

```go
// shortcuts/bot/session.go
type SessionManager struct {
    sessions map[string]*Session  // chat_id -> Session
    mutex    sync.RWMutex
    ttl      time.Duration
}

type Session struct {
    ChatID    string
    SessionID string   // Claude Code session_id
    CreatedAt time.Time
    UpdatedAt time.Time
}

func (sm *SessionManager) Get(chatID string) (*Session, bool) {
    sm.mutex.RLock()
    defer sm.mutex.RUnlock()
    session, ok := sm.sessions[chatID]
    return session, ok
}

func (sm *SessionManager) Set(chatID string, sessionID string) {
    sm.mutex.Lock()
    defer sm.mutex.Unlock()
    sm.sessions[chatID] = &Session{
        ChatID:    chatID,
        SessionID: sessionID,
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }
}
```

### 2. Claude Code 集成

```go
// shortcuts/bot/claude.go
type ClaudeClient struct {
    workDir     string
    systemPrompt string
}

func (c *ClaudeClient) Ask(ctx context.Context, content string, sessionID string) (string, string, error) {
    args := []string{
        "-p", content,
        "--add-dir", c.workDir,
        "--dangerously-skip-permissions",
        "--output-format", "json",
    }

    if sessionID != "" {
        args = append(args, "--resume", sessionID)
    }

    cmd := exec.CommandContext(ctx, "claude", args...)
    output, err := cmd.Output()
    if err != nil {
        return "", "", err
    }

    var result struct {
        Result    string `json:"result"`
        SessionID string `json:"session_id"`
    }
    if err := json.Unmarshal(output, &result); err != nil {
        return "", "", err
    }

    return result.Result, result.SessionID, nil
}
```

### 3. 消息处理

```go
// shortcuts/bot/handler.go
type BotHandler struct {
    sessionManager *SessionManager
    claudeClient   *ClaudeClient
    larkClient     *lark.Client
}

func (h *BotHandler) HandleMessage(ctx context.Context, msg *MessageEvent) error {
    // 1. 获取或创建 session
    session, ok := h.sessionManager.Get(msg.ChatID)
    sessionID := ""
    if ok {
        sessionID = session.SessionID
    }

    // 2. 调用 Claude Code
    answer, newSessionID, err := h.claudeClient.Ask(ctx, msg.Content, sessionID)
    if err != nil {
        return h.sendError(msg.ChatID, err)
    }

    // 3. 保存新 session
    if newSessionID != "" {
        h.sessionManager.Set(msg.ChatID, newSessionID)
    }

    // 4. 回复到飞书
    return h.sendReply(msg.ChatID, answer)
}
```

---

## 部署方案

### PM2 部署

```bash
# 生成 PM2 配置
lark-cli bot start --generate-pm2-config

# 启动
pm2 start lark-bot.ecosystem.config.js

# 查看日志
pm2 logs lark-bot

# 重启
pm2 restart lark-bot
```

### Systemd 部署

```bash
# 生成 systemd service
lark-cli bot start --generate-systemd-service

# 启用服务
sudo systemctl enable lark-bot
sudo systemctl start lark-bot

# 查看状态
sudo systemctl status lark-bot

# 查看日志
sudo journalctl -u lark-bot -f
```

---

## 项目信息

- **Fork 仓库**: https://github.com/richardiitse/cli
- **上游仓库**: https://github.com/larksuite/cli
- **当前分支**: feature/claude-code-bot
- **基础分支**: main

---

## 参考资料

- lark-cli 代码结构分析报告（由 Explore agent 生成）
- Claude Code CLI 文档
- 飞书开放平台文档
- `/tmp/lark-claude-bot.sh` - Shell 脚本参考实现

---

**下一步**: 开始实施 Phase 1 - 核心 Bot 框架开发
