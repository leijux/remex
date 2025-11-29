package remex

import (
	"errors"
	"net/netip"
	"testing"
	"time"
)

// mockAddr 是一个模拟的 fmt.Stringer 接口实现，用于测试
type mockAddr struct {
	addr string
}

func (m mockAddr) String() string {
	return m.addr
}

// TestExecResult_String 测试 ExecResult 的 String 方法
func TestExecResult_String(t *testing.T) {
	fixedTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	testCases := []struct {
		name     string
		input    ExecResult
		expected string
	}{
		{
			name: "正常结果无错误",
			input: ExecResult{
				ID:         "test-id",
				Command:    "test command",
				RemoteAddr: mockAddr{addr: "192.168.1.1:22"},
				Stage:      StageStart,
				Error:      nil,
				Output:     "test output",
				Time:       fixedTime,
			},
			expected: `{"command":test command, "id":test-id, "remote_addr":192.168.1.1:22, "error":<nil>, "output":test output, "time":2023-01-01 00:00:00 +0000 UTC}`,
		},
		{
			name: "包含错误信息",
			input: ExecResult{
				ID:         "error-id",
				Command:    "failing command",
				RemoteAddr: mockAddr{addr: "192.168.1.2:22"},
				Stage:      StageConnected,
				Error:      errors.New("test error"),
				Output:     "",
				Time:       fixedTime,
			},
			expected: `{"command":failing command, "id":error-id, "remote_addr":192.168.1.2:22, "error":test error, "output":, "time":2023-01-01 00:00:00 +0000 UTC}`,
		},
		{
			name: "空输出",
			input: ExecResult{
				ID:         "empty-output",
				Command:    "empty command",
				RemoteAddr: mockAddr{addr: "192.168.1.3:22"},
				Stage:      StageFinish,
				Error:      nil,
				Output:     "",
				Time:       fixedTime,
			},
			expected: `{"command":empty command, "id":empty-output, "remote_addr":192.168.1.3:22, "error":<nil>, "output":, "time":2023-01-01 00:00:00 +0000 UTC}`,
		},
		{
			name: "空命令",
			input: ExecResult{
				ID:         "empty-command",
				Command:    "",
				RemoteAddr: mockAddr{addr: "192.168.1.4:22"},
				Stage:      StageStart,
				Error:      nil,
				Output:     "output",
				Time:       fixedTime,
			},
			expected: `{"command":, "id":empty-command, "remote_addr":192.168.1.4:22, "error":<nil>, "output":output, "time":2023-01-01 00:00:00 +0000 UTC}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.input.String()
			if result != tc.expected {
				t.Errorf("String() = %v, want %v", result, tc.expected)
			}
		})
	}
}

// TestNewSSHConfig 测试 NewSSHConfig 函数
func TestNewSSHConfig(t *testing.T) {
	testCases := []struct {
		name           string
		remoteAddr     netip.Addr
		username       string
		password       string
		expectedConfig *SSHConfig
	}{
		{
			name:       "默认配置",
			remoteAddr: netip.MustParseAddr("192.168.1.1"),
			username:   "testuser",
			password:   "testpass",
			expectedConfig: &SSHConfig{
				Username:         "testuser",
				Password:         "testpass",
				Addr:             netip.MustParseAddr("192.168.1.1"),
				Port:             DefaultSSHPort,
				autoRootPassword: true,
			},
		},
		{
			name:       "空用户名",
			remoteAddr: netip.MustParseAddr("192.168.1.1"),
			username:   "",
			password:   "testpass",
			expectedConfig: &SSHConfig{
				Username:         "",
				Password:         "testpass",
				Addr:             netip.MustParseAddr("192.168.1.1"),
				Port:             DefaultSSHPort,
				autoRootPassword: true,
			},
		},
		{
			name:       "空密码",
			remoteAddr: netip.MustParseAddr("192.168.1.1"),
			username:   "testuser",
			password:   "",
			expectedConfig: &SSHConfig{
				Username:         "testuser",
				Password:         "",
				Addr:             netip.MustParseAddr("192.168.1.1"),
				Port:             DefaultSSHPort,
				autoRootPassword: true,
			},
		},
		{
			name:       "IPv6地址",
			remoteAddr: netip.MustParseAddr("2001:db8::1"),
			username:   "testuser",
			password:   "testpass",
			expectedConfig: &SSHConfig{
				Username:         "testuser",
				Password:         "testpass",
				Addr:             netip.MustParseAddr("2001:db8::1"),
				Port:             DefaultSSHPort,
				autoRootPassword: true,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := NewSSHConfig(tc.remoteAddr, tc.username, tc.password)

			if config.Username != tc.expectedConfig.Username {
				t.Errorf("Username = %v, want %v", config.Username, tc.expectedConfig.Username)
			}
			if config.Password != tc.expectedConfig.Password {
				t.Errorf("Password = %v, want %v", config.Password, tc.expectedConfig.Password)
			}
			if config.Addr != tc.expectedConfig.Addr {
				t.Errorf("Addr = %v, want %v", config.Addr, tc.expectedConfig.Addr)
			}
			if config.Port != tc.expectedConfig.Port {
				t.Errorf("Port = %v, want %v", config.Port, tc.expectedConfig.Port)
			}
			if config.autoRootPassword != tc.expectedConfig.autoRootPassword {
				t.Errorf("autoRootPassword = %v, want %v", config.autoRootPassword, tc.expectedConfig.autoRootPassword)
			}
		})
	}
}
