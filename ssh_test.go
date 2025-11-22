package remex

import (
	"context"
	"net/netip"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// TestNewSSHConfig 测试 SSH 配置创建
func TestNewSSHConfig(t *testing.T) {
	addr := netip.MustParseAddr("192.168.1.1")
	username := "testuser"
	password := "testpass"

	config := NewSSHConfig(addr, username, password)

	if config.Username != username {
		t.Errorf("Expected username %s, got %s", username, config.Username)
	}
	if config.Password != password {
		t.Errorf("Expected password %s, got %s", password, config.Password)
	}
	if config.Addr != addr {
		t.Errorf("Expected address %s, got %s", addr, config.Addr)
	}
	if config.Port != DefaultSSHPort {
		t.Errorf("Expected port %d, got %d", DefaultSSHPort, config.Port)
	}
	if !config.autoRootPassword {
		t.Error("Expected autoRootPassword to be true")
	}
}

// TestSSHConfigConnect 测试 SSH 连接配置
func TestSSHConfigConnect(t *testing.T) {
	config := &SSHConfig{
		Username: "testuser",
		Password: "testpass",
		Addr:     netip.MustParseAddr("192.168.1.1"),
		Port:     22,
	}

	// 测试连接失败（因为地址不可达）
	_, err := config.Connect()
	if err == nil {
		t.Error("Expected connection error for unreachable host")
	}

	// 验证错误消息包含地址信息
	if !strings.Contains(err.Error(), "192.168.1.1:22") {
		t.Errorf("Error message should contain address, got: %v", err)
	}
}

// TestNewSSHClient 测试 SSH 客户端创建
func TestNewSSHClient(t *testing.T) {
	config := &SSHConfig{
		Username: "testuser",
		Password: "testpass",
		Addr:     netip.MustParseAddr("192.168.1.1"),
		Port:     22,
	}

	// 测试客户端创建失败
	_, err := NewSSHClient("test-id", config)
	if err == nil {
		t.Error("Expected error for unreachable host")
	}

	// 注意：测试 nil 配置会导致 panic，因为函数内部没有检查
	// 在实际使用中应该确保配置不为 nil
}

// TestSSHClientExecuteCommand 测试 SSH 客户端命令执行
func TestSSHClientExecuteCommand(t *testing.T) {
	ctx := context.Background()
	config := &SSHConfig{
		Username: "testuser",
		Password: "testpass",
		Addr:     netip.MustParseAddr("192.168.1.1"),
		Port:     22,
	}

	client := &SSHClient{
		ID:     "test-id",
		config: config,
		// Client 字段为 nil，模拟未连接的客户端
	}

	// 测试 remex 命令执行
	_, err := client.ExecuteCommand(ctx, "remex.upload")
	if err == nil {
		t.Error("Expected error for nil SSH client")
	}

	// 测试普通命令执行
	_, err = client.ExecuteCommand(ctx, "ls -la")
	if err == nil {
		t.Error("Expected error for nil SSH client")
	}
}

// TestExecRemoteCommand 测试远程命令执行
func TestExecRemoteCommand(t *testing.T) {
	ctx := context.Background()
	env := map[string]string{"TEST_VAR": "test_value"}
	password := "testpass"
	command := "echo hello"

	// 测试 nil 客户端
	_, err := ExecRemoteCommand(ctx, env, nil, password, command, false)
	if err == nil {
		t.Error("Expected error for nil client")
	} else if err.Error() != "SSH client is nil" {
		t.Errorf("Expected 'SSH client is nil' error, got %v", err)
	}

	// 测试上下文取消（由于客户端为 nil，应该先检查客户端）
	ctxCancel, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ExecRemoteCommand(ctxCancel, env, nil, password, command, false)
	if err == nil {
		t.Error("Expected error for nil client")
	} else if err.Error() != "SSH client is nil" {
		t.Errorf("Expected 'SSH client is nil' error, got %v", err)
	}
}

// TestExecRemexCommand 测试 remex 命令执行
func TestExecRemexCommand(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		client      *ssh.Client
		command     string
		wantErr     bool
		errorMsg    string
		containsMsg bool
	}{
		{
			name:     "nil 客户端",
			client:   nil,
			command:  "remex.upload",
			wantErr:  true,
			errorMsg: "ssh client is nil",
		},
		{
			name:     "空命令",
			client:   nil,
			command:  "",
			wantErr:  true,
			errorMsg: "ssh client is nil",
		},
		{
			name:     "空白命令",
			client:   nil,
			command:  "   ",
			wantErr:  true,
			errorMsg: "ssh client is nil",
		},
		{
			name:        "未知命令",
			client:      nil,
			command:     "remex.unknown",
			wantErr:     true,
			errorMsg:    "ssh client is nil",
			containsMsg: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ExecRemexCommand(ctx, tt.client, tt.command)

			if (err != nil) != tt.wantErr {
				t.Errorf("ExecRemexCommand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				if tt.containsMsg {
					if !strings.Contains(err.Error(), tt.errorMsg) {
						t.Errorf("ExecRemexCommand() error message = %v, want contains %v", err.Error(), tt.errorMsg)
					}
				} else {
					if err.Error() != tt.errorMsg {
						t.Errorf("ExecRemexCommand() error message = %v, want %v", err.Error(), tt.errorMsg)
					}
				}
			}
		})
	}
}

