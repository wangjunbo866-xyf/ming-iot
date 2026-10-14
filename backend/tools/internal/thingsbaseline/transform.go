package thingsbaseline

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/format"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/mod/modfile"
	"gopkg.in/yaml.v3"
)

var credentialEnv = map[string]string{
	"Database.DSN":                   "${THINGS_DB_DSN}",
	"TSDB.DSN":                       "${THINGS_TSDB_DSN}",
	"OssConf.AccessKeyID":            "${THINGS_OSS_ACCESS_KEY_ID}",
	"OssConf.AccessKeySecret":        "${THINGS_OSS_ACCESS_KEY_SECRET}",
	"AuthWhite.Users[].Password":     "${THINGS_INTERNAL_AUTH_PASSWORD}",
	"DevLink.OpenApi.ApiKey":         "${THINGS_EMQX_API_KEY}",
	"DevLink.OpenApi.SecretKey":      "${THINGS_EMQX_SECRET_KEY}",
	"DevLink.Mqtt.OpenApi.ApiKey":    "${THINGS_EMQX_API_KEY}",
	"DevLink.Mqtt.OpenApi.SecretKey": "${THINGS_EMQX_SECRET_KEY}",
}

var sensitiveCredentialKeys = map[string]bool{
	"DSN": true, "AccessKeyID": true, "AccessKeySecret": true,
	"Password": true, "ApiKey": true, "SecretKey": true,
}

var sensitiveCredentialKeyOrder = []string{
	"AccessKeySecret", "AccessKeyID", "SecretKey", "Password", "ApiKey", "DSN",
}

const (
	privateKeySourcePath            = "service/dgsvr/internal/logic/deviceauth/loginAuthLogic.go"
	vetSourcePath                   = "service/apisvr/internal/logic/things/device/edge/sendLogic.go"
	schemaFixtureSourcePath         = "share/domain/schema/schema_test.go"
	cloudProtocolCredentialPath     = "sdk/protocol/cloudProtocol.go"
	productSecretPath               = "service/apisvr/etc/init/product/产品列表.json"
	cloudAccessKeyIDPlaceholder     = "${THINGS_CLOUD_ACCESS_KEY_ID}"
	cloudAccessKeySecretPlaceholder = "${THINGS_CLOUD_ACCESS_KEY_SECRET}"
	productSecretPlaceholder        = "${THINGS_INIT_PRODUCT_SECRET}"
)

var (
	cloudAccessKeyIDPattern     = regexp.MustCompile(`("accessKeyID"[[:space:]]*:[[:space:]]*)"(?:\\.|[^"\\])*"`)
	cloudAccessKeySecretPattern = regexp.MustCompile(`("accessKeySecret"[[:space:]]*:[[:space:]]*)"(?:\\.|[^"\\])*"`)
	productSecretPattern        = regexp.MustCompile(`("Secret"[[:space:]]*:[[:space:]]*)"(?:\\.|[^"\\])*"`)
)

// RenderedFile is one transformed regular source file.
type RenderedFile struct {
	Mode string
	Data []byte
}

// TransformReport records the exact transformations consumed from the lock.
type TransformReport struct {
	CorePrefixes             int `json:"corePrefixCount"`
	ThingsPrefixes           int `json:"thingsPrefixCount"`
	CredentialLines          int `json:"credentialLineCount"`
	PrivateKeyBlocks         int `json:"privateKeyBlockCount"`
	VetFixes                 int `json:"vetFixCount"`
	SchemaFixtureIdentifiers int `json:"schemaFixtureIdentifierCount"`
	NonYAMLCredentialValues  int `json:"nonYamlCredentialValueCount"`
}

