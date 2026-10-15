package thingsbaseline

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

type trailingWhitespaceFingerprint struct {
	Path   string
	Line   int
	SHA256 string
}

func collectSourceTrailingWhitespace(files []SourceFile) map[trailingWhitespaceFingerprint]struct{} {
	result := make(map[trailingWhitespaceFingerprint]struct{})
	for _, file := range files {
		for _, fingerprint := range trailingWhitespaceForFile(file.Path, file.Data) {
			result[fingerprint] = struct{}{}
		}
	}
	return result
}

func trailingWhitespaceForFile(path string, data []byte) []trailingWhitespaceFingerprint {
	lines := bytes.SplitAfter(data, []byte("\n"))
	result := make([]trailingWhitespaceFingerprint, 0)
	for index, withEnding := range lines {
		line := bytes.TrimSuffix(withEnding, []byte("\n"))
		line = bytes.TrimSuffix(line, []byte("\r"))
		if len(line) == 0 || line[len(line)-1] != ' ' && line[len(line)-1] != '\t' {
			continue
		}
		sum := sha256.Sum256(line)
		result = append(result, trailingWhitespaceFingerprint{Path: path, Line: index + 1, SHA256: hex.EncodeToString(sum[:])})
	}
	return result
}

func scanTree(root string, policy securityPolicy) (SecurityScanReport, []ManifestEntry, CredentialProfileReport, error) {
	if policy.lock == nil {
		return SecurityScanReport{}, nil, CredentialProfileReport{}, fmt.Errorf("security policy lock is nil")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return SecurityScanReport{}, nil, CredentialProfileReport{}, fmt.Errorf("resolve scan root: %w", err)
	}
	absolute = filepath.Clean(absolute)
	physical, err := filepath.EvalSymlinks(absolute)
	if err != nil || filepath.Clean(physical) != absolute {
		return SecurityScanReport{}, nil, CredentialProfileReport{}, fmt.Errorf("scan root contains a symlink")
	}
	rootFD, err := unix.Open(absolute, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return SecurityScanReport{}, nil, CredentialProfileReport{}, fmt.Errorf("scan root is not a real directory")
	}
	defer unix.Close(rootFD)
	return scanOpenedTree(absolute, rootFD, policy)
}

func scanTreeFromFD(root string, rootFD int, policy securityPolicy) (SecurityScanReport, []ManifestEntry, CredentialProfileReport, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return SecurityScanReport{}, nil, CredentialProfileReport{}, fmt.Errorf("resolve scan root: %w", err)
	}
	return scanOpenedTree(filepath.Clean(absolute), rootFD, policy)
}

