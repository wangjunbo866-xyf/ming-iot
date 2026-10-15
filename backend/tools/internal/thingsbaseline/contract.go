// Package thingsbaseline defines the deterministic source contract for the
// L0 Things baseline migration.
package thingsbaseline

import (
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var lowercaseHex = regexp.MustCompile(`^[0-9a-f]+$`)

// Lock freezes every input and expected count used by the migration.
type Lock struct {
	SchemaVersion int               `yaml:"schemaVersion"`
	LoopID        string            `yaml:"loopId"`
	Source        SourceContract    `yaml:"source"`
	Target        TargetContract    `yaml:"target"`
	Transforms    TransformContract `yaml:"transforms"`
	Contracts     ContractCounts    `yaml:"contracts"`
	Security      SecurityContract  `yaml:"security"`
}

// SourceContract identifies immutable Git objects and the selected file set.
type SourceContract struct {
	Repository        string   `yaml:"repository"`
	Ref               string   `yaml:"ref"`
	Commit            string   `yaml:"commit"`
	Tree              string   `yaml:"tree"`
	Module            string   `yaml:"module"`
	TrackedFiles      int      `yaml:"trackedFiles"`
	SelectedFiles     int      `yaml:"selectedFiles"`
	SelectedBlobBytes int64    `yaml:"selectedBlobBytes"`
	ArchiveBytes      int64    `yaml:"archiveBytes"`
	ArchiveSHA256     string   `yaml:"archiveSha256"`
	Include           []string `yaml:"include"`
	Exclude           []string `yaml:"exclude"`
}

// TargetContract identifies the local destination and module substitutions.
type TargetContract struct {
	Directory        string `yaml:"directory"`
	Module           string `yaml:"module"`
	SaaSModule       string `yaml:"saasModule"`
	SaaSRelativePath string `yaml:"saasRelativePath"`
	ShareModule      string `yaml:"shareModule"`
	ShareVersion     string `yaml:"shareVersion"`
}

// TransformContract fixes the expected transformations for later tasks.
type TransformContract struct {
	CorePrefixCount                     int                  `yaml:"corePrefixCount"`
	ThingsPrefixCount                   int                  `yaml:"thingsPrefixCount"`
	CredentialLineCount                 int                  `yaml:"credentialLineCount"`
	CredentialFileCount                 int                  `yaml:"credentialFileCount"`
	CredentialActiveLineCount           int                  `yaml:"credentialActiveLineCount"`
	CredentialCommentedLineCount        int                  `yaml:"credentialCommentedLineCount"`
	CredentialPaths                     CredentialPathCounts `yaml:"credentialPaths"`
	PrivateKeyBlockCount                int                  `yaml:"privateKeyBlockCount"`
	VetFixCount                         int                  `yaml:"vetFixCount"`
	PrivateKeySourceSHA256              string               `yaml:"privateKeySourceSha256"`
	VetSourceSHA256                     string               `yaml:"vetSourceSha256"`
	SchemaFixtureIdentifierCount        int                  `yaml:"schemaFixtureIdentifierCount"`
	SchemaFixtureSourceSHA256           string               `yaml:"schemaFixtureSourceSha256"`
	SchemaFixtureOutputSHA256           string               `yaml:"schemaFixtureOutputSha256"`
	NonYAMLCredentialValueCount         int                  `yaml:"nonYamlCredentialValueCount"`
	CloudProtocolCredentialSourceSHA256 string               `yaml:"cloudProtocolCredentialSourceSha256"`
	CloudProtocolCredentialOutputSHA256 string               `yaml:"cloudProtocolCredentialOutputSha256"`
	ProductSecretSourceSHA256           string               `yaml:"productSecretSourceSha256"`
	ProductSecretOutputSHA256           string               `yaml:"productSecretOutputSha256"`
}

// CredentialPathCounts fixes the complete, ordered credential profile.
// A struct (rather than a map) keeps YAML fields and validation errors stable.
type CredentialPathCounts struct {
	DatabaseDSN                 int `yaml:"databaseDSN" json:"databaseDSN"`
	TSDBDSN                     int `yaml:"tsdbDSN" json:"tsdbDSN"`
	OssAccessKeyID              int `yaml:"ossAccessKeyID" json:"ossAccessKeyID"`
	OssAccessKeySecret          int `yaml:"ossAccessKeySecret" json:"ossAccessKeySecret"`
	AuthWhiteUsersPassword      int `yaml:"authWhiteUsersPassword" json:"authWhiteUsersPassword"`
	DevLinkOpenAPIAPIKey        int `yaml:"devLinkOpenApiApiKey" json:"devLinkOpenApiApiKey"`
	DevLinkOpenAPISecretKey     int `yaml:"devLinkOpenApiSecretKey" json:"devLinkOpenApiSecretKey"`
	DevLinkMQTTOpenAPIAPIKey    int `yaml:"devLinkMqttOpenApiApiKey" json:"devLinkMqttOpenApiApiKey"`
	DevLinkMQTTOpenAPISecretKey int `yaml:"devLinkMqttOpenApiSecretKey" json:"devLinkMqttOpenApiSecretKey"`
}

// ContractCounts fixes the source contracts preserved by later tasks.
type ContractCounts struct {
	APIFiles            int `yaml:"apiFiles" json:"apiFiles"`
	ProtoFiles          int `yaml:"protoFiles" json:"protoFiles"`
	GeneratedProtoFiles int `yaml:"generatedProtoFiles" json:"generatedProtoFiles"`
	TestFiles           int `yaml:"testFiles" json:"testFiles"`
}

// SecurityContract fixes the source-file size limit.
type SecurityContract struct {
	MaxFileBytes int64 `yaml:"maxFileBytes"`
}

// LoadLock decodes one strict YAML document and validates its contract.
func LoadLock(filename string) (*Lock, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open lock %q: %w", filename, err)
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	decoder.KnownFields(true)
	var lock Lock
	if err := decoder.Decode(&lock); err != nil {
		return nil, fmt.Errorf("decode lock %q: %w", filename, err)
	}

	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return nil, fmt.Errorf("decode lock %q: multiple YAML documents", filename)
	} else if err != io.EOF {
		return nil, fmt.Errorf("decode lock %q: trailing YAML: %w", filename, err)
	}
	if err := ValidateLock(&lock); err != nil {
		return nil, fmt.Errorf("validate lock %q: %w", filename, err)
	}
	return &lock, nil
}

