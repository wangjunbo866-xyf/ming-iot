package thingsbaseline

import (
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

const maxGoToolBytes = int64(256 << 20)

type gateGoTool interface {
	inheritedFile() *os.File
	attestedVersion() string
	revalidate(context.Context) error
}

type attestedGoTool struct {
	mu        sync.Mutex
	file      *os.File
	state     unix.Stat_t
	sha256    string
	goVersion string
	buildPath string
}

func openAttestedGoTool(ctx context.Context, path string) (*attestedGoTool, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == string(filepath.Separator) {
		return nil, fmt.Errorf("Go tool path is not a clean absolute path")
	}
	rootFD, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open filesystem root for Go tool: %w", err)
	}
	defer unix.Close(rootFD)
	parts := strings.Split(strings.TrimPrefix(path, string(filepath.Separator)), string(filepath.Separator))
	if len(parts) < 2 {
		return nil, fmt.Errorf("Go tool path has no safe parent")
	}
	parentFD, err := openExistingDirectoryAt(rootFD, parts[:len(parts)-1])
	if err != nil {
		return nil, fmt.Errorf("open Go tool parent without symlinks: %w", err)
	}
	defer unix.Close(parentFD)
	name := parts[len(parts)-1]
	var namedBefore unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &namedBefore, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return nil, fmt.Errorf("inspect Go tool: %w", err)
	}
	if err := validateGoToolStat(namedBefore); err != nil {
		return nil, err
	}
	fd, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open Go tool without following links: %w", err)
	}
	file := os.NewFile(uintptr(fd), "attested-go-tool")
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("wrap Go tool file descriptor")
	}
	keepOpen := false
	defer func() {
		if !keepOpen {
			_ = file.Close()
		}
	}()
	var opened unix.Stat_t
	if err := unix.Fstat(fd, &opened); err != nil || !sameGoToolState(namedBefore, opened) {
		return nil, fmt.Errorf("Go tool identity or metadata changed while opening")
	}
	firstSHA, err := hashRetainedFile(ctx, file, opened.Size, maxGoToolBytes)
	if err != nil {
		return nil, fmt.Errorf("hash Go tool before attestation: %w", err)
	}
	info, err := buildinfo.Read(file)
	if err != nil {
		return nil, fmt.Errorf("read Go tool build info: %w", err)
	}
	if info.GoVersion != lockedGoVersion || info.Path != "cmd/go" {
		return nil, fmt.Errorf("Go tool build info = version %q path %q, want %q and cmd/go", info.GoVersion, info.Path, lockedGoVersion)
	}
	secondSHA, err := hashRetainedFile(ctx, file, opened.Size, maxGoToolBytes)
	if err != nil {
		return nil, fmt.Errorf("hash Go tool after attestation: %w", err)
	}
	var afterFD, namedAfter unix.Stat_t
	if err := unix.Fstat(fd, &afterFD); err != nil ||
		unix.Fstatat(parentFD, name, &namedAfter, unix.AT_SYMLINK_NOFOLLOW) != nil ||
		!sameGoToolState(opened, afterFD) || !sameGoToolState(afterFD, namedAfter) || firstSHA != secondSHA {
		return nil, fmt.Errorf("Go tool identity, metadata, or content changed during attestation")
	}
	keepOpen = true
	return &attestedGoTool{file: file, state: afterFD, sha256: firstSHA, goVersion: info.GoVersion, buildPath: info.Path}, nil
}

func validateGoToolStat(stat unix.Stat_t) error {
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 || stat.Size <= 0 || stat.Size > maxGoToolBytes {
		return fmt.Errorf("Go tool is not a bounded single-link regular file")
	}
	if stat.Mode&0o7777 != 0o755 {
		return fmt.Errorf("Go tool mode = %04o, want 0755", stat.Mode&0o7777)
	}
	return nil
}

func (tool *attestedGoTool) inheritedFile() *os.File {
	if tool == nil {
		return nil
	}
	tool.mu.Lock()
	defer tool.mu.Unlock()
	return tool.file
}

func (tool *attestedGoTool) attestedVersion() string {
	if tool == nil {
		return ""
	}
	tool.mu.Lock()
	defer tool.mu.Unlock()
	return tool.goVersion
}

func (tool *attestedGoTool) revalidate(ctx context.Context) error {
	if tool == nil {
		return fmt.Errorf("attested Go tool is nil")
	}
	tool.mu.Lock()
	defer tool.mu.Unlock()
	if tool.file == nil {
		return fmt.Errorf("attested Go tool is closed")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var before unix.Stat_t
	if err := unix.Fstat(int(tool.file.Fd()), &before); err != nil || !sameGoToolState(tool.state, before) {
		return fmt.Errorf("retained Go tool identity or metadata changed")
	}
	sha, err := hashRetainedFile(ctx, tool.file, before.Size, maxGoToolBytes)
	if err != nil {
		return fmt.Errorf("rehash retained Go tool: %w", err)
	}
	var after unix.Stat_t
	if err := unix.Fstat(int(tool.file.Fd()), &after); err != nil || !sameGoToolState(tool.state, after) || sha != tool.sha256 {
		return fmt.Errorf("retained Go tool changed after attestation")
	}
	return nil
}

func (tool *attestedGoTool) Close() error {
	if tool == nil {
		return nil
	}
	tool.mu.Lock()
	defer tool.mu.Unlock()
	if tool.file == nil {
		return nil
	}
	err := tool.file.Close()
	tool.file = nil
	return err
}

func hashRetainedFile(ctx context.Context, file *os.File, size, maximum int64) (string, error) {
	if file == nil || size <= 0 || size > maximum {
		return "", fmt.Errorf("retained file size is outside the allowed range")
	}
	hasher := sha256.New()
	buffer := make([]byte, 128<<10)
	var offset int64
	for offset < size {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		want := int64(len(buffer))
		if remaining := size - offset; remaining < want {
			want = remaining
		}
		read, err := file.ReadAt(buffer[:want], offset)
		if read > 0 {
			_, _ = hasher.Write(buffer[:read])
			offset += int64(read)
		}
		if err != nil && err != io.EOF {
			return "", err
		}
		if read == 0 {
			return "", fmt.Errorf("retained file read made no progress")
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func sameGoToolState(left, right unix.Stat_t) bool {
	return sameFileIdentity(left, right) &&
		left.Mode == right.Mode && left.Size == right.Size && left.Nlink == right.Nlink &&
		left.Mtim.Sec == right.Mtim.Sec && left.Mtim.Nsec == right.Mtim.Nsec &&
		left.Ctim.Sec == right.Ctim.Sec && left.Ctim.Nsec == right.Ctim.Nsec
}
