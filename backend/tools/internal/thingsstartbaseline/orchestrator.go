package thingsstartbaseline

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// StageRuntime runs the work and readiness check for one startup stage.
type StageRuntime interface {
	RunStage(context.Context, StageID) error
}

// StageProbe verifies that a startup stage is ready after its compose command.
type StageProbe interface {
	Probe(context.Context, StageID) error
}

// ComposeRuntime runs baseline compose commands without invoking a shell.
type ComposeRuntime struct {
	runner        CommandRunner
	probe         StageProbe
	probeInterval time.Duration
	repoRoot      string
	envFile       string
	composeFile   string
	composeDir    string
	environment   map[string]string
	envDigest     [sha256.Size]byte
	composeDigest [sha256.Size]byte
}

var composeSecretEnvironmentNames = []string{
	"POSTGRES_ADMIN_PASSWORD",
	"THINGS_BASELINE_DB_PASSWORD",
	"CORE_USER_TOKEN_SECRET",
	"THINGS_INTERNAL_AUTH_PASSWORD",
	"THINGS_EMQX_API_KEY",
	"THINGS_EMQX_SECRET_KEY",
	"TDENGINE_ROOT_PASSWORD",
}

// NewComposeRuntime creates a compose runtime rooted in one repository.
func NewComposeRuntime(runner CommandRunner, probe StageProbe, repoRoot, envFile, composeFile string) (*ComposeRuntime, error) {
	if runner == nil {
		return nil, errors.New("command runner is nil")
	}
	if probe == nil {
		return nil, errors.New("stage probe is nil")
	}
	root, err := absoluteRealDirectory(repoRoot, "repository root")
	if err != nil {
		return nil, err
	}
	env, err := absoluteVerifiedRegularFile(envFile, "environment file", true)
	if err != nil {
		return nil, err
	}
	compose, err := absoluteVerifiedRegularFile(composeFile, "compose file", false)
	if err != nil {
		return nil, err
	}
	if !pathWithin(root, env) || !pathWithin(root, compose) {
		return nil, errors.New("compose inputs must be inside repository root")
	}
	environment, err := loadComposeEnvironment(env)
	if err != nil {
		return nil, err
	}
	envDigest, err := fileSHA256(env)
	if err != nil {
		return nil, err
	}
	composeDigest, err := fileSHA256(compose)
	if err != nil {
		return nil, err
	}
	return &ComposeRuntime{
		runner:        runner,
		probe:         probe,
		probeInterval: 500 * time.Millisecond,
		repoRoot:      root,
		envFile:       env,
		composeFile:   compose,
		composeDir:    filepath.Dir(compose),
		environment:   environment,
		envDigest:     envDigest,
		composeDigest: composeDigest,
	}, nil
}

// RunStage runs the compose action, if any, then probes the stage.
func (runtime *ComposeRuntime) RunStage(ctx context.Context, stage StageID) error {
	if runtime == nil {
		return stageDiagnostic(stage, errors.New("compose runtime is nil"))
	}
	if err := ctx.Err(); err != nil {
		return timeoutDiagnostic(err)
	}
	if extra, ok := composeStageArguments(stage); ok {
		if err := runtime.verifyComposeInputs(); err != nil {
			return diagnostic(CodeContractMismatch, err)
		}
		args := append(runtime.composeArguments(), extra...)
		result, err := runtime.runner.Run(ctx, Command{
			Path: "docker",
			Args: args,
			Dir:  runtime.repoRoot,
			Env:  composeEnvironment(runtime.envFile, runtime.environment),
		})
		if ctxErr := ctx.Err(); ctxErr != nil {
			return timeoutDiagnostic(ctxErr)
		}
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return timeoutDiagnostic(err)
			}
			return stageDiagnostic(stage, err)
		}
		if result.ExitCode != 0 {
			return stageDiagnostic(stage, fmt.Errorf("compose command exited with status %d", result.ExitCode))
		}
	}
	if err := runtime.probeUntilReady(ctx, stage); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return timeoutDiagnostic(ctxErr)
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return timeoutDiagnostic(err)
		}
		return stageDiagnostic(stage, err)
	}
	if err := ctx.Err(); err != nil {
		return timeoutDiagnostic(err)
	}
	return nil
}

