// 本文件验证 Kafka 部署契约的静态安全边界。
package kafkacontract

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDeployContractRequiresPinnedLocalAndProductionProfiles(t *testing.T) {
	repoRoot := repositoryRoot(t)
	compose := readRepositoryFile(t, repoRoot, "deploy/kafka/compose.yaml")
	versionLock := readRepositoryFile(t, repoRoot, "deploy/kafka/version-lock.json")
	local := readRepositoryFile(t, repoRoot, "deploy/kafka/config/local/server.properties")
	production := readRepositoryFile(t, repoRoot, "deploy/kafka/config/production/server.properties")

	assertContains(t, compose, "apache/kafka:4.3.1@sha256:")
	assertNotContains(t, strings.ToLower(compose), ":latest")
	assertContains(t, compose, "local-only")
	assertContains(t, compose, `"127.0.0.1:39092:39092"`)
	assertContains(t, compose, "kafka_data:/var/lib/kafka/data")
	assertContains(t, versionLock, `"version": "4.3.1"`)
	assertContains(t, versionLock, `"digest": "sha256:`)

	assertContains(t, local, "ming-iot.profile=local-only")
	assertContains(t, local, "process.roles=broker,controller")
	assertContains(t, local, "EXTERNAL://localhost:39092")
	assertContains(t, local, "log.dirs=/var/lib/kafka/data")
	assertContains(t, local, "auto.create.topics.enable=false")
	assertContains(t, local, "unclean.leader.election.enable=false")
	assertContains(t, local, "log.message.timestamp.type=LogAppendTime")

	assertContains(t, production, "ming-iot.profile=production-static-contract")
	assertContains(t, production, "config.providers=file")
	assertContains(t, production, "config.providers.file.class=org.apache.kafka.common.config.provider.FileConfigProvider")
	assertContains(t, production, "ssl.keystore.password=${file:/run/secrets/kafka/passwords.properties:keystore.password}")
	assertContains(t, production, "default.replication.factor=3")
	assertContains(t, production, "min.insync.replicas=2")
	assertContains(t, production, "offsets.topic.replication.factor=3")
	assertContains(t, production, "transaction.state.log.replication.factor=3")
	assertContains(t, production, "transaction.state.log.min.isr=2")
	assertContains(t, production, "auto.create.topics.enable=false")
	assertContains(t, production, "unclean.leader.election.enable=false")
	assertContains(t, production, "log.message.timestamp.type=LogAppendTime")
	assertNotContains(t, production, "PLAINTEXT")
}

func TestDeployContractCatalogAndScriptsAreExplicitAndPortable(t *testing.T) {
	repoRoot := repositoryRoot(t)
	catalog := readRepositoryFile(t, repoRoot, "deploy/kafka/catalog/data-plane.yaml")
	bootstrap := readRepositoryFile(t, repoRoot, "deploy/kafka/scripts/bootstrap-topics.sh")
	readiness := readRepositoryFile(t, repoRoot, "deploy/kafka/scripts/readiness.sh")

	assertContains(t, catalog, "timestamp_type: LogAppendTime")
	assertContains(t, catalog, "auto_create: false")
	assertContains(t, catalog, "replication_factor: 3")
	assertContains(t, catalog, "min_isr: 2")
	assertNotContains(t, catalog, "*")
	assertContains(t, readiness, "docker inspect")
	assertContains(t, readiness, "exited|dead")
	assertContains(t, bootstrap, "local-only bootstrap requires replication_factor=1 and min_isr=1")

	for _, content := range []string{catalog, bootstrap, readiness} {
		assertNotContains(t, content, "/home/")
		assertNotContains(t, content, "/Users/")
		assertNotContains(t, strings.ToLower(content), "password=")
		assertNotContains(t, strings.ToLower(content), "token=")
		assertNotContains(t, strings.ToLower(content), "private_key=")
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate deploy contract test")
	}
	root, err := filepath.Abs(filepath.Join(filepath.Dir(filename), "..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

func readRepositoryFile(t *testing.T, repoRoot, relativePath string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(relativePath)))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}
	return string(content)
}

func assertContains(t *testing.T, content, expected string) {
	t.Helper()
	if !strings.Contains(content, expected) {
		t.Fatalf("expected content to contain %q", expected)
	}
}

func assertNotContains(t *testing.T, content, forbidden string) {
	t.Helper()
	if strings.Contains(content, forbidden) {
		t.Fatalf("expected content not to contain %q", forbidden)
	}
}
