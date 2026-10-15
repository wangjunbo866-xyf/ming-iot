package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	"gopkg.in/yaml.v3"
	"ming-iot/backend/tools/internal/thingsstartbaseline"
)

func (executor defaultExecutor) ValidateEnvironment(command string, options commandOptions) error {
	if command != "contract" {
		return nil
	}
	contract, err := thingsstartbaseline.LoadContract(options.ContractPath)
	if err != nil {
		return err
	}
	return validateBaselineDatabaseEnvironment(contract, options.EnvFile)
}

func (executor defaultExecutor) Execute(command string, options commandOptions) error {
	switch command {
	case "manifest":
		binDir, output := options.BinDir, options.Output
		if options.RequestedCommand != "manifest" {
			binDir = filepath.Join(options.Repo, "backend", "bin")
			output = filepath.Join(options.RuntimeRoot, "build-manifest.json")
		}
		writeManifest := executor.writeManifest
		if writeManifest == nil {
			writeManifest = thingsstartbaseline.WriteBuildManifest
		}
		return writeManifest(options.ContractPath, options.Repo, binDir, output)
	case "stage":
		contract, err := thingsstartbaseline.LoadContract(options.ContractPath)
		if err != nil {
			return err
		}
		if err := validateBaselineDatabaseEnvironment(contract, options.EnvFile); err != nil {
			return err
		}
		environment, err := loadRuntimeEnvironment(options.EnvFile)
		if err != nil {
			return err
		}
		if err := thingsstartbaseline.PrepareEMQXBootstrapSecret(options.RuntimeRoot, environment["THINGS_EMQX_API_KEY"], environment["THINGS_EMQX_SECRET_KEY"]); err != nil {
			return err
		}
		stage := executor.stage
		if stage == nil {
			stage = thingsstartbaseline.Stage
		}
		_, err = stage(thingsstartbaseline.StageRequest{
			RepoRoot:    options.Repo,
			RuntimeRoot: options.RuntimeRoot,
			Contract:    contract,
		})
		if err != nil {
			return err
		}
		return thingsstartbaseline.WriteEMQXBootstrapSecret(
			options.RuntimeRoot,
			environment["THINGS_EMQX_API_KEY"],
			environment["THINGS_EMQX_SECRET_KEY"],
		)
	case "image":
		return executor.buildImages(options)
	case "database", "infra", "roles":
		if options.RequestedCommand != "up" {
			return fmt.Errorf("%s is only valid during up", command)
		}
		// The observable CLI workflow keeps these phase boundaries for callers
		// and tests. The production Orchestrator owns the actual ten-stage
		// sequence and runs it exactly once from the final verify phase.
		return nil
	case "verify":
		if options.RequestedCommand == "up" {
			runStartup := executor.up
			if runStartup == nil {
				runStartup = func(options commandOptions) error {
					return runProductionStartupWithRunner(options, executor.runner)
				}
			}
			return runStartup(options)
		}
		if options.RequestedCommand == "verify" {
			runVerification := executor.verify
			if runVerification == nil {
				runVerification = func(options commandOptions) error {
					return runProductionVerificationWithRunner(options, executor.runner)
				}
			}
			return runVerification(options)
		}
		return errors.New("verify is only valid during up or verify")
	case "logs":
		if options.RequestedCommand != "logs" {
			return errors.New("logs is only valid during logs")
		}
		return runProductionLogsWithRunner(options, executor.runner, executor.output)
	default:
		return fmt.Errorf("%s is not implemented", command)
	}
}

func validateBaselineDatabaseEnvironment(contract *thingsstartbaseline.Contract, filename string) error {
	if err := validateEnvFile(filename); err != nil {
		return err
	}
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("read environment protection target: %w", err)
	}
	defer file.Close()

	var database string
	found := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) != "THINGS_BASELINE_DB" {
			continue
		}
		if found {
			return errors.New("environment protection target THINGS_BASELINE_DB is duplicated")
		}
		found = true
		database, err = parseEnvironmentTarget(value)
		if err != nil {
			return fmt.Errorf("environment protection target THINGS_BASELINE_DB is invalid: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read environment protection target: %w", err)
	}
	if !found {
		return errors.New("environment protection target THINGS_BASELINE_DB is missing")
	}
	if database != contract.Database.RequiredName {
		return fmt.Errorf("environment protection target THINGS_BASELINE_DB must be %s", contract.Database.RequiredName)
	}
	return nil
}

