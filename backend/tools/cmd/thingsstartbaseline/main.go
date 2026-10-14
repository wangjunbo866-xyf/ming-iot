package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ming-iot/backend/tools/internal/thingsstartbaseline"
)

type commandOptions struct {
	RequestedCommand string
	ContractPath     string
	Repo             string
	BinDir           string
	Output           string
	EnvFile          string
	RuntimeRoot      string
	ComposeFile      string
}

func (options commandOptions) pathMap() map[string]string {
	return map[string]string{
		"contract":     options.ContractPath,
		"repo":         options.Repo,
		"bin-dir":      options.BinDir,
		"output":       options.Output,
		"env-file":     options.EnvFile,
		"runtime-root": options.RuntimeRoot,
		"compose-file": options.ComposeFile,
	}
}

type commandExecutor interface {
	ValidateEnvironment(command string, options commandOptions) error
	Execute(command string, options commandOptions) error
}

type defaultExecutor struct {
	runner        thingsstartbaseline.CommandRunner
	output        io.Writer
	writeManifest func(string, string, string, string) error
	stage         func(thingsstartbaseline.StageRequest) (thingsstartbaseline.StageResult, error)
	up            func(commandOptions) error
	verify        func(commandOptions) error
}

var commandRequirements = map[string][]string{
	"contract": {"contract", "env-file"},
	"manifest": {"contract", "repo", "bin-dir", "output"},
	"stage":    {"contract", "repo", "env-file", "runtime-root"},
	"image":    {"contract", "repo", "runtime-root"},
	"up":       {"contract", "repo", "env-file", "compose-file", "runtime-root"},
	"verify":   {"contract", "repo", "env-file", "compose-file", "runtime-root"},
	"logs":     {"contract", "repo", "env-file", "compose-file", "runtime-root"},
}

var commandWorkflows = map[string][]string{
	"contract": {"contract"},
	"manifest": {"manifest"},
	"stage":    {"stage"},
	"image":    {"image"},
	"up":       {"contract", "manifest", "stage", "database", "infra", "roles", "verify"},
	"verify":   {"contract", "verify"},
	"logs":     {"logs"},
}

func main() {
	os.Exit(run(os.Args[1:], defaultExecutor{output: os.Stdout}, os.Stdout, os.Stderr))
}

func run(args []string, executor commandExecutor, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: thingsstartbaseline <contract|manifest|stage|image|up|verify|logs> [parameters]")
		return 2
	}
	command := args[0]
	required, known := commandRequirements[command]
	if !known {
		fmt.Fprintln(stderr, "usage: thingsstartbaseline <contract|manifest|stage|image|up|verify|logs> [parameters]")
		return 2
	}
	options, err := parseOptions(command, required, args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "thingsstartbaseline %s: %v\n", command, err)
		return 2
	}
	options.RequestedCommand = command
	if options.EnvFile != "" {
		if err := validateEnvFile(options.EnvFile); err != nil {
			fmt.Fprintf(stderr, "thingsstartbaseline %s: %v\n", command, err)
			return 2
		}
	}
	for _, step := range commandWorkflows[command] {
		if step == "contract" {
			if _, err := thingsstartbaseline.LoadContract(options.ContractPath); err != nil {
				fmt.Fprintf(stderr, "thingsstartbaseline %s failed: %s: %v\n", command, thingsstartbaseline.CodeContractMismatch, err)
				return 1
			}
			if err := executor.ValidateEnvironment(step, options); err != nil {
				fmt.Fprintf(stderr, "thingsstartbaseline %s failed: %s: %v\n", command, thingsstartbaseline.CodeContractMismatch, err)
				return 1
			}
			continue
		}
		if err := executor.Execute(step, options); err != nil {
			fmt.Fprintf(stderr, "thingsstartbaseline %s failed: %v\n", command, err)
			return 1
		}
	}
	return 0
}

func parseOptions(command string, required, args []string) (commandOptions, error) {
	allowed := make(map[string]struct{}, len(required))
	for _, name := range required {
		allowed[name] = struct{}{}
	}
	values := make(map[string]string, len(required))
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if !strings.HasPrefix(argument, "--") || argument == "--" {
			return commandOptions{}, fmt.Errorf("unknown parameter %q", argument)
		}
		name, value, hasValue := strings.Cut(strings.TrimPrefix(argument, "--"), "=")
		if _, ok := allowed[name]; !ok {
			return commandOptions{}, fmt.Errorf("unknown parameter --%s", name)
		}
		if _, duplicate := values[name]; duplicate {
			return commandOptions{}, fmt.Errorf("duplicate parameter --%s", name)
		}
		if !hasValue {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return commandOptions{}, fmt.Errorf("missing value for --%s", name)
			}
			index++
			value = args[index]
		}
		if value == "" {
			return commandOptions{}, fmt.Errorf("missing value for --%s", name)
		}
		absolute, err := filepath.Abs(value)
		if err != nil {
			return commandOptions{}, fmt.Errorf("resolve --%s: %w", name, err)
		}
		values[name] = absolute
	}
	for _, name := range required {
		if values[name] == "" {
			return commandOptions{}, fmt.Errorf("missing required parameter --%s", name)
		}
	}
	return commandOptions{
		ContractPath: values["contract"],
		Repo:         values["repo"],
		BinDir:       values["bin-dir"],
		Output:       values["output"],
		EnvFile:      values["env-file"],
		RuntimeRoot:  values["runtime-root"],
		ComposeFile:  values["compose-file"],
	}, nil
}

func validateEnvFile(filename string) error {
	info, err := os.Lstat(filename)
	if err != nil {
		return fmt.Errorf("--env-file %q: %w", filename, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("--env-file %q must not be a symbolic link", filename)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("--env-file %q must be a regular file", filename)
	}
	if info.Mode().Perm() != 0o600 {
		return fmt.Errorf("--env-file %q must have permissions 0600", filename)
	}
	return nil
}
