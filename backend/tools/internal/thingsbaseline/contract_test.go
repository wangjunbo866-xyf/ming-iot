package thingsbaseline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validLockForTest() Lock {
	return Lock{
		SchemaVersion: 1,
		LoopID:        "L0",
		Source: SourceContract{
			Repository:        "https://gitee.com/unitedrhino/things",
			Ref:               "refs/remotes/origin/master",
			Commit:            "243ab1aa03b8a3f106761a81cdd9bd8827d448c8",
			Tree:              "544b638ae3ee5e5f3a041bea22ca035dbb7e5859",
			Module:            "gitee.com/unitedrhino/things",
			TrackedFiles:      2,
			SelectedFiles:     1,
			SelectedBlobBytes: 1,
			ArchiveBytes:      1,
			ArchiveSHA256:     strings.Repeat("a", 64),
			Include:           []string{"src"},
			Exclude:           []string{"docs"},
		},
		Target: TargetContract{
			Directory:        "things",
			Module:           "gitee.com/ming-iot/things",
			SaaSModule:       "gitee.com/ming-iot/saas",
			SaaSRelativePath: "../saas",
			ShareModule:      "gitee.com/ming-iot/share",
			ShareVersion:     "v1.6.4",
		},
		Transforms: TransformContract{
			CorePrefixCount:              1,
			ThingsPrefixCount:            1,
			CredentialLineCount:          1,
			CredentialFileCount:          1,
			CredentialActiveLineCount:    1,
			CredentialCommentedLineCount: 0,
			CredentialPaths: CredentialPathCounts{
				DatabaseDSN: 1,
			},
			PrivateKeyBlockCount:                1,
			VetFixCount:                         1,
			PrivateKeySourceSHA256:              strings.Repeat("b", 64),
			VetSourceSHA256:                     strings.Repeat("c", 64),
			SchemaFixtureIdentifierCount:        1,
			SchemaFixtureSourceSHA256:           strings.Repeat("d", 64),
			SchemaFixtureOutputSHA256:           strings.Repeat("e", 64),
			NonYAMLCredentialValueCount:         5,
			CloudProtocolCredentialSourceSHA256: strings.Repeat("1", 64),
			CloudProtocolCredentialOutputSHA256: strings.Repeat("2", 64),
			ProductSecretSourceSHA256:           strings.Repeat("3", 64),
			ProductSecretOutputSHA256:           strings.Repeat("4", 64),
		},
		Contracts: ContractCounts{
			APIFiles:            1,
			ProtoFiles:          1,
			GeneratedProtoFiles: 1,
			TestFiles:           1,
		},
		Security: SecurityContract{MaxFileBytes: 1},
	}
}