func parseEnvironmentTarget(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("value is empty")
	}
	if strings.HasPrefix(value, "'") {
		if len(value) < 2 || !strings.HasSuffix(value, "'") {
			return "", errors.New("unterminated single-quoted value")
		}
		return value[1 : len(value)-1], nil
	}
	if strings.HasPrefix(value, "\"") {
		parsed, err := strconv.Unquote(value)
		if err != nil {
			return "", errors.New("invalid double-quoted value")
		}
		return parsed, nil
	}
	if index := strings.Index(value, " #"); index >= 0 {
		value = strings.TrimSpace(value[:index])
	}
	if strings.ContainsAny(value, "\"'") {
		return "", errors.New("unquoted value contains a quote")
	}
	return value, nil
}

type productionRuntime struct {
	contract      *thingsstartbaseline.Contract
	runtime       *thingsstartbaseline.ComposeRuntime
	probe         *thingsstartbaseline.ProtocolProbe
	runner        thingsstartbaseline.CommandRunner
	environment   map[string]string
	inputIdentity productionInputIdentity
}

type productionInputIdentity struct {
	contractSHA256 string
	envSHA256      string
	composeSHA256  string
}

type productionEvidenceSnapshot struct {
	runID               string
	manifest            thingsstartbaseline.BuildManifest
	contractSHA256      string
	versions            map[string]string
	composeConfigSHA256 string
	images              map[string]string
	secrets             []string
}

var requiredRuntimeEnvironment = []string{
	"POSTGRES_ADMIN_HOST",
	"POSTGRES_ADMIN_PORT",
	"POSTGRES_ADMIN_USER",
	"POSTGRES_ADMIN_PASSWORD",
	"THINGS_BASELINE_DB",
	"THINGS_BASELINE_DB_USER",
	"THINGS_BASELINE_DB_PASSWORD",
	"CORE_USER_TOKEN_SECRET",
	"THINGS_INTERNAL_AUTH_PASSWORD",
	"THINGS_EMQX_API_KEY",
	"THINGS_EMQX_SECRET_KEY",
	"TDENGINE_ROOT_PASSWORD",
}

var runtimeSecretEnvironmentNames = []string{
	"POSTGRES_ADMIN_PASSWORD",
	"THINGS_BASELINE_DB_PASSWORD",
	"CORE_USER_TOKEN_SECRET",
	"THINGS_INTERNAL_AUTH_PASSWORD",
	"THINGS_EMQX_API_KEY",
	"THINGS_EMQX_SECRET_KEY",
	"TDENGINE_ROOT_PASSWORD",
}

var infrastructureServices = []string{"redis", "etcd", "nats", "tdengine", "emqx"}

func runProductionStartup(options commandOptions) error {
	return runProductionStartupWithRunner(options, nil)
}

func runProductionStartupWithRunner(options commandOptions, runner thingsstartbaseline.CommandRunner) error {
	components, err := newProductionRuntime(options, runner)
	if err != nil {
		return err
	}
	snapshot, err := prepareProductionEvidence(options, components)
	if err != nil {
		return fmt.Errorf("prepare startup evidence: %w", err)
	}
	stateRoot := filepath.Join(options.RuntimeRoot, "state")
	stages, startupErr := thingsstartbaseline.NewOrchestrator(components.runtime, components.contract, stateRoot).UpWithEvidence(context.Background())
	stages = completeProductionFailureProjection(components.contract, stages, startupErr)
	stages, startupErr = closeProductionInputIdentity(options, components, stages, startupErr)
	_, evidenceErr := publishPreparedProductionEvidence(options, snapshot, stages, startupErr)
	return errors.Join(startupErr, evidenceErr)
}

func runProductionVerification(options commandOptions) error {
	return runProductionVerificationWithRunner(options, nil)
}

func runProductionVerificationWithRunner(options commandOptions, runner thingsstartbaseline.CommandRunner) error {
	components, err := newProductionRuntime(options, runner)
	if err != nil {
		return err
	}
	snapshot, err := prepareProductionEvidence(options, components)
	if err != nil {
		return fmt.Errorf("prepare verification evidence: %w", err)
	}
	verification := verificationRuntime{compose: components.runtime, probe: components.probe}
	stages, verifyErr := verifyProductionStages(context.Background(), components.contract, verification)
	stages = completeProductionFailureProjection(components.contract, stages, verifyErr)
	stages, verifyErr = closeProductionInputIdentity(options, components, stages, verifyErr)
	_, evidenceErr := publishPreparedProductionEvidence(options, snapshot, stages, verifyErr)
	return errors.Join(verifyErr, evidenceErr)
}

