> 本文档面向 `larksuite/cli` 仓库的 Maintainer，按照日常工作流程时间线组织：从环境搭建、日常开发、提交审查、处理社区贡献，到安全维护和发版。
>
> https://github.com/larksuite/cli

---

## 1. 仓库概览

<lark-table rows="8" cols="2" column-widths="365,365">

  <lark-tr>
    <lark-td>
      属性
    </lark-td>
    <lark-td>
      值
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      语言
    </lark-td>
    <lark-td>
      <text bgcolor="light-yellow">Go 1.23+ （待升级）</text>
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      许可证
    </lark-td>
    <lark-td>
      MIT
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      包管理
    </lark-td>
    <lark-td>
      Go Modules + npm（用于分发）
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      CI 平台
    </lark-td>
    <lark-td>
      GitHub Actions
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      发布工具
    </lark-td>
    <lark-td>
      GoReleaser v2
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      Lint 工具
    </lark-td>
    <lark-td>
      golangci-lint v2
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      目标平台
    </lark-td>
    <lark-td>
      darwin/linux/windows × amd64/arm64
    </lark-td>
  </lark-tr>
</lark-table>

## 2. 开发环境搭建

### 必要工具

- **Go** >= 1.23（版本锁定在 `go.mod`）

- **Python 3**（用于 `scripts/fetch_meta.py` 构建前拉取元数据）

- **Node.js** >= 16（npm 分发、`scripts/install.js`）

- **Git**

### 本地构建
```bash
# 方式一：使用 Makefile（推荐）
make build          # 构建二进制 → ./lark-cli

# 方式二：使用 build.sh
./build.sh

# 安装到系统（默认 /usr/local/bin）
make install
```

### 首次构建说明

构建前会自动执行 `python3 scripts/fetch_meta.py` 拉取 API 元数据。确保网络可达。

## 3. 项目结构
```plaintext
├── main.go                 # 入口
├── cmd/                    # CLI 命令定义（cobra 框架）
│   ├── root.go             # 根命令
│   ├── api/                # lark api
│   ├── auth/               # lark auth
│   ├── config/             # lark config
│   ├── doctor/             # lark doctor
│   ├── schema/             # lark schema
│   ├── service/            # lark service
│   └── completion/         # shell 补全
├── internal/               # 内部库（不对外导出）
│   ├── auth/               # 认证逻辑
│   ├── build/              # 构建信息（Version, Date）
│   ├── client/             # HTTP 客户端
│   ├── cmdutil/            # 命令行工具函数
│   ├── core/               # 核心抽象
│   ├── httpmock/           # 测试 mock
│   ├── keychain/           # OS 原生密钥链
│   ├── lockfile/           # 文件锁
│   ├── output/             # 输出格式化
│   ├── registry/           # 命令注册
│   ├── util/               # 通用工具
│   └── validate/           # 输入校验
├── shortcuts/              # 业务域快捷命令（im, drive, sheets 等）
├── skills/                 # AI Agent 技能定义（19 个技能）
├── skill-template/         # 技能模板
├── scripts/                # 构建与发布脚本
│   ├── fetch_meta.py       # 拉取 API 元数据
│   ├── install.js          # npm postinstall
│   ├── run.js              # npm bin 入口
│   └── tag-release.sh      # 创建并推送 release tag
├── .github/
│   ├── CODEOWNERS          # 代码所有者
│   └── workflows/          # CI 自动化（提交 PR 后自动运行，详见第 5 节）
├── .golangci.yml           # golangci-lint 配置
├── .goreleaser.yml         # GoReleaser 配置
├── .codecov.yml            # 覆盖率阈值配置
├── Makefile                # 本地开发 targets
├── go.mod / go.sum         # Go 依赖
└── package.json            # npm 分发配置
```

## 4. 开发规范

本节涵盖日常开发中需要遵守的规范：从创建分支、编写代码，到本地测试通过。

### 4.1 分支模型

<lark-table rows="7" cols="3" column-widths="244,244,244">

  <lark-tr>
    <lark-td>
      分支
    </lark-td>
    <lark-td>
      用途
    </lark-td>
    <lark-td>
      保护规则
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `main`
    </lark-td>
    <lark-td>
      稳定主线，所有 PR 合入此分支
    </lark-td>
    <lark-td>
      必须通过 CI，必须经过 Code Review
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `release/vX.Y.Z`

      <text bgcolor="light-yellow">无需关心</text>
    </lark-td>
    <lark-td>
      发版准备分支（可选）
    </lark-td>
    <lark-td>
      同 main
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `ci/*`
    </lark-td>
    <lark-td>
      CI/配置改进
    </lark-td>
    <lark-td>
      开发分支
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `feat/*`
    </lark-td>
    <lark-td>
      新功能
    </lark-td>
    <lark-td>
      开发分支
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `fix/*`
    </lark-td>
    <lark-td>
      Bug 修复
    </lark-td>
    <lark-td>
      开发分支
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `docs/*`
    </lark-td>
    <lark-td>
      文档修改
    </lark-td>
    <lark-td>
      开发分支
    </lark-td>
  </lark-tr>
</lark-table>

