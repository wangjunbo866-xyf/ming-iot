// 本文件实现 Kafka 仓库契约的确定性加载、校验与摘要。
package kafkacontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"
	"gopkg.in/yaml.v3"
)

const (
	// CodeCatalogReadFailed 表示 Topic Catalog 无法读取或解析。
	CodeCatalogReadFailed = "KAFKA_CATALOG_READ_FAILED"
	// CodeCatalogDynamicTopic 表示 Catalog 出现动态维度或通配符 Topic。
	CodeCatalogDynamicTopic = "KAFKA_CATALOG_DYNAMIC_TOPIC"
	// CodeCatalogGenerationRequired 表示 Topic 缺少合法 generation。
	CodeCatalogGenerationRequired = "KAFKA_CATALOG_GENERATION_REQUIRED"
	// CodeCatalogRetentionRequired 表示 Topic 缺少显式 retention。
	CodeCatalogRetentionRequired = "KAFKA_CATALOG_RETENTION_REQUIRED"
	// CodeCatalogOwnerRequired 表示 Topic 缺少 producer 或 consumer owner。
	CodeCatalogOwnerRequired = "KAFKA_CATALOG_OWNER_REQUIRED"
	// CodeCatalogKeyCodecRequired 表示 Topic 缺少 key codec。
	CodeCatalogKeyCodecRequired = "KAFKA_CATALOG_KEY_CODEC_REQUIRED"
	// CodeCatalogTimestampUnsafe 表示 Topic 没有使用 LogAppendTime。
	CodeCatalogTimestampUnsafe = "KAFKA_CATALOG_TIMESTAMP_UNSAFE"
	// CodeCatalogEntryInvalid 表示 Topic 的其他必填契约无效。
	CodeCatalogEntryInvalid = "KAFKA_CATALOG_ENTRY_INVALID"
	// CodeCatalogDuplicateTopic 表示 Catalog 存在重复物理 Topic。
	CodeCatalogDuplicateTopic = "KAFKA_CATALOG_DUPLICATE_TOPIC"
	// CodeCatalogFrozenSetMismatch 表示第一版受管 Topic 集合发生未审批变化。
	CodeCatalogFrozenSetMismatch = "KAFKA_CATALOG_FROZEN_SET_MISMATCH"
	// CodeProductionDurabilityUnsafe 表示生产副本或 minISR 低于基线。
	CodeProductionDurabilityUnsafe = "KAFKA_PRODUCTION_DURABILITY_UNSAFE"
	// CodeProductionAutoCreateEnabled 表示生产配置允许自动建 Topic。
	CodeProductionAutoCreateEnabled = "KAFKA_PRODUCTION_AUTO_CREATE_ENABLED"
	// CodeProductionSecurityUnsafe 表示生产配置出现明文 listener 或缺少 TLS/SASL。
	CodeProductionSecurityUnsafe = "KAFKA_PRODUCTION_SECURITY_UNSAFE"
	// CodeDeploymentContractInvalid 表示部署文件缺失或与版本锁不一致。
	CodeDeploymentContractInvalid = "KAFKA_DEPLOYMENT_CONTRACT_INVALID"
	// CodeVersionLockMismatch 表示 Kafka、franz-go 或 Go 版本不匹配。
	CodeVersionLockMismatch = "KAFKA_VERSION_LOCK_MISMATCH"
	// CodeProbeUnavailable 表示显式集成探针无法连接 broker。
	CodeProbeUnavailable = "KAFKA_PROBE_UNAVAILABLE"
)

const (
	expectedKafkaVersion  = "4.3.1"
	expectedFranzGo       = "v1.20.5"
	expectedWorkspaceGo   = "1.24.4"
	expectedImageDigest   = "sha256:77e3df9054047a88b520d0cc46e16696d3b22022e1d580aeccd2632df6532837"
	expectedCatalogDigest = "cc117b5985b874f53707d997ea46e4e3b9a8e2c3827a11a330fe0c97d036d67c"
	expectedProbeTopic    = "iot.data.contract.probe.v1.g01"
	dataPlaneCatalogPath  = "deploy/kafka/catalog/data-plane.yaml"
)

