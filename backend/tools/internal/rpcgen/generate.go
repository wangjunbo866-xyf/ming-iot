package rpcgen

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"ming-iot/backend/tools/internal/clientcontract"
	"ming-iot/backend/tools/internal/saastxn"
)

// GenerateOptions fixes every repository and toolchain input used by a
// generation transaction. The attested goctl binary is never resolved via
// PATH and ModuleRoot is the only tree that may be atomically replaced.
type GenerateOptions struct {
	Manifest         *Manifest
	ModuleRoot       string
	BackendRoot      string
	ProtobufToolsDir string
	Goctl            GoctlAttestation
}

// RuntimeRoot 返回 manifest 所属运行时在 module root 下的绝对或相对路径。
func (m *Manifest) RuntimeRoot(moduleRoot string) string {
	return filepath.Join(moduleRoot, m.RuntimeDir)
}

// ClientRoot 返回 manifest 所属运行时的 client 根目录。
func (m *Manifest) ClientRoot(moduleRoot string) string {
	return filepath.Join(m.RuntimeRoot(moduleRoot), "client")
}

// PBImportPath 返回生成 client alias 使用的 protobuf Go import path。
func (m *Manifest) PBImportPath() string {
	return path.Join(m.ModulePath, m.RuntimeDir, m.GoPackage)
}

// Generate 通过 saastxn 原子事务重新生成 manifest 指定运行时的 PB / client / server / alias。
//
// 流程（全部在 saastxn 的 candidate 副本中进行）：
//  1. protoc 一次性编译全部 proto（shared + 领域 + 聚合）生成 PB
//  2. 按 manifest 顺序对每个领域 proto 调用固定 goctl（-m 模式）生成 client/server/logic
//  3. 生成 compatibility alias（补齐 client contract golden 缺失的别名）
//  4. gofmt 生成目录
//  5. 在 candidate 内 GOWORK=off 编译验证
//  6. saastxn 复核只有 allowlist 内文件变化，原子交换整棵 core
func Generate(ctx context.Context, opts GenerateOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	m := opts.Manifest
	moduleRoot := filepath.Clean(opts.ModuleRoot)
	if err := CheckAll(m, moduleRoot, opts.ProtobufToolsDir); err != nil {
		return fmt.Errorf("stage=manifest: %w", err)
	}
	if err := verifyGoctlAttestation(opts); err != nil {
		return fmt.Errorf("stage=goctl_attestation: %w", err)
	}

	var baseline *CompatibilitySnapshot
	var desiredClientAPI clientcontract.Document
	if m.Version == 2 {
		snapshot, err := CaptureCompatibility(moduleRoot, m, opts.ProtobufToolsDir)
		if err != nil {
			return fmt.Errorf("stage=baseline: %w", err)
		}
		baseline = &snapshot
		desiredClientAPI = snapshot.ClientAPI
	} else {
		live, err := clientcontract.ExtractWithSelector(m.ClientRoot(moduleRoot), m.PBSelector)
		if err != nil {
			return fmt.Errorf("stage=baseline: capture live client API: %w", err)
		}
		golden, err := loadGolden(filepath.Join(moduleRoot, m.ClientContractGolden))
		if err != nil {
			return fmt.Errorf("stage=baseline: %w", err)
		}
		if !clientAPIEqual(live, *golden) {
			return fmt.Errorf("stage=baseline: live client API differs from clientContractGolden")
		}
		desiredClientAPI = *golden
	}

	allowlist := buildAllowlist(m)
	txnDir := filepath.Join(filepath.Dir(moduleRoot), ".saastxn")
	res, err := saastxn.Commit(ctx, saastxn.Options{
		LiveDir:   moduleRoot,
		TxnDir:    txnDir,
		Allowlist: allowlist,
		MutateCandidate: func(candidate string) error {
			return mutateCandidate(ctx, opts, candidate, baseline, desiredClientAPI)
		},
	})
	if err != nil {
		if errors.Is(err, saastxn.ErrAllowlistViolation) {
			return fmt.Errorf("stage=allowlist: %w", err)
		}
		return fmt.Errorf("stage=transaction: %w", err)
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(os.Stderr, "rpcgen: warning: %s\n", w)
	}
	return nil
}

