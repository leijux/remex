package remex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// remexCommand represents an remex command function
type remexCommand func(context.Context, *ssh.Client, ...string) (string, error)

// remexRegistry manages remex commands
type remexRegistry struct {
	commands map[string]remexCommand
}

var registry = &remexRegistry{
	commands: map[string]remexCommand{
		"remex.upload":   uploadFile,
		"remex.download": downloadFile,
		"remex.sh":       shScript,
		"remex.mkdir":    createRemoteDirectory,
	},
}

// RegisterCommand registers a new remex command
func RegisterCommand(name string, command remexCommand) error {
	if name == "" {
		return errors.New("command name cannot be empty")
	}
	if command == nil {
		return errors.New("command function cannot be nil")
	}
	if !strings.HasPrefix(name, "remex.") {
		name = "remex." + name
	}

	registry.commands[name] = command
	return nil
}

// GetCommand returns an remex command by name
func GetCommand(name string) (remexCommand, bool) {
	cmd, exists := registry.commands[name]
	return cmd, exists
}

// ListCommands returns all registered remex command names
func ListCommands() []string {
	names := make([]string, 0, len(registry.commands))
	for name := range registry.commands {
		names = append(names, name)
	}
	return names
}

// downloadFile downloads a file from remote host to local machine
func downloadFile(ctx context.Context, client *ssh.Client, args ...string) (string, error) {
	if len(args) != 2 {
		return "", errors.New("download requires exactly 2 arguments: remoteFilePath localFilePath")
	}

	remoteFilePath := strings.TrimSpace(args[0])
	localFilePath := strings.TrimSpace(args[1])

	if remoteFilePath == "" {
		return "", errors.New("remote file path cannot be empty")
	}
	if localFilePath == "" {
		return "", errors.New("local file path cannot be empty")
	}

	// Create directory for local file if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(localFilePath), 0755); err != nil {
		return "", fmt.Errorf("failed to create local directory: %w", err)
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return "", fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	// Check if remote file exists
	remoteFileInfo, err := sftpClient.Stat(remoteFilePath)
	if err != nil {
		return "", fmt.Errorf("remote file not found: %w", err)
	}
	if remoteFileInfo.IsDir() {
		return "", errors.New("remote path is a directory, not a file")
	}

	remoteFile, err := sftpClient.Open(remoteFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to open remote file: %w", err)
	}
	defer remoteFile.Close()

	localFile, err := os.Create(localFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to create local file: %w", err)
	}
	defer localFile.Close()

	bytesCopied, err := io.Copy(localFile, NewInterruptibleReader(ctx, remoteFile))
	if err != nil {
		// Clean up partially downloaded file
		os.Remove(localFilePath)
		return "", fmt.Errorf("failed to copy file content: %w", err)
	}

	return fmt.Sprintf("Download completed: %d bytes transferred from %s to %s",
		bytesCopied, remoteFilePath, localFilePath), nil
}

// uploadFile uploads a file from local machine to remote host
func uploadFile(ctx context.Context, client *ssh.Client, args ...string) (string, error) {
	if len(args) != 2 {
		return "", errors.New("upload requires exactly 2 arguments: localFilePath remoteFilePath")
	}

	localFilePath := strings.TrimSpace(args[0])
	remoteFilePath := strings.TrimSpace(args[1])

	if localFilePath == "" {
		return "", errors.New("local file path cannot be empty")
	}
	if remoteFilePath == "" {
		return "", errors.New("remote file path cannot be empty")
	}

	// Check if local file exists
	localFileInfo, err := os.Stat(localFilePath)
	if err != nil {
		return "", fmt.Errorf("local file not found: %w", err)
	}
	if localFileInfo.IsDir() {
		return "", errors.New("local path is a directory, not a file")
	}

	localFile, err := os.Open(localFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to open local file: %w", err)
	}
	defer localFile.Close()

	bytesCopied, err := UploadMemoryFile(ctx, client, localFile, remoteFilePath)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Upload completed: %d bytes transferred from %s to %s",
		bytesCopied, localFilePath, remoteFilePath), nil
}

func UploadMemoryFileCommand(reader io.Reader, remoteFilePath string) remexCommand {
	return func(ctx context.Context, client *ssh.Client, _ ...string) (string, error) {
		bytesCopied, err := UploadMemoryFile(ctx, client, reader, remoteFilePath)
		if err != nil {
			return "", err
		}

		return fmt.Sprintf("Upload completed: %d bytes to %s",
			bytesCopied, remoteFilePath), nil
	}
}

// UploadMemoryFile uploads a file from memory to the remote server.
func UploadMemoryFile(ctx context.Context, client *ssh.Client, reader io.Reader, remoteFilePath string) (int64, error) {
	if client == nil {
		return 0, errors.New("ssh client is nil")
	}
	if remoteFilePath == "" {
		return 0, errors.New("remote file path cannot be empty")
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return 0, fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	// Create remote directory if it doesn't exist
	if err := sftpClient.MkdirAll(filepath.ToSlash(filepath.Dir(remoteFilePath))); err != nil {
		return 0, fmt.Errorf("failed to create remote directory: %w", err)
	}

	remoteFile, err := sftpClient.Create(remoteFilePath)
	if err != nil {
		return 0, fmt.Errorf("failed to create remote file: %w", err)
	}
	defer remoteFile.Close()

	bytesCopied, err := io.Copy(remoteFile, NewInterruptibleReader(ctx, reader))
	if err != nil {
		// Clean up partially uploaded file
		sftpClient.Remove(remoteFilePath)
		return 0, fmt.Errorf("failed to copy file content: %w", err)
	}

	return bytesCopied, nil
}

// shCommand run sh script
func shScript(ctx context.Context, _ *ssh.Client, args ...string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("command execution requires at least one argument")
	}

	var b bytes.Buffer
	file, _ := syntax.NewParser().Parse(strings.NewReader(strings.Join(args, " ")), "")
	runner, _ := interp.New(
		interp.Env(expand.ListEnviron(os.Environ()...)),
		interp.StdIO(nil, &b, &b),
	)
	if err := runner.Run(ctx, file); err != nil {
		return "", err
	}
	return b.String(), nil
}

// createRemoteDirectory creates a directory on the remote host
func createRemoteDirectory(ctx context.Context, client *ssh.Client, args ...string) (string, error) {
	if len(args) != 1 {
		return "", errors.New("mkdir requires exactly one argument: directoryPath")
	}

	directoryPath := strings.TrimSpace(args[0])
	if directoryPath == "" {
		return "", errors.New("directory path cannot be empty")
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return "", fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	if err := sftpClient.MkdirAll(directoryPath); err != nil {
		return "", fmt.Errorf("failed to create remote directory: %w", err)
	}

	return fmt.Sprintf("Directory created successfully: %s", directoryPath), nil
}

// fileExists checks if a file exists on the remote host
func fileExists(client *ssh.Client, args ...string) (string, error) {
	if len(args) != 1 {
		return "", errors.New("fileExists requires exactly one argument: filePath")
	}

	filePath := strings.TrimSpace(args[0])
	if filePath == "" {
		return "", errors.New("file path cannot be empty")
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return "", fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	_, err = sftpClient.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "false", nil
		}
		return "", fmt.Errorf("failed to check file existence: %w", err)
	}

	return "true", nil
}

type interruptibleReader func(p []byte) (n int, err error)

func (r interruptibleReader) Read(p []byte) (n int, err error) {
	return r(p)
}

func NewInterruptibleReader(ctx context.Context, r io.Reader) io.Reader {
	return interruptibleReader(func(p []byte) (n int, err error) {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
			return r.Read(p)
		}
	})
}
