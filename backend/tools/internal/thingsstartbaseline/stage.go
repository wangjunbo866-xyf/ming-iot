package thingsstartbaseline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// ErrContractMismatch identifies input that does not match the fixed baseline contract.
var ErrContractMismatch = errors.New("things startup baseline contract mismatch")

// StageRequest identifies the immutable repository inputs and isolated runtime destination.
type StageRequest struct {
	RepoRoot    string
	RuntimeRoot string
	Contract    *Contract
}

// StageContext identifies one staged role context.
type StageContext struct {
	Role string
	Path string
}

// StageResult records the fully staged role contexts.
type StageResult struct {
	Contexts []StageContext
}

type stagedFile struct {
	Path   string `json:"path"`
	Mode   string `json:"mode"`
	SHA256 string `json:"sha256"`
}

type stageManifest struct {
	SchemaVersion int          `json:"schemaVersion"`
	Files         []stagedFile `json:"files"`
}

type roleStageAssets struct {
	i18n string
	init string
}

var roleStageAssetPaths = map[string]roleStageAssets{
	"sys": {
		i18n: "backend/saas/service/apisvr/etc/i18n",
		init: "backend/saas/service/apisvr/etc/init",
	},
	"timedjob": {},
	"dm": {
		i18n: "backend/things/service/apisvr/etc/i18n",
		init: "backend/things/service/apisvr/etc/init",
	},
	"dg":  {},
	"api": {i18n: "backend/things/service/apisvr/etc/i18n"},
}

type stageOperations struct {
	syncDirectory       func(string) error
	exchangeDirectories func(string, string) error
	removeAll           func(string) error
}

func newStageOperations() stageOperations {
	return stageOperations{
		syncDirectory: syncDirectory,
		exchangeDirectories: func(oldStage, newStage string) error {
			return unix.Renameat2(unix.AT_FDCWD, oldStage, unix.AT_FDCWD, newStage, unix.RENAME_EXCHANGE)
		},
		removeAll: os.RemoveAll,
	}
}

// ValidateRuntimeEnvironment verifies that THINGS_DB_DSN addresses only the contract database.
func ValidateRuntimeEnvironment(contract *Contract) error {
	if err := ValidateContract(contract); err != nil {
		return fmt.Errorf("%w: invalid contract: %v", ErrContractMismatch, err)
	}
	dsn := os.Getenv("THINGS_DB_DSN")
	database, err := postgresDatabaseName(dsn)
	if err != nil {
		return fmt.Errorf("%w: THINGS_DB_DSN: %v", ErrContractMismatch, err)
	}
	for _, forbidden := range contract.Database.ForbiddenNames {
		if strings.EqualFold(database, forbidden) {
			return fmt.Errorf("%w: THINGS_DB_DSN targets forbidden database", ErrContractMismatch)
		}
	}
	if database != contract.Database.RequiredName {
		return fmt.Errorf("%w: THINGS_DB_DSN database must match the baseline target", ErrContractMismatch)
	}
	return nil
}

func postgresDatabaseName(dsn string) (string, error) {
	if dsn == "" {
		return "", errors.New("is empty")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" {
		return "", errors.New("must be a PostgreSQL URL with a host and database")
	}
	for key := range parsed.Query() {
		if strings.EqualFold(key, "database") || strings.EqualFold(key, "dbname") {
			return "", errors.New("must not contain database query parameters")
		}
	}
	database, err := url.PathUnescape(strings.TrimPrefix(parsed.EscapedPath(), "/"))
	if err != nil || database == "" || strings.Contains(database, "/") {
		return "", errors.New("must contain exactly one database name")
	}
	return database, nil
}

