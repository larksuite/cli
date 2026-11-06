# lark-cli 32-bit Linux 版本

> 飞书 lark-cli 命令行工具的多架构 Linux 编译版本

## 下载

请到 [Releases](https://github.com/larksuite/cli/releases) 页面下载对应架构的二进制文件。

## 支持的架构

| 二进制文件 | 架构 | 类型 |
|-----------|------|------|
| lark-cli-linux-386 | i386 | 32-bit x86 |
| lark-cli-linux-arm | ARM | 32-bit ARM |
| lark-cli-linux-arm64 | ARM64 | 64-bit ARM |
| lark-cli-linux-mips | MIPS | 32-bit 大端 |
| lark-cli-linux-mipsle | MIPS | 32-bit 小端 |
| lark-cli-linux-mips64 | MIPS64 | 64-bit 大端 |
| lark-cli-linux-mips64le | MIPS64 | 64-bit 小端 |
| lark-cli-linux-ppc64 | PowerPC | 64-bit |
| lark-cli-linux-s390x | IBM S/390 | 64-bit |
| lark-cli-linux-riscv64 | RISC-V | 64-bit |

## 使用方法

```bash
# 下载对应架构的版本
wget https://github.com/larksuite/cli/releases/download/v32bit/assets/lark-cli-linux-386

# 赋予执行权限
chmod +x lark-cli-linux-386

# 运行
./lark-cli-linux-386 --help
```

## 编译说明

这些版本使用修复后的 `oapi-sdk-go` SDK 编译，以支持 32 位系统。

SDK 修复 PR: https://github.com/larksuite/oapi-sdk-go/pull/211