func (runtime *ComposeRuntime) probeUntilReady(ctx context.Context, stage StageID) error {
	for {
		err := runtime.probe.Probe(ctx, stage)
		if err == nil {
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return timeoutDiagnostic(ctxErr)
		}
		if stage == StageVerified || DiagnosticCode(err) != CodeDependencyNotReady {
			return err
		}
		timer := time.NewTimer(runtime.probeInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return timeoutDiagnostic(ctx.Err())
		case <-timer.C:
		}
	}
}

func (runtime *ComposeRuntime) verifyComposeInputs() error {
	root, err := absoluteRealDirectory(runtime.repoRoot, "repository root")
	if err != nil {
		return err
	}
	if root != runtime.repoRoot {
		return errors.New("repository root identity changed")
	}
	env, err := absoluteVerifiedRegularFile(runtime.envFile, "environment file", true)
	if err != nil {
		return err
	}
	compose, err := absoluteVerifiedRegularFile(runtime.composeFile, "compose file", false)
	if err != nil {
		return err
	}
	if env != runtime.envFile || compose != runtime.composeFile || !pathWithin(root, env) || !pathWithin(root, compose) {
		return errors.New("compose input identity changed")
	}
	envDigest, err := fileSHA256(env)
	if err != nil {
		return err
	}
	composeDigest, err := fileSHA256(compose)
	if err != nil {
		return err
	}
	if envDigest != runtime.envDigest || composeDigest != runtime.composeDigest {
		return errors.New("compose input content changed")
	}
	return nil
}

func (runtime *ComposeRuntime) composeArguments() []string {
	return []string{"compose", "--project-name", "ming-iot-things-baseline", "--project-directory", runtime.composeDir, "--env-file", runtime.envFile, "-f", runtime.composeFile}
}

func composeStageArguments(stage StageID) ([]string, bool) {
	switch stage {
	case StagePrecheck:
		return []string{"config", "--quiet"}, true
	case StageDatabaseBootstrap:
		return []string{"--profile", "tools", "run", "--rm", "postgres-bootstrap"}, true
	case StageInfraReady:
		return []string{"up", "-d", "redis", "etcd", "nats", "tdengine", "emqx"}, true
	case StageTimedJobReady:
		return []string{"up", "-d", "timedjob"}, true
	case StageSysReady:
		return []string{"up", "-d", "sys"}, true
	case StageDMReady:
		return []string{"up", "-d", "dm"}, true
	case StageDGProcessReady:
		return []string{"up", "-d", "dg"}, true
	case StageAPIReady:
		return []string{"up", "-d", "api"}, true
	case StageDGLinkReady, StageVerified:
		return nil, false
	default:
		return nil, false
	}
}

func loadComposeEnvironment(filename string) (map[string]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("read compose environment file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return nil, errors.New("read compose environment file: file is no longer a regular 0600 file")
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read compose environment file: %w", err)
	}
	environment := make(map[string]string)
	for lineNumber, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, found := strings.Cut(line, "=")
		if !found {
			return nil, fmt.Errorf("parse compose environment file: line %d is not KEY=value", lineNumber+1)
		}
		name = strings.TrimSpace(name)
		if !validEnvironmentName(name) {
			return nil, fmt.Errorf("parse compose environment file: line %d has invalid key", lineNumber+1)
		}
		parsed, err := parseComposeEnvironmentValue(value)
		if err != nil {
			return nil, fmt.Errorf("parse compose environment file: line %d: %w", lineNumber+1, err)
		}
		if _, exists := environment[name]; exists {
			return nil, fmt.Errorf("parse compose environment file: line %d duplicates %s", lineNumber+1, name)
		}
		environment[name] = parsed
	}
	for _, name := range composeSecretEnvironmentNames {
		value, found := environment[name]
		if !found {
			return nil, fmt.Errorf("parse compose environment file: missing %s", name)
		}
		if value == "" {
			return nil, fmt.Errorf("parse compose environment file: %s must not be empty", name)
		}
	}
	return environment, nil
}

func parseComposeEnvironmentValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if value[0] == '\'' {
		if len(value) < 2 || value[len(value)-1] != '\'' {
			return "", errors.New("unterminated single-quoted value")
		}
		return value[1 : len(value)-1], nil
	}
	if value[0] == '"' {
		parsed, err := strconv.Unquote(value)
		if err != nil {
			return "", fmt.Errorf("invalid double-quoted value: %w", err)
		}
		if strings.Contains(parsed, "$") {
			return "", errors.New("unsupported interpolation in double-quoted value")
		}
		return parsed, nil
	}
	if strings.ContainsAny(value, "\"'") {
		return "", errors.New("unquoted value contains a quote")
	}
	value = strings.TrimSpace(stripComposeEnvironmentComment(value))
	if strings.Contains(value, "$") {
		return "", errors.New("unsupported interpolation in unquoted value")
	}
	return value, nil
}

func fileSHA256(filename string) ([sha256.Size]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("open input for digest: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("digest input: %w", err)
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return digest, nil
}

func stripComposeEnvironmentComment(value string) string {
	for index := 1; index < len(value); index++ {
		if value[index] == '#' && (value[index-1] == ' ' || value[index-1] == '\t') {
			return value[:index]
		}
	}
	return value
}

func validEnvironmentName(name string) bool {
	if name == "" {
		return false
	}
	for index := range name {
		character := name[index]
		if (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || character == '_' || (index > 0 && character >= '0' && character <= '9') {
			continue
		}
		return false
	}
	return true
}

func composeEnvironment(envFile string, fileEnvironment map[string]string) []string {
	const environmentFileName = "THINGS_BASELINE_ENV_FILE="
	environment := os.Environ()
	result := make([]string, 0, len(environment)+len(fileEnvironment)+1)
	for _, item := range environment {
		name, _, _ := strings.Cut(item, "=")
		if name != "THINGS_BASELINE_ENV_FILE" {
			if _, overridden := fileEnvironment[name]; overridden {
				continue
			}
			result = append(result, item)
		}
	}
	names := make([]string, 0, len(fileEnvironment))
	for name := range fileEnvironment {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result = append(result, name+"="+fileEnvironment[name])
	}
	return append(result, environmentFileName+envFile)
}

func absoluteRealDirectory(path, name string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%s is empty", name)
	}
	resolved, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", name, err)
	}
	realPath, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve %s symlinks: %w", name, err)
	}
	info, err := os.Stat(realPath)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", name)
	}
	return filepath.Clean(realPath), nil
}

func absoluteVerifiedRegularFile(path, name string, require0600 bool) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%s is empty", name)
	}
	resolved, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", name, err)
	}
	input, err := os.Lstat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect %s: %w", name, err)
	}
	if input.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%s must not be a symbolic link", name)
	}
	realPath, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve %s symlinks: %w", name, err)
	}
	realPath = filepath.Clean(realPath)
	if filepath.Clean(resolved) != realPath {
		return "", fmt.Errorf("%s must not traverse a symbolic link", name)
	}
	info, err := os.Stat(realPath)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", name)
	}
	if require0600 && info.Mode().Perm() != 0o600 {
		return "", fmt.Errorf("%s must have permissions 0600", name)
	}
	return realPath, nil
}

func pathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

// Orchestrator enforces the validated baseline startup order and records progress.
type Orchestrator struct {
	runtime      StageRuntime
	contract     *Contract
	stateRoot    string
	probeTimeout time.Duration
	totalTimeout time.Duration
}