// TransformTree applies the declared deterministic transformations without
// mutating the input map or its SourceFile byte slices.
func TransformTree(files map[string]SourceFile, lock *Lock) (map[string]RenderedFile, TransformReport, error) {
	if err := ValidateLock(lock); err != nil {
		return nil, TransformReport{}, fmt.Errorf("validate transform lock: %w", err)
	}
	if err := validateTransformInput(files); err != nil {
		return nil, TransformReport{}, err
	}
	privateFile, ok := files[privateKeySourcePath]
	if !ok {
		return nil, TransformReport{}, fmt.Errorf("missing private-key source %q", privateKeySourcePath)
	}
	vetFile, ok := files[vetSourcePath]
	if !ok {
		return nil, TransformReport{}, fmt.Errorf("missing vet source %q", vetSourcePath)
	}
	schemaFixtureFile, ok := files[schemaFixtureSourcePath]
	if !ok {
		return nil, TransformReport{}, fmt.Errorf("missing schema fixture source %q", schemaFixtureSourcePath)
	}
	cloudCredentialFile, ok := files[cloudProtocolCredentialPath]
	if !ok {
		return nil, TransformReport{}, fmt.Errorf("missing cloud protocol credential source %q", cloudProtocolCredentialPath)
	}
	productSecretFile, ok := files[productSecretPath]
	if !ok {
		return nil, TransformReport{}, fmt.Errorf("missing product secret source %q", productSecretPath)
	}

	// These SHA-locked transformations deliberately run on the raw blobs before
	// module rewrites. They are the integrity boundary for every source-specific
	// edit in this stage.
	privateData, privateCount, err := RemoveEmbeddedPrivateKey(privateFile.Data, lock.Transforms.PrivateKeySourceSHA256)
	if err != nil {
		return nil, TransformReport{}, fmt.Errorf("transform %s: %w", privateKeySourcePath, err)
	}
	vetData, vetCount, err := FixDynamicFormat(vetFile.Data, lock.Transforms.VetSourceSHA256)
	if err != nil {
		return nil, TransformReport{}, fmt.Errorf("transform %s: %w", vetSourcePath, err)
	}
	schemaFixtureData, schemaFixtureCount, err := FixSchemaFixtureIdentifiers(
		schemaFixtureFile.Data,
		lock.Transforms.SchemaFixtureSourceSHA256,
		lock.Transforms.SchemaFixtureOutputSHA256,
		lock.Transforms.SchemaFixtureIdentifierCount,
	)
	if err != nil {
		return nil, TransformReport{}, fmt.Errorf("transform %s: %w", schemaFixtureSourcePath, err)
	}
	cloudCredentialData, cloudCredentialCount, err := SanitizeCloudProtocolCredentials(
		cloudCredentialFile.Data,
		lock.Transforms.CloudProtocolCredentialSourceSHA256,
		lock.Transforms.CloudProtocolCredentialOutputSHA256,
	)
	if err != nil {
		return nil, TransformReport{}, fmt.Errorf("transform %s: %w", cloudProtocolCredentialPath, err)
	}
	productSecretData, productSecretCount, err := SanitizeProductSecrets(
		productSecretFile.Data,
		lock.Transforms.ProductSecretSourceSHA256,
		lock.Transforms.ProductSecretOutputSHA256,
	)
	if err != nil {
		return nil, TransformReport{}, fmt.Errorf("transform %s: %w", productSecretPath, err)
	}
	if privateCount != lock.Transforms.PrivateKeyBlockCount {
		return nil, TransformReport{}, fmt.Errorf("transforms.privateKeyBlockCount = %d, want %d", privateCount, lock.Transforms.PrivateKeyBlockCount)
	}
	if vetCount != lock.Transforms.VetFixCount {
		return nil, TransformReport{}, fmt.Errorf("transforms.vetFixCount = %d, want %d", vetCount, lock.Transforms.VetFixCount)
	}
	if schemaFixtureCount != lock.Transforms.SchemaFixtureIdentifierCount {
		return nil, TransformReport{}, fmt.Errorf("transforms.schemaFixtureIdentifierCount = %d, want %d", schemaFixtureCount, lock.Transforms.SchemaFixtureIdentifierCount)
	}
	nonYAMLCredentialCount := cloudCredentialCount + productSecretCount
	if nonYAMLCredentialCount != lock.Transforms.NonYAMLCredentialValueCount {
		return nil, TransformReport{}, fmt.Errorf("transforms.nonYamlCredentialValueCount = %d, want %d", nonYAMLCredentialCount, lock.Transforms.NonYAMLCredentialValueCount)
	}

	rendered, report, err := transformModules(files, lock)
	if err != nil {
		return nil, TransformReport{}, err
	}
	rendered[privateKeySourcePath] = RenderedFile{Mode: privateFile.Mode, Data: rewriteModulePrefixes(privateData, lock)}
	rendered[vetSourcePath] = RenderedFile{Mode: vetFile.Mode, Data: rewriteModulePrefixes(vetData, lock)}
	rendered[schemaFixtureSourcePath] = RenderedFile{Mode: schemaFixtureFile.Mode, Data: rewriteModulePrefixes(schemaFixtureData, lock)}
	rendered[cloudProtocolCredentialPath] = RenderedFile{Mode: cloudCredentialFile.Mode, Data: rewriteModulePrefixes(cloudCredentialData, lock)}
	rendered[productSecretPath] = RenderedFile{Mode: productSecretFile.Mode, Data: rewriteModulePrefixes(productSecretData, lock)}

	paths := make([]string, 0, len(rendered))
	for path := range rendered {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var credentialProfileActual credentialProfile
	for _, path := range paths {
		if !isYAMLPath(path) {
			continue
		}
		file := rendered[path]
		data, beforeProfile, err := analyzeCredentialYAML(path, file.Data, true)
		if err != nil {
			return nil, TransformReport{}, err
		}
		afterProfile, err := verifySanitizedCredentialYAML(path, data)
		if err != nil {
			return nil, TransformReport{}, err
		}
		if !credentialProfilesEqual(beforeProfile, afterProfile) {
			return nil, TransformReport{}, fmt.Errorf("%s: YAML credential profile changed during sanitation", path)
		}
		file.Data = append([]byte(nil), data...)
		rendered[path] = file
		credentialProfileActual.merge(afterProfile)
	}
	wantCredentialProfile := credentialProfileFromContract(lock.Transforms)
	if !credentialProfilesEqual(credentialProfileActual, wantCredentialProfile) {
		return nil, TransformReport{}, fmt.Errorf("credential profile = %+v, want locked profile %+v", credentialProfileActual, wantCredentialProfile)
	}
	report.CredentialLines = credentialProfileActual.total()
	if err := verifyTransformedTree(rendered, lock); err != nil {
		return nil, TransformReport{}, err
	}
	report.PrivateKeyBlocks = privateCount
	report.VetFixes = vetCount
	report.SchemaFixtureIdentifiers = schemaFixtureCount
	report.NonYAMLCredentialValues = nonYAMLCredentialCount
	return rendered, report, nil
}

func validateTransformInput(files map[string]SourceFile) error {
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		file := files[key]
		if key != file.Path {
			return fmt.Errorf("source map key %q does not equal SourceFile.Path %q", key, file.Path)
		}
		if file.Mode != "100644" && file.Mode != "100755" {
			return fmt.Errorf("source path %q has unsupported mode %q", file.Path, file.Mode)
		}
		if !utf8.Valid(file.Data) {
			return fmt.Errorf("source path %q is not valid UTF-8", file.Path)
		}
	}
	return nil
}