#### 分支工作流程图
```plaintext
 feat/xxx ─────●────●──────────┐
              │    │          │ PR (squash merge)
              │    │          ▼
main ─────────┼────┼──────────●──────────●──────────●───────── ···
              │    │                     │          │
fix/yyy ──────┘    │                     │          │
                   │                     │          │
ci/zzz ────────────┘                     │          │
                                         │          │
                              tag v1.1.0 ●          │
                                         │          │
                                    GoReleaser      │
                                    自动发布         │
                                    GitHub Release  │
                                    + npm publish   │
                                                    │
release/v2.0.0 ─────────────────────────────────────┘ (大版本可选)

日常开发流程：

  ┌──────────┐         ┌──────────┐         ┌──────────┐         ┌──────────┐
  │ 创建分支  │────▶│ 本地开发  │────▶│ 推送分支   │────▶│ 创建 PR  │
  │feat/fix/ │         │ & 测试   │           to origin│         │ to main  │
  │ci/docs   │         │          │         │          │         │          │
  └──────────┘         └──────────┘         └──────────┘         └──────────┘
                                                                     │
                   ┌──────────┐               ┌──────────┐           │
                   │  合入     │◀────      │ Code     │◀─────────┘
                   │  main    │               │ Review    │           CI 自动运行:
                   │(squash)  │               │  审批     │           Tests + Lint
                   └──────────┘               └────────── ┘           + Coverage
                        │
                        ▼
                 需要发版时打 tag
                   git tag vX.Y.Z
                        │
                        ▼
                ┌───────────────┐
                │ GoReleaser    │
                │ 自动构建发布   │
                │ 6 平台产物    │
                └───────────────┘
```

### 4.2 Commit 规范

采用 [Conventional Commits](https%3A%2F%2Fwww.conventionalcommits.org%2F) 规范，**commit message 必须使用英文**：
```plaintext
<type>: <short description>

[optional body]

[optional footer]
```

> **严禁在 Commit 信息和分支代码中包含任何敏感数据！**
>
> Commit message、分支代码以及仓库中的所有内容**绝对不允许**包含敏感数据，包括但不限于：

- API Key、Token、Secret、密码等凭据

- 内部主机名、IP 地址、内网端点 URL（比如boe域名）

- 个人身份信息（PII），如真实姓名、邮箱、手机号等

- L3/L4 数据

- 内部项目代号、商业机密数据、保密配置

- 数据库连接串、云服务凭据
> Git 历史是永久且公开的——一旦推送，敏感数据几乎**无法彻底撤回**。
>
> **所有 commit message 和代码贡献必须使用英文编写。**

常用 type：

<lark-table rows="8" cols="3" column-widths="244,244,244">

  <lark-tr>
    <lark-td>
      Type
    </lark-td>
    <lark-td>
      说明
    </lark-td>
    <lark-td>
      会出现在 Changelog {align="center"}
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `feat`
    </lark-td>
    <lark-td>
      新功能
    </lark-td>
    <lark-td>
      Yes {align="center"}
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `fix`
    </lark-td>
    <lark-td>
      缺陷修复
    </lark-td>
    <lark-td>
      Yes {align="center"}
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `docs`
    </lark-td>
    <lark-td>
      文档
    </lark-td>
    <lark-td>
      No {align="center"}
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `ci`
    </lark-td>
    <lark-td>
      CI 配置
    </lark-td>
    <lark-td>
      No {align="center"}
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `test`
    </lark-td>
    <lark-td>
      测试
    </lark-td>
    <lark-td>
      No {align="center"}
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `chore`
    </lark-td>
    <lark-td>
      杂项
    </lark-td>
    <lark-td>
      No {align="center"}
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `refactor`
    </lark-td>
    <lark-td>
      重构
    </lark-td>
    <lark-td>
      视情况 {align="center"}
    </lark-td>
  </lark-tr>
</lark-table>

> **注意**：GoReleaser 自动生成 Changelog 时会过滤 `docs:`, `test:`, `chore:` 前缀的提交。

示例：
```plaintext
# 好的 commit message
feat: support batch record upsert for Base shortcut

Add --batch flag to `lark base record upsert` that accepts
a JSON array, reducing API round-trips for bulk operations.

Closes #42

# 不好的 commit message
fix bug                     # 缺少 type 前缀，描述模糊
feat: 添加日历功能            # 必须使用英文
update code                 # 缺少 type 前缀，无法理解变更内容
feat: add api key AK-xxxx  # 严禁包含敏感数据！
```

### 4.3 代码风格

代码格式化通过 CI 自动检查，本地开发时注意以下要点：

- **格式化**：使用 `gofmt` + `goimports`（CI 通过 golangci-lint formatters 统一执行）

- **编码风格**：遵循 [Effective Go](https%3A%2F%2Fgo.dev%2Fdoc%2Feffective_go) 官方指南

- **版权头**：所有源码文件须包含：
```go
// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
```

当前 CI 启用了 20 个 linter（详见 `.golangci.yml`），核心规则包括：

<lark-table rows="8" cols="2" column-widths="365,365">

  <lark-tr>
    <lark-td>
      Linter
    </lark-td>
    <lark-td>
      作用
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `govet`
    </lark-td>
    <lark-td>
      可疑构造检测
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `unused`
    </lark-td>
    <lark-td>
      未使用的代码
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `ineffassign`
    </lark-td>
    <lark-td>
      无效赋值
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `bodyclose`
    </lark-td>
    <lark-td>
      HTTP Response Body 关闭检查
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `gocritic`
    </lark-td>
    <lark-td>
      综合诊断（bugs、性能）
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `bidichk`
    </lark-td>
    <lark-td>
      危险 Unicode 序列
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `nilerr`
    </lark-td>
    <lark-td>
      nil error 返回检查
    </lark-td>
  </lark-tr>
</lark-table>

CI 使用 `--new-from-rev=origin/main` 增量检查，只检查你的新增代码。本地可运行全量检查：
```bash
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6 run
```

### 4.4 编写测试

#### 测试分层

<lark-table rows="4" cols="3" column-widths="244,244,244">

  <lark-tr>
    <lark-td>
      层级
    </lark-td>
    <lark-td>
      命令
    </lark-td>
    <lark-td>
      范围
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      单元测试
    </lark-td>
    <lark-td>
      `make unit-test`
    </lark-td>
    <lark-td>
      `./cmd/...` `./internal/...` `./shortcuts/...`
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      集成测试
    </lark-td>
    <lark-td>
      `make integration-test`
    </lark-td>
    <lark-td>
      `./tests/...`（需要先 build）
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      全量测试
    </lark-td>
    <lark-td>
      `make test`
    </lark-td>
    <lark-td>
      vet + unit + integration
    </lark-td>
  </lark-tr>
