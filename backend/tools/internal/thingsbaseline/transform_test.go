package thingsbaseline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testCoreModule           = "gitee.com/unitedrhino/core"
	testThingsModule         = "gitee.com/unitedrhino/things"
	testShareModule          = "gitee.com/ming-iot/share"
	testPrivateKeySourcePath = "service/dgsvr/internal/logic/deviceauth/loginAuthLogic.go"
	testVetSourcePath        = "service/apisvr/internal/logic/things/device/edge/sendLogic.go"
)

func TestTransformModulesRewritesModulesAndNormalizesGoMod(t *testing.T) {
	lock := moduleLockForTest(4, 3)
	files := moduleFilesForTest()

	rendered, report, err := transformModules(files, &lock)
	if err != nil {
		t.Fatalf("TransformTree() error: %v", err)
	}
	if got, want := report.CorePrefixes, 4; got != want {
		t.Fatalf("core prefix count = %d, want %d", got, want)
	}
	if got, want := report.ThingsPrefixes, 3; got != want {
		t.Fatalf("things prefix count = %d, want %d", got, want)
	}
	if got, want := string(rendered["go.sum"].Data), string(files["go.sum"].Data); got != want {
		t.Fatalf("go.sum changed: got %q, want byte-identical input", got)
	}
	if got := string(rendered["README.md"].Data); !strings.Contains(got, testShareModule) {
		t.Fatalf("share module was rewritten: %q", got)
	}
	wantGoMod := `module gitee.com/ming-iot/things

require gitee.com/ming-iot/saas v1.2.3

replace (
	cloud.google.com/go => cloud.google.com/go v0.110.10
	cloud.google.com/go/compute/metadata => cloud.google.com/go/compute/metadata v0.6.0
	gitee.com/ming-iot/saas => ../saas
)
`
	if got := string(rendered["go.mod"].Data); got != wantGoMod {
		t.Fatalf("go.mod =\n%s\nwant:\n%s", got, wantGoMod)
	}
	if _, ok := rendered["other-key"]; ok {
		t.Fatal("output map included a key other than SourceFile.Path")
	}
	files["README.md"].Data[0] = 'X'
	if bytes.Equal(rendered["README.md"].Data, files["README.md"].Data) {
		t.Fatal("rendered data shares mutable backing array with input")
	}
}

func TestTransformModulesRejectsModuleContractMismatches(t *testing.T) {
	tests := []struct {
		name  string
		lock  Lock
		files map[string]SourceFile
		want  string
	}{
		{"too few core", moduleLockForTest(5, 3), moduleFilesForTest(), "corePrefixCount"},
		{"too many things", moduleLockForTest(4, 2), moduleFilesForTest(), "thingsPrefixCount"},
		{"duplicate SaaS replace", moduleLockForTest(4, 3), moduleFilesWithSaaSReplace(), "duplicate SaaS replace"},
		{"alternate SaaS replace", moduleLockForTest(4, 3), moduleFilesWithAlternateSaaSReplace(), "duplicate SaaS replace"},
		{"local cloud replace", moduleLockForTest(4, 3), moduleFilesWithLocalCloudReplace(), "cloud replace"},
		{"changed cloud version", moduleLockForTest(4, 3), moduleFilesWithChangedCloudVersion(), "cloud replace"},
		{"comment cloud decoy", moduleLockForTest(4, 3), moduleFilesWithCommentCloudDecoy(), "cloud replace"},
		{"comment replace block decoy", moduleLockForTest(4, 3), moduleFilesWithCommentReplaceBlockDecoy(), "replace"},
		{"extra replace directive", moduleLockForTest(4, 3), moduleFilesWithExtraReplace(), "replace directives"},
		{"duplicate cloud directive", moduleLockForTest(4, 3), moduleFilesWithDuplicateCloudReplace(), "replace directives"},
		{"unsafe SaaS relative path", moduleLockWithSaaSPathForTest("/tmp/saas"), moduleFilesForTest(), "SaaS relative path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := cloneSourceFiles(tt.files)
			_, _, err := transformModules(tt.files, &tt.lock)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("transformModules() error = %v, want substring %q", err, tt.want)
			}
			if !sourceFilesEqual(tt.files, before) {
				t.Fatal("transformModules() changed input on failure")
			}
		})
	}
}

func TestTransformTreeRejectsMismatchedInputMapKey(t *testing.T) {
	lock := validLockForTest()
	files := map[string]SourceFile{
		"wrong-key": {Path: "actual-path", Mode: "100644", Data: []byte("package fixture\n")},
	}

	_, _, err := TransformTree(files, &lock)
	if err == nil || !strings.Contains(err.Error(), "does not equal SourceFile.Path") {
		t.Fatalf("TransformTree() error = %v, want map-key mismatch", err)
	}
}

func TestTransformTreeRejectsUnverifiedSourceMode(t *testing.T) {
	files, lock := fullTransformFixture(t)
	file := files["README.md"]
	file.Mode = "120000"
	files["README.md"] = file

	_, _, err := TransformTree(files, &lock)
	if err == nil || !strings.Contains(err.Error(), `README.md" has unsupported mode "120000"`) {
		t.Fatalf("TransformTree() error = %v, want unsupported source mode rejection", err)
	}
}

func TestTransformTreeRejectsIncompleteTransformLock(t *testing.T) {
	files, lock := fullTransformFixture(t)
	lock.Transforms.CredentialLineCount = 0

	_, _, err := TransformTree(files, &lock)
	if err == nil || !strings.Contains(err.Error(), "transforms.credentialLineCount must be positive") {
		t.Fatalf("TransformTree() error = %v, want fail-closed positive transform count", err)
	}
}

