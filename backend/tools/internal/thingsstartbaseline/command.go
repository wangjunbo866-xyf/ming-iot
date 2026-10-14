package thingsstartbaseline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"syscall"
	"time"
)

const (
	commandSummaryLimit          = 4 * 1024
	commandSummaryLabelsLength   = len("stdout:\n") + len("\nstderr:\n")
	commandSummaryPerStreamLimit = (commandSummaryLimit - commandSummaryLabelsLength) / 2
	commandMaximumSecretBytes    = 4 * 1024
	commandRawTailLimit          = commandSummaryPerStreamLimit + commandMaximumSecretBytes - 1
	commandWaitDelay             = 250 * time.Millisecond
)

var commandSecretEnvironmentNames = map[string]struct{}{
	"POSTGRES_ADMIN_PASSWORD":       {},
	"THINGS_BASELINE_DB_PASSWORD":   {},
	"CORE_USER_TOKEN_SECRET":        {},
	"THINGS_INTERNAL_AUTH_PASSWORD": {},
	"THINGS_EMQX_API_KEY":           {},
	"THINGS_EMQX_SECRET_KEY":        {},
	"TDENGINE_ROOT_PASSWORD":        {},
}

// Command is an argv-only command invocation.
type Command struct {
	Path          string
	Args          []string
	Dir           string
	Env           []string
	stdoutCapture *boundedCommandCapture
}

// CommandResult records the observable result of a command invocation.
type CommandResult struct {
	ExitCode     int
	DurationMS   int64
	StdoutSHA256 string
	StderrSHA256 string
	Summary      string
}

// CommandRunner executes commands at the process boundary.
type CommandRunner interface {
	Run(context.Context, Command) (CommandResult, error)
}

type commandRunner struct{}

// NewCommandRunner creates a runner which executes argv directly, without a shell.
func NewCommandRunner() CommandRunner {
	return commandRunner{}
}

func (commandRunner) Run(ctx context.Context, command Command) (CommandResult, error) {
	started := time.Now()
	result := CommandResult{ExitCode: -1}
	if name := commandOversizedSensitiveVariable(command.Env); name != "" {
		empty := newCommandOutputTail(commandSummaryPerStreamLimit)
		result.DurationMS = time.Since(started).Milliseconds()
		result.StdoutSHA256 = empty.SHA256()
		result.StderrSHA256 = empty.SHA256()
		result.Summary = commandSummary("", "", nil)
		return result, fmt.Errorf("sensitive environment variable %s exceeds %d bytes", name, commandMaximumSecretBytes)
	}
	secrets := commandSecrets(command.Env)
	tailLimit := commandOutputTailLimit(secrets)
	stdout := newCommandOutputTail(tailLimit)
	stderr := newCommandOutputTail(tailLimit)
	finish := func() CommandResult {
		result.DurationMS = time.Since(started).Milliseconds()
		result.StdoutSHA256 = stdout.SHA256()
		result.StderrSHA256 = stderr.SHA256()
		result.Summary = commandSummary(stdout.String(), stderr.String(), secrets)
		return result
	}

	if strings.TrimSpace(command.Path) == "" || strings.IndexByte(command.Path, 0) >= 0 {
		return finish(), errors.New("invalid command path")
	}
	if command.Dir != "" {
		info, err := os.Stat(command.Dir)
		if err != nil || !info.IsDir() {
			return finish(), errors.New("invalid command directory")
		}
	}
	if err := ctx.Err(); err != nil {
		result.ExitCode = 124
		return finish(), fmt.Errorf("command context ended: %w", err)
	}

	process := exec.CommandContext(ctx, command.Path, command.Args...)
	process.Dir = command.Dir
	process.Env = command.Env
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	process.Stdout = stdout
	if command.stdoutCapture != nil {
		process.Stdout = io.MultiWriter(stdout, command.stdoutCapture)
	}
	process.Stderr = stderr
	process.WaitDelay = commandWaitDelay
	process.Cancel = func() error {
		if process.Process == nil {
			return os.ErrProcessDone
		}
		if err := syscall.Kill(-process.Process.Pid, syscall.SIGKILL); err == nil {
			return nil
		}
		if err := process.Process.Kill(); err == nil || errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return errors.New("command process cancellation failed")
	}

	err := process.Run()
	result = finish()
	if contextErr := ctx.Err(); contextErr != nil {
		result.ExitCode = 124
		return result, fmt.Errorf("command context ended: %w", contextErr)
	}
	if err == nil {
		result.ExitCode = 0
		return result, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.ExitCode = exitError.ExitCode()
		return result, fmt.Errorf("command exited with status %d", result.ExitCode)
	}
	if process.ProcessState == nil {
		return result, errors.New("command failed to start")
	}
	return result, errors.New("command execution failed")
}