// transformModules is the module/go.mod-only stage. It is intentionally
// separate so its reduced synthetic fixtures cannot weaken TransformTree's
// fail-closed full-lock contract.
func transformModules(files map[string]SourceFile, lock *Lock) (map[string]RenderedFile, TransformReport, error) {
	if lock == nil {
		return nil, TransformReport{}, fmt.Errorf("transform lock is nil")
	}
	if err := validateTransformInput(files); err != nil {
		return nil, TransformReport{}, err
	}
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	coreNeedle := []byte("gitee.com/unitedrhino/core")
	thingsNeedle := []byte(lock.Source.Module)
	var report TransformReport
	for _, key := range keys {
		file := files[key]
		if file.Path == "go.sum" {
			continue
		}
		report.CorePrefixes += bytes.Count(file.Data, coreNeedle)
		report.ThingsPrefixes += bytes.Count(file.Data, thingsNeedle)
	}
	if report.CorePrefixes != lock.Transforms.CorePrefixCount {
		return nil, TransformReport{}, fmt.Errorf("transforms.corePrefixCount = %d, want %d", report.CorePrefixes, lock.Transforms.CorePrefixCount)
	}
	if report.ThingsPrefixes != lock.Transforms.ThingsPrefixCount {
		return nil, TransformReport{}, fmt.Errorf("transforms.thingsPrefixCount = %d, want %d", report.ThingsPrefixes, lock.Transforms.ThingsPrefixCount)
	}

	rendered := make(map[string]RenderedFile, len(files))
	for _, key := range keys {
		file := files[key]
		data := append([]byte(nil), file.Data...)
		if file.Path != "go.sum" {
			if file.Path == "go.mod" {
				var err error
				data, err = normalizeGoMod(data, lock)
				if err != nil {
					return nil, TransformReport{}, err
				}
			} else {
				data = rewriteModulePrefixes(data, lock)
			}
		}
		rendered[file.Path] = RenderedFile{Mode: file.Mode, Data: append([]byte(nil), data...)}
	}
	return rendered, report, nil
}

func rewriteModulePrefixes(data []byte, lock *Lock) []byte {
	out := bytes.ReplaceAll(data, []byte("gitee.com/unitedrhino/core"), []byte(lock.Target.SaaSModule))
	out = bytes.ReplaceAll(out, []byte(lock.Source.Module), []byte(lock.Target.Module))
	return append([]byte(nil), out...)
}

func isYAMLPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")
}

func verifySanitizedCredentialYAML(path string, data []byte) (credentialProfile, error) {
	verified, profile, err := analyzeCredentialYAML(path, data, false)
	if err != nil {
		return credentialProfile{}, err
	}
	if !bytes.Equal(verified, data) {
		return credentialProfile{}, fmt.Errorf("%s: YAML credential reverse verification changed bytes", path)
	}
	return profile, nil
}

func verifySanitizedNonYAMLCredentialFile(path string, data []byte) (int, error) {
	switch path {
	case cloudProtocolCredentialPath:
		if len(cloudAccessKeyIDPattern.FindAllIndex(data, -1)) != 1 ||
			len(cloudAccessKeySecretPattern.FindAllIndex(data, -1)) != 1 ||
			bytes.Count(data, []byte(cloudAccessKeyIDPlaceholder)) != 1 ||
			bytes.Count(data, []byte(cloudAccessKeySecretPlaceholder)) != 1 {
			return 0, fmt.Errorf("%s: cloud protocol credential replacement count is invalid", path)
		}
		if err := validateCloudCredentialExample(data, true); err != nil {
			return 0, fmt.Errorf("%s: %w", path, err)
		}
		return 2, nil
	case productSecretPath:
		if len(productSecretPattern.FindAllIndex(data, -1)) != 3 || bytes.Count(data, []byte(productSecretPlaceholder)) != 3 {
			return 0, fmt.Errorf("%s: product secret replacement count is invalid", path)
		}
		if err := validateProductSecretSemantics(data, true); err != nil {
			return 0, fmt.Errorf("%s: %w", path, err)
		}
		return 3, nil
	default:
		return 0, nil
	}
}