// ValidateLock rejects mutable, ambiguous, unsafe, or incomplete contracts.
func ValidateLock(lock *Lock) error {
	if lock == nil {
		return fmt.Errorf("lock is nil")
	}
	if lock.SchemaVersion != 1 {
		return fmt.Errorf("schemaVersion must be 1")
	}
	if lock.LoopID == "" {
		return fmt.Errorf("loopId must not be empty")
	}
	if lock.Source.Repository == "" || lock.Source.Module == "" {
		return fmt.Errorf("source repository and module must not be empty")
	}
	if err := validateGitRef(lock.Source.Ref); err != nil {
		return err
	}
	if !isLowercaseHex(lock.Source.Commit, 40) {
		return fmt.Errorf("source.commit must be 40 lowercase hex characters")
	}
	if !isLowercaseHex(lock.Source.Tree, 40) {
		return fmt.Errorf("source.tree must be 40 lowercase hex characters")
	}
	if !isLowercaseHex(lock.Source.ArchiveSHA256, 64) {
		return fmt.Errorf("source.archiveSha256 must be 64 lowercase hex characters")
	}
	if lock.Source.TrackedFiles <= 0 {
		return fmt.Errorf("source.trackedFiles must be positive")
	}
	if lock.Source.SelectedFiles <= 0 || lock.Source.SelectedFiles > lock.Source.TrackedFiles {
		return fmt.Errorf("source.selectedFiles must be positive and no greater than trackedFiles")
	}
	if lock.Source.SelectedBlobBytes <= 0 {
		return fmt.Errorf("source.selectedBlobBytes must be positive")
	}
	if lock.Source.ArchiveBytes <= 0 {
		return fmt.Errorf("source.archiveBytes must be positive")
	}
	if err := validatePathRoots("include", lock.Source.Include); err != nil {
		return err
	}
	if err := validatePathRoots("exclude", lock.Source.Exclude); err != nil {
		return err
	}
	for _, include := range lock.Source.Include {
		for _, exclude := range lock.Source.Exclude {
			if rootsOverlap(include, exclude) {
				return fmt.Errorf("source.include %q overlaps exclude %q", include, exclude)
			}
		}
	}
	if err := validateGitPath("target directory", lock.Target.Directory); err != nil {
		return err
	}
	if lock.Target.Module == "" || strings.Contains(strings.ToLower(lock.Target.Module), "unitedrhino") || lock.Target.Module == lock.Source.Module {
		return fmt.Errorf("target module must differ from source and must not be UnitedRhino")
	}
	if lock.Target.SaaSModule == "" || lock.Target.SaaSRelativePath == "" || lock.Target.ShareModule == "" || lock.Target.ShareVersion == "" {
		return fmt.Errorf("target module contracts must not be empty")
	}
	if lock.Target.SaaSRelativePath != "../saas" {
		return fmt.Errorf("target.saasRelativePath must be ../saas")
	}
	if err := validatePositiveCount("transforms.corePrefixCount", lock.Transforms.CorePrefixCount); err != nil {
		return err
	}
	if err := validatePositiveCount("transforms.thingsPrefixCount", lock.Transforms.ThingsPrefixCount); err != nil {
		return err
	}
	if err := validatePositiveCount("transforms.credentialLineCount", lock.Transforms.CredentialLineCount); err != nil {
		return err
	}
	if err := validateCredentialProfile(lock.Transforms, lock.Source.SelectedFiles); err != nil {
		return err
	}
	if err := validatePositiveCount("transforms.privateKeyBlockCount", lock.Transforms.PrivateKeyBlockCount); err != nil {
		return err
	}
	if err := validatePositiveCount("transforms.vetFixCount", lock.Transforms.VetFixCount); err != nil {
		return err
	}
	if err := validatePositiveCount("transforms.schemaFixtureIdentifierCount", lock.Transforms.SchemaFixtureIdentifierCount); err != nil {
		return err
	}
	if !isLowercaseHex(lock.Transforms.PrivateKeySourceSHA256, 64) || !isLowercaseHex(lock.Transforms.VetSourceSHA256, 64) || !isLowercaseHex(lock.Transforms.SchemaFixtureSourceSHA256, 64) {
		return fmt.Errorf("transform source SHA-256 values must be 64 lowercase hex characters")
	}
	if !isLowercaseHex(lock.Transforms.SchemaFixtureOutputSHA256, 64) {
		return fmt.Errorf("transform output SHA-256 values must be 64 lowercase hex characters")
	}
	if lock.Transforms.NonYAMLCredentialValueCount != 5 {
		return fmt.Errorf("transforms.nonYamlCredentialValueCount must be 5")
	}
	if !isLowercaseHex(lock.Transforms.CloudProtocolCredentialSourceSHA256, 64) ||
		!isLowercaseHex(lock.Transforms.CloudProtocolCredentialOutputSHA256, 64) ||
		!isLowercaseHex(lock.Transforms.ProductSecretSourceSHA256, 64) ||
		!isLowercaseHex(lock.Transforms.ProductSecretOutputSHA256, 64) {
		return fmt.Errorf("non-YAML credential SHA-256 values must be 64 lowercase hex characters")
	}
	if err := validatePositiveCount("contracts.apiFiles", lock.Contracts.APIFiles); err != nil {
		return err
	}
	if err := validatePositiveCount("contracts.protoFiles", lock.Contracts.ProtoFiles); err != nil {
		return err
	}
	if err := validatePositiveCount("contracts.generatedProtoFiles", lock.Contracts.GeneratedProtoFiles); err != nil {
		return err
	}
	if err := validatePositiveCount("contracts.testFiles", lock.Contracts.TestFiles); err != nil {
		return err
	}
	if lock.Security.MaxFileBytes <= 0 {
		return fmt.Errorf("security.maxFileBytes must be positive")
	}
	return nil
}

