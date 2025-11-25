package remex

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasttemplate"
	"golang.org/x/sync/errgroup"
)

const remexID = "REMEX_ID"

type Stage string

const (
	Connected Stage = "connected"
	Start     Stage = "start"
	Finish    Stage = "finish"
)

// ExecResult represents the result of command execution
type ExecResult struct {
	ID string `json:"id"`

	Command    string       `json:"command"`
	RemoteAddr fmt.Stringer `json:"remote_addr"`
	Stage      Stage        `json:"stage"`
	Error      error        `json:"error,omitempty"`
	Output     string       `json:"output,omitempty"`

	Time time.Time `json:"time"`
}

func (er ExecResult) String() string {
	return fmt.Sprintf(`{"command":%s, "id":%s, "remote_addr":%v, "error":%v, "output":%s, "time":%v}`,
		er.Command, er.ID, er.RemoteAddr, er.Error, er.Output, er.Time)
}

// ResultHandler is a function type for handling execution results
type ResultHandler func(ExecResult)

// Remex represents a distributed command execution engine
type Remex struct {
	clients map[string]RemoteClient
	configs map[string]*SSHConfig

	logger *slog.Logger

	results  chan ExecResult
	handlers []ResultHandler

	ctx        context.Context
	cancelFunc context.CancelFunc

	errGroup *errgroup.Group
	mutex    sync.RWMutex

	newSSHClient func(string, *SSHConfig) (RemoteClient, error)
}

// NewWithContext creates a new DistExec instance with the given context and configuration
func NewWithContext(ctx context.Context, logger *slog.Logger, configs map[string]*SSHConfig) *Remex {
	if logger == nil {
		logger = slog.Default()
	}

	ctx, cancel := context.WithCancel(ctx)
	g, ctx := errgroup.WithContext(ctx)
	return &Remex{
		clients:    make(map[string]RemoteClient),
		configs:    configs,
		logger:     logger,
		results:    make(chan ExecResult),
		ctx:        ctx,
		cancelFunc: cancel,
		errGroup:   g,

		newSSHClient: NewSSHClient,
	}
}

// setNewSSHClient sets a custom function for creating SSH clients
// test using custom SSH client
func (r *Remex) setNewSSHClient(newF func(string, *SSHConfig) (RemoteClient, error)) {
	r.newSSHClient = newF
}

// RegisterHandler registers handler functions for receiving execution results
func (r *Remex) RegisterHandler(handlers ...ResultHandler) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.handlers = append(r.handlers, handlers...)
}

// notifyHandlers sends execution results to all registered handlers
func (r *Remex) notifyHandlers(result ExecResult) {
	result.Time = time.Now()

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	go func() {
		for res := range r.results {
			for _, h := range r.handlers {
				r.logger.Debug("notifying handler", "ID", result.ID, "remote", result.RemoteAddr, "command", result.Command)
				h(res)
			}
		}
	}()
}

// Connect establishes SSH connections to all remote hosts
func (r *Remex) Connect() error {
	var connectionErrors []error

	for id, config := range r.configs {
		select {
		case <-r.ctx.Done():
			return r.ctx.Err()
		default:
			client, err := r.newSSHClient(id, config)
			if err != nil {
				r.logger.Error("failed to establish SSH connection",
					"remote", config.Addr, "error", err)
				connectionErrors = append(connectionErrors, fmt.Errorf("host %s (%s): %w", id, config.Addr, err))
				r.notifyHandlers(ExecResult{ID: client.ID(), Stage: Connected, RemoteAddr: config.Addr})

				continue
			}

			r.mutex.Lock()
			if _, ok := r.clients[id]; ok {
				r.clients[id].Close()
			}

			r.clients[id] = client
			r.mutex.Unlock()

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

// ExecuteWithID executes commands on a specific remote host identified by its ID
func (r *Remex) ExecuteWithID(id string, command string) (string, error) {
	client, ok := r.clients[id]
	if !ok {
		return "", fmt.Errorf("no client found for id %s", id)
	}

	r.logger.Debug("executing commands", "id", id, "remote", client.RemoteAddr())

	return client.ExecuteCommand(r.ctx, command)
}

// Execute executes commands on all connected remote hosts
func (r *Remex) Execute(commands []string) error {
	for id, client := range r.clients {
		r.logger.Debug("executing commands", "id", id, "remote", client.RemoteAddr())

		commands = strings.Split(fasttemplate.ExecuteString(strings.Join(commands, "\n"), "{{", "}}", map[string]any{
			remexID: id,
		}), "\n")

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
func (r *Remex) execCommands(client RemoteClient, commands []string) error {
	var (
		remoteAddr = client.RemoteAddr()
		logger     = r.logger.With("id", client.ID(), "remote", remoteAddr)
	)

	for _, command := range commands {
		select {
		case <-r.ctx.Done():
			logger.Debug("execution cancelled")
			return r.ctx.Err()
		default:
			logger.Info("executing command", "command", command)

			r.notifyHandlers(ExecResult{Command: command, ID: client.ID(), Stage: Start, RemoteAddr: remoteAddr})

			output, err := client.ExecuteCommand(r.ctx, command)

			r.notifyHandlers(ExecResult{Command: command, ID: client.ID(), Stage: Finish, RemoteAddr: remoteAddr,
				Output: output, Error: err})

			if err != nil {
				return fmt.Errorf("failed to execute command %q: %w", command, err)
			}
			logger.Info("command done", "command", command, "output", output)
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
		// 安全地处理可能为 nil 的客户端
		if client != nil {
			addr := client.RemoteAddr()
			// 检查 addr 是否为零值
			if addr != (netip.AddrPort{}) {
				hosts[client.ID()] = addr.String()
			} else {
				hosts[client.ID()] = "unknown"
			}
		}
	}

	return hosts
}

// GetClientByID returns the SSHClient with the given ID
func (r *Remex) GetClientByID(id string) (RemoteClient, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	client, ok := r.clients[id]
	if ok {
		return client, true
	}
	return nil, false
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
		return fmt.Errorf("errors closing clients: %w", errors.Join(closeErrors...))
	}

	r.logger.Info("all connections closed")
	return nil
}