func verifyTransformedTree(rendered map[string]RenderedFile, lock *Lock) error {
	vet := rendered[vetSourcePath].Data
	if bytes.Count(vet, []byte("AddDetailf(strProjectID)")) != 0 || bytes.Count(vet, []byte(`AddDetailf("%s", strProjectID)`)) != 1 {
		return fmt.Errorf("%s: vet format replacement verification failed", vetSourcePath)
	}
	paths := make([]string, 0, len(rendered))
	for path := range rendered {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var nonYAMLCredentialValues int
	for _, path := range paths {
		file := rendered[path]
		if containsPrivateKeyPEMMarker(file.Data) {
			return fmt.Errorf("%s: private-key marker remains after transform", path)
		}
		if path == "go.sum" {
			continue
		}
		if bytes.Contains(file.Data, []byte("gitee.com/unitedrhino/core")) || bytes.Contains(file.Data, []byte(lock.Source.Module)) {
			return fmt.Errorf("%s: old module prefix remains after transform", path)
		}
		count, err := verifySanitizedNonYAMLCredentialFile(path, file.Data)
		if err != nil {
			return err
		}
		nonYAMLCredentialValues += count
	}
	if nonYAMLCredentialValues != lock.Transforms.NonYAMLCredentialValueCount {
		return fmt.Errorf("non-YAML credential value count = %d, want %d", nonYAMLCredentialValues, lock.Transforms.NonYAMLCredentialValueCount)
	}
	return nil
}

func normalizeGoMod(src []byte, lock *Lock) ([]byte, error) {
	if lock == nil {
		return nil, fmt.Errorf("go.mod transform lock is nil")
	}
	if lock.Target.SaaSRelativePath != "../saas" {
		return nil, fmt.Errorf("go.mod SaaS relative path must be ../saas")
	}

	text := string(src)
	coreHint := "//gitee.com/unitedrhino/core => ../core\n"
	shareHint := "//" + lock.Target.ShareModule + " => ../share\n"
	if strings.Count(text, coreHint) != 1 || strings.Count(text, shareHint) != 1 {
		return nil, fmt.Errorf("go.mod must contain exactly the declared Core/Share local path hints")
	}
	text = strings.Replace(text, coreHint, "", 1)
	text = strings.Replace(text, shareHint, "", 1)

	// Prefixes are rewritten before parsing so the parsed module graph is the
	// exact graph that will be written, rather than a string-level proxy for it.
	rewritten := rewriteModulePrefixes([]byte(text), lock)
	parsed, err := modfile.Parse("go.mod", rewritten, nil)
	if err != nil {
		return nil, fmt.Errorf("parse transformed go.mod: %w", err)
	}
	if parsed.Module == nil || parsed.Module.Mod.Path != lock.Target.Module {
		return nil, fmt.Errorf("go.mod module path = %q, want %q", modulePath(parsed), lock.Target.Module)
	}

	replaceBlocks := 0
	var replaceBlock *modfile.LineBlock
	for _, statement := range parsed.Syntax.Stmt {
		block, ok := statement.(*modfile.LineBlock)
		if ok && len(block.Token) == 1 && block.Token[0] == "replace" {
			replaceBlocks++
			replaceBlock = block
		}
	}
	if replaceBlocks != 1 {
		return nil, fmt.Errorf("go.mod replace block count = %d, want 1", replaceBlocks)
	}
	for _, replacement := range parsed.Replace {
		if replacement.Old.Path == lock.Target.SaaSModule {
			return nil, fmt.Errorf("go.mod has duplicate SaaS replace")
		}
	}
	if len(parsed.Replace) != 2 {
		return nil, fmt.Errorf("go.mod replace directives = %d, want exactly 2 locked cloud directives", len(parsed.Replace))
	}
	wantCloud := map[string]string{
		"cloud.google.com/go":                  "v0.110.10",
		"cloud.google.com/go/compute/metadata": "v0.6.0",
	}
	seenCloud := make(map[string]bool, len(wantCloud))
	for _, replacement := range parsed.Replace {
		wantVersion, ok := wantCloud[replacement.Old.Path]
		if !ok || replacement.Old.Version != "" || replacement.New.Path != replacement.Old.Path || replacement.New.Version != wantVersion || seenCloud[replacement.Old.Path] {
			return nil, fmt.Errorf("go.mod cloud replace must retain the two locked version targets")
		}
		seenCloud[replacement.Old.Path] = true
	}
	if len(seenCloud) != len(wantCloud) {
		return nil, fmt.Errorf("go.mod cloud replace must retain the two locked version targets")
	}
	// Append to the one locked block explicitly. modfile.File.AddReplace may
	// choose a standalone directive when no same-module hint exists; that would
	// violate this migration's one-block output contract.
	replaceBlock.Line = append(replaceBlock.Line, &modfile.Line{
		Token:   []string{modfile.AutoQuote(lock.Target.SaaSModule), "=>", modfile.AutoQuote(lock.Target.SaaSRelativePath)},
		InBlock: true,
	})
	formatted, err := parsed.Format()
	if err != nil {
		return nil, fmt.Errorf("format transformed go.mod: %w", err)
	}
	verified, err := modfile.Parse("go.mod", formatted, nil)
	if err != nil {
		return nil, fmt.Errorf("reparse transformed go.mod: %w", err)
	}
	if len(verified.Replace) != 3 {
		return nil, fmt.Errorf("transformed go.mod replace directives = %d, want 3", len(verified.Replace))
	}
	last := verified.Replace[len(verified.Replace)-1]
	if last.Old.Path != lock.Target.SaaSModule || last.Old.Version != "" || last.New.Path != lock.Target.SaaSRelativePath || last.New.Version != "" {
		return nil, fmt.Errorf("transformed go.mod SaaS replace verification failed")
	}
	return append([]byte(nil), formatted...), nil
}

func modulePath(file *modfile.File) string {
	if file == nil || file.Module == nil {
		return ""
	}
	return file.Module.Mod.Path
}

// SanitizeCredentialYAML replaces only declared credential paths while
// preserving each line's indentation and commented-example form.
func SanitizeCredentialYAML(path string, src []byte) ([]byte, int, error) {
	data, profile, err := analyzeCredentialYAML(path, src, true)
	if err != nil {
		return nil, 0, err
	}
	return data, profile.total(), nil
}

type credentialProfile struct {
	Files     int
	Active    int
	Commented int
	Paths     CredentialPathCounts
}

func (profile *credentialProfile) add(semanticPath string, commented bool) error {
	var target *int
	switch semanticPath {
	case "Database.DSN":
		target = &profile.Paths.DatabaseDSN
	case "TSDB.DSN":
		target = &profile.Paths.TSDBDSN
	case "OssConf.AccessKeyID":
		target = &profile.Paths.OssAccessKeyID
	case "OssConf.AccessKeySecret":
		target = &profile.Paths.OssAccessKeySecret
	case "AuthWhite.Users[].Password":
		target = &profile.Paths.AuthWhiteUsersPassword
	case "DevLink.OpenApi.ApiKey":
		target = &profile.Paths.DevLinkOpenAPIAPIKey
	case "DevLink.OpenApi.SecretKey":
		target = &profile.Paths.DevLinkOpenAPISecretKey
	case "DevLink.Mqtt.OpenApi.ApiKey":
		target = &profile.Paths.DevLinkMQTTOpenAPIAPIKey
	case "DevLink.Mqtt.OpenApi.SecretKey":
		target = &profile.Paths.DevLinkMQTTOpenAPISecretKey
	default:
		return fmt.Errorf("unsupported credential path %s", semanticPath)
	}
	*target++
	if commented {
		profile.Commented++
	} else {
		profile.Active++
	}
	return nil
}

func (profile credentialProfile) total() int { return profile.Active + profile.Commented }

func (profile *credentialProfile) merge(other credentialProfile) {
	profile.Files += other.Files
	profile.Active += other.Active
	profile.Commented += other.Commented
	profile.Paths.DatabaseDSN += other.Paths.DatabaseDSN
	profile.Paths.TSDBDSN += other.Paths.TSDBDSN
	profile.Paths.OssAccessKeyID += other.Paths.OssAccessKeyID
	profile.Paths.OssAccessKeySecret += other.Paths.OssAccessKeySecret
	profile.Paths.AuthWhiteUsersPassword += other.Paths.AuthWhiteUsersPassword
	profile.Paths.DevLinkOpenAPIAPIKey += other.Paths.DevLinkOpenAPIAPIKey
	profile.Paths.DevLinkOpenAPISecretKey += other.Paths.DevLinkOpenAPISecretKey
	profile.Paths.DevLinkMQTTOpenAPIAPIKey += other.Paths.DevLinkMQTTOpenAPIAPIKey
	profile.Paths.DevLinkMQTTOpenAPISecretKey += other.Paths.DevLinkMQTTOpenAPISecretKey
}

func credentialProfileFromContract(contract TransformContract) credentialProfile {
	return credentialProfile{
		Files:     contract.CredentialFileCount,
		Active:    contract.CredentialActiveLineCount,
		Commented: contract.CredentialCommentedLineCount,
		Paths:     contract.CredentialPaths,
	}
}

func credentialProfilesEqual(left, right credentialProfile) bool {
	return left.Files == right.Files && left.Active == right.Active && left.Commented == right.Commented && left.Paths == right.Paths
}

func analyzeCredentialYAML(path string, src []byte, rewrite bool) ([]byte, credentialProfile, error) {
	if !utf8.Valid(src) {
		return nil, credentialProfile{}, fmt.Errorf("%s: YAML is not valid UTF-8", path)
	}
	astProfile, err := scanActiveCredentialAST(path, src)
	if err != nil {
		return nil, credentialProfile{}, err
	}
	lines := bytes.SplitAfter(src, []byte("\n"))
	var output bytes.Buffer
	var activeScopes, commentScopes []yamlScope
	previousComment := false
	var profile, lexicalActive credentialProfile
	for lineNumber, lineWithEnding := range lines {
		line, ending := splitLineEnding(lineWithEnding)
		parsed, ok := parseYAMLMappingLine(line)
		if !ok {
			if containsSuspectedCredentialKey(line) {
				return nil, credentialProfile{}, fmt.Errorf("%s:%d: unsupported YAML syntax near sensitive credential", path, lineNumber+1)
			}
			output.Write(lineWithEnding)
			if len(bytes.TrimSpace(line)) > 0 {
				previousComment = false
			}
			continue
		}
		if containsSuspectedCredentialKey(line[parsed.valueStart:]) {
			return nil, credentialProfile{}, fmt.Errorf("%s:%d: unsupported nested YAML syntax near sensitive credential", path, lineNumber+1)
		}
		var scopes []yamlScope
		if parsed.commented && previousComment {
			scopes = append([]yamlScope(nil), commentScopes...)
		} else {
			scopes = append([]yamlScope(nil), activeScopes...)
		}
		pathParts, nextScopes := yamlPathForLine(scopes, parsed)
		semanticPath := strings.Join(pathParts, ".")
		if sensitiveCredentialKeys[parsed.key] {
			placeholder, allowed := credentialEnv[semanticPath]
			if !allowed {
				return nil, credentialProfile{}, fmt.Errorf("%s:%d: sensitive credential at unsupported path %s", path, lineNumber+1, semanticPath)
			}
			if err := profile.add(semanticPath, parsed.commented); err != nil {
				return nil, credentialProfile{}, fmt.Errorf("%s:%d: %w", path, lineNumber+1, err)
			}
			if !parsed.commented {
				if err := lexicalActive.add(semanticPath, false); err != nil {
					return nil, credentialProfile{}, fmt.Errorf("%s:%d: %w", path, lineNumber+1, err)
				}
			}
			if rewrite {
				prefix := append([]byte(nil), line[:parsed.valueStart]...)
				if !parsed.hasValue && len(prefix) > 0 && prefix[len(prefix)-1] == ':' {
					prefix = append(prefix, ' ')
				}
				line = append(append(prefix, []byte(placeholder)...), line[parsed.valueEnd:]...)
			} else if !parsed.hasValue || string(line[parsed.valueStart:parsed.valueEnd]) != placeholder {
				return nil, credentialProfile{}, fmt.Errorf("%s:%d: credential placeholder does not match path %s", path, lineNumber+1, semanticPath)
			}
		}
		output.Write(line)
		output.Write(ending)
		if parsed.commented {
			commentScopes = nextScopes
			previousComment = true
		} else {
			activeScopes = nextScopes
			commentScopes = nil
			previousComment = false
		}
	}
	if lexicalActive.Active != astProfile.Active || lexicalActive.Paths != astProfile.Paths {
		return nil, credentialProfile{}, fmt.Errorf("%s: active YAML credential syntax is not safely line-rewritable", path)
	}
	if profile.total() > 0 {
		profile.Files = 1
	}
	return output.Bytes(), profile, nil
}

type yamlScope struct {
	indent int
	path   []string
}

type yamlMappingLine struct {
	indent     int
	commented  bool
	sequence   bool
	key        string
	hasValue   bool
	valueStart int
	valueEnd   int
}

func splitLineEnding(line []byte) ([]byte, []byte) {
	if len(line) > 0 && line[len(line)-1] == '\n' {
		return line[:len(line)-1], line[len(line)-1:]
	}
	return line, nil
}

func parseYAMLMappingLine(line []byte) (yamlMappingLine, bool) {
	i := 0
	for i < len(line) && line[i] == ' ' {
		i++
	}
	indent := i
	commented := false
	if i < len(line) && line[i] == '#' {
		commented = true
		i++
		spaces := 0
		for i < len(line) && line[i] == ' ' {
			spaces++
			i++
		}
		if spaces > 1 {
			indent += spaces
		}
	}
	sequence := false
	if i+1 < len(line) && line[i] == '-' && line[i+1] == ' ' {
		sequence = true
		i += 2
	}
	keyStart := i
	for i < len(line) && ((line[i] >= 'A' && line[i] <= 'Z') || (line[i] >= 'a' && line[i] <= 'z') || (line[i] >= '0' && line[i] <= '9') || line[i] == '_') {
		i++
	}
	if keyStart == i || i >= len(line) || line[i] != ':' {
		return yamlMappingLine{}, false
	}
	key := string(line[keyStart:i])
	i++
	valueStart := i
	for valueStart < len(line) && line[valueStart] == ' ' {
		valueStart++
	}
	valueEnd := len(line)
	for j := valueStart; j < len(line); j++ {
		if line[j] == '#' && (j == valueStart || line[j-1] == ' ') {
			valueEnd = j
			for valueEnd > valueStart && line[valueEnd-1] == ' ' {
				valueEnd--
			}
			break
		}
	}
	hasValue := valueStart < valueEnd
	return yamlMappingLine{indent: indent, commented: commented, sequence: sequence, key: key, hasValue: hasValue, valueStart: valueStart, valueEnd: valueEnd}, true
}

func yamlPathForLine(scopes []yamlScope, line yamlMappingLine) ([]string, []yamlScope) {
	for len(scopes) > 0 && scopes[len(scopes)-1].indent >= line.indent {
		scopes = scopes[:len(scopes)-1]
	}
	parent := []string(nil)
	if len(scopes) > 0 {
		parent = append(parent, scopes[len(scopes)-1].path...)
	}
	path := append([]string(nil), parent...)
	if line.sequence {
		if len(path) > 0 {
			path[len(path)-1] += "[]"
		}
		scopes = append(scopes, yamlScope{indent: line.indent, path: append([]string(nil), path...)})
	}
	path = append(path, line.key)
	if !line.hasValue && !sensitiveCredentialKeys[line.key] {
		scopes = append(scopes, yamlScope{indent: line.indent, path: append([]string(nil), path...)})
	}
	return path, scopes
}

func containsSuspectedCredentialKey(line []byte) bool {
	for _, key := range sensitiveCredentialKeyOrder {
		for _, token := range [][]byte{[]byte(key), []byte(`"` + key + `"`), []byte(`'` + key + `'`)} {
			for start := 0; start <= len(line)-len(token); {
				relative := bytes.Index(line[start:], token)
				if relative < 0 {
					break
				}
				index := start + relative
				beforeOK := index == 0 || !isYAMLIdentifierByte(line[index-1])
				after := index + len(token)
				for after < len(line) && (line[after] == ' ' || line[after] == '\t') {
					after++
				}
				if beforeOK && after < len(line) && line[after] == ':' {
					return true
				}
				start = index + 1
			}
		}
	}
	return false
}

func isYAMLIdentifierByte(value byte) bool {
	return value == '_' || value >= '0' && value <= '9' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func scanActiveCredentialAST(path string, src []byte) (credentialProfile, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(src, &document); err != nil {
		return credentialProfile{}, fmt.Errorf("%s: parse YAML syntax failed", path)
	}
	var profile credentialProfile
	if len(document.Content) == 0 {
		return profile, nil
	}
	if err := walkCredentialYAMLNode(path, document.Content[0], nil, &profile); err != nil {
		return credentialProfile{}, err
	}
	return profile, nil
}

func walkCredentialYAMLNode(filename string, node *yaml.Node, currentPath []string, profile *credentialProfile) error {
	if node.Kind == yaml.AliasNode {
		if node.Alias != nil && yamlNodeContainsSensitiveKey(node.Alias) {
			return fmt.Errorf("%s:%d: YAML alias near sensitive credential is unsupported", filename, node.Line)
		}
		return nil
	}
	if node.Style&yaml.FlowStyle != 0 && yamlNodeContainsSensitiveKey(node) {
		return fmt.Errorf("%s:%d: flow YAML syntax near sensitive credential is unsupported", filename, node.Line)
	}
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if sensitiveCredentialKeys[key.Value] {
				if key.Style != 0 || value.Kind != yaml.ScalarNode || value.Style != 0 || value.Alias != nil {
					return fmt.Errorf("%s:%d: unsupported YAML syntax near sensitive credential", filename, key.Line)
				}
				semanticPath := strings.Join(append(append([]string(nil), currentPath...), key.Value), ".")
				if _, ok := credentialEnv[semanticPath]; !ok {
					return fmt.Errorf("%s:%d: sensitive credential at unsupported path %s", filename, key.Line, semanticPath)
				}
				if err := profile.add(semanticPath, false); err != nil {
					return fmt.Errorf("%s:%d: %w", filename, key.Line, err)
				}
				continue
			}
			if err := walkCredentialYAMLNode(filename, value, append(append([]string(nil), currentPath...), key.Value), profile); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		sequencePath := append([]string(nil), currentPath...)
		if len(sequencePath) > 0 {
			sequencePath[len(sequencePath)-1] += "[]"
		}
		for _, child := range node.Content {
			if err := walkCredentialYAMLNode(filename, child, sequencePath, profile); err != nil {
				return err
			}
		}
	}
	return nil
}

func yamlNodeContainsSensitiveKey(node *yaml.Node) bool {
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			if sensitiveCredentialKeys[node.Content[i].Value] || yamlNodeContainsSensitiveKey(node.Content[i+1]) {
				return true
			}
		}
	}
	for _, child := range node.Content {
		if yamlNodeContainsSensitiveKey(child) {
			return true
		}
	}
	return false
}