func validateCredentialProfile(transforms TransformContract, selectedFiles int) error {
	if transforms.CredentialFileCount <= 0 {
		return fmt.Errorf("transforms.credentialFileCount must be positive")
	}
	if transforms.CredentialFileCount > selectedFiles {
		return fmt.Errorf("transforms.credentialFileCount must not exceed source.selectedFiles")
	}
	if transforms.CredentialActiveLineCount < 0 {
		return fmt.Errorf("transforms.credentialActiveLineCount must not be negative")
	}
	if transforms.CredentialCommentedLineCount < 0 {
		return fmt.Errorf("transforms.credentialCommentedLineCount must not be negative")
	}
	if transforms.CredentialActiveLineCount+transforms.CredentialCommentedLineCount != transforms.CredentialLineCount {
		return fmt.Errorf("transforms.credentialActiveLineCount + transforms.credentialCommentedLineCount must equal transforms.credentialLineCount")
	}
	paths := []struct {
		name  string
		count int
	}{
		{"databaseDSN", transforms.CredentialPaths.DatabaseDSN},
		{"tsdbDSN", transforms.CredentialPaths.TSDBDSN},
		{"ossAccessKeyID", transforms.CredentialPaths.OssAccessKeyID},
		{"ossAccessKeySecret", transforms.CredentialPaths.OssAccessKeySecret},
		{"authWhiteUsersPassword", transforms.CredentialPaths.AuthWhiteUsersPassword},
		{"devLinkOpenApiApiKey", transforms.CredentialPaths.DevLinkOpenAPIAPIKey},
		{"devLinkOpenApiSecretKey", transforms.CredentialPaths.DevLinkOpenAPISecretKey},
		{"devLinkMqttOpenApiApiKey", transforms.CredentialPaths.DevLinkMQTTOpenAPIAPIKey},
		{"devLinkMqttOpenApiSecretKey", transforms.CredentialPaths.DevLinkMQTTOpenAPISecretKey},
	}
	total := 0
	for _, path := range paths {
		if path.count < 0 {
			return fmt.Errorf("transforms.credentialPaths.%s must not be negative", path.name)
		}
		total += path.count
	}
	if total != transforms.CredentialLineCount {
		return fmt.Errorf("transforms.credentialPaths sum must equal transforms.credentialLineCount")
	}
	return nil
}