func verifyGoctlAttestation(opts GenerateOptions) error {
	backendRoot := filepath.Clean(opts.BackendRoot)
	if backendRoot == "." || backendRoot == "" {
		backendRoot = filepath.Dir(filepath.Clean(opts.ModuleRoot))
	}
	actual, err := AttestGoctl(backendRoot)
	if err != nil {
		return err
	}
	if filepath.Clean(opts.Goctl.BinaryPath) != filepath.Clean(actual.BinaryPath) ||
		opts.Goctl.BinarySHA256 != actual.BinarySHA256 ||
		opts.Goctl.Repository != actual.Repository || opts.Goctl.Commit != actual.Commit ||
		opts.Goctl.GoVersion != actual.GoVersion || opts.Goctl.GOOS != actual.GOOS ||
		opts.Goctl.GOARCH != actual.GOARCH || opts.Goctl.CGOEnabled != actual.CGOEnabled {
		return fmt.Errorf("provided goctl attestation does not match pinned backend/bin/goctl")
	}
	return nil
}

func clientAPIEqual(left, right clientcontract.Document) bool {
	return reflect.DeepEqual(normalizeClientAPI(left), normalizeClientAPI(right))
}

// buildAllowlist 返回允许变化的 repo-relative 路径（filepath.Slash 形式）。
func buildAllowlist(m *Manifest) []string {
	var allow []string
	// pb 目录下全部文件
	allow = append(allow, m.PbDir+"/")
	// client 文件
	for _, s := range m.Services {
		allow = append(allow,
			path.Join(m.RuntimeDir, "client", s.PackageDir, s.ClientFile),
			path.Join(m.RuntimeDir, "client", s.PackageDir, "compat_aliases.go"),
			path.Join(m.RuntimeDir, "internal/server", s.PackageDir, s.ServerFile),
		)
	}
	return allow
}