func TestSanitizeCloudProtocolCredentialsIsExactAndFailClosed(t *testing.T) {
	source := []byte("package protocol\n\n/*\n{\n\"accessKeyID\":\"cloud-id-redaction-sentinel\",\n\"accessKeySecret\":\"cloud-secret-redaction-sentinel\",\n\"regionID\":\"region\"\n}\n*/\n")
	want := bytes.Replace(source, []byte(`"accessKeyID":"cloud-id-redaction-sentinel"`), []byte(`"accessKeyID":"${THINGS_CLOUD_ACCESS_KEY_ID}"`), 1)
	want = bytes.Replace(want, []byte(`"accessKeySecret":"cloud-secret-redaction-sentinel"`), []byte(`"accessKeySecret":"${THINGS_CLOUD_ACCESS_KEY_SECRET}"`), 1)
	original := append([]byte(nil), source...)

	got, count, err := SanitizeCloudProtocolCredentials(source, sha256Hex(source), sha256Hex(want))
	if err != nil {
		t.Fatalf("SanitizeCloudProtocolCredentials() error: %v", err)
	}
	if count != 2 || !bytes.Equal(got, want) {
		t.Fatalf("cloud credential result count/hash = %d/%s, want 2/%s", count, sha256Hex(got), sha256Hex(want))
	}
	if !bytes.Equal(source, original) {
		t.Fatal("cloud credential sanitation mutated source")
	}
	got[0] ^= 0xff
	if !bytes.Equal(source, original) {
		t.Fatal("cloud credential sanitation returned shared storage")
	}

	tests := []struct {
		name      string
		source    []byte
		sourceSHA string
		outputSHA string
		want      string
	}{
		{name: "source SHA", source: source, sourceSHA: strings.Repeat("0", 64), outputSHA: sha256Hex(want), want: "source SHA-256"},
		{name: "output SHA", source: source, sourceSHA: sha256Hex(source), outputSHA: strings.Repeat("0", 64), want: "output SHA-256"},
		{name: "missing key", source: bytes.Replace(source, []byte("accessKeySecret"), []byte("otherKey"), 1), want: "semantic values"},
		{name: "preexisting placeholder", source: bytes.Replace(source, []byte("cloud-id-redaction-sentinel"), []byte("${THINGS_CLOUD_ACCESS_KEY_ID}"), 1), want: "already contains"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.sourceSHA == "" {
				tt.sourceSHA = sha256Hex(tt.source)
			}
			if tt.outputSHA == "" {
				tt.outputSHA = sha256Hex(tt.source)
			}
			_, _, err := SanitizeCloudProtocolCredentials(tt.source, tt.sourceSHA, tt.outputSHA)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("SanitizeCloudProtocolCredentials() error = %v, want %q", err, tt.want)
			}
			for _, forbidden := range []string{"cloud-id-redaction-sentinel", "cloud-secret-redaction-sentinel"} {
				if strings.Contains(err.Error(), forbidden) {
					t.Fatalf("cloud credential error leaked source value: %v", err)
				}
			}
		})
	}
}

func TestSanitizeProductSecretsIsExactAndFailClosed(t *testing.T) {
	source := []byte(`[{"Info":{"Secret":"product-secret-redaction-one","Name":"one"}},{"Info":{"Secret":"product-secret-redaction-two"}},{"Info":{"Secret":"product-secret-redaction-three"}}]`)
	want := source
	for _, old := range []string{"product-secret-redaction-one", "product-secret-redaction-two", "product-secret-redaction-three"} {
		want = bytes.Replace(want, []byte(old), []byte("${THINGS_INIT_PRODUCT_SECRET}"), 1)
	}
	want = append([]byte(nil), want...)
	original := append([]byte(nil), source...)

	got, count, err := SanitizeProductSecrets(source, sha256Hex(source), sha256Hex(want))
	if err != nil {
		t.Fatalf("SanitizeProductSecrets() error: %v", err)
	}
	if count != 3 || !bytes.Equal(got, want) {
		t.Fatalf("product secret result count/hash = %d/%s, want 3/%s", count, sha256Hex(got), sha256Hex(want))
	}
	if !bytes.Equal(source, original) {
		t.Fatal("product secret sanitation mutated source")
	}
	got[0] ^= 0xff
	if !bytes.Equal(source, original) {
		t.Fatal("product secret sanitation returned shared storage")
	}

	tests := []struct {
		name      string
		source    []byte
		sourceSHA string
		outputSHA string
		want      string
	}{
		{name: "source SHA", source: source, sourceSHA: strings.Repeat("0", 64), outputSHA: sha256Hex(want), want: "source SHA-256"},
		{name: "output SHA", source: source, sourceSHA: sha256Hex(source), outputSHA: strings.Repeat("0", 64), want: "output SHA-256"},
		{name: "invalid JSON", source: append(append([]byte(nil), source...), '{'), want: "valid JSON"},
		{name: "wrong path", source: bytes.Replace(source, []byte(`"Info":{"Secret"`), []byte(`"Other":{"Secret"`), 1), want: "semantic paths"},
		{name: "preexisting placeholder", source: bytes.Replace(source, []byte("product-secret-redaction-one"), []byte("${THINGS_INIT_PRODUCT_SECRET}"), 1), want: "already contains"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.sourceSHA == "" {
				tt.sourceSHA = sha256Hex(tt.source)
			}
			if tt.outputSHA == "" {
				tt.outputSHA = sha256Hex(tt.source)
			}
			_, _, err := SanitizeProductSecrets(tt.source, tt.sourceSHA, tt.outputSHA)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("SanitizeProductSecrets() error = %v, want %q", err, tt.want)
			}
			for _, forbidden := range []string{"product-secret-redaction-one", "product-secret-redaction-two", "product-secret-redaction-three"} {
				if strings.Contains(err.Error(), forbidden) {
					t.Fatalf("product secret error leaked source value: %v", err)
				}
			}
		})
	}
}

func TestSanitizeCredentialYAMLUsesFullSemanticPaths(t *testing.T) {
	src := []byte(`Database:
  DSN: literal-db-value
#TSDB:
#  DSN: literal-db-value
OssConf:
  AccessKeyID: literal-key-value
  AccessKeySecret: literal-key-value
AuthWhite:
  Users:
    - UserName: worker
      Password: literal-password-value
DevLink:
  OpenApi:
    ApiKey: literal-key-value
    SecretKey: literal-key-value
  Mqtt:
    OpenApi:
      # ApiKey: literal-key-value
      # SecretKey: literal-key-value
`)

	got, count, err := SanitizeCredentialYAML("fixture.yaml", src)
	if err != nil {
		t.Fatalf("SanitizeCredentialYAML() error: %v", err)
	}
	if gotCount, want := count, 9; gotCount != want {
		t.Fatalf("sanitized lines = %d, want %d", gotCount, want)
	}
	text := string(got)
	for _, want := range []string{
		"  DSN: ${THINGS_DB_DSN}",
		"#  DSN: ${THINGS_TSDB_DSN}",
		"  AccessKeyID: ${THINGS_OSS_ACCESS_KEY_ID}",
		"  AccessKeySecret: ${THINGS_OSS_ACCESS_KEY_SECRET}",
		"      Password: ${THINGS_INTERNAL_AUTH_PASSWORD}",
		"    ApiKey: ${THINGS_EMQX_API_KEY}",
		"    SecretKey: ${THINGS_EMQX_SECRET_KEY}",
		"      # ApiKey: ${THINGS_EMQX_API_KEY}",
		"      # SecretKey: ${THINGS_EMQX_SECRET_KEY}",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("sanitized YAML missing %q:\n%s", want, text)
		}
	}
	for _, literal := range []string{"literal-db-value", "literal-key-value", "literal-password-value"} {
		if strings.Contains(text, literal) {
			t.Fatalf("sanitized YAML leaked literal %q", literal)
		}
	}
}