// SanitizeCloudProtocolCredentials replaces only the two SHA-locked
// credential values in the cloud protocol JSON example comment. Error text is
// deliberately content-free so rejected source values cannot enter evidence.
func SanitizeCloudProtocolCredentials(src []byte, expectedSourceSHA, expectedOutputSHA string) ([]byte, int, error) {
	if sha256Digest(src) != expectedSourceSHA {
		return nil, 0, fmt.Errorf("cloud protocol credential source SHA-256 mismatch")
	}
	if bytes.Contains(src, []byte(cloudAccessKeyIDPlaceholder)) || bytes.Contains(src, []byte(cloudAccessKeySecretPlaceholder)) {
		return nil, 0, fmt.Errorf("cloud protocol credential source already contains a placeholder")
	}
	if err := validateCloudCredentialExample(src, false); err != nil {
		return nil, 0, err
	}
	out, idCount, err := replaceLockedJSONStringValues(src, cloudAccessKeyIDPattern, cloudAccessKeyIDPlaceholder, 1)
	if err != nil {
		return nil, 0, fmt.Errorf("cloud protocol access-key ID: %w", err)
	}
	out, secretCount, err := replaceLockedJSONStringValues(out, cloudAccessKeySecretPattern, cloudAccessKeySecretPlaceholder, 1)
	if err != nil {
		return nil, 0, fmt.Errorf("cloud protocol access-key secret: %w", err)
	}
	if err := validateCloudCredentialExample(out, true); err != nil {
		return nil, 0, err
	}
	if sha256Digest(out) != expectedOutputSHA {
		return nil, 0, fmt.Errorf("cloud protocol credential output SHA-256 mismatch")
	}
	return append([]byte(nil), out...), idCount + secretCount, nil
}

