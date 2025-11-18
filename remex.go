package remex

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"slices"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// Config holds the configuration for distributed execution
type Config struct {
	Name string

	SSHConfig SSHConfig
	Commands  []string
}

// NewRemoteConfig creates a default configuration
func NewRemoteConfig(remoteAddr netip.Addr, username, password string, commands []string) *Config {
	config := &Config{
		SSHConfig: SSHConfig{
			Username: username,
			Password: password,
			Addr:     remoteAddr,
			Port:     DefaultSSHPort,
		},
		Commands: commands,
	}

	return config
}

// ExecResult represents the result of command execution
type ExecResult struct {
	Index      int    `json:"index"`
	RemoteAddr string `json:"remote_addr"`

	Error  error  `json:"error,omitempty"`
	Output string `json:"output,omitempty"`

	Time string `json:"time"`
}

func (er ExecResult) String() string {
	return fmt.Sprintln(
		"index:", er.Index,
		"remote_addr:", er.RemoteAddr,
		"error:", er.Error,
		"output:", er.Output,
		"time:", er.Time,
	)
}

// ResultHandler is a function type for handling execution results
type ResultHandler func(ExecResult)

// Remex represents a distributed command execution engine
type Remex struct {
	clients map[string]*SSHClient

	config []*Config

	logger *slog.Logger

	handlers []ResultHandler

	ctx        context.Context
	cancelFunc context.CancelFunc

	errGroup *errgroup.Group
	mutex    sync.RWMutex
}

// NewWithConfig creates a new DistExec instance with the given configuration
func NewWithConfig(config []*Config, logger *slog.Logger) *Remex {
	ctx, cancel := context.WithCancel(context.Background())
	return &Remex{
		logger:     logger,
		config:     config,
		ctx:        ctx,
		cancelFunc: cancel,
		errGroup:   &errgroup.Group{},
	}
}

// NewWithContext creates a new DistExec instance with the given context and configuration
func NewWithContext(ctx context.Context, config []*Config, logger *slog.Logger) *Remex {
	ctx, cancel := context.WithCancel(ctx)
	de := NewWithConfig(config, logger)

	g, ctx := errgroup.WithContext(ctx)

	de.ctx = ctx
	de.cancelFunc = cancel
	de.errGroup = g

	return de
}

// RegisterHandler registers handler functions for receiving execution results
func (r *Remex) RegisterHandler(handlers ...ResultHandler) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.handlers = append(r.handlers, handlers...)
}

// notifyHandlers sends execution results to all registered handlers
func (r *Remex) notifyHandlers(result ExecResult) {
	result.Time = time.Now().Format(time.DateTime)

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	for _, handler := range r.handlers {
		r.logger.Debug("notifying handler", "remote", result.RemoteAddr, "index", result.Index)

		handler(result)
	}
}

// Connect establishes SSH connections to all remote hosts
func (r *Remex) Connect() error {
	var connectionErrors []error

	for _, config := range r.config {
		select {
		case <-r.ctx.Done():
			return r.ctx.Err()
		default:
			client, err := NewSSHClient(config.Name, config.SSHConfig)
			if err != nil {
				r.logger.Error("failed to establish SSH connection",
					"remote", config.SSHConfig.Addr, "error", err)
				connectionErrors = append(connectionErrors, err)

				r.notifyHandlers(ExecResult{
					Index:      -1,
					RemoteAddr: config.SSHConfig.Addr.String(),
					Error:      err,
				})

				continue
			}
			r.clients[config.Name] = client

			r.logger.Info("SSH connection established", "remote", config.SSHConfig.Addr)
		}
	}

	if len(r.clients) == 0 {
		return fmt.Errorf("no successful connections: %w", errors.Join(connectionErrors...))
	}

	r.logger.Info("connections established",
		"successful", len(r.clients),
		"total", len(r.config))

	return nil
}

// Execute executes commands on all connected remote hosts
func (r *Remex) Execute() error {
	if len(r.clients) == 0 {
		if err := r.Connect(); err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
	}

	for _, client := range r.clients {
		i := slices.IndexFunc(r.config, func(config *Config) bool {
			return netip.AddrPortFrom(config.SSHConfig.Addr, config.SSHConfig.Port).String() == client.RemoteAddr().String()
		})

		if i == -1 {
			return fmt.Errorf("failed to find config for remote host: %s", client.RemoteAddr().String())
		}
		client := client
		commands := r.config[i].Commands
		r.errGroup.Go(func() error {
			return r.execCommands(client, commands)
		})
	}

	if err := r.errGroup.Wait(); err != nil {
		return err
	}

	return nil
}

// executeCommands executes all commands on a single remote host
func (r *Remex) execCommands(client *SSHClient, commands []string) error {
	var (
		remoteAddr = client.RemoteAddr().String()
		logger     = r.logger.With("remote", remoteAddr)
	)

	for i := 0; i < len(commands)*2; i += 2 {
		command := commands[i/2]
		select {
		case <-r.ctx.Done():
			logger.Info("execution cancelled")
			return r.ctx.Err()
		default:
			r.notifyHandlers(ExecResult{Index: i, RemoteAddr: remoteAddr})

			output, err := client.ExecuteCommand(r.ctx, command)

			r.notifyHandlers(ExecResult{Index: i + 1, RemoteAddr: remoteAddr, Output: output, Error: err})

			if err != nil {
				return fmt.Errorf("failed to execute command: %s", command)
			} else {
				logger.Debug("command execution details", "command", command, "output", output)
			}

		}
	}

	logger.Info("command execution completed successfully")
	return nil
}

// Close closes all SSH connections and cleans up resources
func (r *Remex) Close() error {
	r.cancelFunc()
	if err := r.errGroup.Wait(); err != nil {
		return err
	}

	var closeErrors []error
	for _, client := range r.clients {
		if err := client.Close(); err != nil {
			closeErrors = append(closeErrors, err)
		}
	}

	if len(closeErrors) > 0 {
		return fmt.Errorf("errors closing clients: %v", closeErrors)
	}

	r.logger.Info("all connections closed")
	return nil
}

// GetConnectedHosts returns the list of currently connected hosts
func (r *Remex) GetConnectedHosts() map[string]string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	hosts := make(map[string]string, len(r.clients))
	for _, client := range r.clients {
		hosts[client.Name] = client.RemoteAddr().String()
	}
	return hosts
}