</lark-table>

#### 测试标准

- 使用 `-race` 启用竞态检测

- 使用 `-count=1` 禁用测试缓存

- 超时设置：5 分钟

- HTTP Mock：使用 `internal/httpmock` 包

- **新增代码 patch 覆盖率 >= 60%**

#### 编写示例

优先使用 Go 标准库 `testing` 包编写测试，保持简洁直观：
```go
func TestXxx(t *testing.T) {
    got := Xxx(input)
    if got != expected {
        t.Errorf("Xxx(%v) = %v, want %v", input, got, expected)
    }
}
```

表驱动测试适用于多组输入场景：
```go
func TestXxx(t *testing.T) {
    tests := []struct {
        name  string
        input string
        want  string
    }{
        {"empty input", "", ""},
        {"normal case", "hello", "HELLO"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if got := Xxx(tt.input); got != tt.want {
                t.Errorf("Xxx(%q) = %q, want %q", tt.input, got, tt.want)
            }
        })
    }
}
```

**本地开发完成后，确保**`**make unit-test**`** 通过再提交 PR。**

## 5. 提交 PR

### 5.1 PR 规范

#### 标题

PR 标题遵循与 Commit 相同的 Conventional Commits 格式，因为 squash merge 后它将成为最终的 commit message：
```plaintext
<type>: <short description>
```

- 使用英文，首字母小写，不加句号

- 控制在 **70 个字符**以内

- 聚焦于"做了什么"，而非"怎么做的"

#### 描述（Body）

PR 描述应包含以下内容：
```markdown
## Summary
<!-- 简要说明本次变更的动机和内容，1-3 句话 -->

## Changes
<!-- 列出主要变更点 -->
- 变更 1
- 变更 2

## Test Plan
<!-- 描述如何验证本次变更 -->
- [ ] 单元测试通过
- [ ] 本地手动验证 `lark xxx` 命令正常

## Related Issues
<!-- 关联的 Issue，使用 Closes/Fixes 关键字自动关闭 -->
Closes #123
```

示例（PR 标题：`feat: support batch record upsert for Base shortcut`）：
```markdown
## Summary

Currently `lark base record upsert` only supports single record operations,
requiring N API calls for N records. This PR adds a `--batch` flag that accepts
a JSON array to upsert multiple records in one call.

## Changes

- Add `--batch` flag to `shortcuts/base/record_upsert.go`
- Implement chunked upload (max 500 records per request) with progress output
- Add validation for JSON array input format in `internal/validate`

## Test Plan

- [x] `make unit-test` passed
- [x] Added `TestRecordUpsertBatch` with table-driven cases (empty array,
  single record, 500+ records chunking)
- [x] Manual verification:
  - `lark base record upsert --batch '[{"fields":{"Name":"test"}}]' --table tblXXX --app appXXX`
  - Confirmed 1000 records upserted in 2 requests

## Related Issues

Closes #42
```

#### Label 标签

为每个 PR 添加合适的标签，便于分类和检索：

<lark-table rows="7" cols="2" column-widths="365,365">

  <lark-tr>
    <lark-td>
      标签
    </lark-td>
    <lark-td>
      用途
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `feature`
    </lark-td>
    <lark-td>
      新功能
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `bugfix`
    </lark-td>
    <lark-td>
      缺陷修复
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `ci`
    </lark-td>
    <lark-td>
      CI/CD 变更
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `docs`
    </lark-td>
    <lark-td>
      文档变更
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `breaking-change`
    </lark-td>
    <lark-td>
      不兼容变更，需特别关注
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `dependencies`
    </lark-td>
    <lark-td>
      依赖更新
    </lark-td>
  </lark-tr>
</lark-table>

### 5.2 提交后会发生什么

PR 创建或更新后，GitHub Actions 会自动运行以下检查（无需手动触发）：

<lark-table rows="7" cols="4" column-widths="183,183,183,183">

  <lark-tr>
    <lark-td>
      检查项
    </lark-td>
    <lark-td>
      Workflow
    </lark-td>
    <lark-td>
      内容
    </lark-td>
    <lark-td>
      是否阻塞合入 {align="center"}
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **单元测试**
    </lark-td>
    <lark-td>
      `tests.yml`
    </lark-td>
    <lark-td>
      `go test -race` + `go build`
    </lark-td>
    <lark-td>
      Yes {align="center"}
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **静态分析**
    </lark-td>
    <lark-td>
      `lint.yml`
    </lark-td>
    <lark-td>
      golangci-lint 增量检查（仅检查新代码）
    </lark-td>
    <lark-td>
      Yes {align="center"}
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **依赖整洁**
    </lark-td>
    <lark-td>
      `lint.yml`
    </lark-td>
    <lark-td>
      `go mod tidy` 一致性检查
    </lark-td>
    <lark-td>
      Yes {align="center"}
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **许可证合规**
    </lark-td>
    <lark-td>
      `lint.yml`
    </lark-td>
    <lark-td>
      禁止 forbidden/restricted/reciprocal/unknown 许可证
    </lark-td>
    <lark-td>
      Yes {align="center"}
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **漏洞扫描**
    </lark-td>
    <lark-td>
      `lint.yml`
    </lark-td>
    <lark-td>
      `govulncheck` 已知漏洞检测
    </lark-td>
    <lark-td>
      No（informational） {align="center"}
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **覆盖率报告**
    </lark-td>
    <lark-td>
      `coverage.yml`
    </lark-td>
    <lark-td>
      生成覆盖率报告到 Job Summary（patch 目标 60%）
    </lark-td>
    <lark-td>
      No（informational） {align="center"}
    </lark-td>
  </lark-tr>
</lark-table>

如果 CI 失败：

1. 点击 GitHub PR 页面的 "Details" 查看具体错误

2. 修复问题后推送新 commit，CI 会自动重新运行