// Issue 是可机器解析且排序稳定的契约失败项。
type Issue struct {
	Code   string `json:"code"`
	Path   string `json:"path"`
	Detail string `json:"detail"`
}

// Report 是 kafkacontract 纯契约模式的稳定摘要。
type Report struct {
	Status          string  `json:"status"`
	KafkaVersion    string  `json:"kafka_version"`
	FranzGoVersion  string  `json:"franz_go_version"`
	GoVersion       string  `json:"go_version"`
	ImageDigest     string  `json:"image_digest"`
	CatalogEntries  int     `json:"catalog_entries"`
	CatalogDigest   string  `json:"catalog_digest"`
	ContractVersion int     `json:"contract_version"`
	Issues          []Issue `json:"issues"`
}

// OK 判断纯契约检查是否通过。
func (report Report) OK() bool {
	return report.Status == "pass" && len(report.Issues) == 0
}

// JSON 返回使用固定字段顺序和缩进的机器摘要。
func (report Report) JSON() ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}

// CatalogDocument 对应 deploy/kafka/catalog/data-plane.yaml。
type CatalogDocument struct {
	SchemaVersion      int                `yaml:"schema_version" json:"schema_version"`
	KafkaVersion       string             `yaml:"kafka_version" json:"kafka_version"`
	TimestampType      string             `yaml:"timestamp_type" json:"timestamp_type"`
	DeploymentProfiles DeploymentProfiles `yaml:"deployment_profiles" json:"deployment_profiles"`
	RawTopicTemplate   CatalogTopic       `yaml:"raw_topic_template" json:"raw_topic_template"`
	ProbeTopic         ProbeTopic         `yaml:"probe_topic" json:"probe_topic"`
	ManagedTopics      []CatalogTopic     `yaml:"managed_topics" json:"managed_topics"`
}

// DeploymentProfiles 描述 local 与 production 的静态耐久边界。
type DeploymentProfiles struct {
	Local      DeploymentProfile `yaml:"local" json:"local"`
	Production DeploymentProfile `yaml:"production" json:"production"`
}

// DeploymentProfile 描述某个环境的副本与自动建 Topic 约束。
type DeploymentProfile struct {
	LocalOnly         bool `yaml:"local_only" json:"local_only"`
	MinimumBrokers    int  `yaml:"minimum_brokers" json:"minimum_brokers"`
	ReplicationFactor int  `yaml:"replication_factor" json:"replication_factor"`
	MinISR            int  `yaml:"min_isr" json:"min_isr"`
	AutoCreate        bool `yaml:"auto_create" json:"auto_create"`
}

// CatalogTopic 描述一个受管数据面 Topic。
type CatalogTopic struct {
	LogicalName       string   `yaml:"logical_name" json:"logical_name"`
	PhysicalName      string   `yaml:"physical_name" json:"physical_name"`
	Generation        int      `yaml:"generation" json:"generation"`
	Plane             string   `yaml:"plane" json:"plane"`
	FactKind          string   `yaml:"fact_kind" json:"fact_kind"`
	SchemaMajor       int      `yaml:"schema_major" json:"schema_major"`
	Partitions        int      `yaml:"partitions" json:"partitions"`
	KeyCodec          string   `yaml:"key_codec" json:"key_codec"`
	CleanupPolicy     string   `yaml:"cleanup_policy" json:"cleanup_policy"`
	RetentionMS       int64    `yaml:"retention_ms" json:"retention_ms"`
	TimestampType     string   `yaml:"timestamp_type" json:"timestamp_type"`
	ProducerOwner     string   `yaml:"producer_owner" json:"producer_owner"`
	ConsumerOwners    []string `yaml:"consumer_owners" json:"consumer_owners"`
	ACLProfile        string   `yaml:"acl_profile" json:"acl_profile"`
	ReplicationFactor int      `yaml:"replication_factor" json:"replication_factor"`
	MinISR            int      `yaml:"min_isr" json:"min_isr"`
	AutoCreate        bool     `yaml:"auto_create" json:"auto_create"`
}