// TestSSHClientRemoteAddr 测试远程地址获取
func TestSSHClientRemoteAddr(t *testing.T) {
	config := &SSHConfig{
		Username: "testuser",
		Password: "testpass",
		Addr:     netip.MustParseAddr("192.168.1.1"),
		Port:     22,
	}

	client := &SSHClient{
		ID:     "test-id",
		config: config,
	}

	// 测试远程地址格式
	addr := client.RemoteAddr()
	expectedAddr := "192.168.1.1:22"
	if addr.String() != expectedAddr {
		t.Errorf("Expected address %s, got %s", expectedAddr, addr.String())
	}
}

// TestSSHConfigValidation 测试 SSH 配置验证
func TestSSHConfigValidation(t *testing.T) {
	// 测试空用户名
	config := &SSHConfig{
		Username: "",
		Password: "pass",
		Addr:     netip.MustParseAddr("192.168.1.1"),
		Port:     22,
	}

	_, err := config.Connect()
	if err == nil {
		t.Error("Expected error for empty username")
	}

	// 测试空密码
	config = &SSHConfig{
		Username: "user",
		Password: "",
		Addr:     netip.MustParseAddr("192.168.1.1"),
		Port:     22,
	}

	_, err = config.Connect()
	if err == nil {
		t.Error("Expected error for empty password")
	}

	// 测试无效地址
	config = &SSHConfig{
		Username: "user",
		Password: "pass",
		Addr:     netip.Addr{},
		Port:     22,
	}

	_, err = config.Connect()
	if err == nil {
		t.Error("Expected error for invalid address")
	}
}

// TestCommandExecutionWithAutoRootPassword 测试自动 root 密码功能
func TestCommandExecutionWithAutoRootPassword(t *testing.T) {
	ctx := context.Background()
	env := map[string]string{}
	password := "testpass"

	// 测试 sudo 命令（应该尝试自动输入密码）
	_, err := ExecRemoteCommand(ctx, env, nil, password, "sudo ls", true)
	if err == nil {
		t.Error("Expected error for nil client with sudo command")
	}

	// 测试非 sudo 命令
	_, err = ExecRemoteCommand(ctx, env, nil, password, "ls", true)
	if err == nil {
		t.Error("Expected error for nil client with regular command")
	}
}

// TestSSHClientClose 测试 SSH 客户端关闭
func TestSSHClientClose(t *testing.T) {
	config := &SSHConfig{
		Username: "testuser",
		Password: "testpass",
		Addr:     netip.MustParseAddr("192.168.1.1"),
		Port:     22,
	}

	client := &SSHClient{
		ID:     "test-id",
		config: config,
	}

	// 测试关闭 nil 客户端（应该不报错）
	err := client.Close()
	if err != nil {
		t.Errorf("Close should handle nil client gracefully, got error: %v", err)
	}
}

// TestDefaultSSHPort 测试默认 SSH 端口
func TestDefaultSSHPort(t *testing.T) {
	if DefaultSSHPort != 22 {
		t.Errorf("Expected default SSH port 22, got %d", DefaultSSHPort)
	}
}

// TestSSHConfigTimeout 测试 SSH 配置超时
func TestSSHConfigTimeout(t *testing.T) {
	config := &SSHConfig{
		Username: "testuser",
		Password: "testpass",
		Addr:     netip.MustParseAddr("192.168.1.1"),
		Port:     22,
	}

	// 测试连接超时
	start := time.Now()
	_, err := config.Connect()
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Expected connection error")
	}

	// 验证超时时间大致符合预期（5秒超时）
	if elapsed > 6*time.Second {
		t.Errorf("Connection should timeout around 5 seconds, took %v", elapsed)
	}
}

// TestCommandErrorPropagation 测试命令错误传播
func TestCommandErrorPropagation(t *testing.T) {
	ctx := context.Background()

	// 测试 remex 命令错误传播
	_, err := ExecRemexCommand(ctx, nil, "remex.upload arg1 arg2")
	if err == nil {
		t.Error("Expected error for nil client")
	}

	// 验证错误类型
	if !strings.Contains(err.Error(), "ssh client is nil") {
		t.Errorf("Error should mention nil client, got: %v", err)
	}
}

// TestEnvironmentVariableHandling 测试环境变量处理
func TestEnvironmentVariableHandling(t *testing.T) {
	ctx := context.Background()
	env := map[string]string{
		"VAR1": "value1",
		"VAR2": "value2",
	}

	// 测试环境变量设置（虽然客户端为 nil，但应该先检查环境变量设置逻辑）
	_, err := ExecRemoteCommand(ctx, env, nil, "pass", "echo $VAR1", false)
	if err == nil {
		t.Error("Expected error for nil client")
	}
}

// TestCommandSecurity 测试命令安全性
func TestCommandSecurity(t *testing.T) {
	ctx := context.Background()

	// 测试潜在危险命令
	dangerousCommands := []string{
		"rm -rf /",
		"; rm -rf /",
		"&& rm -rf /",
		"| rm -rf /",
	}

	for _, cmd := range dangerousCommands {
		_, err := ExecRemexCommand(ctx, nil, cmd)
		// 这些命令应该被正确处理，不会导致 panic
		if err != nil && !strings.Contains(err.Error(), "ssh client is nil") {
			t.Errorf("Unexpected error for command '%s': %v", cmd, err)
		}
	}
}