3. lint 失败通常是代码格式或新引入的代码问题，参照 4.3 节修复

### 5.3 合入要求

PR 合入 `main` 前必须满足：

- [ ] CI 全部通过（Tests + Lint）

- [ ] 至少 1 位 Maintainer 审批通过

- [ ] 分支已与 main 同步（up to date）

- [ ] 使用 Squash Merge 合入，保持线性历史

### 5.4 谁来 Review？（CODEOWNERS）

PR 创建后，GitHub 会根据 `.github/CODEOWNERS` 自动分配 Reviewer。当前分工：

<!-- Unsupported block type: 53 -->

<text bgcolor="light-yellow">待补充</text>

<lark-table rows="8" cols="4" column-widths="244,100,244,244">

  <lark-tr>
    <lark-td>
      代码区域
    </lark-td>
    <lark-td>
      目录
    </lark-td>
    <lark-td>
      Owner
    </lark-td>
    <lark-td>
      职责
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `*`（全局默认）
    </lark-td>
    <lark-td>
    </lark-td>
    <lark-td>
      <mention-user id="ou_c1b0dbb762d79bae383ca0d3d5bdd804"/>
    </lark-td>
    <lark-td>
      所有未单独指定的文件
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `.github/`、`.golangci.yml`
    </lark-td>
    <lark-td>
    </lark-td>
    <lark-td>
    </lark-td>
    <lark-td>
      CI 配置与 lint 规则
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `cmd/`、`internal/`
    </lark-td>
    <lark-td>
    </lark-td>
    <lark-td>
    </lark-td>
    <lark-td>
      CLI 框架与核心库
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `shortcuts/<domain>/`
    </lark-td>
    <lark-td>
    </lark-td>
    <lark-td>
    </lark-td>
    <lark-td>
      各业务域快捷命令
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
    </lark-td>
    <lark-td>
    </lark-td>
    <lark-td>
    </lark-td>
    <lark-td>
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
    </lark-td>
    <lark-td>
    </lark-td>
    <lark-td>
    </lark-td>
    <lark-td>
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
    </lark-td>
    <lark-td>
    </lark-td>
    <lark-td>
    </lark-td>
    <lark-td>
    </lark-td>
  </lark-tr>
</lark-table>

修改 CODEOWNERS 后，相关区域的 PR 会自动分配给对应 owner review。

## 6. Code Review

### 审查检查清单

#### 功能性

- [ ] 逻辑正确，满足需求

- [ ] 边界条件已处理

- [ ] 错误处理合理（不吞错误、不暴露敏感信息）

#### 代码质量

- [ ] 遵循 Go 官方风格（Effective Go）

- [ ] 无未使用的导入、变量、函数

- [ ] 命名清晰、一致

- [ ] 公开 API 有适当的注释

#### 安全

- [ ] 无硬编码凭据或密钥

- [ ] 用户输入已校验（internal/validate）

- [ ] HTTP 响应 Body 已正确关闭

- [ ] 无命令注入、路径遍历风险

#### 测试

- [ ] 新功能附带测试

- [ ] 测试覆盖核心路径和边界条件

- [ ] patch 覆盖率 >= 60%

#### 兼容性

- [ ] 不破坏现有命令的接口

- [ ] 不引入 forbidden/restricted/reciprocal 许可证的依赖

### 

## 7. 处理外部 Issue 与 PR

作为 Maintainer，及时、专业地处理社区提交的 Issue 和 PR 是核心职责之一。

### 7.1 Issue 分类与响应

#### 收到新 Issue 后的处理流程
```plaintext
新 Issue 到达
    │
    ▼
┌─────────────┐    不完整           ┌──────────────────┐
│ 信息是否完整？          │───────────▶│ 添加标签，要求补充              │
│ (复现步骤、            │                   │ 环境/步骤/日志                 │
│  环境、日志)           │                    └──────────────────┘
└─────────────          ┘
    │ 完整
    ▼
┌─────────────┐
│ 分类并打标签  │
└─────────────┘
    │
    ├─── bug        → 确认复现，评估优先级
    ├─── enhancement → 评估可行性，标记 milestone（如适用）
    ├─── question    → 直接回答或引导至文档
    └─── duplicate   → 关联原 Issue 并关闭
```

#### 标签体系

<lark-table rows="14" cols="2" column-widths="365,365">

  <lark-tr>
    <lark-td>
      标签
    </lark-td>
    <lark-td>
      说明
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `bug`
    </lark-td>
    <lark-td>
      已确认的缺陷
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `enhancement`
    </lark-td>
    <lark-td>
      功能增强请求
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `question`
    </lark-td>
    <lark-td>
      使用咨询
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `good first issue`
    </lark-td>
    <lark-td>
      适合新贡献者入手
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `help wanted`
    </lark-td>
    <lark-td>
      欢迎社区认领
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `duplicate`
    </lark-td>
    <lark-td>
      重复 Issue
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `wontfix`
    </lark-td>
    <lark-td>
      不予修复（需说明原因）
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `priority/critical`
    </lark-td>
    <lark-td>
      紧急（影响核心功能或安全）
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `priority/high`
    </lark-td>
    <lark-td>
      高优先级
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `domain/im`
    </lark-td>
    <lark-td>
      IM 模块相关
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `domain/drive`
    </lark-td>
    <lark-td>
      Drive 模块相关
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `domain/base`
    </lark-td>
    <lark-td>
      Base 模块相关
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `security`
    </lark-td>
    <lark-td>
      安全相关（私下处理，见第 8 节）
    </lark-td>
  </lark-tr>
</lark-table>

#### 响应时效