func TestSanitizeCredentialYAMLRejectsSensitiveLeavesAtWrongPaths(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"active DSN", "Other:\n  DSN: literal-db-value\n"},
		{"commented DSN", "#Other:\n#  DSN: literal-db-value\n"},
		{"active AccessKeyID", "Other:\n  AccessKeyID: literal-key-value\n"},
		{"commented AccessKeyID", "#Other:\n#  AccessKeyID: literal-key-value\n"},
		{"active AccessKeySecret", "Other:\n  AccessKeySecret: literal-key-value\n"},
		{"commented AccessKeySecret", "#Other:\n#  AccessKeySecret: literal-key-value\n"},
		{"active Password", "Other:\n  Password: literal-password-value\n"},
		{"commented Password", "#Other:\n#  Password: literal-password-value\n"},
		{"active ApiKey", "Other:\n  ApiKey: literal-key-value\n"},
		{"commented ApiKey", "#Other:\n#  ApiKey: literal-key-value\n"},
		{"active SecretKey", "Other:\n  SecretKey: literal-key-value\n"},
		{"commented SecretKey", "#Other:\n#  SecretKey: literal-key-value\n"},
		{"active empty", "Other:\n  DSN:\n"},
		{"commented empty", "#Other:\n#  DSN:\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := SanitizeCredentialYAML("wrong-parent.yaml", []byte(tt.src))
			if err == nil || !strings.Contains(err.Error(), "wrong-parent.yaml") || strings.Contains(err.Error(), "literal-") {
				t.Fatalf("SanitizeCredentialYAML() error = %v, want path-only fail-closed error", err)
			}
		})
	}
}

func TestSanitizeCredentialYAMLFailsClosedForEmptyAndComplexSensitiveForms(t *testing.T) {
	got, count, err := SanitizeCredentialYAML("empty.yaml", []byte("Database:\n  DSN:\n"))
	if err != nil || count != 1 || !bytes.Contains(got, []byte("DSN: ${THINGS_DB_DSN}")) {
		t.Fatalf("SanitizeCredentialYAML() empty value = %q, %d, %v; want one placeholder", got, count, err)
	}

	for _, src := range []string{
		"Database:\n  \"DSN\": literal-db-value\n",
		"Database:\n  'DSN' : literal-db-value\n",
		"Database: {DSN: literal-db-value}\n",
		"Database:\n  DSN: |\n    literal-db-value\n",
		"Database:\n  DSN: \"literal-db-value\"\n",
		"#\tDSN: literal-db-value\n",
		"# Database: {DSN: literal-db-value}\n",
		"# \"DSN\" : literal-db-value\n",
		"AuthWhite: {Users: [{Password: literal-password-value}]}\n",
		"Database:\n  DSN: *credential\n",
	} {
		_, _, err := SanitizeCredentialYAML("complex.yaml", []byte(src))
		if err == nil || strings.Contains(err.Error(), "literal-") {
			t.Fatalf("SanitizeCredentialYAML() error = %v, want safe rejection for %q", err, src)
		}
	}
}

func TestVerifySanitizedCredentialYAMLRequiresExactPathPlaceholder(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"wrong variable", "Database:\n  DSN: ${OTHER_DSN}\n"},
		{"generic variable", "Database:\n  DSN: ${THINGS_DSN}\n"},
		{"literal", "Database:\n  DSN: literal-db-value\n"},
		{"empty", "Database:\n  DSN:\n"},
		{"commented wrong variable", "#TSDB:\n#  DSN: ${THINGS_DB_DSN}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := verifySanitizedCredentialYAML("reverse.yaml", []byte(tt.src))
			if err == nil || strings.Contains(err.Error(), "literal-db-value") {
				t.Fatalf("verifySanitizedCredentialYAML() error = %v, want safe exact-placeholder rejection", err)
			}
		})
	}

	profile, err := verifySanitizedCredentialYAML("valid.yaml", []byte("Database:\n  DSN: ${THINGS_DB_DSN}\n"))
	if err != nil {
		t.Fatalf("verifySanitizedCredentialYAML() valid placeholder: %v", err)
	}
	if want := (credentialProfile{Files: 1, Active: 1, Paths: CredentialPathCounts{DatabaseDSN: 1}}); !credentialProfilesEqual(profile, want) {
		t.Fatalf("verified profile = %+v, want %+v", profile, want)
	}
}

func TestSanitizeCredentialYAMLRejectsFiftyFiveValidPlusOneBypass(t *testing.T) {
	var fixture strings.Builder
	fixture.WriteString("AuthWhite:\n  Users:\n")
	for i := 0; i < 55; i++ {
		fmt.Fprintf(&fixture, "    - Password: literal-password-value-%02d\n", i)
	}
	fixture.WriteString("Other: {Password: bypass-value}\n")
	src := []byte(fixture.String())
	before := append([]byte(nil), src...)

	_, _, err := SanitizeCredentialYAML("bypass.yaml", src)
	if err == nil || strings.Contains(err.Error(), "bypass-value") || strings.Contains(err.Error(), "literal-password") {
		t.Fatalf("SanitizeCredentialYAML() error = %v, want safe bypass rejection", err)
	}
	if !bytes.Equal(src, before) {
		t.Fatal("SanitizeCredentialYAML() mutated input on bypass rejection")
	}
}

func TestTransformTreeRejectsCredentialProfileDimensionMismatches(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Lock)
	}{
		{"active versus commented", func(lock *Lock) {
			lock.Transforms.CredentialActiveLineCount = 0
			lock.Transforms.CredentialCommentedLineCount = 1
		}},
		{"file count", func(lock *Lock) {
			lock.Source.TrackedFiles = 2
			lock.Source.SelectedFiles = 2
			lock.Transforms.CredentialFileCount = 2
		}},
		{"semantic path", func(lock *Lock) {
			lock.Transforms.CredentialPaths.DatabaseDSN = 0
			lock.Transforms.CredentialPaths.TSDBDSN = 1
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, lock := fullTransformFixture(t)
			tt.mutate(&lock)
			before := cloneSourceFiles(files)

			_, _, err := TransformTree(files, &lock)
			if err == nil || !strings.Contains(err.Error(), "credential profile") {
				t.Fatalf("TransformTree() error = %v, want exact credential profile rejection", err)
			}
			if !sourceFilesEqual(files, before) {
				t.Fatal("TransformTree() mutated input on credential profile failure")
			}
		})
	}
}