// ProbeTopic 描述显式集成探针唯一允许使用的临时 Topic。
type ProbeTopic struct {
	PhysicalName  string `yaml:"physical_name" json:"physical_name"`
	Partitions    int    `yaml:"partitions" json:"partitions"`
	RetentionMS   int64  `yaml:"retention_ms" json:"retention_ms"`
	TimestampType string `yaml:"timestamp_type" json:"timestamp_type"`
}

type versionLock struct {
	SchemaVersion int `json:"schema_version"`
	Kafka         struct {
		Version   string `json:"version"`
		Image     string `json:"image"`
		Digest    string `json:"digest"`
		Reference string `json:"reference"`
	} `json:"kafka"`
}

// LoadCatalog 从指定路径严格读取 Topic Catalog。
func LoadCatalog(path string) (CatalogDocument, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return CatalogDocument{}, fmt.Errorf("read catalog: %w", err)
	}
	var document CatalogDocument
	if err := yaml.Unmarshal(content, &document); err != nil {
		return CatalogDocument{}, fmt.Errorf("parse catalog: %w", err)
	}
	return document, nil
}

// ValidateCatalog 校验 Topic Catalog 的名称、所有权、保留和生产耐久契约。
func ValidateCatalog(document CatalogDocument, path string) []Issue {
	issues := make([]Issue, 0)
	add := func(code, detail string) {
		issues = append(issues, Issue{Code: code, Path: path, Detail: detail})
	}

	if document.SchemaVersion != 1 || document.KafkaVersion != expectedKafkaVersion {
		add(CodeVersionLockMismatch, "catalog schema or Kafka version does not match the frozen baseline")
	}
	if document.TimestampType != "LogAppendTime" {
		add(CodeCatalogTimestampUnsafe, "catalog default timestamp_type must be LogAppendTime")
	}
	local := document.DeploymentProfiles.Local
	if !local.LocalOnly || local.ReplicationFactor != 1 || local.MinISR != 1 {
		add(CodeDeploymentContractInvalid, "local profile must be local-only with RF=1 and minISR=1")
	}
	production := document.DeploymentProfiles.Production
	if production.MinimumBrokers < 3 || production.ReplicationFactor < 3 || production.MinISR < 2 {
		add(CodeProductionDurabilityUnsafe, "production profile requires at least 3 brokers, RF=3 and minISR=2")
	}
	if production.AutoCreate {
		add(CodeProductionAutoCreateEnabled, "production profile must disable auto-create")
	}

	if document.RawTopicTemplate.LogicalName != "iot.data.raw.v1.<protocolKey>" ||
		document.RawTopicTemplate.PhysicalName != "iot.data.raw.v1.<protocolKey>.g01" {
		add(CodeCatalogEntryInvalid, "raw topic template must use the frozen protocolKey template")
	}
	issues = append(issues, validateCatalogTopic(document.RawTopicTemplate, path, true)...)

	if document.ProbeTopic.PhysicalName != expectedProbeTopic || document.ProbeTopic.Partitions != 1 ||
		document.ProbeTopic.RetentionMS <= 0 || document.ProbeTopic.TimestampType != "LogAppendTime" {
		add(CodeCatalogEntryInvalid, "probe topic must be the fixed one-partition LogAppendTime resource")
	}

	seen := make(map[string]struct{}, len(document.ManagedTopics))
	for _, topic := range document.ManagedTopics {
		issues = append(issues, validateCatalogTopic(topic, path, false)...)
		if _, exists := seen[topic.PhysicalName]; exists {
			add(CodeCatalogDuplicateTopic, "duplicate physical topic "+topic.PhysicalName)
		}
		seen[topic.PhysicalName] = struct{}{}
	}

	return sortedIssues(issues)
}

