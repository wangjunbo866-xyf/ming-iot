// Command rpcgen 编排 manifest 驱动的多 proto 受控生成。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"ming-iot/backend/tools/internal/rpcgen"
)

type cliOptions struct {
	command       string
	manifest      string
	moduleRoot    string
	saas          string
	goctl         string
	goctlSet      bool
	protobufTools string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		usage(stdout)
		return 0
	}
	if args[0] != "check" && args[0] != "generate" {
		fmt.Fprintf(stderr, "rpcgen: unknown subcommand %q\n", args[0])
		usage(stderr)
		return 2
	}

	opts, err := parseCLIOptions(args[0], args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			usage(stdout)
			return 0
		}
		fmt.Fprintf(stderr, "rpcgen: %v\n", err)
		return 2
	}
	if err := validateCLIShape(opts); err != nil {
		fmt.Fprintf(stderr, "rpcgen: %v\n", err)
		return 2
	}
	m, err := rpcgen.LoadManifest(opts.manifest)
	if err != nil {
		fmt.Fprintf(stderr, "rpcgen: %v\n", err)
		return 1
	}
	pinnedGoctl, err := filepath.Abs(filepath.Join(opts.backendRoot(), "bin", "goctl"))
	if err != nil {
		fmt.Fprintf(stderr, "rpcgen: normalize pinned goctl path: %v\n", err)
		return 2
	}
	if err := validateCLIOptions(m, opts, pinnedGoctl); err != nil {
		fmt.Fprintf(stderr, "rpcgen: %v\n", err)
		return 2
	}

	moduleRoot := opts.moduleRootPath()
	protobufTools := opts.protobufToolsPath()
	if opts.command == "check" {
		if err := rpcgen.CheckAll(m, moduleRoot, protobufTools); err != nil {
			fmt.Fprintf(stderr, "rpcgen: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "rpcgen check: OK — %d services, package %s, runtime %s\n", m.ServiceCount(), m.Package, m.Runtime)
		return 0
	}

	attestation, err := rpcgen.AttestGoctl(opts.backendRoot())
	if err != nil {
		fmt.Fprintf(stderr, "rpcgen: %v\n", err)
		return 1
	}
	if err := validateCLIOptions(m, opts, attestation.BinaryPath); err != nil {
		fmt.Fprintf(stderr, "rpcgen: %v\n", err)
		return 2
	}
	if err := rpcgen.Generate(context.Background(), rpcgen.GenerateOptions{
		Manifest:         m,
		ModuleRoot:       moduleRoot,
		BackendRoot:      opts.backendRoot(),
		ProtobufToolsDir: protobufTools,
		Goctl:            attestation,
	}); err != nil {
		fmt.Fprintf(stderr, "rpcgen: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "rpcgen generate: committed — goctl %s (%s)\n", attestation.BinarySHA256, attestation.VersionText)
	return 0
}

func parseCLIOptions(command string, args []string) (cliOptions, error) {
	opts := cliOptions{command: command}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.manifest, "manifest", "", "rpcgen.yaml 路径")
	fs.StringVar(&opts.moduleRoot, "module-root", "", "模块根目录，例如 backend/things")
	fs.StringVar(&opts.saas, "saas", "", "兼容参数：backend/saas 根目录")
	fs.StringVar(&opts.goctl, "goctl", "", "兼容参数：必须等于 backend/bin/goctl")
	fs.StringVar(&opts.protobufTools, "protobuf-tools", "", "protobuf-tools 目录")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if fs.NArg() != 0 {
		return opts, fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "goctl" {
			opts.goctlSet = true
		}
	})
	return opts, nil
}

func validateCLIOptions(m *rpcgen.Manifest, opts cliOptions, pinnedGoctl string) error {
	if err := validateCLIShape(opts); err != nil {
		return err
	}
	if opts.command == "check" {
		return nil
	}
	if m == nil {
		return fmt.Errorf("generate: manifest is required")
	}
	if m.Version == 2 && opts.goctlSet {
		return fmt.Errorf("generate: manifest v2 forbids --goctl override; use pinned backend/bin/goctl")
	}
	if m.Version == 1 && opts.goctlSet && pinnedGoctl != "" {
		provided, err := filepath.Abs(opts.goctl)
		if err != nil {
			return fmt.Errorf("generate: normalize --goctl: %w", err)
		}
		pinned, err := filepath.Abs(pinnedGoctl)
		if err != nil {
			return fmt.Errorf("generate: normalize pinned goctl: %w", err)
		}
		if filepath.Clean(provided) != filepath.Clean(pinned) {
			return fmt.Errorf("generate: --goctl must equal pinned goctl %s", pinned)
		}
	}
	return nil
}

func validateCLIShape(opts cliOptions) error {
	if opts.manifest == "" {
		return fmt.Errorf("%s: --manifest is required", opts.command)
	}
	if (opts.moduleRoot == "") == (opts.saas == "") {
		return fmt.Errorf("%s: exactly one of --module-root and --saas is required", opts.command)
	}
	if opts.command == "check" {
		if opts.goctlSet {
			return fmt.Errorf("check: --goctl is only valid for generate")
		}
		return nil
	}
	if opts.command != "generate" {
		return fmt.Errorf("unknown command %q", opts.command)
	}
	return nil
}

func (o cliOptions) moduleRootPath() string {
	if o.moduleRoot != "" {
		return filepath.Clean(o.moduleRoot)
	}
	return filepath.Clean(o.saas)
}

func (o cliOptions) backendRoot() string {
	return filepath.Dir(o.moduleRootPath())
}

func (o cliOptions) protobufToolsPath() string {
	if o.protobufTools != "" {
		return filepath.Clean(o.protobufTools)
	}
	return filepath.Join(o.backendRoot(), "bin", "protobuf-tools")
}

func usage(w io.Writer) {
	fmt.Fprint(w, "rpcgen — manifest 驱动的多 proto 受控生成器\n\n"+
		"用法:\n"+
		"  rpcgen check    --manifest FILE --module-root backend/things [--protobuf-tools DIR]\n"+
		"  rpcgen generate --manifest FILE --module-root backend/things [--protobuf-tools DIR]\n"+
		"  rpcgen check    --manifest FILE --saas backend/saas [--protobuf-tools DIR]\n"+
		"  rpcgen generate --manifest FILE --saas backend/saas [--goctl backend/bin/goctl] [--protobuf-tools DIR]\n\n"+
		"generate 始终校验并执行 backend/bin/goctl；manifest v2 禁止 --goctl 覆盖。\n")
}
