package remex

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"sync"
	"testing"
	"time"
)

// TestExecResult 测试 ExecResult 结构体
func TestExecResult(t *testing.T) {
	addr := netip.MustParseAddr("192.168.1.1")
	addrPort := netip.AddrPortFrom(addr, 22)
	now := time.Now()

	result := ExecResult{
		Index:      1,
		ID:         "test-id",
		RemoteAddr: addrPort,
		Error:      errors.New("test error"),
		Output:     "test output",
		Time:       now,
	}

	// 测试 String 方法
	str := result.String()
	if str == "" {
		t.Error("ExecResult.String() returned empty string")
	}

	// 测试字段值
	if result.Index != 1 {
		t.Errorf("Expected Index 1, got %d", result.Index)
	}
	if result.ID != "test-id" {
		t.Errorf("Expected ID 'test-id', got '%s'", result.ID)
	}
	if result.Error.Error() != "test error" {
		t.Errorf("Expected error 'test error', got '%v'", result.Error)
	}
	if result.Output != "test output" {
		t.Errorf("Expected output 'test output', got '%s'", result.Output)
	}
}

// TestNewWithContext 测试 Remex 实例创建
func TestNewWithContext(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()
	configs := map[string]*SSHConfig{
		"host1": NewSSHConfig(netip.MustParseAddr("192.168.1.1"), "user", "pass"),
	}

	remex := NewWithContext(ctx, logger, configs)

	if remex == nil {
		t.Fatal("NewWithContext returned nil")
	}
	if remex.ctx == nil {
		t.Error("Context is nil")
	}
	if remex.logger == nil {
		t.Error("Logger is nil")
	}
	if len(remex.configs) != 1 {
		t.Errorf("Expected 1 config, got %d", len(remex.configs))
	}
}

// TestRegisterHandler 测试处理器注册
func TestRegisterHandler(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()
	configs := map[string]*SSHConfig{}
	remex := NewWithContext(ctx, logger, configs)

	// 测试注册单个处理器
	called := false
	handler := func(result ExecResult) {
		called = true
	}

	remex.RegisterHandler(handler)

	if len(remex.handlers) != 1 {
		t.Errorf("Expected 1 handler, got %d", len(remex.handlers))
	}

	// 测试注册多个处理器
	handler2 := func(result ExecResult) {}
	remex.RegisterHandler(handler, handler2)

	if len(remex.handlers) != 3 {
		t.Errorf("Expected 3 handlers, got %d", len(remex.handlers))
	}

	// 测试处理器调用
	remex.notifyHandlers(ExecResult{})
	if !called {
		t.Error("Handler was not called")
	}
}

// TestGetConnectedHosts 测试获取已连接主机
func TestGetConnectedHosts(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()
	configs := map[string]*SSHConfig{}
	remex := NewWithContext(ctx, logger, configs)

	// 测试空客户端列表
	hosts := remex.GetConnectedHosts()
	if len(hosts) != 0 {
		t.Errorf("Expected 0 hosts, got %d", len(hosts))
	}

	// 添加模拟客户端
	config := NewSSHConfig(netip.MustParseAddr("192.168.1.1"), "user", "pass")
	remex.clients = map[string]*SSHClient{
		"host1": {ID: "host1", config: config},
	}

	hosts = remex.GetConnectedHosts()
	if len(hosts) != 1 {
		t.Errorf("Expected 1 host, got %d", len(hosts))
	}
	// 注意：由于 SSHClient 没有实际的 SSH 连接，RemoteAddr() 可能返回空
	// 我们主要测试函数不 panic
}

// TestGetClientByName 测试按名称获取客户端
func TestGetClientByName(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()
	configs := map[string]*SSHConfig{}
	remex := NewWithContext(ctx, logger, configs)

	// 测试不存在的客户端
	client := remex.GetClientByName("nonexistent")
	if client != nil {
		t.Error("Expected nil for nonexistent client")
	}

	// 添加模拟客户端
	testClient := &SSHClient{ID: "host1", config: NewSSHConfig(netip.MustParseAddr("192.168.1.1"), "user", "pass")}
	remex.clients = map[string]*SSHClient{
		"host1": testClient,
	}

	client = remex.GetClientByName("host1")
	if client != testClient {
		t.Error("Did not get expected client")
	}
}

// TestClose 测试关闭功能
func TestClose(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()
	configs := map[string]*SSHConfig{}
	remex := NewWithContext(ctx, logger, configs)

	// 测试空客户端关闭
	err := remex.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// 测试带客户端的关闭
	remex.clients = map[string]*SSHClient{
		"host1": {ID: "host1", config: NewSSHConfig(netip.MustParseAddr("192.168.1.1"), "user", "pass")},
	}

	err = remex.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

// TestConcurrentAccess 测试并发访问
func TestConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()
	configs := map[string]*SSHConfig{}
	remex := NewWithContext(ctx, logger, configs)

	var wg sync.WaitGroup
	iterations := 100

	// 并发注册处理器
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			remex.RegisterHandler(func(result ExecResult) {})
		}(i)
	}

	// 并发获取主机列表
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = remex.GetConnectedHosts()
		}()
	}

	wg.Wait()

	// 验证处理器数量
	if len(remex.handlers) != iterations {
		t.Errorf("Expected %d handlers, got %d", iterations, len(remex.handlers))
	}
}

// TestErrorGroup 测试错误组功能
func TestErrorGroup(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()
	configs := map[string]*SSHConfig{}
	remex := NewWithContext(ctx, logger, configs)

	// 测试错误组等待
	err := remex.errGroup.Wait()
	if err != nil {
		t.Errorf("errGroup.Wait failed: %v", err)
	}
}
