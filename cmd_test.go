package remex

import (
	"context"
	"io"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// mockSSHClientForCmd 模拟 SSH 客户端用于 cmd 测试
type mockSSHClientForCmd struct {
	session *ssh.Session
}

// TestRegisterCommand 测试命令注册功能
func TestRegisterCommand(t *testing.T) {
	tests := []struct {
		name           string
		commandName    string
		commandFunc    remexCommand
		wantErr        bool
		wantExists     bool
		checkOutput    bool
		expectedOutput string
	}{
		{
			name:        "正常注册",
			commandName: "test.command",
			commandFunc: func(ctx context.Context, client *ssh.Client, args ...string) (string, error) {
				return "test output", nil
			},
			wantErr:        false,
			wantExists:     true,
			checkOutput:    true,
			expectedOutput: "test output",
		},
		{
			name:        "空名称",
			commandName: "",
			commandFunc: nil,
			wantErr:     true,
			wantExists:  false,
		},
		{
			name:        "nil 命令函数",
			commandName: "nil.command",
			commandFunc: nil,
			wantErr:     true,
			wantExists:  false,
		},
		{
			name:        "自动添加 remex. 前缀",
			commandName: "auto.prefix",
			commandFunc: func(ctx context.Context, client *ssh.Client, args ...string) (string, error) {
				return "auto prefix", nil
			},
			wantErr:        false,
			wantExists:     true,
			checkOutput:    true,
			expectedOutput: "auto prefix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RegisterCommand(tt.commandName, tt.commandFunc)

			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterCommand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// 验证命令已注册
				fullName := tt.commandName
				if !strings.HasPrefix(fullName, "remex.") {
					fullName = "remex." + fullName
				}

				cmd, exists := GetCommand(fullName)
				if exists != tt.wantExists {
					t.Errorf("GetCommand() exists = %v, wantExists %v", exists, tt.wantExists)
				}
				if tt.wantExists && cmd == nil {
					t.Error("Registered command is nil")
				}

				// 验证命令输出
				if tt.checkOutput && tt.wantExists {
					output, err := cmd(context.Background(), nil)
					if err != nil {
						t.Errorf("Command execution failed: %v", err)
					}
					if output != tt.expectedOutput {
						t.Errorf("Expected output %q, got %q", tt.expectedOutput, output)
					}
				}
			}
		})
	}
}

// TestGetCommand 测试获取命令功能
func TestGetCommand(t *testing.T) {
	tests := []struct {
		name        string
		commandName string
		wantExists  bool
		wantNil     bool
	}{
		{
			name:        "存在的命令",
			commandName: "remex.upload",
			wantExists:  true,
			wantNil:     false,
		},
		{
			name:        "不存在的命令",
			commandName: "nonexistent.command",
			wantExists:  false,
			wantNil:     true,
		},
		{
			name:        "空命令名称",
			commandName: "",
			wantExists:  false,
			wantNil:     true,
		},
		{
			name:        "下载命令",
			commandName: "remex.download",
			wantExists:  true,
			wantNil:     false,
		},
		{
			name:        "shell 命令",
			commandName: "remex.sh",
			wantExists:  true,
			wantNil:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, exists := GetCommand(tt.commandName)

			if exists != tt.wantExists {
				t.Errorf("GetCommand() exists = %v, wantExists %v", exists, tt.wantExists)
			}

			if (cmd == nil) != tt.wantNil {
				t.Errorf("GetCommand() cmd = %v, wantNil %v", cmd, tt.wantNil)
			}
		})
	}
}

// TestListCommands 测试列出命令功能
func TestListCommands(t *testing.T) {
	commands := ListCommands()
	if len(commands) == 0 {
		t.Error("No commands found")
	}

	// 检查是否包含预定义命令
	foundUpload := false
	foundDownload := false
	for _, cmd := range commands {
		if cmd == "remex.upload" {
			foundUpload = true
		}
		if cmd == "remex.download" {
			foundDownload = true
		}
	}

	if !foundUpload {
		t.Error("remex.upload not found in command list")
	}
	if !foundDownload {
		t.Error("remex.download not found in command list")
	}
}

