# Remex - 分布式远程执行工具

[![Go Version](https://img.shields.io/badge/Go-1.25+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

Remex（Remote Execution 的简写）是一个用 Go 语言编写的远程执行工具，发音类似 "Remix"，暗示灵活操控。它提供了简洁、现代、技术感强的远程命令执行和文件传输功能。

## 特性

- 🔐 **SSH 连接**: 安全的 SSH 连接和认证
- 📁 **文件传输**: 支持文件上传和下载
- ⚡ **并发处理**: 使用 goroutine 实现高效的并发执行
- 🔧 **可扩展**: 支持自定义内部命令
- 📊 **结果处理**: 灵活的结果处理器机制
- 🛡️ **上下文控制**: 支持超时和取消操作

## 安装

### 前提条件

- Go 1.25 或更高版本

### 获取代码

```bash
git clone https://github.com/your-username/remex.git
cd remex
```

### 构建

```bash
go build
```

## 快速开始

### 基本用法

```go
package main

import (
    "context"
    "log/slog"
    "net/netip"
    "remex"
)

func main() {
    // 创建配置
    configs := []*remex.Config{
        remex.NewRemoteConfig(
            netip.MustParseAddr("192.168.1.100"),
            "username",
            "password",
            []string{"ls -la", "pwd", "whoami"},
        ),
        remex.NewRemoteConfig(
            netip.MustParseAddr("192.168.1.101"),
            "username",
            "password",
            []string{"ls -la", "pwd", "whoami"},
        ),
    }

    // 创建日志器
    logger := slog.Default()

    // 创建 Remex 实例
    remex := remex.NewWithConfig(configs, logger)
    defer remex.Close()

    // 注册结果处理器
    remex.RegisterHandler(func(result remex.ExecResult) {
        logger.Info("执行结果",
            "索引", result.Index,
            "远程地址", result.RemoteAddr,
            "输出", result.Output,
            "错误", result.Error,
            "时间", result.Time,
        )
    })

    // 连接并执行命令
    if err := remex.Execute(); err != nil {
        logger.Error("执行失败", "错误", err)
    }
}
```

### 使用上下文

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

remex := remex.NewWithContext(ctx, configs, logger)
```

## 内部命令

Remex 提供了一系列内置命令，以 `remex.` 前缀开头：

### 文件传输

```bash
# 上传文件到远程主机
remex.upload /local/path/file.txt /remote/path/file.txt

# 从远程主机下载文件
remex.download /remote/path/file.txt /local/path/file.txt
```

### 目录操作

```bash
# 在远程主机创建目录
remex.mkdir /remote/path/new_directory
```

### 本地命令执行

```bash
# 在本地执行命令
remex.exec ls -la
```

## 扩展自定义命令

你可以注册自定义的内部命令：

```go
// 定义自定义命令函数
func myCustomCommand(client *ssh.Client, args ...string) (string, error) {
    // 实现你的逻辑
    return "Custom command executed", nil
}

// 注册命令
remex.RegisterCommand("mycommand", myCustomCommand)

// 使用命令
// remex.mycommand arg1 arg2
```

## 示例

### 批量文件部署

```go
configs := []*remex.Config{
    remex.NewRemoteConfig(
        netip.MustParseAddr("server1.example.com"),
        "deploy",
        "password",
        []string{
            "remex.mkdir /opt/myapp",
            "remex.upload ./build/myapp /opt/myapp/myapp",
            "chmod +x /opt/myapp/myapp",
            "systemctl restart myapp",
        },
    ),
}
```

### 系统监控

```go
commands := []string{
    "uptime",
    "free -h",
    "df -h",
    "ps aux --sort=-%cpu | head -10",
}

configs := []*remex.Config{
    remex.NewRemoteConfig(
        netip.MustParseAddr("monitor1.example.com"),
        "monitor",
        "password",
        commands,       
    ),
}
```

## 贡献

欢迎贡献代码！请阅读 [CONTRIBUTING.md](CONTRIBUTING.md) 了解详情。

## 许可证

本项目采用 MIT 许可证。详见 [LICENSE](LICENSE) 文件。

---

**Remex** - 让远程执行变得简单高效！ 🚀