<lark-table rows="6" cols="3" column-widths="244,244,244">

  <lark-tr>
    <lark-td>
      Issue 类型
    </lark-td>
    <lark-td>
      首次响应 {align="center"}
    </lark-td>
    <lark-td>
      说明
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      安全漏洞
    </lark-td>
    <lark-td>
      24 小时 {align="center"}
    </lark-td>
    <lark-td>
      优先私下处理，见第 8 节
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      Bug（critical）
    </lark-td>
    <lark-td>
      1 个工作日 {align="center"}
    </lark-td>
    <lark-td>
      确认复现并开始调查
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      Bug（普通）
    </lark-td>
    <lark-td>
      3 个工作日 {align="center"}
    </lark-td>
    <lark-td>
      分类打标签，确认是否复现
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      Feature Request
    </lark-td>
    <lark-td>
      1 周 {align="center"}
    </lark-td>
    <lark-td>
      评估可行性，给出初步反馈
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      Question
    </lark-td>
    <lark-td>
      3 个工作日 {align="center"}
    </lark-td>
    <lark-td>
      回答或引导至文档
    </lark-td>
  </lark-tr>
</lark-table>

#### 回复最佳实践

- **始终使用英文**回复，保持与国际社区的一致性

- **感谢贡献者**的反馈，即使是重复 Issue 或 wontfix

- **提供明确的下一步**：需要补充什么信息、计划什么时候修复、或者为什么不予采纳

- **Bug 确认后**标记 `bug` 标签并关联到负责人

- **不确定的问题**可以添加 `needs-triage` 标签，在团队会议中讨论

回复示例：
```markdown
<!-- Bug 确认 -->
Thanks for reporting this! I can reproduce the issue on macOS with v1.0.0.
The root cause is [brief explanation]. Will fix in the next patch release.

<!-- 需要更多信息 -->
Thanks for the report. Could you provide:
1. Output of `lark doctor`
2. The exact command you ran
3. Full error message / stack trace

<!-- Feature Request 评估 -->
Thanks for the suggestion! This is an interesting idea.
We'll evaluate it for the v1.2 milestone. In the meantime,
you can achieve a similar result with `lark api ...`.

<!-- Wontfix -->
Thanks for raising this. After discussion, we've decided not to implement
this because [reason]. [Alternative approach if applicable].
Closing this issue, but feel free to reopen if you have additional context.
```

### 7.2 审查外部 PR

外部贡献者提交的 PR 需要额外关注以下几点：

#### 处理流程
```plaintext
外部 PR 到达
    │
    ▼
┌──────────┐    未签署        ┌───────────────────────┐
│ CLA 已签署？     │──────────▶│ 要求签署 CLA 后再 review                │
└──────────        ┘                  └───────────────────────                ┘
    │ 已签署
    ▼
┌──────────────┐    不通过           ┌─────────────────────┐
│ CI 是否通过？            │───────────▶│ 引导贡献者修复 CI 问题               │
└─────────────          ─┘                     └─────────────────────              ┘
    │ 通过
    ▼
┌──────────────┐
│ Code Review   │
│ (见第 6 节)   │
└──────────────┘
    │
    ▼
┌──────────────┐    需修改          ┌────────────────────────┐
│ 是否达到合入             │───────────▶│ 给出具体、建设性的反馈                    │
│ 标准？        │                             │ 说明需要改什么，为什么                    │
└──────────────┘                              └────────────────────────               ┘
    │ 达标
    ▼
  Approve & Squash Merge
```

#### Review 要点（在第 6 节基础上额外关注）

- **敏感数据检查**：外部 PR 务必仔细检查是否包含内部 URL、密钥、PII 等

- **代码风格一致性**：外部贡献者可能不熟悉项目风格，耐心引导

- **scope 控制**：单个 PR 只做一件事，过大的 PR 要求拆分

- **测试完整性**：确保新增代码有对应测试

#### 给出建设性反馈

- 区分"必须修改"（blocking）和"建议优化"（nit/suggestion）

- 提供具体的代码示例或参考，而不是只说"这里不对"

- 对于新贡献者，可以主动帮助修复小问题（typo、格式），降低参与门槛

示例：
```markdown
<!-- Blocking feedback -->
This will panic if `token` is nil. Please add a nil check:
```go

if token == nil {

    return fmt.Errorf("auth token is required")

}
```

<!-- Non-blocking suggestion -->
nit: Consider renaming `data` to `records` for clarity — not blocking though.

<!-- Encouraging new contributors -->
Great first contribution! Just a couple of small things to fix.
For the test, you can reference `shortcuts/base/record_test.go`
for examples of how we structure table-driven tests in this project.
```

### <text bgcolor="light-yellow">7.3 CLA 检查</text>

所有外部贡献者在合入 PR 前必须签署 CLA：

- **个人贡献者**：点击 "Accept" 接受 ByteDance Individual CLA v1.1

- **企业贡献者**：签署 ByteDance Corporate CLA v1.1（需 VP 或以上级别签署）

  - 扫描签署后发送至：`opensource-cla@bytedance.com`

如果贡献者未签署 CLA，在 PR 中礼貌提醒：
```markdown
Thanks for the PR! Before we can review it, please sign our
[Contributor License Agreement](./CLA.md). Once signed,
please comment here and we'll proceed with the review.
```

### 7.4 过期 Issue 处理

- 超过 **90 天**无活动的 Issue，标记为 `stale`，并留言提醒

- 标记后 **30 天**仍无回应，自动关闭

- 建议使用 `actions/stale` 自动化

- 关闭时留言说明原因，并欢迎重新开启
```markdown
This issue has been automatically marked as stale due to 90 days
of inactivity. It will be closed in 30 days if no further activity
occurs. Feel free to reopen if this is still relevant.
```

## 8. 安全合规

### 许可证

- 项目许可证：**MIT**

- 所有源码文件须包含版权头（见 4.3 节）

### 依赖许可证合规

CI 自动检查（`lint.yml`），禁止引入以下类型许可证的依赖：

- **Forbidden**（禁止使用）

- **Restricted**（受限，如 GPL）

- **Reciprocal**（互惠，如 LGPL/MPL 需审慎）

- **Unknown**（未知许可证）

### 安全扫描