func scanOpenedTree(absolute string, rootFD int, policy securityPolicy) (SecurityScanReport, []ManifestEntry, CredentialProfileReport, error) {
	if policy.lock == nil {
		return SecurityScanReport{}, nil, CredentialProfileReport{}, fmt.Errorf("security policy lock is nil")
	}
	var rootBefore unix.Stat_t
	if err := unix.Fstat(rootFD, &rootBefore); err != nil || rootBefore.Mode&unix.S_IFMT != unix.S_IFDIR {
		return SecurityScanReport{}, nil, CredentialProfileReport{}, fmt.Errorf("scan root is not a real directory")
	}
	if permissions := rootBefore.Mode & 0o777; permissions != 0o755 {
		return SecurityScanReport{}, nil, CredentialProfileReport{}, fmt.Errorf("scan root mode = %04o, want 0755", permissions)
	}
	scanner := secureTreeScanner{
		policy:              policy,
		seen:                make(map[string]struct{}, len(policy.expectedFiles)),
		expectedDirectories: expectedDirectoryPaths(policy.expectedFiles),
	}
	scanner.report.Directories = 1
	if err := scanner.scanDirectory(rootFD, ""); err != nil {
		return SecurityScanReport{}, nil, CredentialProfileReport{}, err
	}
	var rootAfter unix.Stat_t
	if err := unix.Lstat(absolute, &rootAfter); err != nil || !sameFileIdentity(rootBefore, rootAfter) || rootAfter.Mode&unix.S_IFMT != unix.S_IFDIR {
		return SecurityScanReport{}, nil, CredentialProfileReport{}, fmt.Errorf("scan root identity changed during scan")
	}
	if rootAfter.Mode != rootBefore.Mode {
		return SecurityScanReport{}, nil, CredentialProfileReport{}, fmt.Errorf("scan root metadata changed during scan")
	}
	if !policy.allowTreeDifferences && len(scanner.seen) != len(policy.expectedFiles) {
		missing := make([]string, 0)
		for path := range policy.expectedFiles {
			if _, ok := scanner.seen[path]; !ok {
				missing = append(missing, path)
			}
		}
		sort.Strings(missing)
		return SecurityScanReport{}, nil, CredentialProfileReport{}, fmt.Errorf("scanned tree is missing expected paths: %v", missing)
	}
	wantCredentials := credentialProfileFromContract(policy.lock.Transforms)
	if !credentialProfilesEqual(scanner.credentials, wantCredentials) {
		return SecurityScanReport{}, nil, CredentialProfileReport{}, fmt.Errorf("scanned credential profile = %+v, want %+v", scanner.credentials, wantCredentials)
	}
	if scanner.nonYAMLCredentialValues != policy.lock.Transforms.NonYAMLCredentialValueCount {
		return SecurityScanReport{}, nil, CredentialProfileReport{}, fmt.Errorf("scanned non-YAML credential value count = %d, want %d", scanner.nonYAMLCredentialValues, policy.lock.Transforms.NonYAMLCredentialValueCount)
	}
	if !policy.allowTreeDifferences && scanner.report.RegularFiles != policy.lock.Source.SelectedFiles+4 {
		return SecurityScanReport{}, nil, CredentialProfileReport{}, fmt.Errorf("scanned regular files = %d, want %d", scanner.report.RegularFiles, policy.lock.Source.SelectedFiles+4)
	}
	if !policy.allowTreeDifferences && scanner.report.Directories != len(scanner.expectedDirectories) {
		return SecurityScanReport{}, nil, CredentialProfileReport{}, fmt.Errorf("scanned directories = %d, want %d", scanner.report.Directories, len(scanner.expectedDirectories))
	}
	sort.Slice(scanner.manifest, func(i, j int) bool { return scanner.manifest[i].Path < scanner.manifest[j].Path })
	credentials := CredentialProfileReport{Files: scanner.credentials.Files, Active: scanner.credentials.Active, Commented: scanner.credentials.Commented, Paths: scanner.credentials.Paths}
	return scanner.report, scanner.manifest, credentials, nil
}

type secureTreeScanner struct {
	policy                  securityPolicy
	report                  SecurityScanReport
	manifest                []ManifestEntry
	credentials             credentialProfile
	nonYAMLCredentialValues int
	seen                    map[string]struct{}
	expectedDirectories     map[string]struct{}
}

func (scanner *secureTreeScanner) scanDirectory(directoryFD int, relativeDirectory string) error {
	if _, expected := scanner.expectedDirectories[relativeDirectory]; !expected && !scanner.policy.allowTreeDifferences {
		return fmt.Errorf("scanned tree contains unexpected directory %q", relativeDirectory)
	}
	var directoryBefore unix.Stat_t
	if err := unix.Fstat(directoryFD, &directoryBefore); err != nil || directoryBefore.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("scanned directory identity is invalid")
	}
	if permissions := directoryBefore.Mode & 0o777; permissions != 0o755 {
		return fmt.Errorf("scanned directory metadata changed: mode = %04o, want 0755", permissions)
	}
	namesBefore, err := listDirectoryNames(directoryFD)
	if err != nil {
		return fmt.Errorf("list scanned directory: %w", err)
	}
	if scanner.policy.afterDirectoryList != nil {
		scanner.policy.afterDirectoryList(relativeDirectory)
	}
	for _, name := range namesBefore {
		if name == "" || name == "." || name == ".." || strings.Contains(name, "/") {
			return fmt.Errorf("scanned directory contains an unsafe name")
		}
		relative := name
		if relativeDirectory != "" {
			relative = relativeDirectory + "/" + name
		}
		if err := validateGitPath("scanned", relative); err != nil {
			return err
		}
		var before unix.Stat_t
		if err := unix.Fstatat(directoryFD, name, &before, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return fmt.Errorf("stat scanned path %q: %w", relative, err)
		}
		if scanner.policy.afterPathStat != nil {
			scanner.policy.afterPathStat(relative)
		}
		switch before.Mode & unix.S_IFMT {
		case unix.S_IFDIR:
			if permissions := before.Mode & 0o777; permissions != 0o755 {
				return fmt.Errorf("tree directory has mode %04o, want 0755", permissions)
			}
			childFD, err := unix.Openat(directoryFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				return fmt.Errorf("open scanned directory %q: %w", relative, err)
			}
			var opened unix.Stat_t
			if err := unix.Fstat(childFD, &opened); err != nil || !sameFileIdentity(before, opened) {
				_ = unix.Close(childFD)
				return fmt.Errorf("scanned directory %q identity changed before traversal", relative)
			}
			if opened.Mode != before.Mode {
				_ = unix.Close(childFD)
				return fmt.Errorf("scanned directory %q metadata changed before traversal", relative)
			}
			scanner.report.Directories++
			err = scanner.scanDirectory(childFD, relative)
			_ = unix.Close(childFD)
			if err != nil {
				return err
			}
			var after unix.Stat_t
			if err := unix.Fstatat(directoryFD, name, &after, unix.AT_SYMLINK_NOFOLLOW); err != nil || !sameFileIdentity(before, after) || after.Mode&unix.S_IFMT != unix.S_IFDIR {
				return fmt.Errorf("scanned directory %q identity changed during traversal", relative)
			}
			if after.Mode != before.Mode {
				return fmt.Errorf("scanned directory %q metadata changed during traversal", relative)
			}
		case unix.S_IFREG:
			if err := scanner.scanRegular(directoryFD, name, relative, before); err != nil {
				return err
			}
		case unix.S_IFLNK:
			return fmt.Errorf("tree path is a symlink")
		default:
			return fmt.Errorf("scanned path %q is not a regular file", relative)
		}
	}
	namesAfter, err := listDirectoryNames(directoryFD)
	if err != nil || !equalStringSlices(namesBefore, namesAfter) {
		return fmt.Errorf("scanned directory entries changed during scan")
	}
	var directoryAfter unix.Stat_t
	if err := unix.Fstat(directoryFD, &directoryAfter); err != nil || !sameFileIdentity(directoryBefore, directoryAfter) {
		return fmt.Errorf("scanned directory identity changed during scan")
	}
	if directoryAfter.Mode != directoryBefore.Mode {
		return fmt.Errorf("scanned directory metadata changed during scan")
	}
	return nil
}

