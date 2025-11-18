package remex

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

var (
	DefaultSSHPort uint16 = 22
)

// SSHConfig holds the configuration for SSH connection
type SSHConfig struct {
	Username string
	Password string

	Addr netip.Addr
	Port uint16
}

// Connect establishes an SSH connection
func (config SSHConfig) Connect() (*ssh.Client, error) {
	sshConfig := &ssh.ClientConfig{
		User: config.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(config.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	addrPort := netip.AddrPortFrom(config.Addr, config.Port)

	client, err := ssh.Dial("tcp", addrPort.String(), sshConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", addrPort.String(), err)
	}
	return client, nil
}

type SSHClient struct {
	Name string

	*ssh.Client
}

// NewSSHClient creates a new SSHClient instance
func NewSSHClient(name string, config SSHConfig) (*SSHClient, error) {
	client, err := config.Connect()
	if err != nil {
		return nil, err
	}

	return &SSHClient{name, client}, nil
}

// ExecuteCommand executes a command on the remote server and returns the output
func (sc *SSHClient) ExecuteCommand(ctx context.Context, command string) (string, error) {
	var (
		output string
		err    error
	)

	if strings.HasPrefix(command, "remex.") {
		output, err = ExecRemexCommand(ctx, sc.Client, command)
	} else {
		output, err = ExecRemoteCommand(ctx, map[string]string{"REMEX_NAME": sc.Name}, sc.Client, command)
	}

	if err != nil {
		return "", err
	}

	return output, nil
}

// ExecuteRemoteCommand executes a command on the remote server and returns the output
func ExecRemoteCommand(ctx context.Context, env map[string]string, client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	for k, v := range env {
		session.Setenv(k, v)
	}

	outputCh := make(chan []byte)
	errCh := make(chan error)

	// 读取输出 goroutine
	go func() {
		output, err := session.CombinedOutput(command)

		errCh <- err
		outputCh <- output
	}()

	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGKILL) // 发送 KILL 信号到远程

		return "", ctx.Err()
	case err := <-errCh:
		output := <-outputCh // 命令结束

		if err != nil {
			return string(output), fmt.Errorf("command execution failed: %w", err)
		}
		return string(output), nil
	}
}

// ExecuteRemexCommand executes a command on the remote server and returns the output
func ExecRemexCommand(ctx context.Context, client *ssh.Client, command string) (string, error) {
	if client == nil {
		return "", fmt.Errorf("ssh client is nil")
	}

	commandSplit := strings.Split(strings.TrimSpace(command), " ")
	if len(commandSplit) == 0 {
		return "", errors.New("invalid command")
	}

	if iFunc, exists := GetCommand(commandSplit[0]); exists {
		output, err := iFunc(ctx, client, commandSplit[1:]...)
		if err != nil {
			return "", fmt.Errorf("remex command '%s' failed: %w", commandSplit[0], err)
		}
		return output, nil
	}

	return "", fmt.Errorf("unknown remex command: %s", commandSplit[0])
}