func TestFixSchemaFixtureIdentifiersIsExactAndFailClosed(t *testing.T) {
	const sentinel = "schema-identifier-redaction-sentinel"
	src := []byte("package schema\n\nconst fixture = `" + sentinel + " {\"id\":\"temperature\"} {\"id\":\"humidity\"}`\n")
	want := bytes.ReplaceAll(src, []byte(`"id":`), []byte(`"identifier":`))
	before := append([]byte(nil), src...)

	got, count, err := FixSchemaFixtureIdentifiers(src, sha256Hex(src), sha256Hex(want), 2)
	if err != nil {
		t.Fatalf("FixSchemaFixtureIdentifiers() error: %v", err)
	}
	if count != 2 {
		t.Fatalf("FixSchemaFixtureIdentifiers() count = %d, want 2", count)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FixSchemaFixtureIdentifiers() output = %q, want %q", got, want)
	}
	if !bytes.Equal(src, before) {
		t.Fatal("FixSchemaFixtureIdentifiers() mutated source bytes")
	}
	got[0] = 'X'
	if !bytes.Equal(src, before) {
		t.Fatal("FixSchemaFixtureIdentifiers() returned data sharing source backing storage")
	}

	tests := []struct {
		name           string
		src            []byte
		expectedSource string
		expectedOutput string
		expectedCount  int
		wantError      string
	}{
		{
			name:           "wrong source SHA",
			src:            src,
			expectedSource: strings.Repeat("0", 64),
			expectedOutput: sha256Hex(want),
			expectedCount:  2,
			wantError:      "schema fixture source SHA-256 mismatch",
		},
		{
			name:           "wrong output SHA",
			src:            src,
			expectedSource: sha256Hex(src),
			expectedOutput: strings.Repeat("0", 64),
			expectedCount:  2,
			wantError:      "schema fixture output SHA-256 mismatch",
		},
		{
			name:           "wrong expected count",
			src:            src,
			expectedSource: sha256Hex(src),
			expectedOutput: sha256Hex(want),
			expectedCount:  1,
			wantError:      "schema fixture identifier token count = 2, want 1",
		},
		{
			name:           "zero old tokens",
			src:            []byte("package schema\n// " + sentinel + "\n"),
			expectedSource: sha256Hex([]byte("package schema\n// " + sentinel + "\n")),
			expectedOutput: sha256Hex([]byte("package schema\n// " + sentinel + "\n")),
			expectedCount:  1,
			wantError:      "schema fixture identifier token count = 0, want 1",
		},
		{
			name:           "pre-existing identifier token",
			src:            []byte("package schema\n// " + sentinel + " {\"id\":1} {\"identifier\":2}\n"),
			expectedSource: sha256Hex([]byte("package schema\n// " + sentinel + " {\"id\":1} {\"identifier\":2}\n")),
			expectedOutput: sha256Hex([]byte("unused")),
			expectedCount:  1,
			wantError:      "schema fixture already contains identifier token",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caseBefore := append([]byte(nil), tt.src...)
			_, _, err := FixSchemaFixtureIdentifiers(tt.src, tt.expectedSource, tt.expectedOutput, tt.expectedCount)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("FixSchemaFixtureIdentifiers() error = %v, want substring %q", err, tt.wantError)
			}
			if strings.Contains(err.Error(), sentinel) {
				t.Fatalf("FixSchemaFixtureIdentifiers() leaked fixture body in error: %v", err)
			}
			if !bytes.Equal(tt.src, caseBefore) {
				t.Fatal("FixSchemaFixtureIdentifiers() mutated source bytes on failure")
			}
		})
	}
}

func TestRemoveEmbeddedPrivateKeyVerifiesSHAAndSanitizesOneBlock(t *testing.T) {
	src := privateKeyFixture()
	wantPublicCertificate := rawStringPayload(t, src, "clientCert")
	sha := sha256Hex(src)
	got, count, err := RemoveEmbeddedPrivateKey(src, sha)
	if err != nil {
		t.Fatalf("RemoveEmbeddedPrivateKey() error: %v", err)
	}
	if gotCount, want := count, 1; gotCount != want {
		t.Fatalf("removed blocks = %d, want %d", gotCount, want)
	}
	text := string(got)
	for _, forbidden := range []string{"clientKey", "fixture-body", privateKeyMarker()} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("private-key output retained %q", forbidden)
		}
	}
	for _, required := range []string{
		`"encoding/pem"`, "pem.Decode", "x509.ParseCertificate", "x509Cert != nil",
		"public-cert-fixture", "if inLg.IsNeedRegister", testCoreModule,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("private-key output missing %q:\n%s", required, text)
		}
	}
	if gotPublicCertificate := rawStringPayload(t, got, "clientCert"); !bytes.Equal(gotPublicCertificate, wantPublicCertificate) {
		t.Fatalf("public certificate payload changed: got %q, want byte-identical %q", gotPublicCertificate, wantPublicCertificate)
	}
}

func TestRemoveEmbeddedPrivateKeyPreservesRuntimeDecisionsAndNilSafety(t *testing.T) {
	tests := []struct {
		name       string
		publicCert string
	}{
		{"invalid PEM", "public-cert-fixture"},
		{"PEM with invalid DER", "-----BEGIN CERTIFICATE-----\nAQID\n-----END CERTIFICATE-----"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := privateKeyFixtureWithPublicCertificate(tt.publicCert)
			transformed, _, err := RemoveEmbeddedPrivateKey(src, sha256Hex(src))
			if err != nil {
				t.Fatalf("RemoveEmbeddedPrivateKey() error: %v", err)
			}
			runPrivateKeyRuntimeFixture(t, transformed)
		})
	}
}

func TestRemoveEmbeddedPrivateKeyRejectsUnsafeInputsWithoutLeakingBody(t *testing.T) {
	src := privateKeyFixture()
	tests := []struct {
		name string
		src  []byte
		sha  string
	}{
		{"sha mismatch", src, strings.Repeat("0", 64)},
		{"zero blocks", bytes.ReplaceAll(src, []byte("var clientKey"), []byte("var removedKey")), sha256Hex(bytes.ReplaceAll(src, []byte("var clientKey"), []byte("var removedKey")))},
		{"multiple blocks", append(append([]byte(nil), src...), src...), sha256Hex(append(append([]byte(nil), src...), src...))},
		{"broken boundary", bytes.ReplaceAll(src, []byte(privateKeyMarkerEnd()), []byte("broken-boundary")), sha256Hex(bytes.ReplaceAll(src, []byte(privateKeyMarkerEnd()), []byte("broken-boundary")))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := RemoveEmbeddedPrivateKey(tt.src, tt.sha)
			if err == nil || strings.Contains(err.Error(), "fixture-body") {
				t.Fatalf("RemoveEmbeddedPrivateKey() error = %v, want safe failure", err)
			}
		})
	}
}