func expectedDirectoryPaths(files map[string]RenderedFile) map[string]struct{} {
	directories := map[string]struct{}{"": {}}
	for path := range files {
		parts := strings.Split(path, "/")
		for index := 1; index < len(parts); index++ {
			directories[strings.Join(parts[:index], "/")] = struct{}{}
		}
	}
	return directories
}

func (scanner *secureTreeScanner) scanRegular(parentFD int, name, relative string, before unix.Stat_t) error {
	if before.Nlink != 1 {
		return fmt.Errorf("scanned path %q is a hardlink or has unknown link count", relative)
	}
	if before.Size > scanner.policy.lock.Security.MaxFileBytes {
		return fmt.Errorf("scanned path %q size %d exceeds maximum before read", relative, before.Size)
	}
	if err := rejectDangerousPath(relative); err != nil {
		return err
	}
	if err := rejectExcludedTopLevel(relative, scanner.policy); err != nil {
		return err
	}
	expected, ok := scanner.policy.expectedFiles[relative]
	if !ok && !scanner.policy.allowTreeDifferences {
		return fmt.Errorf("scanned tree contains unexpected path %q", relative)
	}
	data, err := readRegularAt(parentFD, name, before, scanner.policy.lock.Security.MaxFileBytes)
	if err != nil {
		return fmt.Errorf("read scanned path %q: %w", relative, err)
	}
	mode := "100644"
	permissions := before.Mode & 0o777
	if permissions == 0o755 {
		mode = "100755"
	} else if permissions != 0o644 {
		return fmt.Errorf("scanned path %q mode = %04o, want 0644/0755", relative, permissions)
	}
	if !scanner.policy.allowTreeDifferences && (mode != expected.Mode || !bytes.Equal(data, expected.Data)) {
		return fmt.Errorf("scanned path %q differs from sealed expected content or mode", relative)
	}
	if len(data) >= 4 && bytes.Equal(data[:4], []byte{0x7f, 'E', 'L', 'F'}) {
		return fmt.Errorf("scanned path %q contains ELF magic", relative)
	}
	if containsPrivateKeyPEMMarker(data) {
		return fmt.Errorf("scanned path %q contains a private-key marker", relative)
	}
	if bytes.Contains(data, []byte("gitee.com/unitedrhino/core")) {
		return fmt.Errorf("scanned path %q retains old Core module", relative)
	}
	if bytes.Contains(data, []byte(scanner.policy.lock.Source.Module)) {
		template, allowed := scanner.policy.expectedTemplates[relative]
		if !allowed || !bytes.Equal(data, template.Data) {
			return fmt.Errorf("scanned path %q retains old Things module outside a golden template", relative)
		}
	}
	if relative == "LICENSE" {
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != scanner.policy.expectedLicenseSHA {
			return fmt.Errorf("LICENSE SHA-256 does not match locked source")
		}
	}
	if isYAMLPath(relative) {
		profile, err := verifySanitizedCredentialYAML(relative, data)
		if err != nil {
			return err
		}
		scanner.credentials.merge(profile)
	}
	nonYAMLCount, err := verifySanitizedNonYAMLCredentialFile(relative, data)
	if err != nil {
		return err
	}
	scanner.nonYAMLCredentialValues += nonYAMLCount
	for _, fingerprint := range trailingWhitespaceForFile(relative, data) {
		if _, allowed := scanner.policy.allowedTrailingWhitespace[fingerprint]; !allowed {
			return fmt.Errorf("scanned path %q line %d introduces trailing whitespace", relative, fingerprint.Line)
		}
		scanner.report.TrailingWhitespace++
	}
	sum := sha256.Sum256(data)
	scanner.manifest = append(scanner.manifest, ManifestEntry{Path: relative, Mode: mode, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data))})
	scanner.report.RegularFiles++
	scanner.report.TotalBytes += int64(len(data))
	scanner.seen[relative] = struct{}{}
	return nil
}