// mutateCandidate 在 candidate 副本上执行实际生成。
func mutateCandidate(ctx context.Context, opts GenerateOptions, candidate string, baseline *CompatibilitySnapshot, desiredClientAPI clientcontract.Document) error {
	m := opts.Manifest
	protobufToolsDir := opts.ProtobufToolsDir
	protoc := filepath.Join(protobufToolsDir, "bin", "protoc")
	include := filepath.Join(protobufToolsDir, "include")
	protoDir := filepath.Join(candidate, m.ProtoDir)
	runtimeRoot := m.RuntimeRoot(candidate)
	pbOut := filepath.Join(runtimeRoot, m.GoPackage)
	if err := resetGeneratedPB(candidate, pbOut); err != nil {
		return fmt.Errorf("stage=generation: reset PB output: %w", err)
	}

	// PATH 只 prepend protobuf-tools（让 protoc 找到 plugins）
	pathAdd := filepath.Join(protobufToolsDir, "bin")

	// 1. protoc 一次性生成全部 PB
	var protocInputs []string
	for _, s := range m.SupportingProtos {
		protocInputs = append(protocInputs, s)
	}
	for _, s := range m.Services {
		protocInputs = append(protocInputs, s.Proto)
	}
	protocInputs = append(protocInputs, m.Aggregate)
	protocArgs := []string{
		"-I", protoDir,
		"-I", include,
		"--go_out=" + runtimeRoot,
		"--go-grpc_out=" + runtimeRoot,
	}
	protocArgs = append(protocArgs, protocInputs...)

	protocCmd := exec.CommandContext(ctx, protoc, protocArgs...)
	protocCmd.Env = append(os.Environ(),
		"PATH="+pathAdd+":"+os.Getenv("PATH"),
		"GOWORK=off",
	)
	// CombinedOutput 会自己管理 Stdout/Stderr，不能再赋值 cmd.Stderr
	// （否则触发 "exec: Stderr already set"）。
	if out, err := protocCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("stage=generation: protoc: %w: %s", err, strings.TrimSpace(string(out)))
	}
	// 2. 每个领域 proto 调用 goctl（-m 模式）生成到独立 staging 目录，再只把
	//    manifest 指定的 client + server 文件复制进 candidate。
	//
	//    不能让 goctl 直接写到 candidate runtime：goctl -m 会重新生成
	//    internal/logic/<svc>/*.go，而 logic 是「首次生成后人工所有」，goctl 重新
	//    生成的 logic 文件名会与现有自定义命名冲突（例如 common 下同时存在
	//    qRCodeReadLogic.go 与 qrCodeReadLogic.go，触发 case-insensitive collision）。
	//    因此每个领域用一个带 go.mod 的独立 DOMAIN_PROJECT 做 zrpc root，只从它
	//    收集 client/<pkgDir>/<clientFile> 与 internal/server/<pkgDir>/<serverFile>。
	protoDirCandidate := filepath.Join(candidate, m.ProtoDir)
	for _, s := range m.Services {
		if err := goctlDomainStaged(ctx, m, s, protoDirCandidate, candidate, opts.Goctl.BinaryPath, protobufToolsDir, pathAdd); err != nil {
			return fmt.Errorf("stage=generation: goctl domain %s: %w", s.Proto, err)
		}
	}

	// 3. 生成 compatibility alias
	if err := generateAliasesForCandidate(m, candidate, desiredClientAPI); err != nil {
		return fmt.Errorf("stage=client_api: generate aliases: %w", err)
	}

	// 4. gofmt 生成目录
	if err := gofmtGenerated(ctx, pbOut, filepath.Join(runtimeRoot, "client"), filepath.Join(runtimeRoot, "internal/server")); err != nil {
		return fmt.Errorf("stage=generation: %w", err)
	}

	if err := RejectForbiddenImports(candidate, []string{legacyShareImportPath()}); err != nil {
		return fmt.Errorf("stage=forbidden_import: %w", err)
	}
	if baseline != nil {
		after, err := CaptureCompatibility(candidate, m, protobufToolsDir)
		if err != nil {
			return fmt.Errorf("stage=compatibility_capture: %w", err)
		}
		if err := VerifyProtectedDigests(baseline.ProtectedSHA256, after.ProtectedSHA256); err != nil {
			return fmt.Errorf("stage=protected_digest: %w", err)
		}
		if err := CompareCompatibility(*baseline, after); err != nil {
			return compatibilityStageError(err)
		}
	} else {
		after, err := clientcontract.ExtractWithSelector(m.ClientRoot(candidate), m.PBSelector)
		if err != nil {
			return fmt.Errorf("stage=client_api: capture candidate: %w", err)
		}
		if !clientAPIEqual(desiredClientAPI, after) {
			return fmt.Errorf("stage=client_api: candidate differs from live/golden client API")
		}
	}

	// 5. candidate 内编译验证
	//
	//    必须用一次性临时 GOCACHE：saastxn 每次在不同时间戳副本里编译，默认共享
	//    cache 会残留指向上一次 candidate 绝对路径的产物；candidate 失败被清理后，
	//    缓存的 vet/compile 子工具 chdir 旧路径会以 "no such file or directory"
	//    失败，表现为大量 "[build failed]"。
	cacheDir, err := os.MkdirTemp("", "rpcgen-gocache-*")
	if err != nil {
		return fmt.Errorf("create temp gocache: %w", err)
	}
	defer os.RemoveAll(cacheDir)
	modfile, cleanupModfile, err := prepareCandidateModfile(ctx, candidate, opts.BackendRoot)
	if err != nil {
		return fmt.Errorf("stage=candidate_compile: prepare modfile: %w", err)
	}
	defer cleanupModfile()
	testCmd := exec.CommandContext(ctx, "go", "test", "-modfile="+modfile, "-run", "^$", "./...")
	testCmd.Dir = candidate
	testCmd.Env = append(os.Environ(), "GOWORK=off", "GOCACHE="+cacheDir)
	if out, err := testCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("stage=candidate_compile: %w: %s", err, strings.TrimSpace(string(out)))
	}

	return nil
}

func resetGeneratedPB(candidate, pbOut string) error {
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return fmt.Errorf("resolve candidate: %w", err)
	}
	pbAbs, err := filepath.Abs(pbOut)
	if err != nil {
		return fmt.Errorf("resolve PB output: %w", err)
	}
	if !pathWithin(candidateAbs, pbAbs) || pbAbs == candidateAbs {
		return fmt.Errorf("PB output %s is outside candidate %s", pbAbs, candidateAbs)
	}
	if err := os.RemoveAll(pbAbs); err != nil {
		return err
	}
	return os.MkdirAll(pbAbs, 0755)
}