// ValidateRepository 校验当前仓库的 Catalog、部署配置和版本锁。
func ValidateRepository(repoRoot string) Report {
	report := Report{ContractVersion: 1, Issues: make([]Issue, 0)}
	catalogRelativePath := dataPlaneCatalogPath
	catalog, err := LoadCatalog(filepath.Join(repoRoot, filepath.FromSlash(catalogRelativePath)))
	if err != nil {
		report.Issues = append(report.Issues, Issue{Code: CodeCatalogReadFailed, Path: catalogRelativePath, Detail: err.Error()})
	} else {
		report.KafkaVersion = catalog.KafkaVersion
		report.CatalogEntries = len(catalog.ManagedTopics)
		report.Issues = append(report.Issues, ValidateCatalog(catalog, catalogRelativePath)...)
		report.CatalogDigest = catalogDigest(catalog)
		report.Issues = append(report.Issues, validateFrozenCatalog(catalog, catalogRelativePath)...)
	}

	lockPath := "deploy/kafka/version-lock.json"
	lockContent, lockErr := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(lockPath)))
	if lockErr != nil {
		report.Issues = append(report.Issues, Issue{Code: CodeDeploymentContractInvalid, Path: lockPath, Detail: "version lock is not readable"})
	} else {
		var lock versionLock
		if err := json.Unmarshal(lockContent, &lock); err != nil {
			report.Issues = append(report.Issues, Issue{Code: CodeDeploymentContractInvalid, Path: lockPath, Detail: "version lock is not valid JSON"})
		} else {
			report.ImageDigest = lock.Kafka.Digest
			if lock.SchemaVersion != 1 || lock.Kafka.Version != expectedKafkaVersion || lock.Kafka.Digest != expectedImageDigest ||
				lock.Kafka.Reference != lock.Kafka.Image+"@"+lock.Kafka.Digest {
				report.Issues = append(report.Issues, Issue{Code: CodeVersionLockMismatch, Path: lockPath, Detail: "Kafka image version or digest does not match the frozen baseline"})
			}
		}
	}

	report.Issues = append(report.Issues, validateDeployFiles(repoRoot, report.ImageDigest)...)
	report.GoVersion, report.FranzGoVersion, report.Issues = validateGoVersions(repoRoot, report.Issues)
	report.Issues = sortedIssues(report.Issues)
	if len(report.Issues) == 0 {
		report.Status = "pass"
	} else {
		report.Status = "fail"
	}
	return report
}

func validateCatalogTopic(topic CatalogTopic, path string, rawTemplate bool) []Issue {
	issues := make([]Issue, 0)
	name := topic.PhysicalName
	if name == "" {
		name = "<missing>"
	}
	add := func(code, detail string) {
		issues = append(issues, Issue{Code: code, Path: path, Detail: name + ": " + detail})
	}

	if topic.Generation <= 0 {
		add(CodeCatalogGenerationRequired, "generation must be positive")
	}
	if topic.RetentionMS <= 0 {
		add(CodeCatalogRetentionRequired, "retention_ms must be explicit and positive")
	}
	if topic.FactKind == "raw" && topic.RetentionMS > 0 && topic.RetentionMS < 72*60*60*1000 {
		add(CodeCatalogRetentionRequired, "raw retention_ms must be at least 72 hours")
	}
	if strings.TrimSpace(topic.ProducerOwner) == "" || !hasNonBlank(topic.ConsumerOwners) {
		add(CodeCatalogOwnerRequired, "producer_owner and consumer_owners are required")
	}
	if strings.TrimSpace(topic.KeyCodec) == "" {
		add(CodeCatalogKeyCodecRequired, "key_codec is required")
	}
	if topic.TimestampType != "LogAppendTime" {
		add(CodeCatalogTimestampUnsafe, "timestamp_type must be LogAppendTime")
	}
	if topic.ReplicationFactor < 3 || topic.MinISR < 2 {
		add(CodeProductionDurabilityUnsafe, "production topic requires RF=3 and minISR=2")
	}
	if topic.AutoCreate {
		add(CodeProductionAutoCreateEnabled, "auto_create must be false")
	}
	if topic.Plane != "data" || strings.TrimSpace(topic.FactKind) == "" || topic.SchemaMajor <= 0 ||
		topic.Partitions <= 0 || topic.CleanupPolicy != "delete" || strings.TrimSpace(topic.ACLProfile) == "" {
		add(CodeCatalogEntryInvalid, "plane, fact_kind, schema_major, partitions, cleanup_policy and acl_profile are required")
	}
	if !rawTemplate {
		if isDynamicTopic(topic.LogicalName) || isDynamicTopic(topic.PhysicalName) {
			add(CodeCatalogDynamicTopic, "managed topic must not contain wildcard or dynamic resource dimensions")
		}
		if topic.Generation > 0 && topic.LogicalName+".g"+fmt.Sprintf("%02d", topic.Generation) != topic.PhysicalName {
			add(CodeCatalogGenerationRequired, "physical_name must end with the declared canonical generation")
		}
	}
	return issues
}