// ValidateSecretFile requires a non-symlink regular .env.local file with mode 0600.
func ValidateSecretFile(filename string) error {
	if filepath.Base(filename) != ".env.local" {
		return errors.New("secret file must be named .env.local")
	}
	info, err := os.Lstat(filename)
	if err != nil {
		return fmt.Errorf("inspect secret file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("secret file must not be a symbolic link")
	}
	if !info.Mode().IsRegular() {
		return errors.New("secret file must be a regular file")
	}
	if info.Mode().Perm() != 0o600 {
		return errors.New("secret file must have permissions 0600")
	}
	return nil
}

// WriteEMQXBootstrapSecret atomically publishes the private EMQX bootstrap
// credential consumed by the baseline Compose profile.
func WriteEMQXBootstrapSecret(runtimeRoot, apiKey, secretKey string) error {
	if err := validateEMQXBootstrapField("EMQX API key", apiKey); err != nil {
		return err
	}
	if err := validateEMQXBootstrapField("EMQX secret key", secretKey); err != nil {
		return err
	}
	root, err := filepath.Abs(runtimeRoot)
	if err != nil {
		return fmt.Errorf("resolve runtime root: %w", err)
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("inspect runtime root: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return errors.New("runtime root must be a non-symlink directory")
	}
	secretDirectory := filepath.Join(root, "secrets")
	if err := os.Mkdir(secretDirectory, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("create secret directory: %w", err)
	}
	directoryInfo, err := os.Lstat(secretDirectory)
	if err != nil {
		return fmt.Errorf("inspect secret directory: %w", err)
	}
	if directoryInfo.Mode()&os.ModeSymlink != 0 || !directoryInfo.IsDir() || directoryInfo.Mode().Perm() != 0o700 {
		return errors.New("secret directory must be a non-symlink directory with permissions 0700")
	}
	filename := filepath.Join(secretDirectory, "emqx-api-key.conf")
	if info, inspectErr := os.Lstat(filename); inspectErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("EMQX bootstrap secret target must be a non-symlink regular file")
		}
	} else if !errors.Is(inspectErr, fs.ErrNotExist) {
		return fmt.Errorf("inspect EMQX bootstrap secret target: %w", inspectErr)
	}
	content := []byte(apiKey + ":" + secretKey + "\n")
	if err := writeFileAtomicallyWithOwnership(filename, content, 0o600, func(path string) error {
		var stat unix.Stat_t
		if err := unix.Stat(path, &stat); err != nil {
			return fmt.Errorf("inspect EMQX bootstrap secret ownership: %w", err)
		}
		if err := ensureEMQXBootstrapOwnership(stat.Uid, stat.Gid, uint32(os.Geteuid()), 1000, 1000, func() error {
			return unix.Chown(path, 1000, 1000)
		}); err != nil {
			return err
		}
		if err := unix.Stat(path, &stat); err != nil {
			return fmt.Errorf("inspect EMQX bootstrap secret ownership: %w", err)
		}
		if stat.Uid != 1000 || stat.Gid != 1000 {
			return errors.New("EMQX bootstrap secret ownership mismatch")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("write EMQX bootstrap secret: %w", err)
	}
	return nil
}

// PrepareEMQXBootstrapSecret validates the next credential and invalidates the
// current one before a new stage can be published.
func PrepareEMQXBootstrapSecret(runtimeRoot, apiKey, secretKey string) error {
	if err := validateEMQXBootstrapField("EMQX API key", apiKey); err != nil {
		return err
	}
	if err := validateEMQXBootstrapField("EMQX secret key", secretKey); err != nil {
		return err
	}
	root, secretDirectory, filename, err := validateEMQXBootstrapPaths(runtimeRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create runtime root: %w", err)
	}
	if err := os.Mkdir(secretDirectory, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("create secret directory: %w", err)
	}
	directoryInfo, err := os.Lstat(secretDirectory)
	if err != nil {
		return fmt.Errorf("inspect secret directory: %w", err)
	}
	if directoryInfo.Mode()&os.ModeSymlink != 0 || !directoryInfo.IsDir() || directoryInfo.Mode().Perm() != 0o700 {
		return errors.New("secret directory must be a non-symlink directory with permissions 0700")
	}
	if info, inspectErr := os.Lstat(filename); inspectErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("EMQX bootstrap secret target must be a non-symlink regular file")
		}
		if err := os.Remove(filename); err != nil {
			return fmt.Errorf("remove old EMQX bootstrap secret: %w", err)
		}
		if err := syncDirectory(secretDirectory); err != nil {
			return fmt.Errorf("sync secret directory: %w", err)
		}
	} else if !errors.Is(inspectErr, fs.ErrNotExist) {
		return fmt.Errorf("inspect EMQX bootstrap secret target: %w", inspectErr)
	}
	return nil
}