func validateCloudCredentialExample(src []byte, sanitized bool) error {
	if !utf8.Valid(src) {
		return fmt.Errorf("cloud protocol credential source is not valid UTF-8")
	}
	keyOffset := bytes.Index(src, []byte(`"accessKeyID"`))
	if keyOffset < 0 {
		return fmt.Errorf("cloud protocol credential replacement count is invalid")
	}
	commentStart := bytes.LastIndex(src[:keyOffset], []byte("/*"))
	commentEndRelative := bytes.Index(src[keyOffset:], []byte("*/"))
	if commentStart < 0 || commentEndRelative < 0 {
		return fmt.Errorf("cloud protocol credential JSON comment boundary is invalid")
	}
	commentEnd := keyOffset + commentEndRelative
	var example map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(src[commentStart+2:commentEnd]), &example); err != nil {
		return fmt.Errorf("cloud protocol credential example is not valid JSON")
	}
	id, idOK := example["accessKeyID"].(string)
	secret, secretOK := example["accessKeySecret"].(string)
	if !idOK || !secretOK || id == "" || secret == "" {
		return fmt.Errorf("cloud protocol credential example has invalid semantic values")
	}
	if sanitized {
		if id != cloudAccessKeyIDPlaceholder || secret != cloudAccessKeySecretPlaceholder {
			return fmt.Errorf("cloud protocol credential placeholders are invalid")
		}
	} else if id == cloudAccessKeyIDPlaceholder || secret == cloudAccessKeySecretPlaceholder {
		return fmt.Errorf("cloud protocol credential source already contains a placeholder")
	}
	return nil
}