func runProductionLogsWithRunner(options commandOptions, runner thingsstartbaseline.CommandRunner, output io.Writer) error {
	if output == nil {
		return errors.New("logs output is nil")
	}
	components, err := newProductionRuntime(options, runner)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(components.contract.Timeouts.ProbeSeconds)*time.Second)
	defer cancel()
	command := productionComposeCommand(components.contract, options, components.environment)
	command.Args = append(command.Args, "logs", "--no-color", "--tail", "200")
	result, runErr := components.runner.Run(ctx, command)
	if runErr == nil && result.ExitCode != 0 {
		runErr = fmt.Errorf("docker exited with status %d", result.ExitCode)
	}
	_, writeErr := io.WriteString(output, result.Summary)
	if writeErr != nil {
		writeErr = fmt.Errorf("write logs summary: %w", writeErr)
	}
	if runErr != nil {
		runErr = fmt.Errorf("read Compose logs: %w", runErr)
	}
	return errors.Join(runErr, writeErr)
}

func newProductionRuntime(options commandOptions, providedRunner ...thingsstartbaseline.CommandRunner) (productionRuntime, error) {
	inputIdentity, err := readProductionInputIdentity(options)
	if err != nil {
		return productionRuntime{}, err
	}
	contract, err := thingsstartbaseline.LoadContract(options.ContractPath)
	if err != nil {
		return productionRuntime{}, err
	}
	if err := validateBaselineDatabaseEnvironment(contract, options.EnvFile); err != nil {
		return productionRuntime{}, err
	}
	environment, err := loadRuntimeEnvironment(options.EnvFile)
	if err != nil {
		return productionRuntime{}, err
	}
	postgresPort, err := runtimePort(environment, "POSTGRES_ADMIN_PORT", "")
	if err != nil {
		return productionRuntime{}, err
	}
	emqxPort, err := runtimePort(environment, "THINGS_BASELINE_EMQX_DASHBOARD_PORT", "18084")
	if err != nil {
		return productionRuntime{}, err
	}
	apiPort, err := runtimePort(environment, "THINGS_BASELINE_API_PORT", "17788")
	if err != nil {
		return productionRuntime{}, err
	}
	var runner thingsstartbaseline.CommandRunner
	if len(providedRunner) > 0 {
		runner = providedRunner[0]
	}
	if runner == nil {
		runner = thingsstartbaseline.NewCommandRunner()
	}
	composeCommand := productionComposeCommand(contract, options, environment)
	probe, err := thingsstartbaseline.NewProtocolProbe(runner, thingsstartbaseline.ProtocolProbeConfig{
		ComposeCommand: composeCommand,
		Contract:       contract,
		PostgreSQL: thingsstartbaseline.PostgreSQLProbeConfig{
			Host:     environment["POSTGRES_ADMIN_HOST"],
			Port:     postgresPort,
			User:     environment["POSTGRES_ADMIN_USER"],
			Password: environment["POSTGRES_ADMIN_PASSWORD"],
		},
		TDengineUsername: "root",
		TDenginePassword: environment["TDENGINE_ROOT_PASSWORD"],
		EMQXURL:          "http://127.0.0.1:" + emqxPort,
		EMQXAPIKey:       environment["THINGS_EMQX_API_KEY"],
		EMQXSecretKey:    environment["THINGS_EMQX_SECRET_KEY"],
		APIURL:           "http://127.0.0.1:" + apiPort,
	})
	if err != nil {
		return productionRuntime{}, fmt.Errorf("create protocol probe: %w", err)
	}
	runtime, err := thingsstartbaseline.NewComposeRuntime(runner, probe, options.Repo, options.EnvFile, options.ComposeFile)
	if err != nil {
		return productionRuntime{}, fmt.Errorf("create compose runtime: %w", err)
	}
	if err := verifyProductionInputIdentity(options, inputIdentity); err != nil {
		return productionRuntime{}, err
	}
	return productionRuntime{
		contract:      contract,
		runtime:       runtime,
		probe:         probe,
		runner:        runner,
		environment:   environment,
		inputIdentity: inputIdentity,
	}, nil
}

func productionComposeCommand(contract *thingsstartbaseline.Contract, options commandOptions, environment map[string]string) thingsstartbaseline.Command {
	return thingsstartbaseline.Command{
		Path: "docker",
		Args: []string{
			"compose",
			"--project-name", contract.ProjectName,
			"--project-directory", filepath.Dir(options.ComposeFile),
			"--env-file", options.EnvFile,
			"-f", options.ComposeFile,
		},
		Dir: options.Repo,
		Env: runtimeCommandEnvironment(environment, options.EnvFile),
	}
}

func readProductionInputIdentity(options commandOptions) (productionInputIdentity, error) {
	contractSHA256, _, err := hashRegularNonSymlink(options.ContractPath, "startup contract")
	if err != nil {
		return productionInputIdentity{}, err
	}
	envSHA256, _, err := hashRegularNonSymlink(options.EnvFile, "runtime environment")
	if err != nil {
		return productionInputIdentity{}, err
	}
	composeSHA256, _, err := hashRegularNonSymlink(options.ComposeFile, "compose file")
	if err != nil {
		return productionInputIdentity{}, err
	}
	return productionInputIdentity{
		contractSHA256: contractSHA256,
		envSHA256:      envSHA256,
		composeSHA256:  composeSHA256,
	}, nil
}