func validateEMQXBootstrapField(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", name)
	}
	for index := 0; index < len(value); index++ {
		if value[index] <= 0x1f || value[index] == 0x7f || value[index] == ':' {
			return fmt.Errorf("%s contains an unsupported character", name)
		}
	}
	return nil
}

func validateEMQXBootstrapPaths(runtimeRoot string) (string, string, string, error) {
	root, err := filepath.Abs(runtimeRoot)
	if err != nil {
		return "", "", "", fmt.Errorf("resolve runtime root: %w", err)
	}
	if info, inspectErr := os.Lstat(root); inspectErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", "", "", errors.New("runtime root must be a non-symlink directory")
		}
	} else if !errors.Is(inspectErr, fs.ErrNotExist) {
		return "", "", "", fmt.Errorf("inspect runtime root: %w", inspectErr)
	}
	secretDirectory := filepath.Join(root, "secrets")
	return root, secretDirectory, filepath.Join(secretDirectory, "emqx-api-key.conf"), nil
}

// Stage copies only the contract binaries, baseline configurations, and explicit role assets.
func Stage(request StageRequest) (StageResult, error) {
	return stageWithOperations(request, newStageOperations())
}

func stageWithOperations(request StageRequest, operations stageOperations) (StageResult, error) {
	if err := ValidateContract(request.Contract); err != nil {
		return StageResult{}, fmt.Errorf("%w: invalid contract: %v", ErrContractMismatch, err)
	}
	repoRoot, err := filepath.Abs(request.RepoRoot)
	if err != nil {
		return StageResult{}, fmt.Errorf("resolve repository root: %w", err)
	}
	repoFD, err := openRepositoryRoot(repoRoot)
	if err != nil {
		return StageResult{}, err
	}
	defer unix.Close(repoFD)
	runtimeRoot, err := filepath.Abs(request.RuntimeRoot)
	if err != nil {
		return StageResult{}, fmt.Errorf("resolve runtime root: %w", err)
	}
	if err := os.MkdirAll(runtimeRoot, 0o755); err != nil {
		return StageResult{}, fmt.Errorf("create runtime root: %w", err)
	}
	temporaryStage, err := os.MkdirTemp(runtimeRoot, ".stage.tmp-")
	if err != nil {
		return StageResult{}, fmt.Errorf("create staging temporary directory: %w", err)
	}
	temporaryOwned := true
	defer func() {
		if temporaryOwned {
			_ = operations.removeAll(temporaryStage)
		}
	}()

	result := StageResult{Contexts: make([]StageContext, 0, len(request.Contract.Roles))}
	for _, role := range request.Contract.Roles {
		if err := stageRole(repoFD, temporaryStage, role); err != nil {
			return StageResult{}, err
		}
		result.Contexts = append(result.Contexts, StageContext{Role: role.ID, Path: filepath.Join(runtimeRoot, "stage", role.ID)})
	}
	manifest, err := collectStageManifest(temporaryStage)
	if err != nil {
		return StageResult{}, err
	}
	if err := writeStageManifest(filepath.Join(temporaryStage, "stage-manifest.json"), manifest); err != nil {
		return StageResult{}, err
	}
	if err := syncTree(temporaryStage); err != nil {
		return StageResult{}, err
	}
	if err := operations.syncDirectory(runtimeRoot); err != nil {
		return StageResult{}, err
	}
	published, err := replaceStage(runtimeRoot, temporaryStage, operations)
	if published {
		temporaryOwned = false
	}
	if err != nil {
		return StageResult{}, err
	}
	return result, nil
}