func TestFixDynamicFormatVerifiesSHAAndChangesOnlyFormatArgument(t *testing.T) {
	src := []byte("package fixture\n\nimport \"" + testCoreModule + "/errors\"\n\nfunc f(strProjectID string) error { return errors.Permissions.AddDetailf(strProjectID) }\n")
	got, count, err := FixDynamicFormat(src, sha256Hex(src))
	if err != nil {
		t.Fatalf("FixDynamicFormat() error: %v", err)
	}
	if gotCount, want := count, 1; gotCount != want {
		t.Fatalf("vet fixes = %d, want %d", gotCount, want)
	}
	if got := string(got); strings.Contains(got, "AddDetailf(strProjectID)") || !strings.Contains(got, `AddDetailf("%s", strProjectID)`) || !strings.Contains(got, testCoreModule) {
		t.Fatalf("unexpected vet output: %s", got)
	}
	for _, bad := range [][]byte{
		bytes.ReplaceAll(src, []byte("AddDetailf(strProjectID)"), []byte("AddDetailf(\"%s\", strProjectID)")),
		append(append([]byte(nil), src...), src...),
		append(append([]byte(nil), src...), []byte("\nvar alreadyFixed = errors.Permissions.AddDetailf(\"%s\", strProjectID)\n")...),
	} {
		_, _, err := FixDynamicFormat(bad, sha256Hex(bad))
		if err == nil {
			t.Fatal("FixDynamicFormat() accepted zero or multiple old calls")
		}
	}
	if _, _, err := FixDynamicFormat(src, strings.Repeat("0", 64)); err == nil {
		t.Fatal("FixDynamicFormat() accepted SHA mismatch")
	}
}

func TestTransformTreeRunsFullValidatedPipeline(t *testing.T) {
	files, lock := fullTransformFixture(t)

	rendered, report, err := TransformTree(files, &lock)
	if err != nil {
		t.Fatalf("TransformTree() error: %v", err)
	}
	if got, want := report, (TransformReport{CorePrefixes: 6, ThingsPrefixes: 3, CredentialLines: 1, PrivateKeyBlocks: 1, VetFixes: 1, SchemaFixtureIdentifiers: 2, NonYAMLCredentialValues: 5}); got != want {
		t.Fatalf("TransformTree() report = %+v, want %+v", got, want)
	}
	if got := string(rendered[testPrivateKeySourcePath].Data); strings.Contains(got, "fixture-body") || strings.Contains(got, testCoreModule) {
		t.Fatalf("private output was not transformed safely: %s", got)
	}
	if got := string(rendered[testVetSourcePath].Data); strings.Contains(got, "AddDetailf(strProjectID)") || !strings.Contains(got, `AddDetailf("%s", strProjectID)`) || strings.Contains(got, testCoreModule) {
		t.Fatalf("vet output was not transformed: %s", got)
	}
	if got := string(rendered["service/dgsvr/etc/dg.yaml"].Data); strings.Contains(got, "literal-db-value") || !strings.Contains(got, "${THINGS_DB_DSN}") {
		t.Fatalf("YAML output was not sanitized: %s", got)
	}
	schemaOutput := rendered[schemaFixtureSourcePath]
	if got := bytes.Count(schemaOutput.Data, []byte(`"id":`)); got != 0 {
		t.Fatalf("schema fixture output old-token count = %d, want 0", got)
	}
	if got := bytes.Count(schemaOutput.Data, []byte(`"identifier":`)); got != 2 {
		t.Fatalf("schema fixture output new-token count = %d, want 2", got)
	}
	if got, want := schemaOutput.Mode, files[schemaFixtureSourcePath].Mode; got != want {
		t.Fatalf("schema fixture output mode = %q, want original %q", got, want)
	}
	if got := string(rendered[cloudProtocolCredentialPath].Data); !strings.Contains(got, cloudAccessKeyIDPlaceholder) || !strings.Contains(got, cloudAccessKeySecretPlaceholder) || strings.Contains(got, "cloud-id-redaction-sentinel") || strings.Contains(got, "cloud-secret-redaction-sentinel") {
		t.Fatalf("cloud credential output is not sanitized, SHA-256=%s", sha256Hex([]byte(got)))
	}
	if got := string(rendered[productSecretPath].Data); strings.Count(got, productSecretPlaceholder) != 3 || strings.Contains(got, "product-secret-redaction") {
		t.Fatalf("product secret output is not sanitized, SHA-256=%s", sha256Hex([]byte(got)))
	}
}

func TestTransformTreePreflightsSpecialSourceBeforeModuleRewrite(t *testing.T) {
	files, lock := fullTransformFixture(t)
	before := cloneSourceFiles(files)
	lock.Transforms.PrivateKeySourceSHA256 = strings.Repeat("0", 64)

	_, _, err := TransformTree(files, &lock)
	if err == nil || !strings.Contains(err.Error(), "SHA-256") || strings.Contains(err.Error(), "fixture-body") {
		t.Fatalf("TransformTree() error = %v, want safe private SHA preflight failure", err)
	}
	if !sourceFilesEqual(files, before) {
		t.Fatal("TransformTree() changed input after private SHA preflight failure")
	}
}

func TestTransformTreePreflightsSchemaFixtureBeforeModuleRewrite(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]SourceFile, *Lock)
		want   string
	}{
		{
			name: "source SHA mismatch",
			mutate: func(_ map[string]SourceFile, lock *Lock) {
				lock.Transforms.SchemaFixtureSourceSHA256 = strings.Repeat("0", 64)
			},
			want: "schema fixture source SHA-256 mismatch",
		},
		{
			name: "output SHA mismatch",
			mutate: func(_ map[string]SourceFile, lock *Lock) {
				lock.Transforms.SchemaFixtureOutputSHA256 = strings.Repeat("0", 64)
			},
			want: "schema fixture output SHA-256 mismatch",
		},
		{
			name: "identifier count mismatch",
			mutate: func(_ map[string]SourceFile, lock *Lock) {
				lock.Transforms.SchemaFixtureIdentifierCount = 1
			},
			want: "schema fixture identifier token count = 2, want 1",
		},
		{
			name: "missing path",
			mutate: func(files map[string]SourceFile, _ *Lock) {
				delete(files, schemaFixtureSourcePath)
			},
			want: "missing schema fixture source",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, lock := fullTransformFixture(t)
			tt.mutate(files, &lock)
			before := cloneSourceFiles(files)

			rendered, report, err := TransformTree(files, &lock)
			if err == nil || !strings.Contains(err.Error(), tt.want) || strings.Contains(err.Error(), "schema-identifier-redaction-sentinel") {
				t.Fatalf("TransformTree() error = %v, want safe substring %q", err, tt.want)
			}
			if rendered != nil {
				t.Fatalf("TransformTree() returned rendered output on schema preflight failure: %#v", rendered)
			}
			if report != (TransformReport{}) {
				t.Fatalf("TransformTree() report = %+v on schema preflight failure, want zero", report)
			}
			if !sourceFilesEqual(files, before) {
				t.Fatal("TransformTree() mutated source map or SourceFile bytes on schema preflight failure")
			}
			if !bytes.Contains(files["README.md"].Data, []byte(testCoreModule)) || !bytes.Contains(files["README.md"].Data, []byte(testThingsModule)) {
				t.Fatal("module rewrite became visible in source input after schema preflight failure")
			}
		})
	}
}