// SanitizeProductSecrets replaces the three SHA-locked Info.Secret values
// while preserving every other byte of the upstream JSON document.
func SanitizeProductSecrets(src []byte, expectedSourceSHA, expectedOutputSHA string) ([]byte, int, error) {
	if sha256Digest(src) != expectedSourceSHA {
		return nil, 0, fmt.Errorf("product secret source SHA-256 mismatch")
	}
	if bytes.Contains(src, []byte(productSecretPlaceholder)) {
		return nil, 0, fmt.Errorf("product secret source already contains a placeholder")
	}
	if err := validateProductSecretSemantics(src, false); err != nil {
		return nil, 0, err
	}
	out, count, err := replaceLockedJSONStringValues(src, productSecretPattern, productSecretPlaceholder, 3)
	if err != nil {
		return nil, 0, fmt.Errorf("product secret: %w", err)
	}
	if err := validateProductSecretSemantics(out, true); err != nil {
		return nil, 0, err
	}
	if sha256Digest(out) != expectedOutputSHA {
		return nil, 0, fmt.Errorf("product secret output SHA-256 mismatch")
	}
	return append([]byte(nil), out...), count, nil
}

func validateProductSecretSemantics(src []byte, sanitized bool) error {
	if !utf8.Valid(src) || !json.Valid(src) {
		return fmt.Errorf("product secret source is not valid JSON")
	}
	var document any
	if err := json.Unmarshal(src, &document); err != nil {
		return fmt.Errorf("product secret source is not valid JSON")
	}
	paths := make([]string, 0, 3)
	values := make([]string, 0, 3)
	collectJSONSecretValues(document, nil, &paths, &values)
	wantPaths := []string{"0/Info/Secret", "1/Info/Secret", "2/Info/Secret"}
	if !equalStringSlices(paths, wantPaths) || len(values) != len(wantPaths) {
		return fmt.Errorf("product secret semantic paths are invalid")
	}
	for _, value := range values {
		if sanitized {
			if value != productSecretPlaceholder {
				return fmt.Errorf("product secret placeholder is invalid")
			}
		} else if value == "" || value == productSecretPlaceholder {
			return fmt.Errorf("product secret source has an invalid semantic value")
		}
	}
	return nil
}

func collectJSONSecretValues(value any, path []string, paths, values *[]string) {
	switch typed := value.(type) {
	case []any:
		for index, child := range typed {
			collectJSONSecretValues(child, append(path, fmt.Sprintf("%d", index)), paths, values)
		}
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			childPath := append(append([]string(nil), path...), key)
			if key == "Secret" {
				*paths = append(*paths, strings.Join(childPath, "/"))
				text, _ := typed[key].(string)
				*values = append(*values, text)
			}
			collectJSONSecretValues(typed[key], childPath, paths, values)
		}
	}
}

func replaceLockedJSONStringValues(src []byte, pattern *regexp.Regexp, placeholder string, expectedCount int) ([]byte, int, error) {
	matches := pattern.FindAllSubmatchIndex(src, -1)
	if len(matches) != expectedCount {
		return nil, 0, fmt.Errorf("replacement count = %d, want %d", len(matches), expectedCount)
	}
	result := make([]byte, 0, len(src)+len(matches)*len(placeholder))
	last := 0
	for _, match := range matches {
		if len(match) != 4 || match[0] < last || match[2] != match[0] || match[3] > match[1] {
			return nil, 0, fmt.Errorf("replacement boundary is invalid")
		}
		result = append(result, src[last:match[3]]...)
		result = append(result, '"')
		result = append(result, placeholder...)
		result = append(result, '"')
		last = match[1]
	}
	result = append(result, src[last:]...)
	return result, len(matches), nil
}

// FixSchemaFixtureIdentifiers corrects the locked legacy JSON field names in
// the upstream schema test fixture without changing production model code.
func FixSchemaFixtureIdentifiers(src []byte, expectedSourceSHA, expectedOutputSHA string, expectedCount int) ([]byte, int, error) {
	if sha256Digest(src) != expectedSourceSHA {
		return nil, 0, fmt.Errorf("schema fixture source SHA-256 mismatch")
	}
	oldToken := []byte(`"id":`)
	newToken := []byte(`"identifier":`)
	if bytes.Contains(src, newToken) {
		return nil, 0, fmt.Errorf("schema fixture already contains identifier token")
	}
	oldCount := bytes.Count(src, oldToken)
	if oldCount != expectedCount || oldCount <= 0 {
		return nil, 0, fmt.Errorf("schema fixture identifier token count = %d, want %d", oldCount, expectedCount)
	}
	out := bytes.ReplaceAll(src, oldToken, newToken)
	if bytes.Count(out, oldToken) != 0 || bytes.Count(out, newToken) != expectedCount {
		return nil, 0, fmt.Errorf("schema fixture replacement count mismatch")
	}
	if sha256Digest(out) != expectedOutputSHA {
		return nil, 0, fmt.Errorf("schema fixture output SHA-256 mismatch")
	}
	return append([]byte(nil), out...), oldCount, nil
}