func stageRole(repoFD int, stageRoot string, role RoleContract) error {
	assets, ok := roleStageAssetPaths[role.ID]
	if !ok {
		return fmt.Errorf("%w: no staging assets for role %q", ErrContractMismatch, role.ID)
	}
	contextRoot := filepath.Join(stageRoot, role.ID)
	for _, directory := range []string{"bin", "etc", "oss"} {
		if err := os.MkdirAll(filepath.Join(contextRoot, directory), 0o755); err != nil {
			return fmt.Errorf("create %s %s: %w", role.ID, directory, err)
		}
	}
	if err := copyRepositoryFile(repoFD, filepath.Join("backend", "bin", role.Binary), filepath.Join(contextRoot, "bin", role.Binary)); err != nil {
		return fmt.Errorf("stage %s binary: %w", role.ID, err)
	}
	if err := copyRepositoryFile(repoFD, role.Config, filepath.Join(contextRoot, "etc", filepath.Base(role.Config))); err != nil {
		return fmt.Errorf("stage %s configuration: %w", role.ID, err)
	}
	for _, asset := range []struct {
		name string
		src  string
	}{
		{name: "i18n", src: assets.i18n},
		{name: "init", src: assets.init},
	} {
		if asset.src == "" {
			continue
		}
		if err := copyRepositoryDirectory(repoFD, asset.src, filepath.Join(contextRoot, "etc", asset.name)); err != nil {
			return fmt.Errorf("stage %s %s: %w", role.ID, asset.name, err)
		}
	}
	return nil
}

func copyRepositoryFile(repoFD int, relative, destination string) error {
	fd, info, err := openRepositoryEntry(repoFD, relative, false)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), relative)
	if file == nil {
		unix.Close(fd)
		return errors.New("create source file descriptor")
	}
	defer file.Close()
	if info.Mode&unix.S_IFMT != unix.S_IFREG {
		return errors.New("source must be a non-symlink regular file")
	}
	return copyFileWithMode(file, destination, fs.FileMode(info.Mode&0o777))
}

func copyRepositoryDirectory(repoFD int, relative, destination string) error {
	fd, info, err := openRepositoryEntry(repoFD, relative, true)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	if info.Mode&unix.S_IFMT != unix.S_IFDIR {
		return errors.New("source must be a non-symlink directory")
	}
	return copyOpenedDirectory(fd, destination)
}

func openRepositoryRoot(repoRoot string) (int, error) {
	fd, err := unix.Open(repoRoot, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, fmt.Errorf("open repository root: %w", err)
	}
	return fd, nil
}

func openRepositoryEntry(rootFD int, relative string, wantDirectory bool) (int, unix.Stat_t, error) {
	if filepath.IsAbs(relative) {
		return -1, unix.Stat_t{}, errors.New("repository path must be relative")
	}
	parts := strings.Split(filepath.Clean(relative), string(os.PathSeparator))
	if len(parts) == 0 || parts[0] == "." {
		return -1, unix.Stat_t{}, errors.New("repository path must not be empty")
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return -1, unix.Stat_t{}, errors.New("repository path escapes repository root")
		}
	}
	current, err := unix.Dup(rootFD)
	if err != nil {
		return -1, unix.Stat_t{}, err
	}
	for index, part := range parts {
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC
		if index < len(parts)-1 || wantDirectory {
			flags |= unix.O_DIRECTORY
		}
		if index == len(parts)-1 {
			flags |= unix.O_NONBLOCK
		}
		next, err := unix.Openat(current, part, flags, 0)
		unix.Close(current)
		if err != nil {
			return -1, unix.Stat_t{}, fmt.Errorf("open repository entry: %w", err)
		}
		current = next
	}
	var stat unix.Stat_t
	if err := unix.Fstat(current, &stat); err != nil {
		unix.Close(current)
		return -1, unix.Stat_t{}, err
	}
	return current, stat, nil
}

