package apicontract

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ming-iot/backend/tools/internal/saastxn"
)

// GenerateAPIOptions configures GenerateAPI.
type GenerateAPIOptions struct {
	BackendRoot  string // backend/saas、backend/things 或仓库 backend 目录
	GoctlBinary  string // 固定 goctl 二进制路径
	Metadata     string // full（默认）或 none
	UpdateGolden bool   // 在同一原子事务内更新六类 golden
}

// stagingGoMod 从当前后端模块读取 module/go 指令，为 goctl staging 生成最小
// go.mod。goctl 只需要它推导 routes.go 的 import 路径；补丁版本被规范为
// major.minor，兼容 go 命令接受的 go directive。
func stagingGoMod(coreRoot string) (string, error) {
	data, err := os.ReadFile(filepath.Join(coreRoot, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("read backend go.mod: %w", err)
	}
	var modulePath, goVersion string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		switch fields[0] {
		case "module":
			modulePath = fields[1]
		case "go":
			parts := strings.Split(fields[1], ".")
			if len(parts) >= 2 {
				goVersion = strings.Join(parts[:2], ".")
			} else {
				goVersion = fields[1]
			}
		}
	}
	if modulePath == "" || goVersion == "" {
		return "", fmt.Errorf("backend go.mod must declare module and go version")
	}
	return fmt.Sprintf("module %s\n\ngo %s\n", modulePath, goVersion), nil
}

func generateAllowlist(coreRoot string, updateGolden bool) []string {
	var allow []string
	for name := range Backends(coreRoot) {
		allow = append(allow,
			filepath.ToSlash(filepath.Join("service", name, "internal/types/types.go")),
			filepath.ToSlash(filepath.Join("service", name, "internal/handler/routes.go")),
		)
		if updateGolden {
			allow = append(allow, filepath.ToSlash(filepath.Join("service", name, "http/testdata")))
			for _, filename := range GoldenFiles {
				allow = append(allow, filepath.ToSlash(filepath.Join("service", name, "http/testdata", filename)))
			}
		}
	}
	sort.Strings(allow)
	return allow
}

// GenerateAPI 用 saastxn 原子事务重新生成 apisvr 与 datasvr 的
// internal/types/types.go 与 internal/handler/routes.go（共四个文件）。
//
// 流程（全部在 saastxn 的 candidate 副本中进行）：
//  1. 在 candidate 上对 apisvr/datasvr 各运行一次 goctl api go；
//  2. 运行 apicheck verify（四类契约 + 元数据）；
//  3. 只把两套 types.go 与 routes.go（共 4 个文件）保留进 candidate；
//     goctl 新增/修改的 handler/logic/main/config/svc/middleware 一律丢弃；
//  4. candidate 内 GOWORK=off go test -run '^$' ./... 编译验证；
//  5. saastxn 复核只有上述四个文件变化，原子交换整棵 core。
//
// 所有 goctl/Go 子进程显式 GOWORK=off；不传 --ws，candidate 会保留 live core
// 中历史未注册的 datasvr wsRoutes.go。
func GenerateAPI(ctx context.Context, opts GenerateAPIOptions) error {
	coreRoot := ResolveCoreRoot(opts.BackendRoot)
	metadata := opts.Metadata
	if metadata == "" {
		metadata = "full"
	}
	if metadata != "full" && metadata != "none" {
		return fmt.Errorf("metadata must be full|none, got %q", metadata)
	}
	// goctl 必须解析为绝对路径：runCmd 把 cmd.Dir 设为 candidate，相对路径会
	// 相对 candidate 解析而失效。
	goctl := opts.GoctlBinary
	if abs, err := filepath.Abs(goctl); err == nil {
		goctl = abs
	}
	if _, err := os.Stat(goctl); err != nil {
		return fmt.Errorf("goctl binary %s: %w", goctl, err)
	}

	allow := generateAllowlist(coreRoot, opts.UpdateGolden)
	if len(allow) == 0 {
		return fmt.Errorf("no HTTP API service found under %s", coreRoot)
	}
	txnDir := filepath.Join(filepath.Dir(coreRoot), ".saastxn")

	res, err := saastxn.Commit(ctx, saastxn.Options{
		LiveDir:   coreRoot,
		TxnDir:    txnDir,
		Allowlist: allow,
		MutateCandidate: func(candidate string) error {
			return mutateAPICandidate(ctx, candidate, coreRoot, goctl, metadata, opts.UpdateGolden)
		},
	})
	if err != nil {
		return fmt.Errorf("saastxn: %w", err)
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(os.Stderr, "apicheck generate: warning: %s\n", w)
	}
	return nil
}