func TestTransformTreePreflightsNonYAMLCredentialsBeforeModuleRewrite(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]SourceFile, *Lock)
		want   string
	}{
		{name: "cloud source SHA", mutate: func(_ map[string]SourceFile, lock *Lock) {
			lock.Transforms.CloudProtocolCredentialSourceSHA256 = strings.Repeat("0", 64)
		}, want: "cloud protocol credential source SHA-256 mismatch"},
		{name: "cloud output SHA", mutate: func(_ map[string]SourceFile, lock *Lock) {
			lock.Transforms.CloudProtocolCredentialOutputSHA256 = strings.Repeat("0", 64)
		}, want: "cloud protocol credential output SHA-256 mismatch"},
		{name: "cloud missing", mutate: func(files map[string]SourceFile, _ *Lock) { delete(files, cloudProtocolCredentialPath) }, want: "missing cloud protocol credential source"},
		{name: "product source SHA", mutate: func(_ map[string]SourceFile, lock *Lock) {
			lock.Transforms.ProductSecretSourceSHA256 = strings.Repeat("0", 64)
		}, want: "product secret source SHA-256 mismatch"},
		{name: "product output SHA", mutate: func(_ map[string]SourceFile, lock *Lock) {
			lock.Transforms.ProductSecretOutputSHA256 = strings.Repeat("0", 64)
		}, want: "product secret output SHA-256 mismatch"},
		{name: "product missing", mutate: func(files map[string]SourceFile, _ *Lock) { delete(files, productSecretPath) }, want: "missing product secret source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, lock := fullTransformFixture(t)
			tt.mutate(files, &lock)
			before := cloneSourceFiles(files)
			rendered, report, err := TransformTree(files, &lock)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("TransformTree() error = %v, want %q", err, tt.want)
			}
			if rendered != nil || report != (TransformReport{}) || !sourceFilesEqual(files, before) {
				t.Fatalf("TransformTree() exposed output or mutated source on non-YAML credential preflight failure")
			}
			for _, forbidden := range []string{"cloud-id-redaction-sentinel", "cloud-secret-redaction-sentinel", "product-secret-redaction"} {
				if strings.Contains(err.Error(), forbidden) {
					t.Fatalf("TransformTree() leaked non-YAML credential value: %v", err)
				}
			}
		})
	}
}

func TestVerifyTransformedTreeIsDeterministicAndScansEveryFileForPrivateKeys(t *testing.T) {
	lock := validLockForTest()
	base := map[string]RenderedFile{
		testPrivateKeySourcePath: {Mode: "100644", Data: []byte("package fixture\n")},
		testVetSourcePath:        {Mode: "100644", Data: []byte(`package fixture; var _ = AddDetailf("%s", strProjectID)`)},
	}
	first := cloneRenderedFiles(base)
	first["z.go"] = RenderedFile{Mode: "100644", Data: []byte(testCoreModule)}
	first["a.go"] = RenderedFile{Mode: "100644", Data: []byte(testThingsModule)}
	second := map[string]RenderedFile{
		"a.go":                   first["a.go"],
		"z.go":                   first["z.go"],
		testVetSourcePath:        first[testVetSourcePath],
		testPrivateKeySourcePath: first[testPrivateKeySourcePath],
	}
	errFirst := verifyTransformedTree(first, &lock)
	errSecond := verifyTransformedTree(second, &lock)
	if errFirst == nil || errSecond == nil || errFirst.Error() != errSecond.Error() || !strings.HasPrefix(errFirst.Error(), "a.go:") {
		t.Fatalf("deterministic verification errors = %v and %v, want identical lexicographically-first path", errFirst, errSecond)
	}

	privateElsewhere := cloneRenderedFiles(base)
	privateElsewhere["unrelated.txt"] = RenderedFile{Mode: "100644", Data: []byte("prefix `  -----" + "BEGIN DSA PRIVATE KEY" + "-----` suffix")}
	err := verifyTransformedTree(privateElsewhere, &lock)
	if err == nil || !strings.Contains(err.Error(), "unrelated.txt: private-key marker") {
		t.Fatalf("verifyTransformedTree() error = %v, want all-file private-key rejection", err)
	}
}

func TestTransformRealLockedSourceWhenEnabled(t *testing.T) {
	repository := os.Getenv("THINGS_BASELINE_SOURCE_REPOSITORY")
	lockPath := os.Getenv("THINGS_BASELINE_LOCK")
	if repository == "" || lockPath == "" {
		t.Skip("set THINGS_BASELINE_SOURCE_REPOSITORY and THINGS_BASELINE_LOCK to run fixed-source transform integration")
	}
	lock, err := LoadLock(lockPath)
	if err != nil {
		t.Fatalf("LoadLock(%q): %v", lockPath, err)
	}
	snapshot, err := ReadSource(context.Background(), repository, lock)
	if err != nil {
		t.Fatalf("ReadSource(%q): %v", repository, err)
	}
	files := make(map[string]SourceFile, len(snapshot.Files))
	for _, file := range snapshot.Files {
		files[file.Path] = file
	}
	_, report, err := TransformTree(files, lock)
	if err != nil {
		t.Fatalf("TransformTree() fixed-source integration: %v", err)
	}
	want := TransformReport{
		CorePrefixes:             94,
		ThingsPrefixes:           2726,
		CredentialLines:          55,
		PrivateKeyBlocks:         1,
		VetFixes:                 1,
		SchemaFixtureIdentifiers: 45,
		NonYAMLCredentialValues:  5,
	}
	if report != want {
		t.Fatalf("TransformTree() fixed-source report = %+v, want %+v", report, want)
	}
}

func moduleLockForTest(core, things int) Lock {
	lock := validLockForTest()
	lock.Transforms.CorePrefixCount = core
	lock.Transforms.ThingsPrefixCount = things
	lock.Transforms.CredentialLineCount = 0
	lock.Transforms.PrivateKeyBlockCount = 0
	lock.Transforms.VetFixCount = 0
	return lock
}

func moduleFilesForTest() map[string]SourceFile {
	return map[string]SourceFile{
		"main.go": {
			Path: "main.go", Mode: "100644",
			Data: []byte("package fixture\n\nimport _ \"" + testCoreModule + "/x\"\nimport _ \"" + testThingsModule + "/sdk\"\n"),
		},
		"README.md": {
			Path: "README.md", Mode: "100644",
			Data: []byte(testCoreModule + " " + testThingsModule + " " + testShareModule + "\n"),
		},
		"go.sum": {
			Path: "go.sum", Mode: "100644",
			Data: []byte(testCoreModule + " v1.2.3 h1:unchanged\n"),
		},
		"go.mod": {
			Path: "go.mod", Mode: "100644",
			Data: []byte(`module gitee.com/unitedrhino/things

//gitee.com/unitedrhino/core => ../core
//gitee.com/ming-iot/share => ../share
require gitee.com/unitedrhino/core v1.2.3

replace (
	cloud.google.com/go => cloud.google.com/go v0.110.10
	cloud.google.com/go/compute/metadata => cloud.google.com/go/compute/metadata v0.6.0
)
`),
		},
	}
}