// NewOrchestrator creates an ordered startup state machine.
func NewOrchestrator(runtime StageRuntime, contract *Contract, stateRoot string) *Orchestrator {
	probeTimeout := time.Duration(0)
	totalTimeout := time.Duration(0)
	if contract != nil {
		probeTimeout = time.Duration(contract.Timeouts.ProbeSeconds) * time.Second
		totalTimeout = time.Duration(contract.Timeouts.TotalSeconds) * time.Second
	}
	return &Orchestrator{runtime: runtime, contract: contract, stateRoot: stateRoot, probeTimeout: probeTimeout, totalTimeout: totalTimeout}
}

// Up executes every contract stage in order and stops at the first failure.
func (orchestrator *Orchestrator) Up(ctx context.Context) error {
	_, err := orchestrator.UpWithEvidence(ctx)
	return err
}

// UpWithEvidence executes every contract stage in order and returns a complete
// ten-stage projection. Stages after the first failure are explicitly marked
// NOT_RUN so a failed run remains publishable without pretending they ran.
func (orchestrator *Orchestrator) UpWithEvidence(ctx context.Context) ([]StageEvidence, error) {
	if orchestrator == nil || orchestrator.runtime == nil {
		return nil, diagnostic(CodeContractMismatch, errors.New("startup runtime is nil"))
	}
	if err := ValidateContract(orchestrator.contract); err != nil {
		return nil, diagnostic(CodeContractMismatch, err)
	}
	if orchestrator.probeTimeout <= 0 || orchestrator.totalTimeout <= 0 {
		return nil, diagnostic(CodeContractMismatch, errors.New("startup timeouts must be positive"))
	}
	stateRoot, err := absoluteStateRoot(orchestrator.stateRoot)
	if err != nil {
		return nil, diagnostic(CodeContractMismatch, err)
	}
	rootContext, cancel := context.WithTimeout(ctx, orchestrator.totalTimeout)
	defer cancel()
	state := startupState{}
	stages := make([]StageEvidence, 0, len(orchestrator.contract.Stages))
	for index, contractStage := range orchestrator.contract.Stages {
		stageEvidence := newRunningStageEvidence(contractStage)
		state.Stages = append(state.Stages, startupStageState{ID: contractStage.ID, Status: "RUNNING"})
		if err := writeStartupState(stateRoot, state); err != nil {
			failure := diagnostic(CodeContractMismatch, err)
			stageEvidence = finishStageEvidence(stageEvidence, failure)
			stages = append(stages, stageEvidence)
			return appendNotRunStageEvidence(stages, orchestrator.contract.Stages[index+1:], contractStage.ID), failure
		}

		stageContext, stageCancel := context.WithTimeout(rootContext, orchestrator.probeTimeout)
		err := orchestrator.runtime.RunStage(stageContext, contractStage.ID)
		contextErr := stageContext.Err()
		stageCancel()
		if err != nil || contextErr != nil || rootContext.Err() != nil {
			code := DiagnosticCode(err)
			if contextErr != nil || rootContext.Err() != nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				code = CodeTimeout
			}
			if code == "" {
				code = stageErrorCode(contractStage.ID)
			}
			state.Stages[len(state.Stages)-1].Status = "FAIL"
			state.Stages[len(state.Stages)-1].ErrorCode = code
			if err == nil {
				err = contextErr
				if err == nil {
					err = rootContext.Err()
				}
			}
			if stateErr := writeStartupState(stateRoot, state); stateErr != nil {
				err = errors.Join(err, stateErr)
			}
			failure := diagnostic(code, err)
			stageEvidence = finishStageEvidence(stageEvidence, failure)
			stages = append(stages, stageEvidence)
			return appendNotRunStageEvidence(stages, orchestrator.contract.Stages[index+1:], contractStage.ID), failure
		}
		state.Stages[len(state.Stages)-1].Status = "PASS"
		if err := writeStartupState(stateRoot, state); err != nil {
			failure := diagnostic(CodeContractMismatch, err)
			stageEvidence = finishStageEvidence(stageEvidence, failure)
			stages = append(stages, stageEvidence)
			return appendNotRunStageEvidence(stages, orchestrator.contract.Stages[index+1:], contractStage.ID), failure
		}
		stages = append(stages, finishStageEvidence(stageEvidence, nil))
	}
	return stages, nil
}