func validateFrozenCatalog(document CatalogDocument, path string) []Issue {
	if catalogDigest(document) == expectedCatalogDigest {
		return nil
	}
	return []Issue{{Code: CodeCatalogFrozenSetMismatch, Path: path, Detail: "catalog semantics do not match the approved v1 skeleton"}}
}

func validateDeployFiles(repoRoot, imageDigest string) []Issue {
	issues := make([]Issue, 0)
	read := func(relativePath string) string {
		content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			issues = append(issues, Issue{Code: CodeDeploymentContractInvalid, Path: relativePath, Detail: "required deployment file is not readable"})
			return ""
		}
		return string(content)
	}
	compose := read("deploy/kafka/compose.yaml")
	local := parseProperties(read("deploy/kafka/config/local/server.properties"))
	productionContent := read("deploy/kafka/config/production/server.properties")
	production := parseProperties(productionContent)

	if imageDigest == "" || !strings.Contains(compose, "apache/kafka:"+expectedKafkaVersion+"@"+imageDigest) ||
		strings.Contains(strings.ToLower(compose), ":latest") || !strings.Contains(compose, "local-only") {
		issues = append(issues, Issue{Code: CodeDeploymentContractInvalid, Path: "deploy/kafka/compose.yaml", Detail: "compose must pin Kafka tag and digest and declare local-only"})
	}
	if local["process.roles"] != "broker,controller" || local["default.replication.factor"] != "1" ||
		local["min.insync.replicas"] != "1" || local["auto.create.topics.enable"] != "false" ||
		local["unclean.leader.election.enable"] != "false" || local["log.message.timestamp.type"] != "LogAppendTime" {
		issues = append(issues, Issue{Code: CodeDeploymentContractInvalid, Path: "deploy/kafka/config/local/server.properties", Detail: "local profile does not match the single-node safe baseline"})
	}
	if strings.Contains(productionContent, "PLAINTEXT") || !strings.Contains(production["listener.security.protocol.map"], "SASL_SSL") ||
		production["ssl.client.auth"] != "required" {
		issues = append(issues, Issue{Code: CodeProductionSecurityUnsafe, Path: "deploy/kafka/config/production/server.properties", Detail: "production listeners must require TLS/SASL and reject PLAINTEXT"})
	}
	if production["config.providers"] != "file" ||
		production["config.providers.file.class"] != "org.apache.kafka.common.config.provider.FileConfigProvider" ||
		!strings.HasPrefix(production["ssl.keystore.password"], "${file:") ||
		!strings.HasPrefix(production["ssl.key.password"], "${file:") ||
		!strings.HasPrefix(production["ssl.truststore.password"], "${file:") {
		issues = append(issues, Issue{Code: CodeProductionSecurityUnsafe, Path: "deploy/kafka/config/production/server.properties", Detail: "production TLS passwords must use the configured file provider"})
	}
	for key, expected := range map[string]string{
		"default.replication.factor":                       "3",
		"min.insync.replicas":                              "2",
		"offsets.topic.replication.factor":                 "3",
		"transaction.state.log.replication.factor":         "3",
		"transaction.state.log.min.isr":                    "2",
		"share.coordinator.state.topic.replication.factor": "3",
		"share.coordinator.state.topic.min.isr":            "2",
		"auto.create.topics.enable":                        "false",
		"unclean.leader.election.enable":                   "false",
		"log.message.timestamp.type":                       "LogAppendTime",
	} {
		if production[key] != expected {
			code := CodeProductionDurabilityUnsafe
			if key == "auto.create.topics.enable" {
				code = CodeProductionAutoCreateEnabled
			}
			issues = append(issues, Issue{Code: code, Path: "deploy/kafka/config/production/server.properties", Detail: key + " must equal " + expected})
		}
	}
	return issues
}