func moduleFilesWithSaaSReplace() map[string]SourceFile {
	files := moduleFilesForTest()
	files["go.mod"] = SourceFile{Path: "go.mod", Mode: "100644", Data: []byte(strings.Replace(string(files["go.mod"].Data), ")\n", "\tgitee.com/ming-iot/saas => ../saas\n)\n", 1))}
	return files
}

func moduleFilesWithLocalCloudReplace() map[string]SourceFile {
	files := moduleFilesForTest()
	files["go.mod"] = SourceFile{Path: "go.mod", Mode: "100644", Data: []byte(strings.Replace(string(files["go.mod"].Data), "cloud.google.com/go v0.110.10", "../cloud", 1))}
	return files
}

func moduleFilesWithAlternateSaaSReplace() map[string]SourceFile {
	files := moduleFilesForTest()
	files["go.mod"] = SourceFile{Path: "go.mod", Mode: "100644", Data: []byte(strings.Replace(string(files["go.mod"].Data), ")\n", "\tgitee.com/ming-iot/saas => ../other-saas\n)\n", 1))}
	return files
}

func moduleFilesWithChangedCloudVersion() map[string]SourceFile {
	files := moduleFilesForTest()
	files["go.mod"] = SourceFile{Path: "go.mod", Mode: "100644", Data: []byte(strings.Replace(string(files["go.mod"].Data), "v0.110.10", "v0.110.9", 1))}
	return files
}

func moduleFilesWithCommentCloudDecoy() map[string]SourceFile {
	files := moduleFilesWithLocalCloudReplace()
	text := string(files["go.mod"].Data) + "// cloud.google.com/go => cloud.google.com/go v0.110.10\n"
	files["go.mod"] = SourceFile{Path: "go.mod", Mode: "100644", Data: []byte(text)}
	return files
}

func moduleFilesWithCommentReplaceBlockDecoy() map[string]SourceFile {
	files := moduleFilesForTest()
	replaceBlock := `replace (
	cloud.google.com/go => cloud.google.com/go v0.110.10
	cloud.google.com/go/compute/metadata => cloud.google.com/go/compute/metadata v0.6.0
)
`
	commentDecoy := `// replace (
// cloud.google.com/go => cloud.google.com/go v0.110.10
// cloud.google.com/go/compute/metadata => cloud.google.com/go/compute/metadata v0.6.0
// )
`
	text := strings.Replace(string(files["go.mod"].Data), replaceBlock, commentDecoy, 1)
	files["go.mod"] = SourceFile{Path: "go.mod", Mode: "100644", Data: []byte(text)}
	return files
}

func moduleFilesWithExtraReplace() map[string]SourceFile {
	files := moduleFilesForTest()
	text := strings.Replace(string(files["go.mod"].Data), ")\n", "\texample.invalid/x => example.invalid/x v1.0.0\n)\n", 1)
	files["go.mod"] = SourceFile{Path: "go.mod", Mode: "100644", Data: []byte(text)}
	return files
}

func moduleFilesWithDuplicateCloudReplace() map[string]SourceFile {
	files := moduleFilesForTest()
	line := "\tcloud.google.com/go => cloud.google.com/go v0.110.10\n"
	text := strings.Replace(string(files["go.mod"].Data), ")\n", line+")\n", 1)
	files["go.mod"] = SourceFile{Path: "go.mod", Mode: "100644", Data: []byte(text)}
	return files
}

func moduleLockWithSaaSPathForTest(path string) Lock {
	lock := moduleLockForTest(4, 3)
	lock.Target.SaaSRelativePath = path
	return lock
}

func fullTransformFixture(t *testing.T) (map[string]SourceFile, Lock) {
	t.Helper()
	files := moduleFilesForTest()
	privatePath := testPrivateKeySourcePath
	vetPath := testVetSourcePath
	privateSource := privateKeyFixture()
	vetSource := []byte("package fixture\n\nimport \"" + testCoreModule + "/errors\"\n\nfunc f(strProjectID string) error { return errors.Permissions.AddDetailf(strProjectID) }\n")
	schemaFixtureSource := schemaFixtureForTest()
	cloudCredentialSource, cloudCredentialOutput := cloudCredentialFixtureForTest()
	productSecretSource, productSecretOutput := productSecretFixtureForTest()
	files[privatePath] = SourceFile{Path: privatePath, Mode: "100644", Data: privateSource}
	files[vetPath] = SourceFile{Path: vetPath, Mode: "100644", Data: vetSource}
	files[schemaFixtureSourcePath] = SourceFile{Path: schemaFixtureSourcePath, Mode: "100644", Data: schemaFixtureSource}
	files[cloudProtocolCredentialPath] = SourceFile{Path: cloudProtocolCredentialPath, Mode: "100644", Data: cloudCredentialSource}
	files[productSecretPath] = SourceFile{Path: productSecretPath, Mode: "100644", Data: productSecretSource}
	files["service/dgsvr/etc/dg.yaml"] = SourceFile{
		Path: "service/dgsvr/etc/dg.yaml", Mode: "100644",
		Data: []byte("Database:\n  DSN: literal-db-value\n"),
	}

	lock := validLockForTest()
	lock.Transforms.CorePrefixCount = 6
	lock.Transforms.ThingsPrefixCount = 3
	lock.Transforms.CredentialLineCount = 1
	lock.Transforms.CredentialFileCount = 1
	lock.Transforms.CredentialActiveLineCount = 1
	lock.Transforms.CredentialCommentedLineCount = 0
	lock.Transforms.CredentialPaths = CredentialPathCounts{DatabaseDSN: 1}
	lock.Transforms.PrivateKeyBlockCount = 1
	lock.Transforms.VetFixCount = 1
	lock.Transforms.PrivateKeySourceSHA256 = sha256Hex(privateSource)
	lock.Transforms.VetSourceSHA256 = sha256Hex(vetSource)
	lock.Transforms.SchemaFixtureIdentifierCount = 2
	lock.Transforms.SchemaFixtureSourceSHA256 = sha256Hex(schemaFixtureSource)
	lock.Transforms.SchemaFixtureOutputSHA256 = sha256Hex(bytes.ReplaceAll(schemaFixtureSource, []byte(`"id":`), []byte(`"identifier":`)))
	lock.Transforms.NonYAMLCredentialValueCount = 5
	lock.Transforms.CloudProtocolCredentialSourceSHA256 = sha256Hex(cloudCredentialSource)
	lock.Transforms.CloudProtocolCredentialOutputSHA256 = sha256Hex(cloudCredentialOutput)
	lock.Transforms.ProductSecretSourceSHA256 = sha256Hex(productSecretSource)
	lock.Transforms.ProductSecretOutputSHA256 = sha256Hex(productSecretOutput)
	return files, lock
}

func schemaFixtureForTest() []byte {
	return []byte("package schema\n\nconst fixture = `schema-identifier-redaction-sentinel {\"id\":\"temperature\"} {\"id\":\"humidity\"}`\n")
}