func verifyProductionInputIdentity(options commandOptions, expected productionInputIdentity) error {
	actual, err := readProductionInputIdentity(options)
	if err != nil {
		return err
	}
	if actual != expected {
		return errors.New("startup contract, environment, or compose input changed during execution")
	}
	return nil
}

func closeProductionInputIdentity(
	options commandOptions,
	components productionRuntime,
	stages []thingsstartbaseline.StageEvidence,
	runErr error,
) ([]thingsstartbaseline.StageEvidence, error) {
	identityErr := verifyProductionInputIdentity(options, components.inputIdentity)
	if identityErr == nil {
		return stages, runErr
	}
	if runErr != nil {
		closed := append([]thingsstartbaseline.StageEvidence(nil), stages...)
		for index := range closed {
			if closed[index].Result == "FAIL" {
				closed[index].Reason = strings.TrimSpace(closed[index].Reason + "; " + identityErr.Error())
				break
			}
		}
		return closed, errors.Join(runErr, identityErr)
	}
	if len(stages) == 0 {
		return stages, errors.Join(runErr, identityErr)
	}
	closed := append([]thingsstartbaseline.StageEvidence(nil), stages...)
	last := len(closed) - 1
	if closed[last].Result == "PASS" {
		closed[last].Result = "FAIL"
		closed[last].Code = thingsstartbaseline.CodeConfigInvalid
		closed[last].Reason = identityErr.Error()
	}
	return closed, identityErr
}

func completeProductionFailureProjection(
	contract *thingsstartbaseline.Contract,
	stages []thingsstartbaseline.StageEvidence,
	runErr error,
) []thingsstartbaseline.StageEvidence {
	if runErr == nil || len(stages) != 0 || contract == nil || len(contract.Stages) == 0 {
		return stages
	}
	started := time.Now().UTC()
	result := make([]thingsstartbaseline.StageEvidence, 0, len(contract.Stages))
	first := contract.Stages[0]
	result = append(result, thingsstartbaseline.StageEvidence{
		ID:         first.ID,
		Result:     "FAIL",
		StartedAt:  started,
		FinishedAt: started,
		DurationMS: 0,
		Component:  strings.Join(first.Components, ","),
		Code:       thingsstartbaseline.CodeConfigInvalid,
		Reason:     runErr.Error(),
		LogRef:     "logs/" + strings.ToLower(string(first.ID)) + ".log",
	})
	for _, remaining := range contract.Stages[1:] {
		result = append(result, thingsstartbaseline.StageEvidence{
			ID:        remaining.ID,
			Result:    "NOT_RUN",
			Component: strings.Join(remaining.Components, ","),
			Reason:    fmt.Sprintf("not run after %s failure", first.ID),
		})
	}
	return result
}

func loadRuntimeEnvironment(filename string) (map[string]string, error) {
	if err := validateEnvFile(filename); err != nil {
		return nil, err
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("read runtime environment: %w", err)
	}
	defer file.Close()
	environment := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, rawValue, found := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !found || !validRuntimeEnvironmentName(name) {
			return nil, fmt.Errorf("parse runtime environment line %d: invalid KEY=value", lineNumber)
		}
		if name == "THINGS_BASELINE_ENV_FILE" {
			return nil, fmt.Errorf("parse runtime environment line %d: THINGS_BASELINE_ENV_FILE is reserved", lineNumber)
		}
		if _, duplicate := environment[name]; duplicate {
			return nil, fmt.Errorf("parse runtime environment line %d: duplicate %s", lineNumber, name)
		}
		value, err := parseRuntimeEnvironmentValue(rawValue)
		if err != nil {
			return nil, fmt.Errorf("parse runtime environment line %d for %s: %w", lineNumber, name, err)
		}
		environment[name] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read runtime environment: %w", err)
	}
	for _, name := range requiredRuntimeEnvironment {
		if strings.TrimSpace(environment[name]) == "" {
			return nil, fmt.Errorf("runtime environment %s is missing or empty", name)
		}
	}
	return environment, nil
}

func parseRuntimeEnvironmentValue(raw string) (string, error) {
	value := strings.TrimSpace(raw)
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
			return "", errors.New("invalid double-quoted value")
		}
		if strings.Contains(parsed, "$") {
			return "", errors.New("interpolation is not supported")
		}
		return parsed, nil
	}
	for index := 1; index < len(raw); index++ {
		if raw[index] == '#' && (raw[index-1] == ' ' || raw[index-1] == '\t') {
			value = strings.TrimSpace(raw[:index])
			break
		}
	}
	if strings.ContainsAny(value, "\"'") {
		return "", errors.New("unquoted value contains a quote")
	}
	if strings.Contains(value, "$") {
		return "", errors.New("interpolation is not supported")
	}
	return value, nil
}