<lark-table rows="4" cols="3" column-widths="244,244,244">

  <lark-tr>
    <lark-td>
      工具
    </lark-td>
    <lark-td>
      状态
    </lark-td>
    <lark-td>
      说明
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `govulncheck`
    </lark-td>
    <lark-td>
      Informational
    </lark-td>
    <lark-td>
      已知漏洞扫描（计划升级为 blocking）
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `gosec`
    </lark-td>
    <lark-td>
      待启用
    </lark-td>
    <lark-td>
      Go 安全静态分析
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `bidichk`
    </lark-td>
    <lark-td>
      已启用
    </lark-td>
    <lark-td>
      防御 Unicode 方向覆盖攻击
    </lark-td>
  </lark-tr>
</lark-table>

### 敏感信息

- **绝对不要**提交凭据、密钥、token 到仓库（详见 4.2 节 Commit 规范）

- 凭据存储使用 OS 原生密钥链（`internal/keychain`）

- `.gitignore` 已排除常见敏感文件

- Review 时注意检查是否有硬编码密钥

### 安全漏洞响应

1. 收到安全报告后，**24 小时内**确认

2. 评估影响范围和严重程度

3. 在私有环境中修复并测试

4. 发布修复版本，通知受影响用户

5. 建议开启 GitHub Security Advisories

## 9. 发版流程 {folded="true"}
> 本节操作由执行<mention-user id="ou_c1b0dbb762d79bae383ca0d3d5bdd804"/><mention-user id="ou_96a099001c57790a35e596d92de91034"/>进行发布。
>
> Maintainer 在日常工作中不需要关注发版细节，但了解流程有助于协作。

### 发布周期

- 无特殊限制，默认2天一版本。

- 有需求随时发

### 版本号规范

遵循 [Semantic Versioning](https%3A%2F%2Fsemver.org%2F)：`vMAJOR.MINOR.PATCH`

- **MAJOR**：不兼容的 API 变更

- **MINOR**：向后兼容的新功能

- **PATCH**：向后兼容的缺陷修复

### 发版步骤

#### 1. 准备阶段
```bash
# 确保 main 分支是最新的
git checkout main
git pull origin main

# 更新 package.json 中的 version
# 更新 CHANGELOG.md（添加新版本的变更内容）
```

#### 2. 更新 CHANGELOG

在 `CHANGELOG.md` 中按如下格式添加新版本：
```markdown
## [vX.Y.Z] - YYYY-MM-DD

### Added
- 新功能描述

### Changed
- 变更描述

### Fixed
- 修复描述

[vX.Y.Z]: https://github.com/larksuite/cli/releases/tag/vX.Y.Z
```

#### 3. 创建 Tag 并发布
```bash
# 使用 tag-release.sh（从 package.json 读取版本号）
./scripts/tag-release.sh
```

脚本会：

1. 从 `package.json` 读取版本号

2. 检查 tag 是否已存在（本地和远程）

3. 确保 `package.json` 已提交

4. 确保本地与远程同步

5. 创建并推送 tag

#### 4. 自动发布

Tag 推送后，`release.yml` 自动触发 GoReleaser：

- 构建 6 个平台/架构组合（darwin/linux/windows × amd64/arm64）

- 禁用 CGO，注入版本号和构建日期

- 生成 checksum，发布到 GitHub Releases

#### 5. npm 发布
```bash
npm publish
```

### 发版检查清单

- [ ] 所有 CI 通过

- [ ] CHANGELOG.md 已更新

- [ ] package.json 版本号已更新

- [ ] 无阻塞性的 open Issue

- [ ] Release notes 已准备

## 10 . 代码开发指南

#### Meta API 接入

Meta API 是纯配置驱动的，不需要写 Go 代码。

##### 两个仓库的分工

<lark-table rows="3" cols="3" header-row="true" column-widths="264,180,244">

  <lark-tr>
    <lark-td>
      仓库
    </lark-td>
    <lark-td>
      职责
    </lark-td>
    <lark-td>
      包含内容
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      https://github.com/larksuite/cli
    </lark-td>
    <lark-td>
      CLI 代码 + 生成产物
    </lark-td>
    <lark-td>
      Go 源码、`skills/`、`skill-template/`、`internal/registry/meta_data.json`
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      https://code.byted.org/lark/larksuite-cli-registry（内部）
    </lark-td>
    <lark-td>
      生成逻辑 + OAPI 元数据
    </lark-td>
    <lark-td>
      `registry-conf/`、`scripts/gen-*.py`、`Makefile`、`output/`
    </lark-td>
  </lark-tr>
</lark-table>

**核心原则**：CLI 仓库零侵入，所有生成逻辑和配置都在 registry 仓库。产物通过 `make` 直接写入 CLI 仓库。

流程：

1. [registry 仓库] 配置 registry-config.yaml

1. make all project=X

1. [CLI 仓库] go build

##### 0. 首次配置
```bash
cp local.mk.example local.mk
# 编辑 local.mk，填入 CLI 仓库的本地路径：
# CLI_REPO = /path/to/larksuite/cli

```

##### 1. 配置`registry-config`

编辑 registry 仓库的 `registry-conf/registry-config.yaml`，添加项目配置，声明域名、版本来源、要暴露的 resource/method。多版本场景支持版本融合和资源重命名。参考已有域的配置即可上手。

##### 2. 生成 & 编译

在 **registry 仓库（****https://code.byted.org/lark/larksuite-cli-registry****）**中执行：
```bash
make all project=<domain>

```

这一条命令会自动完成：

- 生成 `meta_data.json` 并写入 CLI 仓库的 `internal/registry/`

- 生成 `SKILL.md` 并写入 CLI 仓库的 `skills/lark-<domain>/`

**这之后，你可以在CLI仓库进行make install进行测试。**

也可以分步执行：
```bash
make gen-registry project=<domain>   # 只生成 meta_data.json
make gen-skills project=<domain>     # 只生成 SKILL.md

```