// mutateAPICandidate 在 candidate 副本上生成并同步 API 代码。
// coreRoot 仅用于解析 service 入口（candidate 是 coreRoot 的完整副本，路径结构相同）。
func mutateAPICandidate(ctx context.Context, candidate, coreRoot, goctl, metadata string, updateGolden bool) error {
	moduleFile, err := stagingGoMod(coreRoot)
	if err != nil {
		return err
	}
	backends := Backends(candidate)
	serviceNames := make([]string, 0, len(backends))
	for name := range backends {
		serviceNames = append(serviceNames, name)
	}
	sort.Strings(serviceNames)

	// 1. 每个 service 在独立 staging 目录运行 goctl api go，再只把 types.go +
	//    routes.go 复制进 candidate。这样 handler/logic/main/config/svc 不会进入
	//    candidate（goctl 会重新生成它们，与人工所有权冲突）。
	//
	//    staging 必须带 module gitee.com/ming-iot/saas 的 go.mod，且 --dir 指向
	//    <staging>/service/<svc>：goctl 依据相对 module 根的目录推算 routes.go 的
	//    handler 子包 import 路径，只有这样才能得到
	//    gitee.com/ming-iot/saas/service/<svc>/internal/handler/...。
	for _, serviceName := range serviceNames {
		svc := backends[serviceName]
		// 把 candidate 的整个 http/ 目录复制到 staging，再让 goctl 读 staging 里的
		// .api 副本。原因：goctl api go 会原位重排（对齐 @doc 字段、补空格）源
		// .api 文件；若直接喂 candidate 的 .api，candidate 里该文件就被改写，触发
		// allowlist 违例（.api 不在允许变更的 types.go/routes.go 之内）。复制一份
		// 到独立 staging，goctl 怎么改都只动 staging 副本。
		httpSrc := svc.ServiceDir
		staging, err := os.MkdirTemp("", "apicheck-gen-"+svc.Name+"-*")
		if err != nil {
			return fmt.Errorf("create staging %s: %w", svc.Name, err)
		}
		if err := os.WriteFile(filepath.Join(staging, "go.mod"), []byte(moduleFile), 0o644); err != nil {
			os.RemoveAll(staging)
			return fmt.Errorf("write staging go.mod: %w", err)
		}
		svcDir := filepath.Join(staging, "service", svc.Name)
		if err := os.MkdirAll(svcDir, 0o755); err != nil {
			os.RemoveAll(staging)
			return err
		}
		// 复制 http/ 整树进 staging（.api 入口与其 import 的同级 .api 一并带上）。
		if err := osCopyDir(httpSrc, filepath.Join(svcDir, "http")); err != nil {
			os.RemoveAll(staging)
			return fmt.Errorf("copy http %s: %w", svc.Name, err)
		}
		if metadata == "none" {
			if err := normalizeStagingMetadata(filepath.Join(svcDir, "http")); err != nil {
				os.RemoveAll(staging)
				return fmt.Errorf("normalize staging metadata %s: %w", svc.Name, err)
			}
		}
		entry := filepath.Join(svcDir, "http", filepath.Base(svc.EntryAPI))
		args := []string{"api", "go",
			"--api", entry,
			"--dir", svcDir,
			"--style", "goZero",
		}
		out, runErr := runCmd(goctl, args, staging)
		if runErr != nil {
			os.RemoveAll(staging)
			return fmt.Errorf("goctl api go %s: %w\n%s", svc.Name, runErr, out)
		}
		// 只复制 types.go + routes.go 进 candidate。
		typesSrc := filepath.Join(svcDir, "internal/types/types.go")
		routesSrc := filepath.Join(svcDir, "internal/handler/routes.go")
		typesDst := svc.TypesGo
		routesDst := svc.RoutesGo
		if err := copyFileFiltered(typesSrc, typesDst); err != nil {
			os.RemoveAll(staging)
			return fmt.Errorf("copy types %s: %w", svc.Name, err)
		}
		if err := copyFileFiltered(routesSrc, routesDst); err != nil {
			os.RemoveAll(staging)
			return fmt.Errorf("copy routes %s: %w", svc.Name, err)
		}
		os.RemoveAll(staging)
	}

	// 2. 在 candidate 上跑完整 apicheck verify。
	snaps, err := LoadBackend(candidate)
	if err != nil {
		return fmt.Errorf("verify load: %w", err)
	}
	for svcName, snap := range snaps {
		dir := filepath.Join(candidate, "service", svcName, "http", "testdata")
		if updateGolden {
			if err := WriteGolden(snap, dir); err != nil {
				return fmt.Errorf("update %s golden: %w", svcName, err)
			}
		}
		if err := CompareGolden(snap, dir); err != nil {
			return fmt.Errorf("verify %s golden: %w", svcName, err)
		}
		if len(snap.UndefinedTypes) > 0 {
			return fmt.Errorf("verify %s: undefined types: %v", svcName, snap.UndefinedTypes)
		}
		if metadata == "full" {
			probs := LintMetadata(candidate, svcName, snap)
			if len(probs) > 0 {
				return fmt.Errorf("verify %s: %d metadata problems; first: %s", svcName, len(probs), probs[0].String())
			}
		}
	}

	// 3. candidate 内编译验证。
	if err := compileCheck(ctx, candidate, filepath.Dir(coreRoot)); err != nil {
		return err
	}
	return nil
}