func newRunningStageEvidence(stage StageContract) StageEvidence {
	return StageEvidence{
		ID:        stage.ID,
		Result:    "RUNNING",
		StartedAt: time.Now().UTC(),
		Component: strings.Join(stage.Components, ","),
		LogRef:    "logs/" + strings.ToLower(string(stage.ID)) + ".log",
	}
}

func finishStageEvidence(stage StageEvidence, err error) StageEvidence {
	stage.FinishedAt = time.Now().UTC()
	stage.DurationMS = stage.FinishedAt.Sub(stage.StartedAt).Milliseconds()
	if err == nil {
		stage.Result = "PASS"
		return stage
	}
	stage.Result = "FAIL"
	stage.Code = DiagnosticCode(err)
	if stage.Code == "" {
		stage.Code = stageErrorCode(stage.ID)
	}
	stage.Reason = err.Error()
	return stage
}

func appendNotRunStageEvidence(stages []StageEvidence, remaining []StageContract, failed StageID) []StageEvidence {
	for _, stage := range remaining {
		stages = append(stages, StageEvidence{
			ID:        stage.ID,
			Result:    "NOT_RUN",
			Component: strings.Join(stage.Components, ","),
			Reason:    fmt.Sprintf("not run after %s failure", failed),
		})
	}
	return stages
}

type startupState struct {
	Stages []startupStageState `json:"stages"`
}

type startupStageState struct {
	ID        StageID   `json:"id"`
	Status    string    `json:"status"`
	ErrorCode ErrorCode `json:"errorCode,omitempty"`
}

func absoluteStateRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("startup state root is empty")
	}
	resolved, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve startup state root: %w", err)
	}
	if err := os.MkdirAll(resolved, 0o755); err != nil {
		return "", fmt.Errorf("create startup state root: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.New("startup state root is not a directory")
	}
	return filepath.Clean(resolved), nil
}

func writeStartupState(root string, state startupState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode startup state: %w", err)
	}
	temporary, err := os.CreateTemp(root, ".startup-state-")
	if err != nil {
		return fmt.Errorf("create startup state temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		temporary.Close()
		return fmt.Errorf("write startup state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync startup state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close startup state: %w", err)
	}
	filename := filepath.Join(root, "startup-state.json")
	if err := os.Rename(temporaryName, filename); err != nil {
		return fmt.Errorf("publish startup state: %w", err)
	}
	directory, err := os.Open(root)
	if err != nil {
		return fmt.Errorf("open startup state directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync startup state directory: %w", err)
	}
	return nil
}

type diagnosticError struct {
	code ErrorCode
	err  error
}

func (err *diagnosticError) Error() string { return string(err.code) + ": " + err.err.Error() }
func (err *diagnosticError) Unwrap() error { return err.err }

func diagnostic(code ErrorCode, err error) error {
	if err == nil {
		return nil
	}
	return &diagnosticError{code: code, err: err}
}

func timeoutDiagnostic(err error) error { return diagnostic(CodeTimeout, err) }

func stageDiagnostic(stage StageID, err error) error { return diagnostic(stageErrorCode(stage), err) }

func stageErrorCode(stage StageID) ErrorCode {
	switch stage {
	case StagePrecheck:
		return CodeConfigInvalid
	case StageDatabaseBootstrap:
		return CodeDatabaseBootstrapFailed
	case StageInfraReady, StageVerified:
		return CodeDependencyNotReady
	default:
		return CodeRoleNotReady
	}
}

// DiagnosticCode returns the stable error code carried by a startup error.
func DiagnosticCode(err error) ErrorCode {
	var diagnostic *diagnosticError
	if errors.As(err, &diagnostic) {
		return diagnostic.code
	}
	return ""
}