<callout emoji="⚠️" background-color="light-orange" border-color="light-orange">

注意，如果遇到meta不生效的问题，可以清空 ~/.lark-cli/cache再重试。
</callout>

##### 3. 补充 Skill 元数据

`gen-skills` 会自动生成域级 `skills/lark-{domain}/SKILL.md`，但默认只含 API 列表和权限表。业务域应补充两类自定义内容：

**域元数据**（registry 仓库 `registry-conf/skill-meta.yaml`）：
```yaml
myservice:
  title: 我的服务 API
  description: "飞书xxx：更完整的描述，告诉 AI 什么场景该加载这个 skill。"

```

<lark-table rows="3" cols="3" header-row="true" column-widths="244,244,244">

  <lark-tr>
    <lark-td>
      字段
    </lark-td>
    <lark-td>
      说明
    </lark-td>
    <lark-td>
      默认值
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `title`
    </lark-td>
    <lark-td>
      SKILL.md 标题
    </lark-td>
    <lark-td>
      取 from_meta 中的 title
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `description`
    </lark-td>
    <lark-td>
      frontmatter description，AI 据此判断是否加载该 skill
    </lark-td>
    <lark-td>
      `"飞书{title}：{from_meta description}。"`
    </lark-td>
  </lark-tr>
</lark-table>

**域介绍模板（可选）**（CLI 仓库 `skill-template/domains/{domain}.md`）：

用于向 AI 解释核心概念和资源关系，插入到 SKILL.md 的 API 列表之前。示例参考 `skill-template/domains/calendar.md`。

修改后在 registry 仓库重新运行 `make gen-skills project=<domain>` 即可更新 SKILL.md。

##### 4. 验证
```bash
# 在 CLI 仓库
./build.sh                                         # 编译
./lark-cli <domain> --help                         # 查看生成的命令
./lark-cli schema <domain>                         # 查看 API schema
./lark-cli <domain> <resource> <method> --dry-run  # 预览请求

```

##### 5. 合入并发布
<callout emoji="⚠️" background-color="light-orange" border-color="light-orange">

**在****PR****合入CLI仓库前，需要先合入registry仓库并提交发布！**

https://code.byted.org/lark/larksuite-cli-registry/

当前CLI项目正式版编译依赖于**服务端动态下发**的meta_data.json数据，在本地验证测试完毕后，在PR合入前。需要先进行registry仓库的MR合入，以免修改在未来被丢失。然后联系项目组进行meta_data的更新发布。
</callout>

#### Shortcut 接入

Shortcut 是用 Go 代码实现的高频场景增强命令，挂载在对应 service 下，以 `+` 前缀区分于 Meta API 命令。

##### 1. 编写 Shortcut

在 `shortcuts/{domain}/` 下新建文件，定义 `common.Shortcut` 变量：
```go
// shortcuts/myservice/myservice_read.go
package myservice

import (
    "context"
    "fmt"
    "io"

    "code.byted.org/lark/larksuite-cli/internal/output"
    "code.byted.org/lark/larksuite-cli/shortcuts/common"
)

var MyRead = common.Shortcut{
    Service:     "myservice",          // 挂载到哪个 service 命令下
    Command:     "+read",              // 必须 + 开头
    Description: "读取某资源",
    Risk:        "read",               // read | write | high-risk-write
    Scopes:      []string{"myservice:resource:read"},
    AuthTypes:   []string{"user", "bot"},  // 默认 ["user"]
    HasJSON:     true,                 // 自动注入 --json flag

    Flags: []common.Flag{
        {Name: "id", Desc: "资源 ID", Required: true},
    },

    DryRun: func(ctx context.Context, runtime *common.RuntimeContext) map[string]interface{} {
        return map[string]interface{}{
            "api": "GET /open-apis/myservice/v1/resources/" + runtime.Str("id"),
        }
    },

    Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
        data, err := runtime.DoAPI("GET",
            "/open-apis/myservice/v1/resources/"+runtime.Str("id"), nil, nil)
        if err != nil {
            return err
        }
        runtime.OutPretty(data, nil, func(w io.Writer) {
            fmt.Fprintf(w, "结果: %v\n", data)
        })
        return nil
    },
}

```

**Shortcut 结构体关键字段**：

<lark-table rows="10" cols="2" header-row="true" column-widths="350,350">

  <lark-tr>
    <lark-td>
      字段
    </lark-td>
    <lark-td>
      说明
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `Service`
    </lark-td>
    <lark-td>
      父命令名（如 `"calendar"`），框架自动挂载
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `Command`
    </lark-td>
    <lark-td>
      子命令名，必须 `+` 开头
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `Risk`
    </lark-td>
    <lark-td>
      `read` / `write` / `high-risk-write`（后者需 `--yes` 确认）
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `Scopes`
    </lark-td>
    <lark-td>
      所需 OAuth scope
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `AuthTypes`
    </lark-td>
    <lark-td>
      声明 API 支持的身份类型。`--as` flag 始终可用，但如果传入未声明的身份会报错
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `Flags`
    </lark-td>
    <lark-td>
      自定义 flag，支持 string/bool/int 类型和 Enum 校验
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `DryRun`
    </lark-td>
    <lark-td>
      `--dry-run` 时的预览输出
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `Validate`
    </lark-td>
    <lark-td>
      参数校验钩子（可选）
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `Execute`
    </lark-td>
    <lark-td>
      业务逻辑（必填）
    </lark-td>
  </lark-tr>
</lark-table>

**RuntimeContext 常用 API**：
```go
runtime.Str("name")  runtime.Bool("name")  runtime.Int("name")  // 读 flag
runtime.CallAPI(method, url, params, body)   // 自动身份 + 错误处理
runtime.DoAPI(req,opts)                      // 自动身份 + 原生响应
runtime.Out(data, meta)                      // JSON 信封输出
runtime.OutPretty(data, meta, prettyFn)      // --json 走 JSON，否则走 prettyFn
runtime.As()  runtime.IsBot()                // 获取当前身份
runtime.AccessToken()                        // 获取 token
runtime.LarkSDK()                            // Lark SDK 客户端

```