// TestDownloadFile 测试下载文件功能
func TestDownloadFile(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		args     []string
		wantErr  bool
		errorMsg string
	}{
		{
			name:     "无参数",
			args:     []string{},
			wantErr:  true,
			errorMsg: "requires exactly 2 arguments",
		},
		{
			name:     "一个参数",
			args:     []string{"remote"},
			wantErr:  true,
			errorMsg: "requires exactly 2 arguments",
		},
		{
			name:     "过多参数",
			args:     []string{"remote", "local", "extra"},
			wantErr:  true,
			errorMsg: "requires exactly 2 arguments",
		},
		{
			name:     "空远程路径",
			args:     []string{"", "local"},
			wantErr:  true,
			errorMsg: "remote file path cannot be empty",
		},
		{
			name:     "空本地路径",
			args:     []string{"remote", ""},
			wantErr:  true,
			errorMsg: "local file path cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := downloadFile(ctx, nil, tt.args...)

			if (err != nil) != tt.wantErr {
				t.Errorf("downloadFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil && tt.errorMsg != "" {
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("downloadFile() error message = %v, want contains %v", err.Error(), tt.errorMsg)
				}
			}
		})
	}
}

// TestUploadFile 测试上传文件功能
func TestUploadFile(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		args     []string
		wantErr  bool
		errorMsg string
	}{
		{
			name:     "无参数",
			args:     []string{},
			wantErr:  true,
			errorMsg: "requires exactly 2 arguments",
		},
		{
			name:     "一个参数",
			args:     []string{"local"},
			wantErr:  true,
			errorMsg: "requires exactly 2 arguments",
		},
		{
			name:     "过多参数",
			args:     []string{"local", "remote", "extra"},
			wantErr:  true,
			errorMsg: "requires exactly 2 arguments",
		},
		{
			name:     "空本地路径",
			args:     []string{"", "remote"},
			wantErr:  true,
			errorMsg: "local file path cannot be empty",
		},
		{
			name:     "空远程路径",
			args:     []string{"local", ""},
			wantErr:  true,
			errorMsg: "remote file path cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := uploadFile(ctx, nil, tt.args...)

			if (err != nil) != tt.wantErr {
				t.Errorf("uploadFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil && tt.errorMsg != "" {
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("uploadFile() error message = %v, want contains %v", err.Error(), tt.errorMsg)
				}
			}
		})
	}
}

// TestCreateRemoteDirectory 测试创建远程目录功能
func TestCreateRemoteDirectory(t *testing.T) {
	ctx := context.Background()

	// 测试参数数量错误
	_, err := createRemoteDirectory(ctx, nil)
	if err == nil {
		t.Error("Expected error for insufficient arguments")
	}

	_, err = createRemoteDirectory(ctx, nil, "dir", "extra")
	if err == nil {
		t.Error("Expected error for too many arguments")
	}

	// 测试空路径
	_, err = createRemoteDirectory(ctx, nil, "")
	if err == nil {
		t.Error("Expected error for empty directory path")
	}
}

// TestFileExists 测试文件存在检查功能
func TestFileExists(t *testing.T) {
	// 测试参数数量错误
	_, err := fileExists(nil)
	if err == nil {
		t.Error("Expected error for insufficient arguments")
	}

	_, err = fileExists(nil, "path", "extra")
	if err == nil {
		t.Error("Expected error for too many arguments")
	}

	// 测试空路径
	_, err = fileExists(nil, "")
	if err == nil {
		t.Error("Expected error for empty file path")
	}
}

// TestUploadMemoryFile 测试内存文件上传功能
func TestUploadMemoryFile(t *testing.T) {
	ctx := context.Background()

	// 创建测试数据
	testData := "test file content"
	reader := strings.NewReader(testData)

	// 测试上传到空路径
	_, err := UploadMemoryFile(ctx, nil, "", reader)
	if err == nil {
		t.Error("Expected error for empty remote file path")
	}

	// 测试 nil 客户端（应该返回错误而不是 panic）
	_, err = UploadMemoryFile(ctx, nil, "/tmp/test.txt", reader)
	if err == nil {
		t.Error("Expected error for nil client")
	}
}

