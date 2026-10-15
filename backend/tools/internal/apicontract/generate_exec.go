package apicontract

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runCmd 以 GOWORK=off 运行命令，工作目录为 dir，返回 combined output。
// CombinedOutput 自管 Stdout/Stderr，禁止再设置 cmd.Stderr。
func runCmd(name string, args []string, dir string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// runCmdEnv 以自定义环境运行命令（工作目录 dir）。env 不含 GOWORK 时默认补 off。
func runCmdEnv(env []string, name string, args []string, dir string) (string, error) {
	hasWork := false
	for _, e := range env {
		if len(e) >= 7 && e[:7] == "GOWORK=" {
			hasWork = true
			break
		}
	}
	if !hasWork {
		env = append(env, "GOWORK=off")
	}
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// copyFileFiltered 以 0644 复制 src 到 dst（父目录自动创建）。
func copyFileFiltered(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// osCopyDir 递归复制 src 目录到 dst（覆盖普通文件，保留子目录；mode 取 0755 目录 /
// 0644 文件）。用于把 candidate 的 http/ 树镜像到 staging，让 goctl 在 staging 内
// 重排 .api 而不触碰 candidate 原件。
func osCopyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, path)
		if rerr != nil {
			return rerr
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !info.Mode().IsRegular() {
			return nil // 跳过符号链接/设备等
		}
		return copyFileFiltered(path, target)
	})
}

// compileCheck 在 root 下运行 GOWORK=off go test -run '^$' ./...。
//
// 关键：必须使用一次性临时 GOCACHE。saastxn 每次都在不同时间戳副本中编译，
// 默认共享 GOCACHE 会保留指向上一次 candidate 绝对路径的构建产物；candidate
// 失败被清理后，缓存的 vet/compile 子工具尝试 chdir 旧路径会以
// "no such file or directory" 失败并表现为大量 "[build failed]"。专用临时
// cache 随函数结束销毁，既消除跨次路径污染，也避免污染默认 cache。
func compileCheck(ctx context.Context, root, backendRoot string) error {
	cacheDir, err := os.MkdirTemp("", "apicheck-gocache-*")
	if err != nil {
		return fmt.Errorf("create temp gocache: %w", err)
	}
	defer os.RemoveAll(cacheDir)
	modfile, cleanup, err := prepareCandidateModfile(ctx, root, backendRoot)
	if err != nil {
		return fmt.Errorf("prepare candidate modfile: %w", err)
	}
	defer cleanup()
	env := append(os.Environ(), "GOCACHE="+cacheDir)
	out, err := runCmdEnv(env, "go", []string{"test", "-modfile=" + modfile, "-run", "^$", "./..."}, root)
	if err != nil {
		return fmt.Errorf("candidate compile check: %w\n%s", err, out)
	}
	return nil
}

// prepareCandidateModfile 把 candidate 中因目录交换而失效的相对 replace，
// 临时重定向到真实 backend 下的同仓模块。临时文件在编译完成后删除，不进入
// saastxn allowlist 或提交树。
func prepareCandidateModfile(ctx context.Context, candidate, backendRoot string) (string, func(), error) {
	backendAbs, err := filepath.Abs(backendRoot)
	if err != nil {
		return "", func() {}, fmt.Errorf("resolve backend root: %w", err)
	}
	goMod, err := os.ReadFile(filepath.Join(candidate, "go.mod"))
	if err != nil {
		return "", func() {}, fmt.Errorf("read candidate go.mod: %w", err)
	}
	temp, err := os.CreateTemp(candidate, ".apicheck-candidate-*.mod")
	if err != nil {
		return "", func() {}, fmt.Errorf("create candidate modfile: %w", err)
	}
	modfile := temp.Name()
	cleanup := func() {
		_ = os.Remove(modfile)
		_ = os.Remove(strings.TrimSuffix(modfile, ".mod") + ".sum")
	}
	if _, err := temp.Write(goMod); err != nil {
		_ = temp.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("write candidate modfile: %w", err)
	}
	if err := temp.Close(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("close candidate modfile: %w", err)
	}
	if goSum, err := os.ReadFile(filepath.Join(candidate, "go.sum")); err == nil {
		if err := os.WriteFile(strings.TrimSuffix(modfile, ".mod")+".sum", goSum, 0o644); err != nil {
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
