package remex

import (
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
	*ssh.Client
}

// NewSSHClient creates a new SSHClient instance
func NewSSHClient(config SSHConfig) (*SSHClient, error) {
	client, err := config.Connect()
	if err != nil {
		return nil, err
	}

	return &SSHClient{client}, nil
}

// ExecuteCommand executes a command on the remote server and returns the output
func (sc *SSHClient) ExecuteCommand(command string) (string, error) {
	var (
		output string
		err    error
	)

	if strings.HasPrefix(command, "remex.") {
		output, err = ExecRemexCommand(sc.Client, command)
	} else {
		output, err = ExecRemoteCommand(sc.Client, command)
	}

	if err != nil {
		return "", err
	}

	return output, nil
}