func cloudCredentialFixtureForTest() ([]byte, []byte) {
	source := []byte("package protocol\n\n/*\n{\n\"accessKeyID\":\"cloud-id-redaction-sentinel\",\n\"accessKeySecret\":\"cloud-secret-redaction-sentinel\",\n\"regionID\":\"region\"\n}\n*/\n")
	output := bytes.Replace(source, []byte(`"accessKeyID":"cloud-id-redaction-sentinel"`), []byte(`"accessKeyID":"${THINGS_CLOUD_ACCESS_KEY_ID}"`), 1)
	output = bytes.Replace(output, []byte(`"accessKeySecret":"cloud-secret-redaction-sentinel"`), []byte(`"accessKeySecret":"${THINGS_CLOUD_ACCESS_KEY_SECRET}"`), 1)
	return append([]byte(nil), source...), append([]byte(nil), output...)
}

func productSecretFixtureForTest() ([]byte, []byte) {
	source := []byte(`[{"Info":{"Secret":"product-secret-redaction-one","Name":"one"}},{"Info":{"Secret":"product-secret-redaction-two"}},{"Info":{"Secret":"product-secret-redaction-three"}}]`)
	output := append([]byte(nil), source...)
	for _, old := range []string{"product-secret-redaction-one", "product-secret-redaction-two", "product-secret-redaction-three"} {
		output = bytes.Replace(output, []byte(old), []byte(productSecretPlaceholder), 1)
	}
	return append([]byte(nil), source...), output
}

func cloneSourceFiles(files map[string]SourceFile) map[string]SourceFile {
	clone := make(map[string]SourceFile, len(files))
	for key, file := range files {
		file.Data = append([]byte(nil), file.Data...)
		clone[key] = file
	}
	return clone
}

func cloneRenderedFiles(files map[string]RenderedFile) map[string]RenderedFile {
	clone := make(map[string]RenderedFile, len(files))
	for key, file := range files {
		file.Data = append([]byte(nil), file.Data...)
		clone[key] = file
	}
	return clone
}

func sourceFilesEqual(a, b map[string]SourceFile) bool {
	if len(a) != len(b) {
		return false
	}
	for key, left := range a {
		right, ok := b[key]
		if !ok || left.Path != right.Path || left.Mode != right.Mode || !bytes.Equal(left.Data, right.Data) {
			return false
		}
	}
	return true
}

func privateKeyFixture() []byte {
	return privateKeyFixtureWithPublicCertificate("public-cert-fixture")
}

func privateKeyFixtureWithPublicCertificate(publicCertificate string) []byte {
	return []byte(`package fixture

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
)

// Dependency contract retained for module-prefix verification: ` + testCoreModule + `/ctxs.
var clientCert string = ` + "`" + publicCertificate + "`" + `
var clientKey string = ` + "`" + privateKeyMarker() + `
fixture-body
` + privateKeyMarkerEnd() + "`" + `
var clientCertificate tls.Certificate
var x509Cert *x509.Certificate

func init() {
	clientCertificate, _ = tls.X509KeyPair([]byte(clientCert), []byte(clientKey))
	x509Cert, _ = x509.ParseCertificate(clientCertificate.Certificate[0])
}

func Login(in CertificateInput, inLg LoginInfo) string {
	if len(in.Certificate) > 0 {
		_ = bytes.Equal(in.Certificate, x509Cert.Signature)
		_ = x509Cert.Raw
	}
	if inLg.IsNeedRegister { return "register" }
	if !inLg.IdentityMatches { return "identity-denied" }
	if !inLg.CacheOK { return "cache-denied" }
	if !inLg.PasswordOK { return "password-denied" }
	return "allowed"
}

type CertificateInput struct { Certificate []byte }
type LoginInfo struct {
	IsNeedRegister bool
	IdentityMatches bool
	CacheOK bool
	PasswordOK bool
}
`)
}

func rawStringPayload(t *testing.T, src []byte, variable string) []byte {
	t.Helper()
	declaration := []byte("var " + variable + " string = `")
	start := bytes.Index(src, declaration)
	if start < 0 {
		t.Fatalf("source has no raw string declaration for %s", variable)
	}
	start += len(declaration)
	end := bytes.IndexByte(src[start:], '`')
	if end < 0 {
		t.Fatalf("source has no raw string terminator for %s", variable)
	}
	return append([]byte(nil), src[start:start+end]...)
}

func runPrivateKeyRuntimeFixture(t *testing.T, transformed []byte) {
	t.Helper()
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte("module fixture\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatalf("write runtime fixture go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "login.go"), transformed, 0o600); err != nil {
		t.Fatalf("write transformed runtime fixture: %v", err)
	}
	runtimeTest := []byte(`package fixture

import (
	"crypto/x509"
	"testing"
)

func TestTransformedLoginRuntime(t *testing.T) {
	// Invalid/malformed embedded public certificates leave x509Cert nil. The
	// diagnostic certificate branch must remain safe and authorization-neutral.
	if got := Login(CertificateInput{Certificate: []byte("probe")}, LoginInfo{IsNeedRegister: true}); got != "register" {
		t.Fatalf("nil-certificate registration decision = %q", got)
	}

	cases := []struct {
		name string
		login LoginInfo
		want string
	}{
		{"registration", LoginInfo{IsNeedRegister: true}, "register"},
		{"identity mismatch", LoginInfo{}, "identity-denied"},
		{"cache failure", LoginInfo{IdentityMatches: true}, "cache-denied"},
		{"password failure", LoginInfo{IdentityMatches: true, CacheOK: true}, "password-denied"},
		{"success", LoginInfo{IdentityMatches: true, CacheOK: true, PasswordOK: true}, "allowed"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := Login(CertificateInput{}, tt.login); got != tt.want {
				t.Fatalf("Login() = %q, want %q", got, tt.want)
			}
		})
	}

	x509Cert = &x509.Certificate{Signature: []byte("signature"), Raw: []byte("raw")}
	if got := Login(CertificateInput{Certificate: []byte("signature")}, LoginInfo{IdentityMatches: true, CacheOK: true, PasswordOK: true}); got != "allowed" {
		t.Fatalf("certificate diagnostic changed successful decision: %q", got)
	}
}
`)
	if err := os.WriteFile(filepath.Join(directory, "login_test.go"), runtimeTest, 0o600); err != nil {
		t.Fatalf("write runtime test fixture: %v", err)
	}
	command := exec.Command("go", "test", "./...")
	command.Dir = directory
	command.Env = append(os.Environ(), "GOWORK=off", "GOENV=off")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("transformed private-key runtime fixture failed: %v\n%s", err, output)
	}
}

func privateKeyMarker() string {
	return "-----" + "BEGIN " + "RSA " + "PRIVATE KEY" + "-----"
}

func privateKeyMarkerEnd() string {
	return "-----" + "END " + "RSA " + "PRIVATE KEY" + "-----"
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}
