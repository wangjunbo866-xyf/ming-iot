// Command apicheck 冻结并校验 Core HTTP API 契约。它把 .api 源、生成的
// routes.go 和历史 types.go 归一化为四类 golden（defined/reachable/registered/
// detached + authorization + types），并对七项元数据做完整性 lint。
//
// 子命令：
//
//	apicheck snapshot --backend BACKEND_ROOT --update
//	apicheck verify   --backend BACKEND_ROOT [--metadata=none]
//	apicheck lint     --backend BACKEND_ROOT --files comma,separated.api
//
// snapshot 没有 --update 必须拒绝写入；verify --metadata=none 只比较四类契约与
// 源闭包，不要求七项元数据；无 flag 的 verify 还要求全部 metadata 完整。任何
// verify/lint 都不得修改 golden。
package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ming-iot/backend/tools/internal/apicontract"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "snapshot":
		cmdSnapshot(os.Args[2:])
	case "verify":
		cmdVerify(os.Args[2:])
	case "lint":
		cmdLint(os.Args[2:])
	case "verify-access":
		cmdVerifyAccess(os.Args[2:])
	case "generate":
		cmdGenerate(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `apicheck freezes and verifies Core HTTP API contracts.

Usage:
  apicheck snapshot       --backend BACKEND_ROOT [--update]
  apicheck verify         --backend BACKEND_ROOT [--metadata=none|full]
  apicheck lint           --backend BACKEND_ROOT --files a.api,b.api
  apicheck verify-access  --backend BACKEND_ROOT --service SERVICE --file ACCESS_JSON [--strict-auth]
  apicheck generate       --backend BACKEND_ROOT --goctl GOCTL_BINARY [--metadata=none|full] [--update-golden]

--backend is the backend/saas root (contains service/apisvr, service/datasvr).
snapshot writes the six golden files per service into its http/testdata dir;
without --update it refuses to write.
`)
}

// backendFlag 解析 --backend，返回规范化后的 core 根绝对路径。
func backendFlag(args []string) string {
	fs := flag.NewFlagSet("apicheck", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	backend := fs.String("backend", "", "backend/saas root")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *backend == "" {
		fmt.Fprintln(os.Stderr, "apicheck: --backend is required")
		os.Exit(2)
	}
	abs, err := filepath.Abs(*backend)
	if err != nil {
		fmt.Fprintf(os.Stderr, "apicheck: resolve backend: %v\n", err)
		os.Exit(2)
	}
	return abs
}

// goldenDir 返回某服务 testdata 目录的绝对路径。
func goldenDir(coreRoot, service string) string {
	return filepath.Join(coreRoot, "service", service, "http", "testdata")
}

// cmdSnapshot 实现 snapshot 子命令。
func cmdSnapshot(args []string) {
	fs := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	backend := fs.String("backend", "", "backend/saas root")
	update := fs.Bool("update", false, "write golden files (required to write)")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *backend == "" {
		fmt.Fprintln(os.Stderr, "apicheck snapshot: --backend is required")
		os.Exit(2)
	}
	if !*update {
		fmt.Fprintln(os.Stderr, "apicheck snapshot: refusing to write without --update")
		os.Exit(2)
	}
	coreRoot, err := filepath.Abs(*backend)
	if err != nil {
		fmt.Fprintf(os.Stderr, "apicheck snapshot: %v\n", err)
		os.Exit(1)
	}
	coreRoot = apicontract.ResolveCoreRoot(coreRoot)
	snaps, err := apicontract.LoadBackend(coreRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "apicheck snapshot: load: %v\n", err)
		os.Exit(1)
	}
	for svc, snap := range snaps {
		dir := goldenDir(coreRoot, svc)
		if err := apicontract.WriteGolden(snap, dir); err != nil {
			fmt.Fprintf(os.Stderr, "apicheck snapshot: write %s: %v\n", svc, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "snapshot: wrote %s golden (%d files) into %s\n", svc, len(apicontract.GoldenFiles), dir)
	}
	reportCounts(snaps)
}

// cmdVerify 实现 verify 子命令。
func cmdVerify(args []string) {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	backend := fs.String("backend", "", "backend/saas root")
	metadata := fs.String("metadata", "full", "metadata check mode: full|none")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *backend == "" {
		fmt.Fprintln(os.Stderr, "apicheck verify: --backend is required")
		os.Exit(2)
	}
	if *metadata != "full" && *metadata != "none" {
		fmt.Fprintf(os.Stderr, "apicheck verify: --metadata must be full|none, got %q\n", *metadata)
		os.Exit(2)
	}
	coreRoot, err := filepath.Abs(*backend)
	if err != nil {
		fmt.Fprintf(os.Stderr, "apicheck verify: %v\n", err)
		os.Exit(1)
	}
	coreRoot = apicontract.ResolveCoreRoot(coreRoot)
	snaps, err := apicontract.LoadBackend(coreRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "apicheck verify: load: %v\n", err)
		os.Exit(1)
	}

	// 1. 四类契约 + types golden 比较。
	failed := false
	for svc, snap := range snaps {
		dir := goldenDir(coreRoot, svc)
		if err := apicontract.CompareGolden(snap, dir); err != nil {
			fmt.Fprintf(os.Stderr, "verify: %s golden mismatch: %v\n", svc, err)
			failed = true
		}
	}

	// 2. 源闭包：UndefinedTypes 应为空（Task 2 已补回 AppleAppConfig）。
	for svc, snap := range snaps {
		if len(snap.UndefinedTypes) > 0 {
			fmt.Fprintf(os.Stderr, "verify: %s undefined types in source: %s\n", svc, strings.Join(snap.UndefinedTypes, ", "))
			failed = true
		}
	}

	// 3. 七项元数据 lint（除非 --metadata=none）。
	metaOK := 0
	if *metadata == "full" {
		var problems []apicontract.MetadataProblem
		for svc, snap := range snaps {
			problems = append(problems, apicontract.LintMetadata(coreRoot, svc, snap)...)
		}
		metaOK = countRoutes(snaps) - len(problems)
		if len(problems) > 0 {
			failed = true
			fmt.Fprintf(os.Stderr, "verify: metadata incomplete (%d problems); first: %s\n",
				len(problems), problems[0].String())
		}
	}

	reportCounts(snaps)
	if *metadata == "full" {
		fmt.Fprintf(os.Stderr, "metadata complete=%d\n", metaOK)
	}
	if failed {
		os.Exit(1)
	}
}

