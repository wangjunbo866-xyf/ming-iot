package thingsbaseline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const maxGoEnvCacheOutputBytes = 64 << 10

type sharedGoCachePaths struct {
	Build   string
	Modules string
}

func resolveSharedGoCachePaths(ctx context.Context, goBinary string) (sharedGoCachePaths, error) {
	if err := ctx.Err(); err != nil {
		return sharedGoCachePaths{}, err
	}
	if !filepath.IsAbs(goBinary) || filepath.Clean(goBinary) != goBinary {
		return sharedGoCachePaths{}, fmt.Errorf("trusted Go binary path is invalid")
	}
	command := exec.CommandContext(ctx, goBinary, "env", "-json", "GOCACHE", "GOMODCACHE")
	command.Env = sharedGoCacheDiscoveryEnvironment(os.Environ())
	return runSharedGoCacheDiscovery(ctx, command)
}

func resolveSharedGoCachePathsFromRetainedFile(ctx context.Context, goTool *os.File) (sharedGoCachePaths, error) {
	if err := ctx.Err(); err != nil {
		return sharedGoCachePaths{}, err
	}
	if goTool == nil {
		return sharedGoCachePaths{}, fmt.Errorf("retained Go tool is unavailable")
	}
	command := exec.CommandContext(ctx, "/proc/self/fd/3", "env", "-json", "GOCACHE", "GOMODCACHE")
	command.ExtraFiles = []*os.File{goTool}
	command.Env = sharedGoCacheDiscoveryEnvironment(os.Environ())
	return runSharedGoCacheDiscovery(ctx, command)
}

func sharedGoCacheDiscoveryEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment)+1)
	for _, item := range environment {
		key, _, ok := strings.Cut(item, "=")
		if !ok || key == "GOTOOLCHAIN" {
			continue
		}
		result = append(result, item)
	}
	return append(result, "GOTOOLCHAIN=local")
}

func runSharedGoCacheDiscovery(ctx context.Context, command *exec.Cmd) (sharedGoCachePaths, error) {
	stdout, err := command.StdoutPipe()
	if err != nil {
		return sharedGoCachePaths{}, fmt.Errorf("prepare shared Go cache response")
	}
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return sharedGoCachePaths{}, fmt.Errorf("resolve shared Go cache paths")
	}
	output, readErr := io.ReadAll(io.LimitReader(stdout, maxGoEnvCacheOutputBytes+1))
	if readErr != nil || len(output) > maxGoEnvCacheOutputBytes {
		_ = command.Process.Kill()
		_ = command.Wait()
		if readErr != nil {
			return sharedGoCachePaths{}, fmt.Errorf("read shared Go cache response")
		}
		return sharedGoCachePaths{}, fmt.Errorf("shared Go cache response is outside the allowed size")
	}
	if err := command.Wait(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return sharedGoCachePaths{}, ctxErr
		}
		return sharedGoCachePaths{}, fmt.Errorf("resolve shared Go cache paths")
	}
	if len(output) == 0 {
		return sharedGoCachePaths{}, fmt.Errorf("shared Go cache response is outside the allowed size")
	}
	return parseSharedGoCachePaths(output)
}

func parseSharedGoCachePaths(output []byte) (sharedGoCachePaths, error) {
	var values struct {
		GOCACHE    string
		GOMODCACHE string
	}
	if err := json.Unmarshal(output, &values); err != nil {
		return sharedGoCachePaths{}, fmt.Errorf("decode shared Go cache response")
	}
	cache := sharedGoCachePaths{Build: values.GOCACHE, Modules: values.GOMODCACHE}
	if err := prepareSharedGoCacheDirectory("GOCACHE", cache.Build); err != nil {
		return sharedGoCachePaths{}, err
	}
	if err := prepareSharedGoCacheDirectory("GOMODCACHE", cache.Modules); err != nil {
		return sharedGoCachePaths{}, err
	}
	return cache, nil
}

func prepareSharedGoCacheDirectory(name, path string) error {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path || path == string(filepath.Separator) {
		return fmt.Errorf("%s is not a safe absolute directory", name)
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("%s is unavailable", name)
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("%s is unavailable", name)
	}
	probe, err := os.CreateTemp(path, ".ming-cache-probe-*")
	if err != nil {
		return fmt.Errorf("%s is not writable", name)
	}
	probePath := probe.Name()
	if closeErr := probe.Close(); closeErr != nil {
		_ = os.Remove(probePath)
		return fmt.Errorf("%s write probe failed", name)
	}
	if err := os.Remove(probePath); err != nil {
		return fmt.Errorf("%s write probe cleanup failed", name)
	}
	return nil
}