func readRegularAt(parentFD int, name string, before unix.Stat_t, maxFileBytes int64) ([]byte, error) {
	fd, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("wrap file descriptor")
	}
	defer file.Close()
	var opened unix.Stat_t
	if err := unix.Fstat(fd, &opened); err != nil || !sameFileIdentity(before, opened) {
		return nil, fmt.Errorf("file identity changed before read")
	}
	if opened.Mode != before.Mode || opened.Size != before.Size || opened.Nlink != before.Nlink || opened.Mode&unix.S_IFMT != unix.S_IFREG || opened.Nlink != 1 {
		return nil, fmt.Errorf("file metadata changed before read")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxFileBytes || int64(len(data)) != before.Size {
		return nil, fmt.Errorf("file size changed during read")
	}
	var afterFD unix.Stat_t
	if err := unix.Fstat(fd, &afterFD); err != nil || !sameFileIdentity(opened, afterFD) {
		return nil, fmt.Errorf("file descriptor identity changed during read")
	}
	if afterFD.Mode != opened.Mode || afterFD.Size != opened.Size || afterFD.Nlink != opened.Nlink {
		return nil, fmt.Errorf("file descriptor metadata changed during read")
	}
	var afterPath unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &afterPath, unix.AT_SYMLINK_NOFOLLOW); err != nil || !sameFileIdentity(opened, afterPath) {
		return nil, fmt.Errorf("file path identity changed during read")
	}
	if afterPath.Mode != opened.Mode || afterPath.Size != opened.Size || afterPath.Nlink != opened.Nlink || afterPath.Mode&unix.S_IFMT != unix.S_IFREG {
		return nil, fmt.Errorf("file path metadata changed during read")
	}
	return data, nil
}

func listDirectoryNames(directoryFD int) ([]string, error) {
	listingFD, err := unix.Openat(directoryFD, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(listingFD), "directory")
	if file == nil {
		_ = unix.Close(listingFD)
		return nil, fmt.Errorf("wrap directory file descriptor")
	}
	entries, err := file.ReadDir(-1)
	_ = file.Close()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

func sameFileIdentity(left, right unix.Stat_t) bool {
	return left.Dev == right.Dev && left.Ino == right.Ino
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func rejectDangerousPath(path string) error {
	base := strings.ToLower(filepath.Base(path))
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return fmt.Errorf("scanned path %q has a forbidden environment filename", path)
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".pem", ".ppk", ".key", ".p12", ".pfx":
		return fmt.Errorf("scanned path %q has a forbidden credential extension", path)
	}
	return nil
}

func rejectExcludedTopLevel(path string, policy securityPolicy) error {
	root := strings.Split(path, "/")[0]
	for _, excluded := range policy.lock.Source.Exclude {
		if root != excluded {
			continue
		}
		if template, ok := policy.expectedTemplates[path]; ok && (path == "README.md" || path == "Makefile") {
			if expected, exists := policy.expectedFiles[path]; exists && bytes.Equal(template.Data, expected.Data) {
				return nil
			}
		}
		return fmt.Errorf("scanned path %q belongs to excluded top-level root %q", path, excluded)
	}
	return nil
}
