package remex

import (
	"context"
	"net/netip"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// TestRegisterCommand 测试 RegisterCommand 函数
func TestRegisterCommand(t *testing.T) {
	// 保存原始命令以便测试后恢复
	originalCommands := make(map[string]remexCommand)
	for k, v := range registry.commands {
		originalCommands[k] = v
	}
	defer func() {
		registry.commands = originalCommands
	}()

	testCases := []struct {
		name          string
		commandName   string
		commandFunc   remexCommand
		shouldError   bool
		expectedError string
	}{
		{
			name:        "正常注册",
			commandName: "test.command",
			commandFunc: func(ctx context.Context, client *ssh.Client, args ...string) (string, error) {
				return "success", nil
			},
			shouldError: false,
		},
		{
			name:        "空命令名",
			commandName: "",
			commandFunc: func(ctx context.Context, client *ssh.Client, args ...string) (string, error) {
				return "success", nil
			},
			shouldError:   true,
			expectedError: "command name cannot be empty",
		},
		{
			name:          "nil 命令函数",
			commandName:   "test.nil",
			commandFunc:   nil,
			shouldError:   true,
			expectedError: "command function cannot be nil",
		},
		{
			name:        "自动添加 remex. 前缀",
			commandName: "custom.command",
			commandFunc: func(ctx context.Context, client *ssh.Client, args ...string) (string, error) {
				return "custom", nil
			},
			shouldError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := RegisterCommand(tc.commandName, tc.commandFunc)

			if tc.shouldError {
				if err == nil {
					t.Errorf("RegisterCommand() expected error, got nil")
				} else if !strings.Contains(err.Error(), tc.expectedError) {
					t.Errorf("RegisterCommand() error = %v, want %v", err, tc.expectedError)
				}
			} else {
				if err != nil {
					t.Errorf("RegisterCommand() unexpected error = %v", err)
				}

				// 验证命令是否已注册
				expectedName := tc.commandName
				if !strings.HasPrefix(expectedName, "remex.") {
					expectedName = "remex." + expectedName
				}

				cmd, exists := GetCommand(expectedName)
				if !exists {
					t.Errorf("GetCommand() command %s not found after registration", expectedName)
				}
				if cmd == nil {
					t.Errorf("GetCommand() returned nil command for %s", expectedName)
				}
			}
		})
	}
}

// TestGetCommand 测试 GetCommand 函数
func TestGetCommand(t *testing.T) {
	testCases := []struct {
		name        string
		commandName string
		exists      bool
	}{
		{
			name:        "存在的命令",
			commandName: "remex.upload",
			exists:      true,
		},
		{
			name:        "不存在的命令",
			commandName: "remex.nonexistent",
			exists:      false,
		},
		{
			name:        "空命令名",
			commandName: "",
			exists:      false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd, exists := GetCommand(tc.commandName)

			if exists != tc.exists {
				t.Errorf("GetCommand() exists = %v, want %v", exists, tc.exists)
			}

			if tc.exists && cmd == nil {
				t.Errorf("GetCommand() returned nil command for existing command %s", tc.commandName)
			}

			if !tc.exists && cmd != nil {
				t.Errorf("GetCommand() returned non-nil command for non-existing command %s", tc.commandName)
			}
		})
	}
}

// TestListCommands 测试 ListCommands 函数
func TestListCommands(t *testing.T) {
	commands := ListCommands()

	// 检查是否返回了命令列表
	if len(commands) == 0 {
		t.Error("ListCommands() returned empty command list")
	}

	// 检查是否包含预期的内置命令
	expectedCommands := []string{
		"remex.upload",
		"remex.download",
		"remex.sh",
		"remex.mkdir",
	}

	for _, expected := range expectedCommands {
		found := false
		for _, cmd := range commands {
			if cmd == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ListCommands() missing expected command: %s", expected)
		}
	}

	// 检查所有命令都有 remex. 前缀
	for _, cmd := range commands {
		if !strings.HasPrefix(cmd, "remex.") {
			t.Errorf("ListCommands() command %s does not have remex. prefix", cmd)
		}
	}
}