func copyOpenedDirectory(directoryFD int, destination string) error {
	listingFD, err := unix.Dup(directoryFD)
	if err != nil {
		return err
	}
	listing := os.NewFile(uintptr(listingFD), "directory")
	if listing == nil {
		unix.Close(listingFD)
		return errors.New("create directory descriptor")
	}
	entries, err := listing.ReadDir(-1)
	listing.Close()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		childFD, err := unix.Openat(directoryFD, name, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return fmt.Errorf("open source entry: %w", err)
		}
		var stat unix.Stat_t
		if err := unix.Fstat(childFD, &stat); err != nil {
			unix.Close(childFD)
			return err
		}
		target := filepath.Join(destination, name)
		switch stat.Mode & unix.S_IFMT {
		case unix.S_IFDIR:
			err = copyOpenedDirectory(childFD, target)
			unix.Close(childFD)
		case unix.S_IFREG:
			file := os.NewFile(uintptr(childFD), name)
			if file == nil {
				unix.Close(childFD)
				return errors.New("create source file descriptor")
			}
			err = copyFileWithMode(file, target, fs.FileMode(stat.Mode&0o777))
			file.Close()
		default:
			unix.Close(childFD)
			return errors.New("source entry must be regular")
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func copyFileWithMode(input *os.File, destination string, mode fs.FileMode) error {
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return err
	}
	if err := output.Chmod(mode); err != nil {
		output.Close()
		return err
	}
	if err := output.Sync(); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}

func collectStageManifest(stageRoot string) (stageManifest, error) {
	manifest := stageManifest{SchemaVersion: 1}
	err := filepath.WalkDir(stageRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("staged entry %q must be a regular file", path)
		}
		relative, err := filepath.Rel(stageRoot, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		digest, err := sha256File(path)
		if err != nil {
			return err
		}
		manifest.Files = append(manifest.Files, stagedFile{
			Path:   filepath.ToSlash(relative),
			Mode:   fmt.Sprintf("%04o", info.Mode().Perm()),
			SHA256: digest,
		})
		return nil
	})
	if err != nil {
		return stageManifest{}, fmt.Errorf("collect stage manifest: %w", err)
	}
	return manifest, nil
}

func sha256File(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func writeStageManifest(filename string, manifest stageManifest) error {
	data, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode stage manifest: %w", err)
	}
	return writeFileAtomically(filename, append(data, '\n'), 0o644)
}

func writeFileAtomically(filename string, data []byte, mode fs.FileMode) error {
	return writeFileAtomicallyWithOwnership(filename, data, mode, nil)
}

func writeFileAtomicallyWithOwnership(filename string, data []byte, mode fs.FileMode, prepare func(string) error) error {
	directory := filepath.Dir(filename)
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(filename)+".tmp-")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if prepare != nil {
		if err := prepare(temporaryName); err != nil {
			temporary.Close()
			return err
		}
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, filename); err != nil {
		return err
	}
	return syncDirectory(directory)
}

func ensureEMQXBootstrapOwnership(currentUID, currentGID, effectiveUID, targetUID, targetGID uint32, chown func() error) error {
	if currentUID == targetUID && currentGID == targetGID {
		return nil
	}
	if effectiveUID != 0 {
		return errors.New("EMQX bootstrap secret ownership cannot be corrected")
	}
	if err := chown(); err != nil {
		return err
	}
	return nil
}

func replaceStage(runtimeRoot, temporaryStage string, operations stageOperations) (bool, error) {
	target := filepath.Join(runtimeRoot, "stage")
	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return false, errors.New("existing stage must be a non-symlink directory")
		}
		if err := operations.exchangeDirectories(target, temporaryStage); err != nil {
			return false, fmt.Errorf("atomically exchange staged runtime: %w", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("inspect existing stage: %w", err)
	} else if err := os.Rename(temporaryStage, target); err != nil {
		return false, fmt.Errorf("activate first staged runtime: %w", err)
	}
	if err := operations.syncDirectory(runtimeRoot); err != nil {
		return true, fmt.Errorf("sync published stage: %w", err)
	}
	if _, err := os.Lstat(temporaryStage); err == nil {
		if err := operations.removeAll(temporaryStage); err != nil {
			return true, fmt.Errorf("remove replaced stage: %w", err)
		}
		if err := operations.syncDirectory(runtimeRoot); err != nil {
			return true, fmt.Errorf("sync stage cleanup: %w", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return true, fmt.Errorf("inspect replaced stage: %w", err)
	}
	return true, nil
}

func ensureDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("must be a directory")
	}
	return nil
}

func syncTree(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		return syncDirectory(path)
	})
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
