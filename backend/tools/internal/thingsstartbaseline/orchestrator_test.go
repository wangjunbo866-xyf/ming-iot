package thingsstartbaseline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v3"
)

func TestCommandHashesRedactsAndBoundsSummary(t *testing.T) {
	secrets := map[string]string{
		"POSTGRES_ADMIN_PASSWORD":       "postgres-secret-sentinel",
		"THINGS_BASELINE_DB_PASSWORD":   "database-secret-sentinel",
		"CORE_USER_TOKEN_SECRET":        "token-secret-sentinel",
		"THINGS_INTERNAL_AUTH_PASSWORD": "internal-secret-sentinel",
		"THINGS_EMQX_API_KEY":           "emqx-key-secret-sentinel",
		"THINGS_EMQX_SECRET_KEY":        "emqx-secret-sentinel",
		"TDENGINE_ROOT_PASSWORD":        "tdengine-secret-sentinel",
	}
	secretValues := make([]string, 0, len(secrets))
	environment := append([]string(nil), os.Environ()...)
	for name, value := range secrets {
		environment = replaceEnvironment(environment, name, value)
		secretValues = append(secretValues, value)
	}
	stdout := strings.Repeat("o", 5000) + strings.Join(secretValues, "|") + "\nSTDOUT-TAIL-SENTINEL"
	stderr := strings.Repeat("e", 5000) + strings.Join(secretValues, "|") + "\nSTDERR-TAIL-SENTINEL"
	environment = replaceEnvironment(environment, "THINGS_COMMAND_HELPER", "output")
	environment = replaceEnvironment(environment, "THINGS_COMMAND_STDOUT", stdout)
	environment = replaceEnvironment(environment, "THINGS_COMMAND_STDERR", stderr)

	result, err := NewCommandRunner().Run(context.Background(), Command{
		Path: os.Args[0],
		Args: []string{"-test.run=TestCommandHelperProcess", "--"},
		Dir:  t.TempDir(),
		Env:  environment,
	})
	if err == nil || result.ExitCode != 23 {
		t.Fatalf("Run() result/error = %+v/%v, want exit 23", result, err)
	}
	if result.StdoutSHA256 != sha256String(stdout) || result.StderrSHA256 != sha256String(stderr) {
		t.Fatalf("Run() hashes = %q/%q", result.StdoutSHA256, result.StderrSHA256)
	}
	if len(result.Summary) > 4*1024 || !strings.Contains(result.Summary, "STDOUT-TAIL-SENTINEL") || !strings.Contains(result.Summary, "STDERR-TAIL-SENTINEL") {
		t.Fatalf("Run() summary boundary/tails = len %d %q", len(result.Summary), result.Summary)
	}
	if !strings.Contains(result.Summary, "[REDACTED]") {
		t.Fatalf("Run() summary did not record redaction: %q", result.Summary)
	}
	for _, secret := range secretValues {
		if strings.Contains(result.Summary, secret) || strings.Contains(err.Error(), secret) {
			t.Fatalf("Run() leaked secret %q", secret)
		}
	}
}

