// 本文件验证 Kafka 仓库门禁的稳定结果与失败原因码。
package kafkacontract

import (
	"path/filepath"
	"testing"
)

func TestValidateCatalogAcceptsExplicitManagedTopic(t *testing.T) {
	document, err := LoadCatalog(filepath.Join("testdata", "catalog-valid.yaml"))
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	if issues := ValidateCatalog(document, "catalog-valid.yaml"); len(issues) != 0 {
		t.Fatalf("ValidateCatalog() issues = %#v", issues)
	}
}

func TestValidateCatalogRejectsDynamicTopicWithStableCode(t *testing.T) {
	document, err := LoadCatalog(filepath.Join("testdata", "catalog-dynamic-topic.yaml"))
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	issues := ValidateCatalog(document, "catalog-dynamic-topic.yaml")
	if !hasIssueCode(issues, CodeCatalogDynamicTopic) {
		t.Fatalf("ValidateCatalog() issues = %#v, want %s", issues, CodeCatalogDynamicTopic)
	}
}

func TestValidateCatalogRejectsMissingAndUnsafeContracts(t *testing.T) {
	document, err := LoadCatalog(filepath.Join("testdata", "catalog-missing-contract.yaml"))
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	issues := ValidateCatalog(document, "catalog-missing-contract.yaml")
	for _, code := range []string{
		CodeCatalogGenerationRequired,
		CodeCatalogRetentionRequired,
		CodeCatalogOwnerRequired,
		CodeCatalogKeyCodecRequired,
		CodeCatalogTimestampUnsafe,
		CodeProductionDurabilityUnsafe,
		CodeProductionAutoCreateEnabled,
	} {
		if !hasIssueCode(issues, code) {
			t.Errorf("ValidateCatalog() missing issue code %s in %#v", code, issues)
		}
	}
}

func TestValidateRepositoryAcceptsCurrentKafkaContract(t *testing.T) {
	report := ValidateRepository(repositoryRoot(t))
	if !report.OK() {
		t.Fatalf("ValidateRepository() issues = %#v", report.Issues)
	}
	if report.KafkaVersion != "4.3.1" || report.FranzGoVersion != "v1.20.5" || report.GoVersion != "1.24.4" {
		t.Fatalf("ValidateRepository() versions = Kafka %q, franz-go %q, Go %q", report.KafkaVersion, report.FranzGoVersion, report.GoVersion)
	}
	if report.CatalogEntries != 7 || report.CatalogDigest == "" {
		t.Fatalf("ValidateRepository() catalog entries = %d, digest = %q", report.CatalogEntries, report.CatalogDigest)
	}
}

func TestFrozenCatalogRejectsSemanticDriftWithUnchangedPhysicalNames(t *testing.T) {
	document, err := LoadCatalog(filepath.Join(repositoryRoot(t), "deploy", "kafka", "catalog", "data-plane.yaml"))
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	document.ManagedTopics[0].LogicalName = "iot.data.canonical.replaced.v1"
	document.ManagedTopics[0].FactKind = "replaced-kind"
	issues := validateFrozenCatalog(document, "data-plane.yaml")
	if !hasIssueCode(issues, CodeCatalogFrozenSetMismatch) {
		t.Fatalf("validateFrozenCatalog() issues = %#v, want %s", issues, CodeCatalogFrozenSetMismatch)
	}
}

func hasIssueCode(issues []Issue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