// TestNewInterruptibleReader 测试可中断读取器
func TestNewInterruptibleReader(t *testing.T) {
	ctx := context.Background()
	testData := "test data"
	reader := strings.NewReader(testData)

	// 创建可中断读取器
	interruptibleReader := NewInterruptibleReader(ctx, reader)

	// 测试正常读取
	buffer := make([]byte, len(testData))
	n, err := interruptibleReader.Read(buffer)
	if err != nil && err != io.EOF {
		t.Errorf("Read failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Expected to read %d bytes, got %d", len(testData), n)
	}
	if string(buffer) != testData {
		t.Errorf("Expected data '%s', got '%s'", testData, string(buffer))
	}

	// 测试上下文取消
	ctxCancel, cancel := context.WithCancel(context.Background())
	cancel()
	interruptibleReaderCanceled := NewInterruptibleReader(ctxCancel, reader)

	_, err = interruptibleReaderCanceled.Read(buffer)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

// TestCommandExecution 测试命令执行流程
func TestCommandExecution(t *testing.T) {
	ctx := context.Background()

	// 测试 remex 命令执行
	_, err := ExecRemexCommand(ctx, nil, "remex.upload")
	if err == nil {
		t.Error("Expected error for nil client")
	}

	// 测试无效命令
	_, err = ExecRemexCommand(ctx, nil, "")
	if err == nil {
		t.Error("Expected error for empty command")
	}

	// 测试未知命令
	_, err = ExecRemexCommand(ctx, nil, "unknown.command")
	if err == nil {
		t.Error("Expected error for unknown command")
	}
}

// TestCommandErrorHandling 测试命令错误处理
func TestCommandErrorHandling(t *testing.T) {
	// 测试注册重复命令（应该成功覆盖）
	err := RegisterCommand("duplicate.test", func(ctx context.Context, client *ssh.Client, args ...string) (string, error) {
		return "first", nil
	})
	if err != nil {
		t.Errorf("First registration failed: %v", err)
	}

	err = RegisterCommand("duplicate.test", func(ctx context.Context, client *ssh.Client, args ...string) (string, error) {
		return "second", nil
	})
	if err != nil {
		t.Errorf("Second registration failed: %v", err)
	}

	// 验证覆盖成功
	cmd, exists := GetCommand("remex.duplicate.test")
	if !exists {
		t.Error("Duplicate command not found")
	}
	output, err := cmd(context.Background(), nil)
	if err != nil {
		t.Errorf("Command execution failed: %v", err)
	}
	if output != "second" {
		t.Errorf("Expected output 'second', got '%s'", output)
	}
}

// TestCommandWithContext 测试带上下文的命令执行
func TestCommandWithContext(t *testing.T) {
	tests := []struct {
		name       string
		setupCtx   func() context.Context
		args       []string
		wantErr    bool
		errorCheck func(error) bool
	}{
		{
			name: "取消的上下文",
			setupCtx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // 立即取消
				return ctx
			},
			args:       []string{"echo", "test"},
			wantErr:    true,
			errorCheck: func(err error) bool { return err != nil },
		},
		{
			name: "正常上下文",
			setupCtx: func() context.Context {
				return context.Background()
			},
			args:       []string{"echo", "test"},
			wantErr:    false,
			errorCheck: func(err error) bool { return err == nil },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupCtx()
			_, err := shScript(ctx, nil, tt.args...)

			if (err != nil) != tt.wantErr {
				t.Errorf("shScript() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.errorCheck != nil && !tt.errorCheck(err) {
				t.Errorf("shScript() error check failed for error: %v", err)
			}
		})
	}
}

// TestCommandOutputFormat 测试命令输出格式
func TestCommandOutputFormat(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantOutput  string
		wantErr     bool
		checkOutput bool
	}{
		{
			name:        "多行输出",
			args:        []string{"echo", "-e", `"line1\nline2\nline3"`},
			wantOutput:  "line1\nline2\nline3\n",
			wantErr:     false,
			checkOutput: true,
		},
		{
			name:        "单行输出",
			args:        []string{"echo", "single line"},
			wantOutput:  "single line\n",
			wantErr:     false,
			checkOutput: true,
		},
		{
			name:        "空输出",
			args:        []string{"true"},
			wantOutput:  "",
			wantErr:     false,
			checkOutput: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			output, err := shScript(ctx, nil, tt.args...)

			if (err != nil) != tt.wantErr {
				t.Errorf("shScript() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.checkOutput && output != tt.wantOutput {
				t.Errorf("shScript() output = %q, want %q", output, tt.wantOutput)
			}
		})
	}
}

// TestCommandArgumentHandling 测试命令参数处理
func TestCommandArgumentHandling(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantOutput  string
		wantErr     bool
		checkOutput bool
	}{
		{
			name:        "带空格的参数",
			args:        []string{"echo", "hello world"},
			wantOutput:  "hello world\n",
			wantErr:     false,
			checkOutput: true,
		},
		{
			name:        "多个参数格式化",
			args:        []string{"printf", `"%s %s"`, "arg1", "arg2"},
			wantOutput:  "arg1 arg2",
			wantErr:     false,
			checkOutput: true,
		},
		{
			name:        "空参数",
			args:        []string{"echo", ""},
			wantOutput:  "\n",
			wantErr:     false,
			checkOutput: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			output, err := shScript(ctx, nil, tt.args...)

			if (err != nil) != tt.wantErr {
				t.Errorf("shScript() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.checkOutput && output != tt.wantOutput {
				t.Errorf("shScript() output = %q, want %q", output, tt.wantOutput)
			}
		})
	}
}