func validateGoVersions(repoRoot string, issues []Issue) (string, string, []Issue) {
	goVersion := ""
	franzVersion := ""
	workPath := filepath.Join(repoRoot, "backend", "go.work")
	if content, err := os.ReadFile(workPath); err == nil {
		if work, parseErr := modfile.ParseWork("backend/go.work", content, nil); parseErr == nil && work.Go != nil {
			goVersion = work.Go.Version
		}
	}
	if goVersion != expectedWorkspaceGo {
		issues = append(issues, Issue{Code: CodeVersionLockMismatch, Path: "backend/go.work", Detail: "workspace Go version must be " + expectedWorkspaceGo})
	}

	for _, relativePath := range []string{"backend/share/go.mod", "backend/tools/go.mod"} {
		content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			issues = append(issues, Issue{Code: CodeVersionLockMismatch, Path: relativePath, Detail: "go.mod is not readable"})
			continue
		}
		moduleFile, err := modfile.Parse(relativePath, content, nil)
		if err != nil {
			issues = append(issues, Issue{Code: CodeVersionLockMismatch, Path: relativePath, Detail: "go.mod is not parseable"})
			continue
		}
		found := ""
		for _, requirement := range moduleFile.Require {
			if requirement.Mod.Path == "github.com/twmb/franz-go" {
				found = requirement.Mod.Version
				break
			}
		}
		if relativePath == "backend/share/go.mod" {
			franzVersion = found
		}
		if found != expectedFranzGo {
			issues = append(issues, Issue{Code: CodeVersionLockMismatch, Path: relativePath, Detail: "franz-go version must be " + expectedFranzGo})
		}
	}
	return goVersion, franzVersion, issues
}

func catalogDigest(document CatalogDocument) string {
	canonical := document
	canonical.ManagedTopics = append([]CatalogTopic(nil), document.ManagedTopics...)
	sort.Slice(canonical.ManagedTopics, func(i, j int) bool {
		return canonical.ManagedTopics[i].PhysicalName < canonical.ManagedTopics[j].PhysicalName
	})
	content, _ := json.Marshal(canonical)
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func parseProperties(content string) map[string]string {
	properties := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if found {
			properties[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return properties
}

func isDynamicTopic(name string) bool {
	if name == "" || strings.ContainsAny(name, "*#+<>{}") {
		return true
	}
	for _, token := range strings.Split(name, ".") {
		lower := strings.ToLower(token)
		if lower == "device-event" {
			continue
		}
		for _, dimension := range []string{"device", "tenant", "scene"} {
			if lower == dimension || strings.HasPrefix(lower, dimension+"-") {
				return true
			}
		}
	}
	return false
}

func hasNonBlank(values []string) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return false
		}
	}
	return true
}

func sortedIssues(issues []Issue) []Issue {
	sort.SliceStable(issues, func(i, j int) bool {
		left := issues[i].Code + "\x00" + issues[i].Path + "\x00" + issues[i].Detail
		right := issues[j].Code + "\x00" + issues[j].Path + "\x00" + issues[j].Detail
		return left < right
	})
	return issues
}