func commandSecrets(environment []string) []string {
	values := make(map[string]struct{})
	for _, item := range environment {
		name, value, found := strings.Cut(item, "=")
		if found && value != "" {
			if _, secret := commandSecretEnvironmentNames[name]; secret {
				values[value] = struct{}{}
			}
		}
	}
	secrets := make([]string, 0, len(values))
	for value := range values {
		secrets = append(secrets, value)
	}
	sort.Slice(secrets, func(left, right int) bool { return len(secrets[left]) > len(secrets[right]) })
	return secrets
}

func commandOversizedSensitiveVariable(environment []string) string {
	for _, item := range environment {
		name, value, found := strings.Cut(item, "=")
		if !found {
			continue
		}
		if _, sensitive := commandSecretEnvironmentNames[name]; sensitive && len(value) > commandMaximumSecretBytes {
			return name
		}
	}
	return ""
}

func commandSummary(stdout, stderr string, secrets []string) string {
	stdout = commandUTF8Tail(commandRedact(stdout, secrets), commandSummaryPerStreamLimit)
	stderr = commandUTF8Tail(commandRedact(stderr, secrets), commandSummaryPerStreamLimit)
	return "stdout:\n" + stdout + "\nstderr:\n" + stderr
}

func commandRedact(value string, secrets []string) string {
	for _, secret := range secrets {
		value = strings.ReplaceAll(value, secret, "[REDACTED]")
	}
	return value
}

func commandUTF8Tail(value string, limit int) string {
	value = strings.ToValidUTF8(value, "\uFFFD")
	if len(value) <= limit {
		return value
	}
	start := len(value) - limit
	for start < len(value) && value[start]&0xc0 == 0x80 {
		start++
	}
	return value[start:]
}

type commandOutputTail struct {
	digest  hash.Hash
	rawTail []byte
	limit   int
}

func newCommandOutputTail(limit int) *commandOutputTail {
	return &commandOutputTail{
		digest:  sha256.New(),
		rawTail: make([]byte, 0, limit),
		limit:   limit,
	}
}

func (tail *commandOutputTail) Write(output []byte) (int, error) {
	_, _ = tail.digest.Write(output)
	if len(output) >= tail.limit {
		tail.rawTail = append(tail.rawTail[:0], output[len(output)-tail.limit:]...)
		return len(output), nil
	}
	if overflow := len(tail.rawTail) + len(output) - tail.limit; overflow > 0 {
		copy(tail.rawTail, tail.rawTail[overflow:])
		tail.rawTail = tail.rawTail[:len(tail.rawTail)-overflow]
	}
	tail.rawTail = append(tail.rawTail, output...)
	return len(output), nil
}

func (tail *commandOutputTail) SHA256() string {
	return hex.EncodeToString(tail.digest.Sum(nil))
}

func (tail *commandOutputTail) String() string {
	return string(tail.rawTail)
}

func commandOutputTailLimit(secrets []string) int {
	longest := 0
	for _, secret := range secrets {
		if len(secret) > longest {
			longest = len(secret)
		}
	}
	if longest == 0 {
		return commandSummaryPerStreamLimit
	}
	return commandSummaryPerStreamLimit + longest - 1
}

type boundedCommandCapture struct {
	limit    int
	data     []byte
	overflow bool
}

func newBoundedCommandCapture(limit int) *boundedCommandCapture {
	if limit < 0 {
		limit = 0
	}
	return &boundedCommandCapture{limit: limit, data: make([]byte, 0, limit)}
}

func (capture *boundedCommandCapture) Write(output []byte) (int, error) {
	written := len(output)
	remaining := capture.limit - len(capture.data)
	if remaining > 0 {
		if len(output) < remaining {
			remaining = len(output)
		}
		capture.data = append(capture.data, output[:remaining]...)
	}
	if written > remaining {
		capture.overflow = true
	}
	return written, nil
}

func (capture *boundedCommandCapture) Result() ([]byte, bool) {
	return append([]byte(nil), capture.data...), capture.overflow
}