func isLowercaseHex(value string, length int) bool {
	return len(value) == length && lowercaseHex.MatchString(value)
}

func validatePositiveCount(field string, count int) error {
	if count <= 0 {
		return fmt.Errorf("%s must be positive", field)
	}
	return nil
}

func validateGitRef(value string) error {
	if !strings.HasPrefix(value, "refs/remotes/") {
		return fmt.Errorf("source.ref must start with refs/remotes/")
	}
	if strings.Contains(value, `\`) || path.Clean(value) != value || containsASCIIControl(value) ||
		strings.Contains(value, "..") || strings.Contains(value, "@{") || strings.HasSuffix(value, ".") ||
		strings.ContainsAny(value, " ~^:?*[") {
		return fmt.Errorf("unsafe source.ref %q", value)
	}
	for _, element := range strings.Split(value, "/") {
		if element == "" || element == "." || element == ".." || element == "@" ||
			strings.HasPrefix(element, ".") || strings.HasPrefix(element, "-") || strings.HasSuffix(element, ".lock") {
			return fmt.Errorf("unsafe source.ref %q", value)
		}
	}
	return nil
}

func validatePathRoots(kind string, roots []string) error {
	if len(roots) == 0 {
		return fmt.Errorf("source.%s must not be empty", kind)
	}
	for i, root := range roots {
		if err := validateGitPath(kind, root); err != nil {
			return err
		}
		for j := 0; j < i; j++ {
			if roots[j] == root {
				return fmt.Errorf("duplicate %s %q", kind, root)
			}
			if rootsOverlap(roots[j], root) {
				return fmt.Errorf("overlapping %s roots %q and %q", kind, roots[j], root)
			}
		}
	}
	return nil
}

// validateGitPath applies Git's slash-separated tree path semantics, never
// filepath.Clean, so host-specific backslash behavior cannot widen the input.
func validateGitPath(kind, value string) error {
	if value == "" || strings.Contains(value, `\`) || path.IsAbs(value) || path.Clean(value) != value || containsASCIIControl(value) {
		return fmt.Errorf("unsafe %s path %q", kind, value)
	}
	for _, element := range strings.Split(value, "/") {
		if element == "" || element == "." || element == ".." {
			return fmt.Errorf("unsafe %s path %q", kind, value)
		}
	}
	return nil
}

func containsASCIIControl(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] <= 0x1f || value[i] == 0x7f {
			return true
		}
	}
	return false
}

func rootsOverlap(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}