// RemoveEmbeddedPrivateKey removes the one declared clientKey raw literal
// after verifying the original blob digest, then makes certificate diagnostics
// nil-safe without changing login authorization branches.
func RemoveEmbeddedPrivateKey(src []byte, expectedSHA string) ([]byte, int, error) {
	if sha256Digest(src) != expectedSHA {
		return nil, 0, fmt.Errorf("private-key source SHA-256 mismatch")
	}
	declaration := []byte("var clientKey string = `")
	if count := bytes.Count(src, declaration); count != 1 {
		return nil, 0, fmt.Errorf("clientKey declaration count = %d, want 1", count)
	}
	begin := privateKeyBeginMarker()
	end := privateKeyEndMarker()
	start := bytes.Index(src, declaration)
	keyStart := start + len(declaration)
	if !bytes.HasPrefix(src[keyStart:], begin) || bytes.Count(src, begin) != 1 || bytes.Count(src, end) != 1 {
		return nil, 0, fmt.Errorf("clientKey private-key boundary is invalid")
	}
	endOffset := bytes.Index(src[keyStart:], end)
	keyEnd := keyStart + endOffset + len(end)
	if keyEnd >= len(src) || src[keyEnd] != '`' {
		return nil, 0, fmt.Errorf("clientKey private-key boundary is invalid")
	}
	keyEnd++
	if keyEnd < len(src) && src[keyEnd] == '\r' {
		keyEnd++
	}
	if keyEnd < len(src) && src[keyEnd] == '\n' {
		keyEnd++
	}
	out := append(append([]byte(nil), src[:start]...), src[keyEnd:]...)
	if bytes.Count(out, []byte(`"crypto/tls"`)) != 1 || bytes.Count(out, []byte("var clientCertificate tls.Certificate")) != 1 {
		return nil, 0, fmt.Errorf("clientKey surrounding initialization is invalid")
	}
	out = bytes.Replace(out, []byte(`"crypto/tls"`), []byte(`"encoding/pem"`), 1)
	out = bytes.Replace(out, []byte("var clientCertificate tls.Certificate\n"), nil, 1)
	oldInit := []byte("clientCertificate, _ = tls.X509KeyPair([]byte(clientCert), []byte(clientKey))\n\tx509Cert, _ = x509.ParseCertificate(clientCertificate.Certificate[0])")
	newInit := []byte("clientCertBlock, _ := pem.Decode([]byte(clientCert))\n\tif clientCertBlock != nil {\n\t\tif parsedCert, err := x509.ParseCertificate(clientCertBlock.Bytes); err == nil {\n\t\t\tx509Cert = parsedCert\n\t\t}\n\t}")
	if bytes.Count(out, oldInit) != 1 {
		return nil, 0, fmt.Errorf("clientKey initialization boundary is invalid")
	}
	out = bytes.Replace(out, oldInit, newInit, 1)
	guard := []byte("if len(in.Certificate) > 0 {")
	if bytes.Count(out, guard) != 1 {
		return nil, 0, fmt.Errorf("clientKey diagnostic guard boundary is invalid")
	}
	out = bytes.Replace(out, guard, []byte("if len(in.Certificate) > 0 && x509Cert != nil {"), 1)
	formatted, err := format.Source(out)
	if err != nil {
		return nil, 0, fmt.Errorf("format private-key transform: %w", err)
	}
	return append([]byte(nil), formatted...), 1, nil
}

// FixDynamicFormat makes the one vet-invalid AddDetailf invocation explicit
// after checking its original blob digest.
func FixDynamicFormat(src []byte, expectedSHA string) ([]byte, int, error) {
	if sha256Digest(src) != expectedSHA {
		return nil, 0, fmt.Errorf("vet source SHA-256 mismatch")
	}
	oldCall := []byte("AddDetailf(strProjectID)")
	newCall := []byte(`AddDetailf("%s", strProjectID)`)
	if oldCount := bytes.Count(src, oldCall); oldCount != 1 {
		return nil, 0, fmt.Errorf("old AddDetailf count = %d, want 1", oldCount)
	}
	if newCount := bytes.Count(src, newCall); newCount != 0 {
		return nil, 0, fmt.Errorf("new AddDetailf count = %d, want 0", newCount)
	}
	out := bytes.Replace(src, oldCall, newCall, 1)
	if bytes.Count(out, oldCall) != 0 || bytes.Count(out, newCall) != 1 {
		return nil, 0, fmt.Errorf("vet format replacement count mismatch")
	}
	return append([]byte(nil), out...), 1, nil
}

func privateKeyBeginMarker() []byte {
	return []byte("-----" + "BEGIN " + "RSA " + "PRIVATE KEY" + "-----")
}

func containsPrivateKeyPEMMarker(data []byte) bool {
	generic := []byte("-----" + "BEGIN " + "PRIVATE KEY" + "-----")
	if bytes.Contains(data, generic) {
		return true
	}
	begin := []byte("-----" + "BEGIN ")
	ending := []byte(" PRIVATE KEY" + "-----")
	for searchFrom := 0; searchFrom < len(data); {
		relative := bytes.Index(data[searchFrom:], begin)
		if relative < 0 {
			return false
		}
		start := searchFrom + relative + len(begin)
		lineEnd := len(data)
		if relativeEnd := bytes.IndexAny(data[start:], "\r\n"); relativeEnd >= 0 {
			lineEnd = start + relativeEnd
		}
		if bytes.Contains(data[start:lineEnd], ending) {
			return true
		}
		searchFrom = start
	}
	return false
}

func privateKeyEndMarker() []byte {
	return []byte("-----" + "END " + "RSA " + "PRIVATE KEY" + "-----")
}

func sha256Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
