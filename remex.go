package remex

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// ExecResult represents the result of command execution
type ExecResult struct {
	Index      int          `json:"index"`
	ID         string       `json:"id"`
	RemoteAddr fmt.Stringer `json:"remote_addr"`

	Error  error  `json:"error,omitempty"`
	Output string `json:"output,omitempty"`

	Time string `json:"time"`
}

func (er ExecResult) String() string {
	return fmt.Sprintln(
		"index:", er.Index,
		"id:", er.ID,
		"remote_addr:", er.RemoteAddr.String(),
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
	configs map[string]*SSHConfig

	logger *slog.Logger

	handlers []ResultHandler

	ctx        context.Context
	cancelFunc context.CancelFunc

	errGroup *errgroup.Group
	mutex    sync.RWMutex
}

// NewWithContext creates a new DistExec instance with the given context and configuration
func NewWithContext(ctx context.Context, logger *slog.Logger, configs map[string]*SSHConfig) *Remex {
	ctx, cancel := context.WithCancel(ctx)
	return &Remex{
		clients:    make(map[string]*SSHClient),
		configs:    configs,
		logger:     logger,
		ctx:        ctx,
		cancelFunc: cancel,
		errGroup:   &errgroup.Group{},
	}
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
		r.logger.Debug("notifying handler", "ID", result.ID, "remote", result.RemoteAddr, "index", result.Index)

		handler(result)
	}
}

// Connect establishes SSH connections to all remote hosts
func (r *Remex) Connect() error {
	var connectionErrors []error

	for id, config := range r.configs {
		select {
		case <-r.ctx.Done():
			return r.ctx.Err()
		default:
			client, err := NewSSHClient(id, config)
			if err != nil {
				r.logger.Error("failed to establish SSH connection",
					"remote", config.Addr, "error", err)
				connectionErrors = append(connectionErrors, err)

				r.notifyHandlers(ExecResult{
					Index:      -1,
					ID:         id,
					RemoteAddr: config.Addr,
					Error:      err,
				})

				continue
			}
			r.clients[id] = client

			r.logger.Info("SSH connection established", "remote", config.Addr)
		}
	}

	if len(r.clients) == 0 {
		return fmt.Errorf("no successful connections: %w", errors.Join(connectionErrors...))
	}

	r.logger.Info("connections established",
		"successful", len(r.clients),
		"total", len(r.configs))

	return nil
}
func (r *Remex) ExecuteWithID(id string, commands []string) error {
	if len(r.clients) == 0 {
		if err := r.Connect(); err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
	}

	client, ok := r.clients[id]
	if !ok {
		return fmt.Errorf("no client found for id %s", id)
	}

	r.logger.Debug("executing commands", "id", id, "remote", client.RemoteAddr().String())

	r.errGroup.Go(func() error {
		return r.execCommands(client, commands)
	})

	if err := r.errGroup.Wait(); err != nil {
		return err
	}

	return nil
}

// Execute executes commands on all connected remote hosts
func (r *Remex) Execute(commands []string) error {
	if len(r.clients) == 0 {
		if err := r.Connect(); err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
	}

	for id, client := range r.clients {
		r.logger.Debug("executing commands", "id", id, "remote", client.RemoteAddr())

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
		remoteAddr = client.RemoteAddr()
		logger     = r.logger.With("name", client.ID, "remote", remoteAddr)
	)

	for i := 0; i < len(commands)*2; i += 2 {
		command := commands[i/2]
		select {
		case <-r.ctx.Done():
			logger.Debug("execution cancelled")
			return r.ctx.Err()
		default:
			r.notifyHandlers(ExecResult{Index: i, ID: client.ID, RemoteAddr: remoteAddr})

			output, err := client.ExecuteCommand(r.ctx, command)

			r.notifyHandlers(ExecResult{Index: i + 1, ID: client.ID, RemoteAddr: remoteAddr, Output: output, Error: err})

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

// GetConnectedHosts returns the list of currently connected hosts
func (r *Remex) GetConnectedHosts() map[string]string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	hosts := make(map[string]string, len(r.clients))
	for _, client := range r.clients {
		hosts[client.ID] = client.RemoteAddr().String()
	}
	return hosts
}

// GetClientByName returns the SSHClient with the given name
func (r *Remex) GetClientByName(name string) *SSHClient {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	for _, client := range r.clients {
		if client.ID == name {
			return client
		}
	}
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