func prepareCandidateModfile(ctx context.Context, candidate, backendRoot string) (string, func(), error) {
	backendAbs, err := filepath.Abs(backendRoot)
	if err != nil {
		return "", func() {}, fmt.Errorf("resolve backend root: %w", err)
	}
	goModPath := filepath.Join(candidate, "go.mod")
	goMod, err := os.ReadFile(goModPath)
	if err != nil {
		return "", func() {}, fmt.Errorf("read candidate go.mod: %w", err)
	}
	temp, err := os.CreateTemp(candidate, ".rpcgen-candidate-*.mod")
	if err != nil {
		return "", func() {}, fmt.Errorf("create candidate modfile: %w", err)
	}
	modfile := temp.Name()
	cleanup := func() {
		_ = os.Remove(modfile)
		_ = os.Remove(strings.TrimSuffix(modfile, ".mod") + ".sum")
	}
	if _, err := temp.Write(goMod); err != nil {
		temp.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("write candidate modfile: %w", err)
	}
	if err := temp.Close(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("close candidate modfile: %w", err)
	}
	if goSum, err := os.ReadFile(filepath.Join(candidate, "go.sum")); err == nil {
		if err := os.WriteFile(strings.TrimSuffix(modfile, ".mod")+".sum", goSum, 0644); err != nil {
			cleanup()
			return "", func() {}, fmt.Errorf("write candidate sum: %w", err)
		}
	} else if !os.IsNotExist(err) {
		cleanup()
		return "", func() {}, fmt.Errorf("read candidate go.sum: %w", err)
	}

	currentModule := modulePathFromGoMod(goMod)
	entries, err := os.ReadDir(backendAbs)
	if err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("read backend root: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		moduleDir := filepath.Join(backendAbs, entry.Name())
		data, err := os.ReadFile(filepath.Join(moduleDir, "go.mod"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			cleanup()
			return "", func() {}, fmt.Errorf("read local module %s: %w", entry.Name(), err)
		}
		modulePath := modulePathFromGoMod(data)
		if modulePath == "" || modulePath == currentModule || !strings.HasPrefix(modulePath, "gitee.com/ming-iot/") {
			continue
		}
		for _, argument := range []string{"-dropreplace=" + modulePath, "-replace=" + modulePath + "=" + moduleDir} {
			cmd := exec.CommandContext(ctx, "go", "mod", "edit", "-modfile="+modfile, argument)
			cmd.Dir = candidate
			cmd.Env = append(os.Environ(), "GOWORK=off")
			if out, err := cmd.CombinedOutput(); err != nil {
				cleanup()
				return "", func() {}, fmt.Errorf("go mod edit %s: %w: %s", modulePath, err, strings.TrimSpace(string(out)))
			}
		}
	}
	return modfile, cleanup, nil
}

func modulePathFromGoMod(data []byte) string {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1]
		}
	}
	return ""
}