func TestCommandRejectsOversizedSensitiveEnvironmentBeforeStart(t *testing.T) {
	secret := strings.Repeat("oversized-secret-sentinel", commandMaximumSecretBytes/len("oversized-secret-sentinel")+1)
	markerFile := filepath.Join(t.TempDir(), "started")
	environment := replaceEnvironment(os.Environ(), "POSTGRES_ADMIN_PASSWORD", secret)
	environment = replaceEnvironment(environment, "THINGS_COMMAND_HELPER", "marker")
	environment = replaceEnvironment(environment, "THINGS_COMMAND_MARKER_FILE", markerFile)

	result, err := NewCommandRunner().Run(context.Background(), Command{
		Path: os.Args[0],
		Args: []string{"-test.run=TestCommandHelperProcess", "--"},
		Dir:  t.TempDir(),
		Env:  environment,
	})
	if err == nil {
		t.Fatal("Run accepted an oversized sensitive environment value")
	}
	if !strings.Contains(err.Error(), "POSTGRES_ADMIN_PASSWORD") || !strings.Contains(err.Error(), strconv.Itoa(commandMaximumSecretBytes)) {
		t.Fatalf("Run error = %q, want variable name and byte limit", err)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(result.Summary, secret) {
		t.Fatal("Run leaked oversized sensitive environment value")
	}
	if _, statErr := os.Stat(markerFile); !os.IsNotExist(statErr) {
		t.Fatalf("helper started despite rejected environment: %v", statErr)
	}
}

func TestCommandTimeoutKillsProcessGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	environment := replaceEnvironment(os.Environ(), "THINGS_COMMAND_HELPER", "process-group")
	environment = replaceEnvironment(environment, "THINGS_COMMAND_PID_FILE", pidFile)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	type outcome struct {
		result CommandResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := NewCommandRunner().Run(ctx, Command{Path: os.Args[0], Args: []string{"-test.run=TestCommandHelperProcess", "--"}, Dir: t.TempDir(), Env: environment})
		done <- outcome{result: result, err: err}
	}()

	var childPID int
	deadline := time.Now().Add(800 * time.Millisecond)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(pidFile)
		if err == nil {
			childPID, _ = strconv.Atoi(string(data))
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if childPID == 0 {
		t.Fatal("helper child did not start")
	}
	var got outcome
	select {
	case got = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after process-group timeout")
	}
	if !errors.Is(got.err, context.DeadlineExceeded) || got.result.ExitCode != 124 {
		t.Fatalf("timeout result/error = %+v/%v, want TIMEOUT exit 124", got.result, got.err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := unix.Kill(childPID, 0); err == unix.ESRCH {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("child process %d survived process-group timeout", childPID)
}

func TestCommandOutputTailIsBoundedAndHashesEntireLargeOutput(t *testing.T) {
	tail := newCommandOutputTail(commandRawTailLimit)
	expected := sha256.New()
	chunk := []byte(strings.Repeat("large-output-chunk-", 4096))
	for written := 0; written < 3*1024*1024; written += len(chunk) {
		if _, err := tail.Write(chunk); err != nil {
			t.Fatal(err)
		}
		if _, err := expected.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := tail.SHA256(), hex.EncodeToString(expected.Sum(nil)); got != want {
		t.Fatalf("SHA256() = %s, want %s", got, want)
	}
	if got := len(tail.rawTail); got != commandRawTailLimit {
		t.Fatalf("raw tail length = %d, want %d", got, commandRawTailLimit)
	}
	if got := cap(tail.rawTail); got > commandRawTailLimit {
		t.Fatalf("raw tail capacity = %d, limit %d", got, commandRawTailLimit)
	}
}

func TestCommandTailRedactsBoundarySecretAndRepairsUTF8(t *testing.T) {
	secret := "tail-boundary-secret"
	tail := newCommandOutputTail(commandRawTailLimit)
	prefix := strings.Repeat("p", commandRawTailLimit)
	suffix := strings.Repeat("s", commandSummaryPerStreamLimit-len(secret)+1) + "TAIL-SENTINEL"
	if _, err := tail.Write([]byte(prefix + secret + suffix)); err != nil {
		t.Fatal(err)
	}
	summary := commandSummary(tail.String(), "", []string{secret})
	if !utf8.ValidString(summary) {
		t.Fatalf("summary is invalid UTF-8: %q", summary)
	}
	if strings.Contains(summary, secret) || strings.Contains(summary, secret[1:]) || !strings.Contains(summary, "TAIL-SENTINEL") {
		t.Fatalf("summary did not safely redact boundary secret: %q", summary)
	}
	if len(summary) > commandSummaryLimit {
		t.Fatalf("summary length = %d, limit %d", len(summary), commandSummaryLimit)
	}
	invalidUTF8 := commandSummary("\xffUTF8-TAIL-SENTINEL", "", nil)
	if !utf8.ValidString(invalidUTF8) || !strings.Contains(invalidUTF8, "UTF8-TAIL-SENTINEL") {
		t.Fatalf("summary did not repair invalid UTF-8: %q", invalidUTF8)
	}
}

func TestCommandCancelReturnsWhenDetachedChildHoldsOutput(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "detached.pid")
	environment := replaceEnvironment(os.Environ(), "THINGS_COMMAND_HELPER", "detached-stdout")
	environment = replaceEnvironment(environment, "THINGS_COMMAND_PID_FILE", pidFile)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type outcome struct {
		result CommandResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := NewCommandRunner().Run(ctx, Command{Path: os.Args[0], Args: []string{"-test.run=TestCommandHelperProcess", "--"}, Dir: t.TempDir(), Env: environment})
		done <- outcome{result: result, err: err}
	}()

	childPID := waitForCommandHelperPID(t, pidFile)
	t.Cleanup(func() {
		if childPID != 0 {
			_ = unix.Kill(childPID, unix.SIGKILL)
		}
	})
	cancel()
	select {
	case got := <-done:
		if !errors.Is(got.err, context.Canceled) || got.result.ExitCode != 124 {
			t.Fatalf("cancel result/error = %+v/%v, want canceled exit 124", got.result, got.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return while detached child held stdout")
	}
}

func waitForCommandHelperPID(t *testing.T, pidFile string) int {
	t.Helper()
	deadline := time.Now().Add(800 * time.Millisecond)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(pidFile)
		if err == nil {
			pid, _ := strconv.Atoi(string(data))
			if pid != 0 {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("helper child did not start")
	return 0
}

func TestCommandHelperProcess(t *testing.T) {
	switch os.Getenv("THINGS_COMMAND_HELPER") {
	case "":
		return
	case "output":
		_, _ = io.WriteString(os.Stdout, os.Getenv("THINGS_COMMAND_STDOUT"))
		_, _ = io.WriteString(os.Stderr, os.Getenv("THINGS_COMMAND_STDERR"))
		os.Exit(23)
	case "marker":
		if err := os.WriteFile(os.Getenv("THINGS_COMMAND_MARKER_FILE"), []byte("started"), 0o600); err != nil {
			os.Exit(96)
		}
		os.Exit(0)
	case "process-group":
		child := exec.Command("sleep", "30")
		if err := child.Start(); err != nil {
			os.Exit(91)
		}
		if err := os.WriteFile(os.Getenv("THINGS_COMMAND_PID_FILE"), []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			os.Exit(92)
		}
		_ = child.Wait()
		os.Exit(0)
	case "detached-stdout":
		child := exec.Command("sleep", "30")
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := child.Start(); err != nil {
			os.Exit(94)
		}
		if err := os.WriteFile(os.Getenv("THINGS_COMMAND_PID_FILE"), []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			_ = child.Process.Kill()
			os.Exit(95)
		}
		select {}
	default:
		os.Exit(93)
	}
}

func TestOrchestratorRunsApprovedOrder(t *testing.T) {
	root, envFile, composeFile := composeRuntimeFixture(t)
	for name := range composeRuntimeFixtureSecrets {
		t.Setenv(name, "host-"+name)
	}
	t.Setenv("COMPOSE_RUNTIME_HOST_OVERRIDE", "host value")
	t.Setenv("COMPOSE_RUNTIME_QUOTED", "host quoted value")
	events := &runtimeEvents{}
	runtime, err := NewComposeRuntime(&fakeCommandRunner{events: events}, &fakeStageProbe{events: events}, root, envFile, composeFile)
	if err != nil {
		t.Fatal(err)
	}
	contract := validContractForTest()
	stateRoot := t.TempDir()
	checkingRuntime := &runningStateCheckingRuntime{runtime: runtime, stateRoot: stateRoot, t: t}
	if err := NewOrchestrator(checkingRuntime, &contract, stateRoot).Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	wantStages := []StageID{StagePrecheck, StageDatabaseBootstrap, StageInfraReady, StageTimedJobReady, StageSysReady, StageDMReady, StageDGProcessReady, StageDGLinkReady, StageAPIReady, StageVerified}
	if got := events.probedStages(); !reflect.DeepEqual(got, wantStages) {
		t.Fatalf("probe stages = %v, want %v", got, wantStages)
	}
	wantServices := []string{"config", "postgres-bootstrap", "redis,etcd,nats,tdengine,emqx", "timedjob", "sys", "dm", "dg", "api"}
	if got := events.commandServices(); !reflect.DeepEqual(got, wantServices) {
		t.Fatalf("command order = %v, want %v", got, wantServices)
	}
	for _, command := range events.commands() {
		if command.Path != "docker" || command.Dir != root || !containsEnvironment(command.Env, "THINGS_BASELINE_ENV_FILE", envFile) {
			t.Fatalf("compose command boundary = %+v", command)
		}
		for name, value := range composeRuntimeFixtureSecrets {
			if !containsEnvironment(command.Env, name, value) {
				t.Fatalf("compose command %v did not receive env-file %s value", command.Args, name)
			}
			if strings.Contains(strings.Join(command.Args, "\x00"), value) {
				t.Fatalf("compose argv leaked %s", name)
			}
		}
		if !containsEnvironment(command.Env, "COMPOSE_RUNTIME_HOST_OVERRIDE", "file value") || !containsEnvironment(command.Env, "COMPOSE_RUNTIME_QUOTED", "file quoted value") {
			t.Fatalf("compose command did not receive non-secret env-file values: %v", command.Env)
		}
		wantPrefix := []string{"compose", "--project-name", "ming-iot-things-baseline", "--project-directory", filepath.Dir(composeFile), "--env-file", envFile, "-f", composeFile}
		if len(command.Args) < len(wantPrefix) || !reflect.DeepEqual(command.Args[:len(wantPrefix)], wantPrefix) {
			t.Fatalf("compose argv = %v, want prefix %v", command.Args, wantPrefix)
		}
	}
	wantEvents := []runtimeEvent{
		{kind: "command", stage: StagePrecheck}, {kind: "probe", stage: StagePrecheck},
		{kind: "command", stage: StageDatabaseBootstrap}, {kind: "probe", stage: StageDatabaseBootstrap},
		{kind: "command", stage: StageInfraReady}, {kind: "probe", stage: StageInfraReady},
		{kind: "command", stage: StageTimedJobReady}, {kind: "probe", stage: StageTimedJobReady},
		{kind: "command", stage: StageSysReady}, {kind: "probe", stage: StageSysReady},
		{kind: "command", stage: StageDMReady}, {kind: "probe", stage: StageDMReady},
		{kind: "command", stage: StageDGProcessReady}, {kind: "probe", stage: StageDGProcessReady},
		{kind: "probe", stage: StageDGLinkReady},
		{kind: "command", stage: StageAPIReady}, {kind: "probe", stage: StageAPIReady},
		{kind: "probe", stage: StageVerified},
	}
	if got := events.stageEvents(); !reflect.DeepEqual(got, wantEvents) {
		t.Fatalf("combined runtime events = %v, want %v", got, wantEvents)
	}
	state := readStartupState(t, stateRoot)
	if len(state.Stages) != len(wantStages) {
		t.Fatalf("state stages = %d, want %d", len(state.Stages), len(wantStages))
	}
	for index, stage := range state.Stages {
		if stage.ID != wantStages[index] || stage.Status != "PASS" || stage.ErrorCode != "" {
			t.Fatalf("state stage %d = %+v", index, stage)
		}
	}
}

func TestComposeRuntimeRetriesTransientDependencyProbe(t *testing.T) {
	root, envFile, composeFile := composeRuntimeFixture(t)
	probe := &eventuallyReadyStageProbe{failures: 2}
	runtime, err := NewComposeRuntime(&fakeCommandRunner{events: &runtimeEvents{}}, probe, root, envFile, composeFile)
	if err != nil {
		t.Fatal(err)
	}
	runtime.probeInterval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := runtime.RunStage(ctx, StageAPIReady); err != nil {
		t.Fatalf("RunStage(API_READY) error = %v", err)
	}
	if probe.calls != 3 {
		t.Fatalf("probe calls = %d, want 3", probe.calls)
	}
}

func TestComposeRuntimeVerifiedStageFailsFast(t *testing.T) {
	root, envFile, composeFile := composeRuntimeFixture(t)
	probe := &eventuallyReadyStageProbe{failures: 5}
	runtime, err := NewComposeRuntime(&fakeCommandRunner{events: &runtimeEvents{}}, probe, root, envFile, composeFile)
	if err != nil {
		t.Fatal(err)
	}
	runtime.probeInterval = time.Millisecond
	err = runtime.RunStage(context.Background(), StageVerified)
	if DiagnosticCode(err) != CodeDependencyNotReady || probe.calls != 1 {
		t.Fatalf("RunStage(VERIFIED) error/calls = %v/%d, want DEPENDENCY_NOT_READY/1", err, probe.calls)
	}
}

func TestOrchestratorStopsAfterDMFailure(t *testing.T) {
	root, envFile, composeFile := composeRuntimeFixture(t)
	events := &runtimeEvents{}
	probe := &fakeStageProbe{events: events, failStage: StageDMReady}
	runtime, err := NewComposeRuntime(&fakeCommandRunner{events: events}, probe, root, envFile, composeFile)
	if err != nil {
		t.Fatal(err)
	}
	contract := validContractForTest()
	stateRoot := t.TempDir()
	err = NewOrchestrator(runtime, &contract, stateRoot).Up(context.Background())
	if DiagnosticCode(err) != CodeRoleNotReady {
		t.Fatalf("error = %v, code = %s, want %s", err, DiagnosticCode(err), CodeRoleNotReady)
	}
	for _, forbidden := range []StageID{StageDGProcessReady, StageDGLinkReady, StageAPIReady, StageVerified} {
		for _, got := range events.probedStages() {
			if got == forbidden {
				t.Fatalf("stage %s ran after DM failure: %v", forbidden, events.probedStages())
			}
		}
	}
	assertNoStartupStateTemporaryFiles(t, stateRoot)
	stateData, err := os.ReadFile(filepath.Join(stateRoot, "startup-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range composeRuntimeFixtureSecrets {
		if strings.Contains(string(stateData), value) {
			t.Fatalf("startup state leaked %s", name)
		}
	}
	state := readStartupState(t, stateRoot)
	last := state.Stages[len(state.Stages)-1]
	if last.ID != StageDMReady || last.Status != "FAIL" || last.ErrorCode != CodeRoleNotReady {
		t.Fatalf("failed state = %+v", last)
	}
	assertNoStartupStateTemporaryFiles(t, stateRoot)
}

func TestOrchestratorUpWithEvidenceRecordsEveryApprovedStage(t *testing.T) {
	contract := validContractForTest()
	runtime := &evidenceRecordingRuntime{}
	stages, err := NewOrchestrator(runtime, &contract, t.TempDir()).UpWithEvidence(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(runtime.stages, requiredStages) {
		t.Fatalf("runtime stages = %v, want %v", runtime.stages, requiredStages)
	}
	if len(stages) != len(requiredStages) {
		t.Fatalf("evidence stages = %d, want %d", len(stages), len(requiredStages))
	}
	for index, stage := range stages {
		if stage.ID != requiredStages[index] || stage.Result != "PASS" || stage.Code != "" {
			t.Fatalf("evidence stage %d = %+v", index, stage)
		}
		if stage.StartedAt.IsZero() || stage.FinishedAt.Before(stage.StartedAt) || stage.DurationMS < 0 {
			t.Fatalf("evidence stage %s has invalid timing: %+v", stage.ID, stage)
		}
		if stage.Component == "" || stage.LogRef == "" {
			t.Fatalf("evidence stage %s is missing component/log reference: %+v", stage.ID, stage)
		}
	}
}

func TestOrchestratorUpWithEvidenceMarksRemainingStagesNotRunAfterFailure(t *testing.T) {
	contract := validContractForTest()
	runtime := &evidenceRecordingRuntime{failStage: StageDMReady}
	stages, err := NewOrchestrator(runtime, &contract, t.TempDir()).UpWithEvidence(context.Background())
	if DiagnosticCode(err) != CodeRoleNotReady {
		t.Fatalf("error = %v, code = %s, want %s", err, DiagnosticCode(err), CodeRoleNotReady)
	}
	if len(stages) != len(requiredStages) {
		t.Fatalf("evidence stages = %d, want %d", len(stages), len(requiredStages))
	}
	failed := stages[5]
	if failed.ID != StageDMReady || failed.Result != "FAIL" || failed.Code != CodeRoleNotReady || failed.Reason == "" {
		t.Fatalf("failed evidence = %+v", failed)
	}
	for _, stage := range stages[6:] {
		if stage.Result != "NOT_RUN" || stage.Code != "" || stage.Reason == "" || !stage.StartedAt.IsZero() || !stage.FinishedAt.IsZero() {
			t.Fatalf("not-run evidence = %+v", stage)
		}
	}
}

func TestOrchestratorRootTimeoutMapsTimeout(t *testing.T) {
	contract := validContractForTest()
	orchestrator := NewOrchestrator(blockingStageRuntime{}, &contract, t.TempDir())
	if want := time.Duration(contract.Timeouts.TotalSeconds) * time.Second; orchestrator.totalTimeout != want {
		t.Fatalf("total timeout = %s, want %s", orchestrator.totalTimeout, want)
	}
	orchestrator.totalTimeout = 20 * time.Millisecond
	err := orchestrator.Up(context.Background())
	if DiagnosticCode(err) != CodeTimeout {
		t.Fatalf("error = %v, code = %s, want %s", err, DiagnosticCode(err), CodeTimeout)
	}
}

func TestWriteStartupStatePublishesReplacementInode(t *testing.T) {
	stateRoot := t.TempDir()
	firstState := startupState{Stages: []startupStageState{{ID: StagePrecheck, Status: "RUNNING"}}}
	if err := writeStartupState(stateRoot, firstState); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(stateRoot, "startup-state.json")
	first, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}

	secondState := startupState{Stages: []startupStageState{{ID: StageDatabaseBootstrap, Status: "PASS"}}}
	if err := writeStartupState(stateRoot, secondState); err != nil {
		t.Fatal(err)
	}
	second, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(first, second) {
		t.Fatal("startup-state.json retained its inode after publication")
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var got startupState
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, secondState) {
		t.Fatalf("published state = %+v, want %+v", got, secondState)
	}
	assertNoStartupStateTemporaryFiles(t, stateRoot)
}

func TestNewComposeRuntimeRejectsMalformedSecretEnvFile(t *testing.T) {
	root, envFile, composeFile := composeRuntimeFixture(t)
	if err := os.WriteFile(envFile, []byte("POSTGRES_ADMIN_PASSWORD='unterminated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewComposeRuntime(&fakeCommandRunner{events: &runtimeEvents{}}, &fakeStageProbe{events: &runtimeEvents{}}, root, envFile, composeFile)
	if err == nil {
		t.Fatal("NewComposeRuntime accepted malformed secret env file")
	}
}

func TestNewComposeRuntimeRejectsSymlinkInputsAndUnsafeEnvironmentFile(t *testing.T) {
	t.Run("env symlink inside repository", func(t *testing.T) {
		root, envFile, composeFile := composeRuntimeFixture(t)
		target := filepath.Join(filepath.Dir(envFile), "env.real")
		if err := os.WriteFile(target, []byte(composeRuntimeFixtureEnvironment()), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(envFile); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, envFile); err != nil {
			t.Fatal(err)
		}
		assertComposeRuntimeRejected(t, root, envFile, composeFile)
	})
	t.Run("compose symlink inside repository", func(t *testing.T) {
		root, envFile, composeFile := composeRuntimeFixture(t)
		target := filepath.Join(filepath.Dir(composeFile), "compose.real.yaml")
		if err := os.WriteFile(target, []byte("services: {}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(composeFile); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, composeFile); err != nil {
			t.Fatal(err)
		}
		assertComposeRuntimeRejected(t, root, envFile, composeFile)
	})
	t.Run("env symlink outside repository", func(t *testing.T) {
		root, envFile, composeFile := composeRuntimeFixture(t)
		outside := filepath.Join(t.TempDir(), "outside.env")
		if err := os.WriteFile(outside, []byte(composeRuntimeFixtureEnvironment()), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(envFile); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, envFile); err != nil {
			t.Fatal(err)
		}
		assertComposeRuntimeRejected(t, root, envFile, composeFile)
	})
	t.Run("environment file permissions", func(t *testing.T) {
		root, envFile, composeFile := composeRuntimeFixture(t)
		if err := os.Chmod(envFile, 0o644); err != nil {
			t.Fatal(err)
		}
		assertComposeRuntimeRejected(t, root, envFile, composeFile)
	})
}

func TestNewComposeRuntimeRejectsEmptyOrDuplicateSecret(t *testing.T) {
	t.Run("empty secret", func(t *testing.T) {
		root, envFile, composeFile := composeRuntimeFixture(t)
		environment := strings.Replace(composeRuntimeFixtureEnvironment(), "POSTGRES_ADMIN_PASSWORD=postgres-file-secret", "POSTGRES_ADMIN_PASSWORD=", 1)
		if err := os.WriteFile(envFile, []byte(environment), 0o600); err != nil {
			t.Fatal(err)
		}
		assertComposeRuntimeRejected(t, root, envFile, composeFile)
	})
	t.Run("duplicate key", func(t *testing.T) {
		root, envFile, composeFile := composeRuntimeFixture(t)
		environment := composeRuntimeFixtureEnvironment() + "CORE_USER_TOKEN_SECRET=duplicate\n"
		if err := os.WriteFile(envFile, []byte(environment), 0o600); err != nil {
			t.Fatal(err)
		}
		assertComposeRuntimeRejected(t, root, envFile, composeFile)
	})
}

func TestNewComposeRuntimeRejectsUnsupportedDotenvValues(t *testing.T) {
	tests := []struct {
		name        string
		replacement string
		want        string
	}{
		{"unquoted interpolation", "COMPOSE_RUNTIME_HOST_OVERRIDE=$HOME", "unsupported interpolation"},
		{"double quoted interpolation", "COMPOSE_RUNTIME_QUOTED=\"${HOME}\"", "unsupported interpolation"},
		{"multiline quote", "COMPOSE_RUNTIME_QUOTED=\"first\nsecond\"", "invalid double-quoted value"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, envFile, composeFile := composeRuntimeFixture(t)
			environment := strings.Replace(composeRuntimeFixtureEnvironment(), "COMPOSE_RUNTIME_HOST_OVERRIDE=file value # inline comment", test.replacement, 1)
			if test.name == "double quoted interpolation" || test.name == "multiline quote" {
				environment = strings.Replace(composeRuntimeFixtureEnvironment(), "COMPOSE_RUNTIME_QUOTED='file quoted value'", test.replacement, 1)
			}
			if err := os.WriteFile(envFile, []byte(environment), 0o600); err != nil {
				t.Fatal(err)
			}
			assertComposeRuntimeRejectedWithMessage(t, root, envFile, composeFile, test.want)
		})
	}

	t.Run("single quoted dollar remains literal", func(t *testing.T) {
		root, envFile, composeFile := composeRuntimeFixture(t)
		environment := strings.Replace(composeRuntimeFixtureEnvironment(), "COMPOSE_RUNTIME_QUOTED='file quoted value'", "COMPOSE_RUNTIME_QUOTED='literal $HOME'", 1)
		if err := os.WriteFile(envFile, []byte(environment), 0o600); err != nil {
			t.Fatal(err)
		}
		if runtime, err := NewComposeRuntime(&fakeCommandRunner{events: &runtimeEvents{}}, &fakeStageProbe{events: &runtimeEvents{}}, root, envFile, composeFile); err != nil || runtime == nil {
			t.Fatalf("NewComposeRuntime() = %v, %v; want literal dollar accepted", runtime, err)
		}
	})
}

func TestComposeRuntimeRejectsChangedInputsBeforeCommand(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string, string)
	}{
		{
			name: "compose symlink",
			mutate: func(t *testing.T, _ string, composeFile string) {
				target := filepath.Join(filepath.Dir(composeFile), "compose-replacement.yaml")
				if err := os.WriteFile(target, []byte("services: {}\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(composeFile); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, composeFile); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "compose digest drift",
			mutate: func(t *testing.T, _ string, composeFile string) {
				if err := os.WriteFile(composeFile, []byte("services:\n  changed: {}\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "environment digest drift",
			mutate: func(t *testing.T, envFile, _ string) {
				if err := os.WriteFile(envFile, []byte(composeRuntimeFixtureEnvironment()+"COMPOSE_RUNTIME_DRIFT=1\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "environment permissions drift",
			mutate: func(t *testing.T, envFile, _ string) {
				if err := os.Chmod(envFile, 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, envFile, composeFile := composeRuntimeFixture(t)
			events := &runtimeEvents{}
			runtime, err := NewComposeRuntime(&fakeCommandRunner{events: events}, &fakeStageProbe{events: events}, root, envFile, composeFile)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(t, envFile, composeFile)
			err = runtime.RunStage(context.Background(), StagePrecheck)
			if DiagnosticCode(err) != CodeContractMismatch {
				t.Fatalf("DiagnosticCode(%v) = %s, want %s", err, DiagnosticCode(err), CodeContractMismatch)
			}
			if got := len(events.commands()); got != 0 {
				t.Fatalf("command calls = %d, want 0 after input rejection", got)
			}
		})
	}
}

func TestOrchestratorComposeContract(t *testing.T) {
	data, err := os.ReadFile(thingsBaselineProfilePath(t, "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var compose struct {
		Common   task6ComposeService            `yaml:"x-app-common"`
		Services map[string]task6ComposeService `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &compose); err != nil {
		t.Fatal(err)
	}
	wantEnvironment := map[string]string{
		"CORE_DB_DSN":                          "postgres://${THINGS_BASELINE_DB_USER}:${THINGS_BASELINE_DB_PASSWORD}@host.docker.internal:${POSTGRES_ADMIN_PORT}/ming_iot_things_baseline?sslmode=disable",
		"THINGS_DB_DSN":                        "postgres://${THINGS_BASELINE_DB_USER}:${THINGS_BASELINE_DB_PASSWORD}@host.docker.internal:${POSTGRES_ADMIN_PORT}/ming_iot_things_baseline?sslmode=disable",
		"THINGS_TSDB_DSN":                      "root:${TDENGINE_ROOT_PASSWORD}@http(tdengine:6041)/ithings?readBufferSize=5242880",
		"CORE_LOCAL_SKIP_STARTUP_SIDE_EFFECTS": "true",
		"CORE_LOCAL_SKIP_AUTO_MIGRATE":         "false",
	}
	if !reflect.DeepEqual(compose.Common.Environment, wantEnvironment) || !reflect.DeepEqual(compose.Common.EnvFile, []string{"${THINGS_BASELINE_ENV_FILE:-.env.local}"}) || !reflect.DeepEqual(compose.Common.ExtraHosts, []string{"host.docker.internal:host-gateway"}) || !reflect.DeepEqual(compose.Common.Networks, []string{"baseline"}) || compose.Common.Restart != "no" || !reflect.DeepEqual(scalarVolumes(t, compose.Common.Volumes), []string{"local_oss:/app/oss"}) {
		t.Fatalf("x-app-common = %+v", compose.Common)
	}
	want := map[string]struct {
		image     string
		dependsOn []string
	}{
		"timedjob": {"ming-iot-timedjob:baseline", []string{"redis", "etcd", "nats"}},
		"sys":      {"ming-iot-sys:baseline", []string{"redis", "etcd", "nats", "timedjob"}},
		"dm":       {"ming-iot-dm:baseline", []string{"redis", "etcd", "nats", "tdengine", "timedjob", "sys"}},
		"dg":       {"ming-iot-dg:baseline", []string{"redis", "nats", "emqx", "timedjob", "dm"}},
		"api":      {"ming-iot-api:baseline", []string{"redis", "etcd", "nats", "timedjob", "sys", "dm", "dg"}},
	}
	for name, expected := range want {
		service, ok := compose.Services[name]
		if !ok || service.Image != expected.image || service.Build.Kind != 0 || !reflect.DeepEqual(service.DependsOn, expected.dependsOn) || service.Restart != "no" || !reflect.DeepEqual(scalarVolumes(t, service.Volumes), []string{"local_oss:/app/oss"}) || !reflect.DeepEqual(service.Networks, []string{"baseline"}) {
			t.Errorf("service %s = %+v", name, service)
		}
	}
	if got := compose.Services["api"].Ports; !reflect.DeepEqual(got, []string{"127.0.0.1:${THINGS_BASELINE_API_PORT:-17788}:7788"}) {
		t.Fatalf("api ports = %v", got)
	}
	if got := scalarVolumes(t, compose.Services["tdengine"].Volumes); !reflect.DeepEqual(got, []string{"tdengine_data:/var/lib/taos", "tdengine_log:/var/log/taos", "./scripts/bootstrap-tdengine.sql:/docker-entrypoint-initdb.d/001-create-ithings.sql:ro"}) {
		t.Fatalf("tdengine volumes = %v", got)
	}
}

type task6ComposeService struct {
	Image       string            `yaml:"image"`
	Build       yaml.Node         `yaml:"build"`
	EnvFile     []string          `yaml:"env_file"`
	Environment map[string]string `yaml:"environment"`
	ExtraHosts  []string          `yaml:"extra_hosts"`
	Volumes     []yaml.Node       `yaml:"volumes"`
	Networks    []string          `yaml:"networks"`
	Restart     string            `yaml:"restart"`
	DependsOn   []string          `yaml:"depends_on"`
	Ports       []string          `yaml:"ports"`
}

type runtimeEvents struct {
	mu      sync.Mutex
	entries []runtimeEvent
}

type runtimeEvent struct {
	kind    string
	stage   StageID
	command Command
}

func (events *runtimeEvents) add(event runtimeEvent) {
	events.mu.Lock()
	defer events.mu.Unlock()
	events.entries = append(events.entries, event)
}

func (events *runtimeEvents) probedStages() []StageID {
	events.mu.Lock()
	defer events.mu.Unlock()
	var stages []StageID
	for _, event := range events.entries {
		if event.kind == "probe" {
			stages = append(stages, event.stage)
		}
	}
	return stages
}

func (events *runtimeEvents) commands() []Command {
	events.mu.Lock()
	defer events.mu.Unlock()
	var commands []Command
	for _, event := range events.entries {
		if event.kind == "command" {
			commands = append(commands, event.command)
		}
	}
	return commands
}

func (events *runtimeEvents) stageEvents() []runtimeEvent {
	events.mu.Lock()
	defer events.mu.Unlock()
	result := make([]runtimeEvent, len(events.entries))
	for index, event := range events.entries {
		result[index] = runtimeEvent{kind: event.kind, stage: event.stage}
	}
	return result
}

func (events *runtimeEvents) commandServices() []string {
	var services []string
	for _, command := range events.commands() {
		args := command.Args
		switch {
		case len(args) >= 2 && args[len(args)-2] == "config":
			services = append(services, "config")
		case len(args) >= 1 && args[len(args)-1] == "postgres-bootstrap":
			services = append(services, "postgres-bootstrap")
		default:
			index := -1
			for i, arg := range args {
				if arg == "-d" {
					index = i
				}
			}
			if index >= 0 {
				services = append(services, strings.Join(args[index+1:], ","))
			}
		}
	}
	return services
}

type fakeCommandRunner struct{ events *runtimeEvents }

func (runner *fakeCommandRunner) Run(_ context.Context, command Command) (CommandResult, error) {
	command.Args = append([]string(nil), command.Args...)
	command.Env = append([]string(nil), command.Env...)
	runner.events.add(runtimeEvent{kind: "command", stage: commandStage(command), command: command})
	return CommandResult{ExitCode: 0}, nil
}

func commandStage(command Command) StageID {
	args := command.Args
	switch {
	case len(args) >= 2 && args[len(args)-2] == "config":
		return StagePrecheck
	case len(args) >= 1 && args[len(args)-1] == "postgres-bootstrap":
		return StageDatabaseBootstrap
	case len(args) >= 1 && args[len(args)-1] == "emqx":
		return StageInfraReady
	case len(args) >= 1 && args[len(args)-1] == "timedjob":
		return StageTimedJobReady
	case len(args) >= 1 && args[len(args)-1] == "sys":
		return StageSysReady
	case len(args) >= 1 && args[len(args)-1] == "dm":
		return StageDMReady
	case len(args) >= 1 && args[len(args)-1] == "dg":
		return StageDGProcessReady
	case len(args) >= 1 && args[len(args)-1] == "api":
		return StageAPIReady
	default:
		return ""
	}
}

type fakeStageProbe struct {
	events    *runtimeEvents
	failStage StageID
}

type eventuallyReadyStageProbe struct {
	calls    int
	failures int
}

func (probe *eventuallyReadyStageProbe) Probe(context.Context, StageID) error {
	probe.calls++
	if probe.calls <= probe.failures {
		return diagnostic(CodeDependencyNotReady, errors.New("fixture dependency is starting"))
	}
	return nil
}

func (probe *fakeStageProbe) Probe(ctx context.Context, stage StageID) error {
	probe.events.add(runtimeEvent{kind: "probe", stage: stage})
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 91*time.Second {
		return fmt.Errorf("stage %s did not receive probe timeout", stage)
	}
	if stage == probe.failStage {
		return errors.New("fixture stage failure")
	}
	return nil
}

type blockingStageRuntime struct{}

func (blockingStageRuntime) RunStage(ctx context.Context, _ StageID) error {
	<-ctx.Done()
	return ctx.Err()
}

type evidenceRecordingRuntime struct {
	stages    []StageID
	failStage StageID
}

func (runtime *evidenceRecordingRuntime) RunStage(_ context.Context, stage StageID) error {
	runtime.stages = append(runtime.stages, stage)
	if stage == runtime.failStage {
		return errors.New("fixture stage failure")
	}
	return nil
}

func TestOrchestratorPreservesStageCodeWhenFailStateWriteFails(t *testing.T) {
	contract := validContractForTest()
	stateRoot := t.TempDir()
	err := NewOrchestrator(&stateWriteFailingRuntime{stateRoot: stateRoot}, &contract, stateRoot).Up(context.Background())
	if DiagnosticCode(err) != CodeRoleNotReady {
		t.Fatalf("DiagnosticCode(%v) = %s, want %s", err, DiagnosticCode(err), CodeRoleNotReady)
	}
	if !strings.Contains(err.Error(), "DM runtime failure") || !strings.Contains(err.Error(), "create startup state temporary file") {
		t.Fatalf("error = %v, want both runtime and state persistence failures", err)
	}
}

type stateWriteFailingRuntime struct{ stateRoot string }

func (runtime *stateWriteFailingRuntime) RunStage(_ context.Context, stage StageID) error {
	if stage != StageDMReady {
		return nil
	}
	if err := os.RemoveAll(runtime.stateRoot); err != nil {
		return err
	}
	if err := os.WriteFile(runtime.stateRoot, []byte("not a directory"), 0o600); err != nil {
		return err
	}
	return errors.New("DM runtime failure")
}

type runningStateCheckingRuntime struct {
	runtime   StageRuntime
	stateRoot string
	t         *testing.T
}

func (runtime *runningStateCheckingRuntime) RunStage(ctx context.Context, stage StageID) error {
	runtime.t.Helper()
	state := readStartupState(runtime.t, runtime.stateRoot)
	if len(state.Stages) == 0 {
		runtime.t.Fatal("startup state is empty before stage execution")
	}
	last := state.Stages[len(state.Stages)-1]
	if last.ID != stage || last.Status != "RUNNING" || last.ErrorCode != "" {
		runtime.t.Fatalf("startup state before %s = %+v, want RUNNING", stage, last)
	}
	return runtime.runtime.RunStage(ctx, stage)
}

type startupStateFixture struct {
	Stages []struct {
		ID        StageID   `json:"id"`
		Status    string    `json:"status"`
		ErrorCode ErrorCode `json:"errorCode,omitempty"`
	} `json:"stages"`
}

func readStartupState(t *testing.T, root string) startupStateFixture {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "startup-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state startupStateFixture
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	return state
}

func composeRuntimeFixture(t *testing.T) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	profile := filepath.Join(root, "deploy", "profiles", "things-baseline")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		t.Fatal(err)
	}
	envFile := filepath.Join(profile, ".env.local")
	env := composeRuntimeFixtureEnvironment()
	if err := os.WriteFile(envFile, []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	composeFile := filepath.Join(profile, "compose.yaml")
	if err := os.WriteFile(composeFile, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, envFile, composeFile
}

func composeRuntimeFixtureEnvironment() string {
	return "# baseline secrets\n\n" +
		"POSTGRES_ADMIN_PASSWORD=" + composeRuntimeFixtureSecrets["POSTGRES_ADMIN_PASSWORD"] + "\n" +
		"THINGS_BASELINE_DB_PASSWORD='" + composeRuntimeFixtureSecrets["THINGS_BASELINE_DB_PASSWORD"] + "'\n" +
		"CORE_USER_TOKEN_SECRET=\"" + composeRuntimeFixtureSecrets["CORE_USER_TOKEN_SECRET"] + "\"\n" +
		"THINGS_INTERNAL_AUTH_PASSWORD=" + composeRuntimeFixtureSecrets["THINGS_INTERNAL_AUTH_PASSWORD"] + "\n" +
		"THINGS_EMQX_API_KEY=" + composeRuntimeFixtureSecrets["THINGS_EMQX_API_KEY"] + "\n" +
		"THINGS_EMQX_SECRET_KEY=" + composeRuntimeFixtureSecrets["THINGS_EMQX_SECRET_KEY"] + "\n" +
		"TDENGINE_ROOT_PASSWORD=" + composeRuntimeFixtureSecrets["TDENGINE_ROOT_PASSWORD"] + "\n" +
		"COMPOSE_RUNTIME_HOST_OVERRIDE=file value # inline comment\n" +
		"COMPOSE_RUNTIME_QUOTED='file quoted value'\n"
}

func assertComposeRuntimeRejected(t *testing.T, root, envFile, composeFile string) {
	t.Helper()
	if runtime, err := NewComposeRuntime(&fakeCommandRunner{events: &runtimeEvents{}}, &fakeStageProbe{events: &runtimeEvents{}}, root, envFile, composeFile); err == nil || runtime != nil {
		t.Fatalf("NewComposeRuntime(%q, %q) = %v, %v; want rejection", envFile, composeFile, runtime, err)
	}
}

func assertComposeRuntimeRejectedWithMessage(t *testing.T, root, envFile, composeFile, want string) {
	t.Helper()
	_, err := NewComposeRuntime(&fakeCommandRunner{events: &runtimeEvents{}}, &fakeStageProbe{events: &runtimeEvents{}}, root, envFile, composeFile)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("NewComposeRuntime() error = %v, want %q", err, want)
	}
}

var composeRuntimeFixtureSecrets = map[string]string{
	"POSTGRES_ADMIN_PASSWORD":       "postgres-file-secret",
	"THINGS_BASELINE_DB_PASSWORD":   "database-file-secret",
	"CORE_USER_TOKEN_SECRET":        "token-file-secret",
	"THINGS_INTERNAL_AUTH_PASSWORD": "internal-file-secret",
	"THINGS_EMQX_API_KEY":           "emqx-key-file-secret",
	"THINGS_EMQX_SECRET_KEY":        "emqx-secret-file-secret",
	"TDENGINE_ROOT_PASSWORD":        "tdengine-file-secret",
}

func assertNoStartupStateTemporaryFiles(t *testing.T, stateRoot string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(stateRoot, ".startup-state-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("startup state temporary files remain: %v", files)
	}
}

func replaceEnvironment(environment []string, name, value string) []string {
	prefix := name + "="
	result := make([]string, 0, len(environment)+1)
	for _, item := range environment {
		if !strings.HasPrefix(item, prefix) {
			result = append(result, item)
		}
	}
	return append(result, prefix+value)
}

func containsEnvironment(environment []string, name, value string) bool {
	want := name + "=" + value
	count := 0
	for _, item := range environment {
		if item == want {
			count++
		}
	}
	return count == 1
}

func sha256String(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func scalarVolumes(t *testing.T, nodes []yaml.Node) []string {
	t.Helper()
	values := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if node.Kind != yaml.ScalarNode {
			t.Fatalf("volume is not scalar: %+v", node)
		}
		values = append(values, node.Value)
	}
	return values
}