// cmdLint 实现 lint 子命令：仅校验指定文件的七项元数据，不比较 golden。
func cmdLint(args []string) {
	fs := flag.NewFlagSet("lint", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	backend := fs.String("backend", "", "backend/saas root")
	files := fs.String("files", "", "comma-separated relative .api paths")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *backend == "" {
		fmt.Fprintln(os.Stderr, "apicheck lint: --backend is required")
		os.Exit(2)
	}
	if *files == "" {
		fmt.Fprintln(os.Stderr, "apicheck lint: --files is required")
		os.Exit(2)
	}
	coreRoot, err := filepath.Abs(*backend)
	if err != nil {
		fmt.Fprintf(os.Stderr, "apicheck lint: %v\n", err)
		os.Exit(1)
	}
	coreRoot = apicontract.ResolveCoreRoot(coreRoot)
	want, err := lintFileSelection(coreRoot, *files)
	if err != nil {
		fmt.Fprintf(os.Stderr, "apicheck lint: --files: %v\n", err)
		os.Exit(2)
	}
	snaps, err := apicontract.LoadBackend(coreRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "apicheck lint: load: %v\n", err)
		os.Exit(1)
	}
	var problems []apicontract.MetadataProblem
	for svc, snap := range snaps {
		for _, p := range apicontract.LintMetadata(coreRoot, svc, snap) {
			if want[p.File] {
				problems = append(problems, p)
			}
		}
	}
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, p.String())
		}
		fmt.Fprintf(os.Stderr, "lint: %d metadata problems\n", len(problems))
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "lint: ok (%d files)\n", len(want))
}