func gofmtGenerated(ctx context.Context, roots ...string) error {
	var files []string
	for _, root := range roots {
		if err := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
				files = append(files, filePath)
			}
			return nil
		}); err != nil {
			return fmt.Errorf("gofmt walk %s: %w", root, err)
		}
	}
	if len(files) == 0 {
		return fmt.Errorf("gofmt found no generated Go files")
	}
	sort.Strings(files)
	cmd := exec.CommandContext(ctx, "gofmt", append([]string{"-w"}, files...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gofmt: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func compatibilityStageError(err error) error {
	message := err.Error()
	stage := "compatibility"
	switch {
	case strings.Contains(message, "descriptor"):
		stage = "descriptor"
	case strings.Contains(message, "client API"), strings.Contains(message, "direct method"), strings.Contains(message, "direct facade"):
		stage = "client_api"
	case strings.Contains(message, "registration"):
		stage = "registry"
	case strings.Contains(message, "protected"):
		stage = "protected_digest"
	}
	return fmt.Errorf("stage=%s: %w", stage, err)
}

// goctlDomainStaged 对单个领域 proto 运行 goctl rpc protoc -m，输出到独立
// staging（DOMAIN_PROJECT），然后只把 manifest 指定的 client + server 文件
// 复制进 candidate。logic/svc/config/main 等生成物留在 staging，不进入 candidate。
//
// DOMAIN_PROJECT 是一个带 manifest module path 的 go.mod 的临时目录，zrpc root
// 固定为 DOMAIN_PROJECT/<runtimeDir>，避免「第一次生成
// 共享骨架」问题，也让 goctl 的 UpdateGoWorkIfExist 找不到真实 go.work。
func goctlDomainStaged(ctx context.Context, m *Manifest, s ServiceEntry, protoDirCandidate, candidate, goctlBinary, protobufToolsDir, pathAdd string) error {
	protoPath := filepath.Join(protoDirCandidate, s.Proto)
	include := filepath.Join(protobufToolsDir, "include")

	domainProject, err := os.MkdirTemp("", "rpcgen-domain-*")
	if err != nil {
		return fmt.Errorf("create domain project: %w", err)
	}
	defer os.RemoveAll(domainProject)

	// 固定 go.mod，让 goctl --module 解析到的 import 路径与真实 module 一致。
	if err := os.WriteFile(filepath.Join(domainProject, "go.mod"), []byte(stagingGoMod(m)), 0644); err != nil {
		return fmt.Errorf("write domain go.mod: %w", err)
	}
	zrpcOut := filepath.Join(domainProject, m.RuntimeDir)
	if err := os.MkdirAll(zrpcOut, 0755); err != nil {
		return err
	}

	args := []string{
		"rpc", "protoc", protoPath,
		"--go_out=" + zrpcOut,
		"--go-grpc_out=" + zrpcOut,
		"--zrpc_out=" + zrpcOut,
		"--proto_path=" + protoDirCandidate,
		"--proto_path=" + include,
		"--style=goZero",
		"--module=" + m.ModulePath,
		"-m",
	}
	cmd := exec.CommandContext(ctx, goctlBinary, args...)
	cmd.Env = append(os.Environ(),
		"GOWORK=off",
		"PATH="+pathAdd+":"+os.Getenv("PATH"),
	)
	// CombinedOutput 自管 Stdout/Stderr，禁止再设 cmd.Stderr。
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("goctl rpc protoc %s: %w\n%s", s.Proto, err, out)
	}

	// 只收集 manifest 指定的 client + server 文件。
	generatedRuntime := filepath.Join(domainProject, m.RuntimeDir)
	clientSrc := filepath.Join(generatedRuntime, "client", s.PackageDir, s.ClientFile)
	serverSrc := filepath.Join(generatedRuntime, "internal/server", s.PackageDir, s.ServerFile)
	clientDst := filepath.Join(m.ClientRoot(candidate), s.PackageDir, s.ClientFile)
	serverDst := filepath.Join(m.RuntimeRoot(candidate), "internal/server", s.PackageDir, s.ServerFile)
	if err := copyFile(clientSrc, clientDst); err != nil {
		return fmt.Errorf("copy client %s/%s: %w", s.PackageDir, s.ClientFile, err)
	}
	if err := copyFile(serverSrc, serverDst); err != nil {
		return fmt.Errorf("copy server %s/%s: %w", s.PackageDir, s.ServerFile, err)
	}
	return nil
}

// stagingGoMod 返回每个 DOMAIN_PROJECT 使用的最小 go.mod。
func stagingGoMod(m *Manifest) string {
	return fmt.Sprintf("module %s\n\ngo 1.24\n", m.ModulePath)
}

// copyFile 以 mode 0644 把 src 复制到 dst（dst 的父目录自动创建）。
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// generateAliasesForCandidate 在 candidate 上为每个 client package 生成 compat_aliases.go。
// alias 补齐 goctl 生成结果缺少但 client contract golden 要求的 message 别名。
func generateAliasesForCandidate(m *Manifest, candidate string, desired clientcontract.Document) error {
	for _, service := range m.Services {
		aliasPath := filepath.Join(m.ClientRoot(candidate), service.PackageDir, "compat_aliases.go")
		if err := os.Remove(aliasPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale alias %s: %w", service.PackageDir, err)
		}
	}
	aliases, err := computeMissingAliases(m, candidate, desired)
	if err != nil {
		return fmt.Errorf("compute aliases: %w", err)
	}
	for pkgDir, names := range aliases {
		if len(names) == 0 {
			continue
		}
		dir := filepath.Join(m.ClientRoot(candidate), pkgDir)
		if err := writeAliasFile(dir, pkgDir, m.PBImportPath(), m.PBSelector, names); err != nil {
			return fmt.Errorf("write alias %s: %w", pkgDir, err)
		}
	}
	return nil
}

// writeAliasFile 写入 compat_aliases.go（确定性，按 name 排序）。
//
// importPath 和 selector 由 manifest 明确派生，避免把 SaaS/sys 固化到生成器。
func writeAliasFile(dir, pkgDir, importPath, selector string, names []string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	names = append([]string(nil), names...)
	sort.Strings(names)
	var sb strings.Builder
	sb.WriteString("// Code generated by rpcgen. DO NOT EDIT.\n\n")
	sb.WriteString(fmt.Sprintf("package %s\n\n", pkgDir))
	sb.WriteString(fmt.Sprintf("import %s %q\n\n", selector, importPath))
	sb.WriteString("type (\n")
	for _, n := range names {
		sb.WriteString(fmt.Sprintf("\t%s = %s.%s\n", n, selector, n))
	}
	sb.WriteString(")\n")
	return os.WriteFile(filepath.Join(dir, "compat_aliases.go"), []byte(sb.String()), 0644)
}