func validRuntimeEnvironmentName(name string) bool {
	if name == "" {
		return false
	}
	for index := range name {
		character := name[index]
		if (character >= 'A' && character <= 'Z') || character == '_' || (index > 0 && character >= '0' && character <= '9') {
			continue
		}
		return false
	}
	return true
}

func runtimePort(environment map[string]string, name, fallback string) (string, error) {
	value := strings.TrimSpace(environment[name])
	if value == "" {
		value = fallback
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("runtime environment %s must be a TCP port", name)
	}
	return strconv.Itoa(port), nil
}

func runtimeCommandEnvironment(fileEnvironment map[string]string, envFile string) []string {
	result := make([]string, 0, len(os.Environ())+len(fileEnvironment)+1)
	for _, item := range os.Environ() {
		name, _, _ := strings.Cut(item, "=")
		if name == "THINGS_BASELINE_ENV_FILE" {
			continue
		}
		if _, overridden := fileEnvironment[name]; !overridden {
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
	return append(result, "THINGS_BASELINE_ENV_FILE="+envFile)
}

type verificationRuntime struct {
	compose *thingsstartbaseline.ComposeRuntime
	probe   *thingsstartbaseline.ProtocolProbe
}

func (runtime verificationRuntime) RunStage(ctx context.Context, stage thingsstartbaseline.StageID) error {
	if stage == thingsstartbaseline.StagePrecheck {
		return runtime.compose.RunStage(ctx, stage)
	}
	return runtime.probe.Probe(ctx, stage)
}

func verifyProductionStages(ctx context.Context, contract *thingsstartbaseline.Contract, runtime thingsstartbaseline.StageRuntime) ([]thingsstartbaseline.StageEvidence, error) {
	if ctx == nil {
		return nil, errors.New("verification context is nil")
	}
	if runtime == nil {
		return nil, errors.New("verification runtime is nil")
	}
	if err := thingsstartbaseline.ValidateContract(contract); err != nil {
		return nil, err
	}
	rootContext, rootCancel := context.WithTimeout(ctx, time.Duration(contract.Timeouts.TotalSeconds)*time.Second)
	defer rootCancel()
	stages := make([]thingsstartbaseline.StageEvidence, 0, len(contract.Stages))
	for index, contractStage := range contract.Stages {
		stage := thingsstartbaseline.StageEvidence{
			ID:        contractStage.ID,
			Result:    "RUNNING",
			StartedAt: time.Now().UTC(),
			Component: strings.Join(contractStage.Components, ","),
			LogRef:    "logs/" + strings.ToLower(string(contractStage.ID)) + ".log",
		}
		stageContext, stageCancel := context.WithTimeout(rootContext, time.Duration(contract.Timeouts.ProbeSeconds)*time.Second)
		err := runtime.RunStage(stageContext, contractStage.ID)
		contextErr := stageContext.Err()
		stageCancel()
		stage.FinishedAt = time.Now().UTC()
		stage.DurationMS = stage.FinishedAt.Sub(stage.StartedAt).Milliseconds()
		if err == nil && contextErr == nil && rootContext.Err() == nil {
			stage.Result = "PASS"
			stages = append(stages, stage)
			continue
		}
		if err == nil {
			err = contextErr
			if err == nil {
				err = rootContext.Err()
			}
		}
		stage.Result = "FAIL"
		if contextErr != nil || rootContext.Err() != nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			stage.Code = thingsstartbaseline.CodeTimeout
		} else {
			stage.Code = thingsstartbaseline.DiagnosticCode(err)
		}
		if stage.Code == "" {
			stage.Code = productionStageErrorCode(contractStage.ID)
		}
		stage.Reason = err.Error()
		stages = append(stages, stage)
		for _, remaining := range contract.Stages[index+1:] {
			stages = append(stages, thingsstartbaseline.StageEvidence{
				ID:        remaining.ID,
				Result:    "NOT_RUN",
				Component: strings.Join(remaining.Components, ","),
				Reason:    fmt.Sprintf("not run after %s failure", contractStage.ID),
			})
		}
		return stages, err
	}
	return stages, nil
}

func productionStageErrorCode(stage thingsstartbaseline.StageID) thingsstartbaseline.ErrorCode {
	switch stage {
	case thingsstartbaseline.StagePrecheck:
		return thingsstartbaseline.CodeConfigInvalid
	case thingsstartbaseline.StageDatabaseBootstrap:
		return thingsstartbaseline.CodeDatabaseBootstrapFailed
	case thingsstartbaseline.StageInfraReady:
		return thingsstartbaseline.CodeDependencyNotReady
	default:
		return thingsstartbaseline.CodeRoleNotReady
	}
}

func prepareProductionEvidence(options commandOptions, components productionRuntime) (productionEvidenceSnapshot, error) {
	if components.runner == nil || components.contract == nil || components.environment == nil {
		return productionEvidenceSnapshot{}, errors.New("production runtime is incomplete")
	}
	if err := verifyProductionInputIdentity(options, components.inputIdentity); err != nil {
		return productionEvidenceSnapshot{}, err
	}
	manifest, err := loadImageBuildManifest(filepath.Join(options.RuntimeRoot, "build-manifest.json"), components.contract)
	if err != nil {
		return productionEvidenceSnapshot{}, err
	}
	composeImages, err := loadComposeImageReferences(options.ComposeFile)
	if err != nil {
		return productionEvidenceSnapshot{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(components.contract.Timeouts.ProbeSeconds)*time.Second)
	defer cancel()
	versions := make(map[string]string, 12)
	versions["docker"], err = runProductionOutputCommand(ctx, components.runner, thingsstartbaseline.Command{
		Path: "docker", Args: []string{"--version"}, Dir: options.Repo, Env: runtimeCommandEnvironment(components.environment, options.EnvFile),
	}, "read Docker version")
	if err != nil {
		return productionEvidenceSnapshot{}, err
	}
	versions["compose"], err = runProductionOutputCommand(ctx, components.runner, thingsstartbaseline.Command{
		Path: "docker", Args: []string{"compose", "version"}, Dir: options.Repo, Env: runtimeCommandEnvironment(components.environment, options.EnvFile),
	}, "read Compose version")
	if err != nil {
		return productionEvidenceSnapshot{}, err
	}
	for _, service := range infrastructureServices {
		versions[service], err = inspectProductionImage(ctx, components.runner, options, components.environment, composeImages[service])
		if err != nil {
			return productionEvidenceSnapshot{}, fmt.Errorf("inspect %s image: %w", service, err)
		}
	}
	images := make(map[string]string, len(components.contract.Roles))
	for _, role := range components.contract.Roles {
		imageID, inspectErr := inspectProductionImage(ctx, components.runner, options, components.environment, role.Image)
		if inspectErr != nil {
			return productionEvidenceSnapshot{}, fmt.Errorf("inspect %s image: %w", role.ID, inspectErr)
		}
		versions[role.ID] = imageID
		images[role.ID] = imageID
	}
	composeCommand := productionComposeCommand(components.contract, options, components.environment)
	composeCommand.Args = append(composeCommand.Args, "config")
	composeResult, err := components.runner.Run(ctx, composeCommand)
	if err != nil || composeResult.ExitCode != 0 {
		if err == nil {
			err = fmt.Errorf("docker exited with status %d", composeResult.ExitCode)
		}
		return productionEvidenceSnapshot{}, fmt.Errorf("hash expanded Compose config: %w", err)
	}
	if !isHexString(composeResult.StdoutSHA256, sha256.Size*2) {
		return productionEvidenceSnapshot{}, errors.New("expanded Compose config SHA-256 must contain 64 hexadecimal characters")
	}
	if err := verifyProductionInputIdentity(options, components.inputIdentity); err != nil {
		return productionEvidenceSnapshot{}, err
	}
	runID := time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + manifest.GitRevision[:12]
	secrets := make([]string, 0, len(runtimeSecretEnvironmentNames))
	for _, name := range runtimeSecretEnvironmentNames {
		secrets = append(secrets, components.environment[name])
	}
	return productionEvidenceSnapshot{
		runID:               runID,
		manifest:            manifest,
		contractSHA256:      components.inputIdentity.contractSHA256,
		versions:            versions,
		composeConfigSHA256: composeResult.StdoutSHA256,
		images:              images,
		secrets:             secrets,
	}, nil
}

func publishPreparedProductionEvidence(
	options commandOptions,
	snapshot productionEvidenceSnapshot,
	stages []thingsstartbaseline.StageEvidence,
	runErr error,
) (string, error) {
	overallResult := "PASS"
	if runErr != nil {
		overallResult = "FAIL"
	}
	evidence := thingsstartbaseline.Evidence{
		SchemaVersion:  1,
		RunID:          snapshot.runID,
		GitRevision:    snapshot.manifest.GitRevision,
		ContractSHA256: snapshot.contractSHA256,
		OverallResult:  overallResult,
		Stages:         append([]thingsstartbaseline.StageEvidence(nil), stages...),
		Binaries:       append([]thingsstartbaseline.Artifact(nil), snapshot.manifest.Artifacts...),
		Images:         snapshot.images,
	}
	bundle := thingsstartbaseline.EvidenceBundle{
		Evidence:            evidence,
		Versions:            snapshot.versions,
		ComposeConfigSHA256: snapshot.composeConfigSHA256,
	}
	runDirectory := filepath.Join(options.RuntimeRoot, "runs", snapshot.runID)
	if err := thingsstartbaseline.WriteEvidenceBundleAtomically(runDirectory, bundle, snapshot.secrets); err != nil {
		return "", err
	}
	return snapshot.runID, nil
}

func loadComposeImageReferences(filename string) (map[string]string, error) {
	if err := requireRegularNonSymlink(filename, "compose file"); err != nil {
		return nil, err
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("read compose file: %w", err)
	}
	defer file.Close()
	var document struct {
		Services map[string]struct {
			Image string `yaml:"image"`
		} `yaml:"services"`
	}
	if err := yaml.NewDecoder(file).Decode(&document); err != nil {
		return nil, fmt.Errorf("decode compose file: %w", err)
	}
	images := make(map[string]string, len(infrastructureServices))
	for _, service := range infrastructureServices {
		image := strings.TrimSpace(document.Services[service].Image)
		if image == "" {
			return nil, fmt.Errorf("compose service %s image is missing", service)
		}
		images[service] = image
	}
	return images, nil
}

func inspectProductionImage(
	ctx context.Context,
	runner thingsstartbaseline.CommandRunner,
	options commandOptions,
	environment map[string]string,
	image string,
) (string, error) {
	if strings.TrimSpace(image) == "" {
		return "", errors.New("image reference is empty")
	}
	imageID, err := runProductionOutputCommand(ctx, runner, thingsstartbaseline.Command{
		Path: "docker",
		Args: []string{"image", "inspect", "--format", "{{.Id}}", image},
		Dir:  options.Repo,
		Env:  runtimeCommandEnvironment(environment, options.EnvFile),
	}, "inspect image ID")
	if err != nil {
		return "", err
	}
	if err := validateProductionImageID(imageID); err != nil {
		return "", err
	}
	return imageID, nil
}

func validateProductionImageID(imageID string) error {
	const prefix = "sha256:"
	if !strings.HasPrefix(imageID, prefix) || !isHexString(strings.TrimPrefix(imageID, prefix), sha256.Size*2) {
		return errors.New("image ID must be a sha256 digest")
	}
	return nil
}

func runProductionOutputCommand(
	ctx context.Context,
	runner thingsstartbaseline.CommandRunner,
	command thingsstartbaseline.Command,
	description string,
) (string, error) {
	result, err := runner.Run(ctx, command)
	if err != nil || result.ExitCode != 0 {
		if err == nil {
			err = fmt.Errorf("command exited with status %d", result.ExitCode)
		}
		return "", fmt.Errorf("%s: %w", description, err)
	}
	stdout, err := productionCommandStdout(result.Summary)
	if err != nil {
		return "", fmt.Errorf("%s: %w", description, err)
	}
	return stdout, nil
}

func productionCommandStdout(summary string) (string, error) {
	const prefix = "stdout:\n"
	const separator = "\nstderr:\n"
	if !strings.HasPrefix(summary, prefix) {
		return "", errors.New("command summary has no stdout section")
	}
	index := strings.LastIndex(summary[len(prefix):], separator)
	if index < 0 {
		return "", errors.New("command summary has no stderr section")
	}
	stdout := strings.TrimSpace(summary[len(prefix) : len(prefix)+index])
	if stdout == "" {
		return "", errors.New("command stdout is empty")
	}
	return stdout, nil
}

func (executor defaultExecutor) buildImages(options commandOptions) error {
	contract, err := thingsstartbaseline.LoadContract(options.ContractPath)
	if err != nil {
		return err
	}
	manifest, err := loadImageBuildManifest(filepath.Join(options.RuntimeRoot, "build-manifest.json"), contract)
	if err != nil {
		return err
	}
	runner := executor.runner
	if runner == nil {
		runner = thingsstartbaseline.NewCommandRunner()
	}
	type imageBuild struct {
		role       thingsstartbaseline.RoleContract
		dockerfile string
		context    string
		sha256     string
	}
	builds := make([]imageBuild, 0, len(contract.Roles))
	for index, role := range contract.Roles {
		dockerfile := filepath.Join(options.Repo, role.Dockerfile)
		if err := requireRegularNonSymlink(dockerfile, "Dockerfile"); err != nil {
			return fmt.Errorf("build %s image: %w", role.ID, err)
		}
		contextRoot := filepath.Join(options.RuntimeRoot, "stage", role.ID)
		if err := requireDirectoryNonSymlink(contextRoot, "staged image context"); err != nil {
			return fmt.Errorf("build %s image: %w", role.ID, err)
		}
		binary := filepath.Join(contextRoot, "bin", role.Binary)
		digest, size, err := hashRegularNonSymlink(binary, "staged binary")
		if err != nil {
			return fmt.Errorf("build %s image: %w", role.ID, err)
		}
		artifact := manifest.Artifacts[index]
		if digest != artifact.SHA256 || size != artifact.Size {
			return fmt.Errorf("build %s image: staged binary does not match build manifest", role.ID)
		}
		builds = append(builds, imageBuild{role: role, dockerfile: dockerfile, context: contextRoot, sha256: digest})
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(contract.Timeouts.TotalSeconds)*time.Second)
	defer cancel()
	for _, build := range builds {
		result, err := runner.Run(ctx, thingsstartbaseline.Command{
			Path: "docker",
			Args: []string{
				"build",
				"--build-arg", "VCS_REF=" + manifest.GitRevision,
				"--build-arg", "BINARY_SHA256=" + build.sha256,
				"-f", build.dockerfile,
				"-t", build.role.Image,
				build.context,
			},
			Dir: options.Repo,
			Env: os.Environ(),
		})
		if err != nil {
			return fmt.Errorf("build %s image: %w", build.role.ID, err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("build %s image: docker exited with status %d", build.role.ID, result.ExitCode)
		}
	}
	return nil
}

func loadImageBuildManifest(filename string, contract *thingsstartbaseline.Contract) (thingsstartbaseline.BuildManifest, error) {
	if err := requireRegularNonSymlink(filename, "build manifest"); err != nil {
		return thingsstartbaseline.BuildManifest{}, err
	}
	file, err := os.Open(filename)
	if err != nil {
		return thingsstartbaseline.BuildManifest{}, fmt.Errorf("open build manifest: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var manifest thingsstartbaseline.BuildManifest
	if err := decoder.Decode(&manifest); err != nil {
		return manifest, fmt.Errorf("decode build manifest: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return manifest, errors.New("decode build manifest: multiple JSON values")
	} else if err != io.EOF {
		return manifest, fmt.Errorf("decode build manifest trailing data: %w", err)
	}
	if err := validateImageBuildManifest(manifest, contract); err != nil {
		return manifest, fmt.Errorf("validate build manifest: %w", err)
	}
	return manifest, nil
}

func validateImageBuildManifest(manifest thingsstartbaseline.BuildManifest, contract *thingsstartbaseline.Contract) error {
	if manifest.SchemaVersion != 1 {
		return errors.New("schema version must be 1")
	}
	if !isHexString(manifest.GitRevision, 40) && !isHexString(manifest.GitRevision, 64) {
		return errors.New("Git revision must be a 40- or 64-character hexadecimal digest")
	}
	if len(manifest.Artifacts) != len(contract.Roles) {
		return fmt.Errorf("manifest must contain exactly %d artifacts", len(contract.Roles))
	}
	for index, role := range contract.Roles {
		artifact := manifest.Artifacts[index]
		if artifact.Role != role.ID || artifact.Binary != role.Binary {
			return fmt.Errorf("artifact %d must map role %s to binary %s", index, role.ID, role.Binary)
		}
		if !isHexString(artifact.SHA256, 64) {
			return fmt.Errorf("artifact %d SHA-256 must contain 64 hexadecimal characters", index)
		}
		if artifact.Size <= 0 || !artifact.StaticELF {
			return fmt.Errorf("artifact %d must describe a non-empty static ELF", index)
		}
	}
	return nil
}

func isHexString(value string, length int) bool {
	if len(value) != length {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func requireRegularNonSymlink(filename, description string) error {
	info, err := os.Lstat(filename)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", description, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a non-symlink regular file", description)
	}
	return nil
}

func requireDirectoryNonSymlink(filename, description string) error {
	info, err := os.Lstat(filename)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", description, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%s must be a non-symlink directory", description)
	}
	return nil
}

func hashRegularNonSymlink(filename, description string) (string, int64, error) {
	before, err := os.Lstat(filename)
	if err != nil {
		return "", 0, fmt.Errorf("inspect %s: %w", description, err)
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() {
		return "", 0, fmt.Errorf("%s must be a non-symlink regular file", description)
	}
	file, err := os.Open(filename)
	if err != nil {
		return "", 0, fmt.Errorf("open %s: %w", description, err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return "", 0, fmt.Errorf("inspect opened %s: %w", description, err)
	}
	if !os.SameFile(before, opened) || !opened.Mode().IsRegular() {
		return "", 0, fmt.Errorf("%s identity changed while opening", description)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", 0, fmt.Errorf("hash %s: %w", description, err)
	}
	return hex.EncodeToString(hash.Sum(nil)), opened.Size(), nil
}