// lintFileSelection uses the same service directories as LintMetadata. Checking
// only os.Stat would accept files outside its scan and falsely report success.
func lintFileSelection(coreRoot, files string) (map[string]bool, error) {
	available := map[string]bool{}
	for _, backend := range apicontract.Backends(coreRoot) {
		err := filepath.WalkDir(backend.ServiceDir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.Type().IsRegular() && filepath.Ext(path) == ".api" {
				rel, err := filepath.Rel(coreRoot, path)
				if err != nil {
					return err
				}
				available[filepath.ToSlash(rel)] = true
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan API files: %w", err)
		}
	}
	want := map[string]bool{}
	for _, item := range strings.Split(files, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		rel := filepath.ToSlash(filepath.Clean(item))
		if filepath.IsAbs(item) || !available[rel] {
			return nil, fmt.Errorf("%q is not a regular .api file in a service HTTP directory", item)
		}
		if _, err := os.ReadFile(filepath.Join(coreRoot, rel)); err != nil {
			return nil, fmt.Errorf("read %q: %w", item, err)
		}
		want[rel] = true
	}
	if len(want) == 0 {
		return nil, fmt.Errorf("select at least one .api file")
	}
	return want, nil
}

// cmdVerifyAccess 实现 verify-access 子命令：把 goctl 生成的 access JSON 与
// 某服务的 authorization golden 比较（method/fullPath/authType/needAuth/operType
// 及路由集合），忽略 JSON 顺序与展示名称。
//
// goctl fork 的 access 生成器对已被 parser 去引号的 @doc 值调用 Unquote 会失败，
// 导致 authType/needAuth 对带显式 @doc 的路由坍缩。默认仅 operType/集合为门禁，
// authType/needAuth 仅报告；--strict-auth 把它们也升级为门禁。
func cmdVerifyAccess(args []string) {
	fs := flag.NewFlagSet("verify-access", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	backend := fs.String("backend", "", "backend/saas root")
	service := fs.String("service", "", "service name: apisvr|datasvr")
	file := fs.String("file", "", "path to goctl-generated access.json")
	strictAuth := fs.Bool("strict-auth", false, "treat authType/needAuth mismatch as failure")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *backend == "" || *service == "" || *file == "" {
		fmt.Fprintln(os.Stderr, "apicheck verify-access: --backend, --service and --file are required")
		os.Exit(2)
	}
	if *service != "apisvr" && *service != "datasvr" {
		fmt.Fprintf(os.Stderr, "apicheck verify-access: --service must be apisvr|datasvr, got %q\n", *service)
		os.Exit(2)
	}
	coreRoot, err := filepath.Abs(*backend)
	if err != nil {
		fmt.Fprintf(os.Stderr, "apicheck verify-access: %v\n", err)
		os.Exit(1)
	}
	data, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "apicheck verify-access: read %s: %v\n", *file, err)
		os.Exit(1)
	}
	rep, err := apicontract.VerifyAccess(coreRoot, *service, data, apicontract.VerifyAccessOptions{FailOnAuth: *strictAuth})
	if err != nil {
		fmt.Fprintf(os.Stderr, "verify-access: %s service=%s FAILED\n%v\n%s", *file, *service, err, rep.Report())
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "verify-access: %s service=%s OK\n%s", *file, *service, rep.Report())
}

// cmdGenerate 实现 generate 子命令：用 saastxn 原子事务重生成 apisvr/datasvr
// 的 types.go 与 routes.go（共 4 个文件），handler/logic/main/config/svc 保持
// 人工所有权。
func cmdGenerate(args []string) {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	backend := fs.String("backend", "", "backend/saas root")
	goctl := fs.String("goctl", "", "path to fixed goctl binary")
	metadata := fs.String("metadata", "full", "metadata check mode: full|none")
	updateGolden := fs.Bool("update-golden", false, "atomically update golden files")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *backend == "" || *goctl == "" {
		fmt.Fprintln(os.Stderr, "apicheck generate: --backend and --goctl are required")
		os.Exit(2)
	}
	if *metadata != "full" && *metadata != "none" {
		fmt.Fprintf(os.Stderr, "apicheck generate: --metadata must be full|none, got %q\n", *metadata)
		os.Exit(2)
	}
	coreRoot, err := filepath.Abs(*backend)
	if err != nil {
		fmt.Fprintf(os.Stderr, "apicheck generate: %v\n", err)
		os.Exit(1)
	}
	ctx := context.Background()
	if err := apicontract.GenerateAPI(ctx, apicontract.GenerateAPIOptions{
		BackendRoot:  coreRoot,
		GoctlBinary:  *goctl,
		Metadata:     *metadata,
		UpdateGolden: *updateGolden,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "apicheck generate: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "apicheck generate: committed")
}

// reportCounts 打印 defined/reachable/registered/detached 总计。
func reportCounts(snaps map[string]apicontract.Snapshot) {
	var def, reach, reg, detach int
	services := make([]string, 0, len(snaps))
	for svc := range snaps {
		services = append(services, svc)
	}
	sort.Strings(services)
	for _, svc := range services {
		s := snaps[svc]
		def += len(s.Defined)
		reach += len(s.Reachable)
		reg += len(s.Registered)
		detach += len(s.Detached)
		fmt.Fprintf(os.Stderr, "%s: defined=%d reachable=%d registered=%d detached=%d\n",
			svc, len(s.Defined), len(s.Reachable), len(s.Registered), len(s.Detached))
	}
	fmt.Fprintf(os.Stderr, "total: defined=%d reachable=%d registered=%d detached=%d\n",
		def, reach, reg, detach)
}

// countRoutes 返回全部服务的 defined 路由总数。
func countRoutes(snaps map[string]apicontract.Snapshot) int {
	n := 0
	for _, s := range snaps {
		n += len(s.Defined)
	}
	return n
}