func TestValidateLockRejectsUnsafeContracts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Lock)
		want   string
	}{
		{"short commit", func(l *Lock) { l.Source.Commit = "243ab1aa" }, "40 lowercase hex"},
		{"uppercase commit", func(l *Lock) { l.Source.Commit = strings.ToUpper(l.Source.Commit) }, "40 lowercase hex"},
		{"moving ref", func(l *Lock) { l.Source.Ref = "master" }, "refs/remotes/"},
		{"traversal ref", func(l *Lock) { l.Source.Ref = "refs/remotes/origin/../master" }, "unsafe source.ref"},
		{"absolute include", func(l *Lock) { l.Source.Include = []string{"/secrets"} }, "unsafe include"},
		{"dot include", func(l *Lock) { l.Source.Include = []string{"."} }, "unsafe include"},
		{"dotdot include", func(l *Lock) { l.Source.Include = []string{".."} }, "unsafe include"},
		{"traversal include", func(l *Lock) { l.Source.Include = []string{"src/../secrets"} }, "unsafe include"},
		{"backslash include", func(l *Lock) { l.Source.Include = []string{`src\protocol`} }, "unsafe include"},
		{"tab include", func(l *Lock) { l.Source.Include = []string{"src\tprotocol"} }, "unsafe include"},
		{"newline include", func(l *Lock) { l.Source.Include = []string{"src\nprotocol"} }, "unsafe include"},
		{"duplicate include", func(l *Lock) { l.Source.Include = []string{"src", "src"} }, "duplicate include"},
		{"overlapping include", func(l *Lock) { l.Source.Include = []string{"src", "src/api"} }, "overlapping include"},
		{"duplicate exclude", func(l *Lock) { l.Source.Exclude = []string{"docs", "docs"} }, "duplicate exclude"},
		{"overlapping exclude", func(l *Lock) { l.Source.Exclude = []string{"docs", "docs/api"} }, "overlapping exclude"},
		{"include excludes same root", func(l *Lock) { l.Source.Exclude = []string{"src"} }, "overlaps exclude"},
		{"include contains excluded root", func(l *Lock) { l.Source.Exclude = []string{"src/api"} }, "overlaps exclude"},
		{"exclude contains include", func(l *Lock) {
			l.Source.Include = []string{"service/apisvr"}
			l.Source.Exclude = []string{"service"}
		}, "overlaps exclude"},
		{"include contains exclude", func(l *Lock) {
			l.Source.Include = []string{"service"}
			l.Source.Exclude = []string{"service/apisvr"}
		}, "overlaps exclude"},
		{"unsafe dot exclude", func(l *Lock) { l.Source.Exclude = []string{"."} }, "unsafe exclude"},
		{"zero tracked files", func(l *Lock) { l.Source.TrackedFiles = 0 }, "trackedFiles"},
		{"zero selected files", func(l *Lock) { l.Source.SelectedFiles = 0 }, "selectedFiles"},
		{"zero selected bytes", func(l *Lock) { l.Source.SelectedBlobBytes = 0 }, "selectedBlobBytes"},
		{"zero archive bytes", func(l *Lock) { l.Source.ArchiveBytes = 0 }, "archiveBytes"},
		{"zero core transforms", func(l *Lock) { l.Transforms.CorePrefixCount = 0 }, "corePrefixCount"},
		{"zero things transforms", func(l *Lock) { l.Transforms.ThingsPrefixCount = 0 }, "thingsPrefixCount"},
		{"zero credential transforms", func(l *Lock) { l.Transforms.CredentialLineCount = 0 }, "credentialLineCount"},
		{"zero private key transforms", func(l *Lock) { l.Transforms.PrivateKeyBlockCount = 0 }, "privateKeyBlockCount"},
		{"zero vet transforms", func(l *Lock) { l.Transforms.VetFixCount = 0 }, "vetFixCount"},
		{"zero schema fixture identifiers", func(l *Lock) { l.Transforms.SchemaFixtureIdentifierCount = 0 }, "schemaFixtureIdentifierCount"},
		{"invalid schema fixture source SHA-256", func(l *Lock) { l.Transforms.SchemaFixtureSourceSHA256 = "bad" }, "transform source SHA-256"},
		{"invalid schema fixture output SHA-256", func(l *Lock) { l.Transforms.SchemaFixtureOutputSHA256 = strings.Repeat("F", 64) }, "transform output SHA-256"},
		{"wrong non-YAML credential count", func(l *Lock) { l.Transforms.NonYAMLCredentialValueCount = 4 }, "nonYamlCredentialValueCount must be 5"},
		{"invalid cloud credential source SHA-256", func(l *Lock) { l.Transforms.CloudProtocolCredentialSourceSHA256 = "bad" }, "non-YAML credential SHA-256"},
		{"invalid cloud credential output SHA-256", func(l *Lock) { l.Transforms.CloudProtocolCredentialOutputSHA256 = strings.Repeat("F", 64) }, "non-YAML credential SHA-256"},
		{"invalid product secret source SHA-256", func(l *Lock) { l.Transforms.ProductSecretSourceSHA256 = "bad" }, "non-YAML credential SHA-256"},
		{"invalid product secret output SHA-256", func(l *Lock) { l.Transforms.ProductSecretOutputSHA256 = strings.Repeat("F", 64) }, "non-YAML credential SHA-256"},
		{"zero api contracts", func(l *Lock) { l.Contracts.APIFiles = 0 }, "apiFiles"},
		{"zero proto contracts", func(l *Lock) { l.Contracts.ProtoFiles = 0 }, "protoFiles"},
		{"zero generated proto contracts", func(l *Lock) { l.Contracts.GeneratedProtoFiles = 0 }, "generatedProtoFiles"},
		{"zero test contracts", func(l *Lock) { l.Contracts.TestFiles = 0 }, "testFiles"},
		{"zero max file bytes", func(l *Lock) { l.Security.MaxFileBytes = 0 }, "maxFileBytes"},
		{"unitedrhino target", func(l *Lock) { l.Target.Module = "gitee.com/unitedrhino/things" }, "target module"},
		{"same source target", func(l *Lock) { l.Target.Module = l.Source.Module }, "target module"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lock := validLockForTest()
			tt.mutate(&lock)
			err := ValidateLock(&lock)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateLock() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestValidateLockPositiveErrorsUseStableFullFieldNames(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Lock)
		want   string
	}{
		{
			name: "transform order",
			mutate: func(l *Lock) {
				l.Transforms.CorePrefixCount = 0
				l.Transforms.ThingsPrefixCount = 0
			},
			want: "transforms.corePrefixCount must be positive",
		},
		{
			name: "contract order",
			mutate: func(l *Lock) {
				l.Contracts.APIFiles = 0
				l.Contracts.ProtoFiles = 0
			},
			want: "contracts.apiFiles must be positive",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lock := validLockForTest()
			tt.mutate(&lock)
			if got := ValidateLock(&lock); got == nil || got.Error() != tt.want {
				t.Fatalf("ValidateLock() error = %v, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateLockRejectsInvalidCredentialProfile(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Lock)
		want   string
	}{
		{"zero file count", func(lock *Lock) { lock.Transforms.CredentialFileCount = 0 }, "transforms.credentialFileCount must be positive"},
		{"too many credential files", func(lock *Lock) { lock.Transforms.CredentialFileCount = lock.Source.SelectedFiles + 1 }, "transforms.credentialFileCount must not exceed source.selectedFiles"},
		{"negative active count", func(lock *Lock) { lock.Transforms.CredentialActiveLineCount = -1 }, "transforms.credentialActiveLineCount must not be negative"},
		{"negative commented count", func(lock *Lock) { lock.Transforms.CredentialCommentedLineCount = -1 }, "transforms.credentialCommentedLineCount must not be negative"},
		{"active commented mismatch", func(lock *Lock) { lock.Transforms.CredentialActiveLineCount++ }, "credentialActiveLineCount + transforms.credentialCommentedLineCount"},
		{"path sum mismatch", func(lock *Lock) { lock.Transforms.CredentialPaths.DatabaseDSN-- }, "transforms.credentialPaths sum"},
		{"negative path", func(lock *Lock) { lock.Transforms.CredentialPaths.DatabaseDSN = -1 }, "transforms.credentialPaths.databaseDSN must not be negative"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lock := validLockForTest()
			tt.mutate(&lock)
			if got := ValidateLock(&lock); got == nil || !strings.Contains(got.Error(), tt.want) {
				t.Fatalf("ValidateLock() error = %v, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateLockRequiresFixedSaaSRelativePath(t *testing.T) {
	lock := validLockForTest()
	lock.Target.SaaSRelativePath = "/tmp/saas"
	if err := ValidateLock(&lock); err == nil || !strings.Contains(err.Error(), "target.saasRelativePath") {
		t.Fatalf("ValidateLock() error = %v, want fixed SaaS relative-path failure", err)
	}
}

func TestValidateLockRejectsInvalidRemoteRefNames(t *testing.T) {
	invalidRefs := []string{
		"refs/remotes/.origin/master",
		"refs/remotes/origin/master.lock",
		"refs/remotes/origin/ma..ster",
		"refs/remotes/origin/ma@{ster",
		"refs/remotes/origin/master.",
		"refs/remotes/origin/@",
		"refs/remotes/origin/master name",
		"refs/remotes/origin/master~",
		"refs/remotes/origin/master^",
		"refs/remotes/origin/master:name",
		"refs/remotes/origin/master?",
		"refs/remotes/origin/master*",
		"refs/remotes/origin/master[",
		`refs/remotes/origin/master\name`,
		"refs/remotes/origin/master\x01name",
	}
	for _, ref := range invalidRefs {
		t.Run(ref, func(t *testing.T) {
			lock := validLockForTest()
			lock.Source.Ref = ref
			err := ValidateLock(&lock)
			if err == nil || !strings.Contains(err.Error(), "unsafe source.ref") {
				t.Fatalf("ValidateLock() error = %v, want unsafe source.ref", err)
			}
		})
	}
}

func TestLoadLockRejectsUnknownAndTrailingDocuments(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "unknown field",
			yaml: "schemaVersion: 1\nloopId: L0\nunknown: value\n",
			want: "field unknown",
		},
		{
			name: "second document",
			yaml: "schemaVersion: 1\nloopId: L0\n---\nschemaVersion: 1\nloopId: L0\n",
			want: "multiple YAML documents",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "lock.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			_, err := LoadLock(path)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadLock() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}