##### 2. 注册

每个域的 shortcuts 通过包函数 `Shortcuts()` 返回列表。在 `shortcuts/register.go` 的 `init()` 中追加注册：
```go
import "code.byted.org/lark/larksuite-cli/shortcuts/myservice"

func init() {
    // ...existing...
    allShortcuts = append(allShortcuts, myservice.Shortcuts()...)
}

```

##### 3. 编写 SKILL.md

在 `skills/lark-{domain}/references/lark-{domain}-{verb}.md` 创建 Shortcut 技能文档，这是给 AI Agent 看的使用手册。内容包括命令格式、参数说明、典型用法示例。参考 `skills/lark-calendar/references/lark-calendar-agenda.md` 的格式。

域级 SKILL.md（`skills/lark-{domain}/SKILL.md`）由 `gen-skills.py` 自动生成，会自动引用 references 下的 Shortcut 文档。

##### 4. 编译验证
```bash
make #不污染全局环境，在当前文件夹下测试
./larksuite-cli myservice +read --help
./larksuite-cli myservice +read --id xxx --dry-run
```

##### 验证 Skill

开发完成后，需要在实际 Agent 中验证 Skill 是否能被正确加载和使用。
<callout emoji="⚠️" background-color="red" border-color="light-red">

CLI需要尽可能同时支持Tenant Access Token 和User Access Token。 需要测试两种身份场景的表现，当两种身份在Agent执行的表现上有明显差异时（例如身份不同导致的权限问题），需要在SKill中进行说明。 
</callout>

##### Skill 目录结构
```plaintext
skills/
├── lark-shared/SKILL.md                              # 共享规则（认证、全局参数）
├── lark-{domain}/SKILL.md                            # 域级 skill（gen-skills 生成）
└── lark-{domain}/references/lark-{domain}-{verb}.md  # Shortcut skill（手写）

skill-pack/
└── lark-suite/SKILL.md                               # 聚合所有域的 master skill（gen-master-skill 生成）
```

##### 安装 Skill 到 Agent

只需要关注 skills 文件夹下自己业务域的 Skill 文件夹即可。

两种方式将 Skill 安装到支持 Skills 的 Agent（Claude Code、OpenClaw 等）中测试：

**方式 1：手动拷贝**

将 `skills/lark-{domain}/` 目录拷贝到 Agent 的 skill 目录中（如 `~/.claude/skills/`）。

**方式 2：npx skills add**
```bash
npx skills add /path/to/larksuite-cli/skills/lark-{domain}

```

##### 验证要点

- Agent 能否通过自然语言触发对应 Skill（如「查看明天日程」→ 加载 lark-calendar）

- `description` 是否足够准确，不会与其他 Skill 混淆

- Shortcut 的 references 文档是否被正确引用

- 实际调用 CLI 命令是否成功

##### 编译&提测

#### 提测

##### 方式一（本地build方式）

###### CLI提测

- 编译可执行文件 `./scripts/build-all.sh`，文件生成目录`dist`

- 将多平台可执行文件交付QA提测

<reference-synced source-block-id="OJCNdZwyMsM4P7b8PCVcqA1Znxh" source-document-id="Mz9mdH62NoJimtxdbARcce7XnQc">

  - 将可执行文件添加到系统路径

    - Mac/Linux 
      ```plaintext
      sudo mv {{可执行文件路径}} /usr/local/bin/larksuite-cli
      # 例如：sudo cp /Users/bytedance/Downloads/Lark\ Suite\ CLI\ darwin\ arm64 /usr/local/bin/larksuite-cli
      
      sudo chmod +x /usr/local/bin/larksuite-cli
      ```

    - Windows
      ```plaintext
      copy your-binary.exe C:\Windows\System32\larksuite-cli.exe
      ```

</reference-synced>

###### Skills提测 {folded="true"}

<grid cols="2">

  <column width="55">

    将仓库中最新的skills/lark-<domain>目录打包交付QA提测。
    
  </column>

  <column width="44">

    <image token="Qwj7bRQ1eoxPSvx1Bt9cv4A7nvd" width="492" height="808" align="center"/>

  </column>

</grid>

##### 方式二（NPM 方式-推荐）

提交PR 后，会生成一个npm包，执行安装即可

<image token="Z36wb3BB3ogNRVxhkL1c082snof" width="2196" height="938" align="center"/>

## CLI 基本能力

## 11. 常见问题（FAQ）

### Q: 本地 lint 和 CI 结果不一致？

CI 使用 `--new-from-rev=origin/main` 增量检查。本地全量运行可能报更多问题。确保本地使用相同版本的 golangci-lint（v2.1.6）。

### Q: `fetch_meta.py` 执行失败？

确保 Python 3 已安装且网络可达。该脚本在构建前拉取 API 元数据，是构建的必要步骤。

### Q: 如何添加新的 linter？

1. 在 `.golangci.yml` 的 `enable` 列表中添加

2. 先在本地运行确认存量问题数量

3. 如果存量问题过多，先修复再启用（参考 `errcheck` 的处理方式）

### Q: GoReleaser 发布失败？

检查：

1. Tag 格式是否正确（`v*`）

2. `GITHUB_TOKEN` 权限是否充足

3. `fetch_meta.py` 是否在 before hook 中正常执行

4. Go 版本是否匹配（release workflow 使用 Go 1.23）

### Q: 如何处理 govulncheck 报告的漏洞？

当前 govulncheck 为 informational 模式（`continue-on-error: true`）。如果报告了漏洞：

1. 评估是否影响 lark-cli（许多漏洞可能在未使用的代码路径中）

2. 升级受影响的依赖

3. 如果是间接依赖，使用 `go get <indirect-dep>@<safe-version>` 升级
