package remex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
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
		"remex.exec":     execLocalCommand,
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

// FileTransferResult represents the result of file transfer operations
type FileTransferResult struct {
	BytesTransferred int64
	SourcePath       string
	TargetPath       string
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

	bytesCopied, err := io.Copy(localFile, newInterruptibleReader(ctx, remoteFile))
	if err != nil {
		// Clean up partially downloaded file
		os.Remove(localFilePath)
		return "", fmt.Errorf("failed to copy file content: %w", err)
	}

	result := FileTransferResult{
		BytesTransferred: bytesCopied,
		SourcePath:       remoteFilePath,
		TargetPath:       localFilePath,
	}

	return fmt.Sprintf("Download completed: %d bytes transferred from %s to %s",
		result.BytesTransferred, result.SourcePath, result.TargetPath), nil
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

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return "", fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	// Create remote directory if it doesn't exist
	remoteDir := filepath.Dir(remoteFilePath)
	if err := sftpClient.MkdirAll(filepath.ToSlash(remoteDir)); err != nil {
		return "", fmt.Errorf("failed to create remote directory: %w", err)
	}

	localFile, err := os.Open(localFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to open local file: %w", err)
	}
	defer localFile.Close()

	remoteFile, err := sftpClient.Create(remoteFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to create remote file: %w", err)
	}
	defer remoteFile.Close()

	bytesCopied, err := io.Copy(remoteFile, newInterruptibleReader(ctx, localFile))
	if err != nil {
		// Clean up partially uploaded file
		sftpClient.Remove(remoteFilePath)
		return "", fmt.Errorf("failed to copy file content: %w", err)
	}

	result := FileTransferResult{
		BytesTransferred: bytesCopied,
		SourcePath:       localFilePath,
		TargetPath:       remoteFilePath,
	}

	return fmt.Sprintf("Upload completed: %d bytes transferred from %s to %s",
		result.BytesTransferred, result.SourcePath, result.TargetPath), nil
}

// executeLocalCommand executes a command on the local machine
func execLocalCommand(ctx context.Context, _ *ssh.Client, args ...string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("command execution requires at least one argument")
	}

	command := strings.TrimSpace(args[0])
	if command == "" {
		return "", errors.New("command cannot be empty")
	}

	var cmdArgs []string
	if len(args) > 1 {
		cmdArgs = args[1:]
	}

	cmd := exec.Command(command, cmdArgs...)

	var output strings.Builder
	var errorOutput strings.Builder

	cmd.Stdout = &output
	cmd.Stderr = &errorOutput

	if err := cmd.Run(); err != nil {
		errorMsg := errorOutput.String()
		if errorMsg == "" {
			errorMsg = err.Error()
		}
		return "", fmt.Errorf("command execution failed: %s - %w", errorMsg, err)
	}

	result := output.String()
	if result == "" {
		result = "Command executed successfully (no output)"
	}

	return result, nil
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

func newInterruptibleReader(ctx context.Context, r io.Reader) io.Reader {
	return interruptibleReader(func(p []byte) (n int, err error) {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
			return r.Read(p)
		}
	})
}