// TestNewInterruptibleReader 测试 NewInterruptibleReader 函数
func TestNewInterruptibleReader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建一个简单的字符串读取器
	reader := strings.NewReader("test data")
	interruptibleReader := NewInterruptibleReader(ctx, reader)

	// 测试正常读取
	buf := make([]byte, 4)
	n, err := interruptibleReader.Read(buf)
	if err != nil {
		t.Errorf("NewInterruptibleReader() Read error = %v", err)
	}
	if n != 4 {
		t.Errorf("NewInterruptibleReader() Read bytes = %v, want %v", n, 4)
	}
	if string(buf[:n]) != "test" {
		t.Errorf("NewInterruptibleReader() Read content = %v, want %v", string(buf[:n]), "test")
	}

	// 测试上下文取消时的读取
	cancel()
	buf2 := make([]byte, 4)
	_, err = interruptibleReader.Read(buf2)
	if err != context.Canceled {
		t.Errorf("NewInterruptibleReader() Read after cancel error = %v, want %v", err, context.Canceled)
	}
}

// TestSSHClient_ID 测试 SSHClient 的 ID 方法
func TestSSHClient_ID(t *testing.T) {
	client := &SSHClient{
		id: "test-client-id",
	}

	id := client.ID()
	if id != "test-client-id" {
		t.Errorf("SSHClient.ID() = %v, want %v", id, "test-client-id")
	}
}

// TestSSHClient_RemoteAddr 测试 SSHClient 的 RemoteAddr 方法
func TestSSHClient_RemoteAddr(t *testing.T) {
	testCases := []struct {
		name     string
		client   *SSHClient
		expected netip.AddrPort
	}{
		{
			name: "正常配置",
			client: &SSHClient{
				config: &SSHConfig{
					Addr: netip.MustParseAddr("192.168.1.1"),
					Port: 22,
				},
			},
			expected: netip.AddrPortFrom(netip.MustParseAddr("192.168.1.1"), 22),
		},
		{
			name: "nil 配置",
			client: &SSHClient{
				config: nil,
			},
			expected: netip.AddrPort{},
		},
		{
			name: "IPv6 地址",
			client: &SSHClient{
				config: &SSHConfig{
					Addr: netip.MustParseAddr("2001:db8::1"),
					Port: 2222,
				},
			},
			expected: netip.AddrPortFrom(netip.MustParseAddr("2001:db8::1"), 2222),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			addr := tc.client.RemoteAddr()
			if addr != tc.expected {
				t.Errorf("SSHClient.RemoteAddr() = %v, want %v", addr, tc.expected)
			}
		})
	}
}

// TestDefaultSSHPort 测试默认 SSH 端口常量
func TestDefaultSSHPort(t *testing.T) {
	if DefaultSSHPort != 22 {
		t.Errorf("DefaultSSHPort = %v, want %v", DefaultSSHPort, 22)
	}
}

// TestResultHandlerType 测试 ResultHandler 类型定义
func TestResultHandlerType(t *testing.T) {
	// 这个测试主要是确保 ResultHandler 类型可以正常使用
	var handler ResultHandler = func(result ExecResult) {
		// 空的处理器函数
	}

	if handler == nil {
		t.Error("ResultHandler type is not usable")
	}
}

// TestStageType 测试 Stage 类型
func TestStageType(t *testing.T) {
	// 测试 Stage 类型的字符串表示
	stages := []Stage{StageError, StageStart, StageFinish}
	expected := []string{"err", "start", "finish"}

	for i, stage := range stages {
		if string(stage) != expected[i] {
			t.Errorf("Stage[%d] = %v, want %v", i, string(stage), expected[i])
		}
	}
}
