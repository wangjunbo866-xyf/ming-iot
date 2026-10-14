package thingsstartbaseline

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type baselineComposeContract struct {
	Name     string                                    `yaml:"name"`
	Services map[string]baselineComposeServiceContract `yaml:"services"`
	Networks map[string]yaml.Node                      `yaml:"networks"`
	Volumes  map[string]yaml.Node                      `yaml:"volumes"`
}

type baselineComposeServiceContract struct {
	Image       string                             `yaml:"image"`
	User        string                             `yaml:"user"`
	Build       yaml.Node                          `yaml:"build"`
	Hostname    string                             `yaml:"hostname"`
	Profiles    []string                           `yaml:"profiles"`
	ExtraHosts  []string                           `yaml:"extra_hosts"`
	EnvFile     []string                           `yaml:"env_file"`
	Environment map[string]string                  `yaml:"environment"`
	Entrypoint  []string                           `yaml:"entrypoint"`
	Command     []string                           `yaml:"command"`
	Healthcheck baselineComposeHealthcheckContract `yaml:"healthcheck"`
	Volumes     []yaml.Node                        `yaml:"volumes"`
	Ports       []string                           `yaml:"ports"`
	Networks    []string                           `yaml:"networks"`
}

type baselineComposeHealthcheckContract struct {
	Test     []string `yaml:"test"`
	Interval string   `yaml:"interval"`
	Timeout  string   `yaml:"timeout"`
	Retries  int      `yaml:"retries"`
}

type baselineComposeLongVolumeContract struct {
	Type     string `yaml:"type"`
	Source   string `yaml:"source"`
	Target   string `yaml:"target"`
	ReadOnly bool   `yaml:"read_only"`
	Bind     struct {
		CreateHostPath *bool `yaml:"create_host_path"`
	} `yaml:"bind"`
}

func TestThingsBaselineComposeContract(t *testing.T) {
	compose, document := loadThingsBaselineCompose(t)

	if compose.Name != "ming-iot-things-baseline" {
		t.Fatalf("compose name = %q, want ming-iot-things-baseline", compose.Name)
	}
	requireExactMapKeys(t, "networks", compose.Networks, []string{"baseline"})
	requireExactMapKeys(t, "volumes", compose.Volumes, []string{
		"emqx_data", "etcd_data", "local_oss", "nats_data", "redis_data", "tdengine_data", "tdengine_log",
	})

	if err := validateBaselineComposeServiceNames(compose.Services); err != nil {
		t.Fatal(err)
	}
	if hasYAMLMappingKey(document, "build") {
		t.Fatal("compose contains forbidden build key")
	}
	for _, token := range []string{"kafka", "minio", "timedscheduler"} {
		if yamlContainsScalarToken(document, token) {
			t.Fatalf("compose contains forbidden token %q", token)
		}
	}

	wantImages := map[string]string{
		"postgres-bootstrap": "postgres:17-alpine",
		"redis":              "redis:7.4-alpine",
		"etcd":               "quay.io/coreos/etcd:v3.5.21",
		"nats":               "nats:2.10-alpine",
		"tdengine":           "tdengine/tdengine:3.3.6.13",
		"emqx":               "emqx/emqx:5.8.9",
		"grpcurl":            "fullstorydev/grpcurl:v1.9.3-alpine",
		"curl":               "curlimages/curl:8.12.1",
	}
	for name, wantImage := range wantImages {
		service := compose.Services[name]
		if service.Image != wantImage {
			t.Errorf("service %s image = %q, want %q", name, service.Image, wantImage)
		}
		requireStringSlice(t, name+" networks", service.Networks, []string{"baseline"})
		if name == "postgres-bootstrap" || name == "grpcurl" || name == "curl" {
			requireStringSlice(t, name+" profiles", service.Profiles, []string{"tools"})
		} else {
			requireStringSlice(t, name+" profiles", service.Profiles, nil)
		}
		if name != "emqx" && len(service.Ports) != 0 {
			t.Errorf("service %s publishes unexpected ports %v", name, service.Ports)
		}
	}

	t.Run("postgres bootstrap", func(t *testing.T) {
		service := compose.Services["postgres-bootstrap"]
		requireStringSlice(t, "extra_hosts", service.ExtraHosts, []string{"host.docker.internal:host-gateway"})
		requireStringSlice(t, "env_file", service.EnvFile, []string{"${THINGS_BASELINE_ENV_FILE:-.env.local}"})
		requireStringSlice(t, "entrypoint", service.Entrypoint, []string{"/usr/local/bin/bootstrap-postgres"})
		requireShortComposeVolumes(t, service.Volumes, []string{"./scripts/bootstrap-postgres.sh:/usr/local/bin/bootstrap-postgres:ro"})
	})

	t.Run("redis", func(t *testing.T) {
		service := compose.Services["redis"]
		requireStringSlice(t, "command", service.Command, []string{"redis-server", "--appendonly", "yes"})
		requireComposeHealthcheck(t, service.Healthcheck, []string{"CMD", "redis-cli", "ping"}, "3s", "2s", 30)
		requireShortComposeVolumes(t, service.Volumes, []string{"redis_data:/data"})
	})

	t.Run("etcd", func(t *testing.T) {
		service := compose.Services["etcd"]
		requireStringSlice(t, "command", service.Command, []string{
			"etcd",
			"--name=etcd",
			"--data-dir=/etcd-data",
			"--listen-client-urls=http://0.0.0.0:2379",
			"--advertise-client-urls=http://etcd:2379",
		})
		requireComposeHealthcheck(t, service.Healthcheck, []string{"CMD", "etcdctl", "endpoint", "health"}, "3s", "3s", 30)
		requireShortComposeVolumes(t, service.Volumes, []string{"etcd_data:/etcd-data"})
	})

	t.Run("nats", func(t *testing.T) {
		service := compose.Services["nats"]
		requireStringSlice(t, "command", service.Command, []string{"-js", "-sd", "/data", "-m", "8222"})
		requireShortComposeVolumes(t, service.Volumes, []string{"nats_data:/data"})
	})

	t.Run("tdengine", func(t *testing.T) {
		service := compose.Services["tdengine"]
		if service.Hostname != "tdengine" {
			t.Errorf("hostname = %q, want tdengine", service.Hostname)
		}
		requireExactStringMap(t, "environment", service.Environment, map[string]string{
			"TAOS_ROOT_PASSWORD": "${TDENGINE_ROOT_PASSWORD}",
		})
		requireShortComposeVolumes(t, service.Volumes, []string{
			"tdengine_data:/var/lib/taos",
			"tdengine_log:/var/log/taos",
			"./scripts/bootstrap-tdengine.sql:/docker-entrypoint-initdb.d/001-create-ithings.sql:ro",
		})
	})

	t.Run("emqx", func(t *testing.T) {
		service := compose.Services["emqx"]
		if service.User != "1000:1000" {
			t.Errorf("user = %q, want 1000:1000", service.User)
		}
		if service.Hostname != "emqx" {
			t.Errorf("hostname = %q, want emqx", service.Hostname)
		}
		wantEnvironment := map[string]string{
			"EMQX_NODE__NAME":              "emqx@127.0.0.1",
			"EMQX_API_KEY__BOOTSTRAP_FILE": "/opt/emqx/etc/default_api_key.conf",
		}
		requireExactStringMap(t, "environment", service.Environment, wantEnvironment)
		requireEMQXComposeVolumes(t, service.Volumes, []string{
			"emqx_data:/opt/emqx/data",
			"./config/emqx/base.hocon:/opt/emqx/etc/base.hocon:ro",
		})
		requireStringSlice(t, "ports", service.Ports, []string{
			"127.0.0.1:${THINGS_BASELINE_MQTT_PORT:-11883}:1883",
			"127.0.0.1:${THINGS_BASELINE_EMQX_DASHBOARD_PORT:-18084}:18083",
		})
		requireComposeHealthcheck(t, service.Healthcheck, []string{"CMD", "/opt/emqx/bin/emqx", "ctl", "status"}, "5s", "10s", 30)
	})
}

func TestThingsBaselineComposeServiceSetAllowsPlannedBusinessRoles(t *testing.T) {
	compose, _ := loadThingsBaselineCompose(t)
	for _, name := range []string{"timedjob", "sys", "dm", "dg", "api"} {
		compose.Services[name] = baselineComposeServiceContract{}
	}
	if err := validateBaselineComposeServiceNames(compose.Services); err != nil {
		t.Fatalf("planned Task 6 service set rejected: %v", err)
	}
}

func TestThingsBaselineComposeServiceSetRejectsUnknownService(t *testing.T) {
	compose, _ := loadThingsBaselineCompose(t)
	compose.Services["unknown"] = baselineComposeServiceContract{}
	if err := validateBaselineComposeServiceNames(compose.Services); err == nil {
		t.Fatal("unknown Compose service accepted")
	}
}

func TestThingsBaselineEMQXConfigContract(t *testing.T) {
	data, err := os.ReadFile(thingsBaselineProfilePath(t, "config", "emqx", "base.hocon"))
	if err != nil {
		t.Fatalf("read EMQX base config: %v", err)
	}
	got := strings.Join(strings.Fields(string(data)), " ")
	want := `api_key { bootstrap_file = "etc/default_api_key.conf" }`
	if got != want {
		t.Fatalf("EMQX base config semantics = %q, want %q", got, want)
	}
}

func TestThingsBaselinePostgresBootstrapContract(t *testing.T) {
	filename := thingsBaselineProfilePath(t, "scripts", "bootstrap-postgres.sh")
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatalf("stat PostgreSQL bootstrap script: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("PostgreSQL bootstrap script mode = %o, want an executable bit", info.Mode().Perm())
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read PostgreSQL bootstrap script: %v", err)
	}
	script := string(data)
	if !strings.HasPrefix(script, "#!/bin/sh\n") {
		t.Fatal("PostgreSQL bootstrap script must use POSIX sh")
	}
	if !regexp.MustCompile(`(?m)^set -eu$`).MatchString(script) {
		t.Fatal("PostgreSQL bootstrap script must enable set -eu")
	}

	requiredEnvironment := []string{
		"POSTGRES_ADMIN_HOST",
		"POSTGRES_ADMIN_PORT",
		"POSTGRES_ADMIN_USER",
		"POSTGRES_ADMIN_PASSWORD",
		"THINGS_BASELINE_DB",
		"THINGS_BASELINE_DB_USER",
		"THINGS_BASELINE_DB_PASSWORD",
	}
	for _, name := range requiredEnvironment {
		t.Run("missing "+name, func(t *testing.T) {
			environment := validPostgresBootstrapEnvironment()
			environment[name] = ""
			output, exitCode := runPostgresBootstrap(t, filename, environment)
			if exitCode != 1 || strings.TrimSpace(output) != "missing "+name {
				t.Fatalf("exit = %d, output = %q, want exit 1 and %q", exitCode, output, "missing "+name)
			}
		})
	}
	for _, tt := range []struct {
		name       string
		field      string
		wantOutput string
	}{
		{name: "database identity", field: "THINGS_BASELINE_DB", wantOutput: "CONFIG_INVALID: protected database target"},
		{name: "role identity", field: "THINGS_BASELINE_DB_USER", wantOutput: "CONFIG_INVALID: unexpected database role"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			environment := validPostgresBootstrapEnvironment()
			environment[tt.field] = "wrong_identity"
			output, exitCode := runPostgresBootstrap(t, filename, environment)
			if exitCode != 2 || strings.TrimSpace(output) != tt.wantOutput {
				t.Fatalf("exit = %d, output = %q, want exit 2 and %q", exitCode, output, tt.wantOutput)
			}
		})
	}
	for _, exactGuard := range []string{
		`[ "$THINGS_BASELINE_DB" = "ming_iot_things_baseline" ]`,
		`[ "$THINGS_BASELINE_DB_USER" = "ming_iot_things_baseline" ]`,
	} {
		if !strings.Contains(script, exactGuard) {
			t.Errorf("PostgreSQL bootstrap script missing exact identity guard %q", exactGuard)
		}
	}
	for _, environmentExport := range []string{
		`export PGHOST="$POSTGRES_ADMIN_HOST"`,
		`export PGPORT="$POSTGRES_ADMIN_PORT"`,
		`export PGUSER="$POSTGRES_ADMIN_USER"`,
		`export PGPASSWORD="$POSTGRES_ADMIN_PASSWORD"`,
	} {
		if !strings.Contains(script, environmentExport) {
			t.Errorf("PostgreSQL bootstrap script missing %q", environmentExport)
		}
	}
	for _, variable := range []string{"app_user", "db_name"} {
		if !regexp.MustCompile(`--set(?:=|[[:space:]])` + variable + `=`).MatchString(script) {
			t.Errorf("PostgreSQL bootstrap psql command missing --set for %s", variable)
		}
	}
	if regexp.MustCompile(`--set(?:=|[[:space:]])app_password=`).MatchString(script) {
		t.Error("PostgreSQL bootstrap psql command exposes app_password in argv")
	}
	if !strings.Contains(script, `\getenv app_password THINGS_BASELINE_DB_PASSWORD`) {
		t.Error("PostgreSQL bootstrap SQL does not read app_password from the environment")
	}

	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)CREATE ROLE`),
		regexp.MustCompile(`(?i)ALTER ROLE`),
		regexp.MustCompile(`(?i)CREATE DATABASE`),
		regexp.MustCompile(`(?i)CREATE EXTENSION IF NOT EXISTS postgis\s*;`),
		regexp.MustCompile(`(?i)CREATE EXTENSION IF NOT EXISTS postgis_topology\s*;`),
		regexp.MustCompile(`(?i)SELECT current_database\(\)\s*;`),
		regexp.MustCompile(`(?i)SELECT postgis_full_version\(\)\s*;`),
	} {
		if !pattern.MatchString(script) {
			t.Errorf("PostgreSQL bootstrap script missing %s", pattern)
		}
	}
	for _, forbidden := range []string{"DROP DATABASE", "DROP ROLE", "TRUNCATE", "iThings"} {
		if strings.Contains(strings.ToUpper(script), strings.ToUpper(forbidden)) {
			t.Errorf("PostgreSQL bootstrap script contains forbidden operation or identifier %q", forbidden)
		}
	}
}

func TestThingsBaselinePostgresBootstrapFakePSQLSuccess(t *testing.T) {
	filename := thingsBaselineProfilePath(t, "scripts", "bootstrap-postgres.sh")
	fakeDirectory, environment := fakePSQLEnvironment(t, 0, 0)
	output, exitCode := runPostgresBootstrap(t, filename, environment)
	if exitCode != 0 {
		t.Fatalf("exit = %d, output = %q, want success", exitCode, output)
	}
	if calls := fakePSQLCallCount(t, fakeDirectory); calls != 2 {
		t.Fatalf("psql calls = %d, want 2", calls)
	}
	for call := 1; call <= 2; call++ {
		arguments := readFakePSQLArtifact(t, fakeDirectory, fmt.Sprintf("argv.%d", call))
		if strings.Contains(arguments, "app-password") {
			t.Fatalf("psql call %d argv contains database password: %q", call, arguments)
		}
	}

	bootstrapSQL := readFakePSQLArtifact(t, fakeDirectory, "stdin.1")
	for _, want := range []string{
		`\getenv app_password THINGS_BASELINE_DB_PASSWORD`,
		`CREATE ROLE %I LOGIN PASSWORD %L`,
		`ALTER ROLE %I WITH LOGIN PASSWORD %L`,
		`CREATE DATABASE %I OWNER %I`,
	} {
		if !strings.Contains(bootstrapSQL, want) {
			t.Errorf("first psql stdin missing %q", want)
		}
	}
	if count := strings.Count(bootstrapSQL, "WHERE NOT EXISTS"); count != 2 {
		t.Errorf("first psql stdin WHERE NOT EXISTS count = %d, want 2", count)
	}

	verificationSQL := readFakePSQLArtifact(t, fakeDirectory, "stdin.2")
	for _, want := range []string{
		"CREATE EXTENSION IF NOT EXISTS postgis;",
		"CREATE EXTENSION IF NOT EXISTS postgis_topology;",
		"SELECT current_database();",
		"SELECT postgis_full_version();",
	} {
		if !strings.Contains(verificationSQL, want) {
			t.Errorf("second psql stdin missing %q", want)
		}
	}
}

func TestThingsBaselinePostgresBootstrapStopsAfterFirstPSQLFailure(t *testing.T) {
	filename := thingsBaselineProfilePath(t, "scripts", "bootstrap-postgres.sh")
	fakeDirectory, environment := fakePSQLEnvironment(t, 1, 41)
	output, exitCode := runPostgresBootstrap(t, filename, environment)
	if exitCode != 41 {
		t.Fatalf("exit = %d, output = %q, want propagated exit 41", exitCode, output)
	}
	if calls := fakePSQLCallCount(t, fakeDirectory); calls != 1 {
		t.Fatalf("psql calls = %d, want exactly 1", calls)
	}
}

func TestThingsBaselinePostgresBootstrapPropagatesSecondPSQLFailure(t *testing.T) {
	filename := thingsBaselineProfilePath(t, "scripts", "bootstrap-postgres.sh")
	fakeDirectory, environment := fakePSQLEnvironment(t, 2, 42)
	output, exitCode := runPostgresBootstrap(t, filename, environment)
	if exitCode != 42 {
		t.Fatalf("exit = %d, output = %q, want propagated exit 42", exitCode, output)
	}
	if calls := fakePSQLCallCount(t, fakeDirectory); calls != 2 {
		t.Fatalf("psql calls = %d, want exactly 2", calls)
	}
}

func validContractForTest() Contract {
	return Contract{
		SchemaVersion: 1,
		Profile:       "things-baseline",
		ProjectName:   "ming-iot-things-baseline",
		Database: DatabaseContract{
			RequiredName:   "ming_iot_things_baseline",
			ForbiddenNames: []string{"iThings"},
		},
		Roles: []RoleContract{
			{ID: "sys", Module: "saas", Binary: "syssvr", Image: "ming-iot-sys:baseline", Dockerfile: "deploy/images/Dockerfile.sys", Config: "deploy/profiles/things-baseline/config/saas/sys.yaml", Endpoint: "sys:9180"},
			{ID: "timedjob", Module: "saas", Binary: "timedjobsvr", Image: "ming-iot-timedjob:baseline", Dockerfile: "deploy/images/Dockerfile.timedjob", Config: "deploy/profiles/things-baseline/config/saas/timedjob.yaml", Endpoint: "timedjob:3256"},
			{ID: "dm", Module: "things", Binary: "dmsvr", Image: "ming-iot-dm:baseline", Dockerfile: "deploy/images/Dockerfile.dm", Config: "deploy/profiles/things-baseline/config/things/dm.yaml", Endpoint: "dm:9081"},
			{ID: "dg", Module: "things", Binary: "dgsvr", Image: "ming-iot-dg:baseline", Dockerfile: "deploy/images/Dockerfile.dg", Config: "deploy/profiles/things-baseline/config/things/dg.yaml", Endpoint: "dg:6166"},
			{ID: "api", Module: "things", Binary: "thingsvr", Image: "ming-iot-api:baseline", Dockerfile: "deploy/images/Dockerfile.api", Config: "deploy/profiles/things-baseline/config/things/api.yaml", Endpoint: "api:7788"},
		},
		Stages: []StageContract{
			{ID: StagePrecheck, Components: []string{"docker", "compose", "contract", "secrets", "postgres"}},
			{ID: StageDatabaseBootstrap, Components: []string{"postgres"}},
			{ID: StageInfraReady, Components: []string{"redis", "etcd", "nats", "tdengine", "emqx"}},
			{ID: StageTimedJobReady, Components: []string{"timedjob"}},
			{ID: StageSysReady, Components: []string{"sys"}},
			{ID: StageDMReady, Components: []string{"dm"}},
			{ID: StageDGProcessReady, Components: []string{"dg"}},
			{ID: StageDGLinkReady, Components: []string{"emqx", "dg.rpc"}},
			{ID: StageAPIReady, Components: []string{"api"}},
			{ID: StageVerified, Components: []string{"all"}},
		},
		Timeouts: TimeoutContract{ProbeSeconds: 90, TotalSeconds: 900},
	}
}

func TestValidateContractAcceptsExactBaseline(t *testing.T) {
	contract := validContractForTest()
	if err := ValidateContract(&contract); err != nil {
		t.Fatalf("ValidateContract() error = %v", err)
	}
}

func TestStableStageAndErrorCodes(t *testing.T) {
	stages := []StageID{
		StagePrecheck, StageDatabaseBootstrap, StageInfraReady, StageTimedJobReady,
		StageSysReady, StageDMReady, StageDGProcessReady, StageDGLinkReady,
		StageAPIReady, StageVerified,
	}
	wantStages := []StageID{
		"PRECHECK", "DATABASE_BOOTSTRAP", "INFRA_READY", "TIMEDJOB_READY",
		"SYS_READY", "DM_READY", "DG_PROCESS_READY", "DG_LINK_READY",
		"API_READY", "VERIFIED",
	}
	if !sameStageIDs(stages, wantStages) {
		t.Fatalf("stage IDs = %v, want %v", stages, wantStages)
	}
	codes := []ErrorCode{
		CodeBuildFailed, CodeConfigInvalid, CodeExternalDependencyUnavailable,
		CodeDatabaseBootstrapFailed, CodeDependencyNotReady, CodeRoleExited,
		CodeRoleNotReady, CodeContractMismatch, CodeTimeout,
	}
	wantCodes := []ErrorCode{
		"BUILD_FAILED", "CONFIG_INVALID", "EXTERNAL_DEPENDENCY_UNAVAILABLE",
		"DATABASE_BOOTSTRAP_FAILED", "DEPENDENCY_NOT_READY", "ROLE_EXITED",
		"ROLE_NOT_READY", "CONTRACT_MISMATCH", "TIMEOUT",
	}
	if !sameErrorCodes(codes, wantCodes) {
		t.Fatalf("error codes = %v, want %v", codes, wantCodes)
	}
}

func TestValidateContractRejectsNil(t *testing.T) {
	if err := ValidateContract(nil); err == nil || err.Error() != "contract is nil" {
		t.Fatalf("ValidateContract(nil) error = %v, want contract is nil", err)
	}
}

func TestValidateContractRejectsContractDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Contract)
		want   string
	}{
		{"role count", func(c *Contract) { c.Roles = c.Roles[:4] }, "roles must contain exactly 5"},
		{"requiredName iThings", func(c *Contract) { c.Database.RequiredName = "iThings" }, "database.requiredName"},
		{"stage ID order", func(c *Contract) { c.Stages[0].ID, c.Stages[1].ID = c.Stages[1].ID, c.Stages[0].ID }, "stages[0].id"},
		{"components drift", func(c *Contract) { c.Stages[0].Components[0] = "podman" }, "stages[0].components"},
		{"empty contract", func(c *Contract) { *c = Contract{} }, "schemaVersion"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := validContractForTest()
			tt.mutate(&contract)
			if err := ValidateContract(&contract); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateContract() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestValidateContractRequiresUniqueRoleFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Contract)
		want   string
	}{
		{"duplicate id", func(c *Contract) { c.Roles[1].ID = c.Roles[0].ID }, "roles.id must be unique"},
		{"duplicate binary", func(c *Contract) { c.Roles[1].Binary = c.Roles[0].Binary }, "roles.binary must be unique"},
		{"duplicate image", func(c *Contract) { c.Roles[1].Image = c.Roles[0].Image }, "roles.image must be unique"},
		{"duplicate config", func(c *Contract) { c.Roles[1].Config = c.Roles[0].Config }, "roles.config must be unique"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := validContractForTest()
			tt.mutate(&contract)
			if err := ValidateContract(&contract); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateContract() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestValidateContractRejectsProbeTimeoutDeviation(t *testing.T) {
	contract := validContractForTest()
	contract.Timeouts.ProbeSeconds = 91
	if err := ValidateContract(&contract); err == nil || !strings.Contains(err.Error(), "timeouts.probeSeconds") {
		t.Fatalf("ValidateContract() error = %v, want probe timeout error", err)
	}
}

func TestValidateContractRejectsTotalTimeoutDeviation(t *testing.T) {
	contract := validContractForTest()
	contract.Timeouts.TotalSeconds = 901
	if err := ValidateContract(&contract); err == nil || !strings.Contains(err.Error(), "timeouts.totalSeconds") {
		t.Fatalf("ValidateContract() error = %v, want total timeout error", err)
	}
}

func TestLoadContractRejectsInvalidYAMLDocuments(t *testing.T) {
	data, err := yaml.Marshal(validContractForTest())
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	tests := []struct {
		name string
		data string
		want string
	}{
		{"empty YAML", "", "decode contract"},
		{"empty contract", "{}\n", "schemaVersion"},
		{"unknown field", string(data) + "unknown: value\n", "field unknown"},
		{"second document", string(data) + "---\nschemaVersion: 1\n", "multiple YAML documents"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "contract.yaml")
			if err := os.WriteFile(filename, []byte(tt.data), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			_, err := LoadContract(filename)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadContract() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func loadThingsBaselineCompose(t *testing.T) (baselineComposeContract, yaml.Node) {
	t.Helper()
	data, err := os.ReadFile(thingsBaselineProfilePath(t, "compose.yaml"))
	if err != nil {
		t.Fatalf("read Things baseline Compose file: %v", err)
	}
	var compose baselineComposeContract
	if err := yaml.Unmarshal(data, &compose); err != nil {
		t.Fatalf("decode Things baseline Compose contract: %v", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode Things baseline Compose syntax tree: %v", err)
	}
	return compose, document
}

func thingsBaselineProfilePath(t *testing.T, elements ...string) string {
	t.Helper()
	return filepath.Join(append([]string{filepath.Dir(baselineContractPath(t))}, elements...)...)
}

func requireStringSlice(t *testing.T, field string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s = %v, want %v", field, got, want)
		return
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("%s = %v, want %v", field, got, want)
			return
		}
	}
}

func requireShortComposeVolumes(t *testing.T, got []yaml.Node, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("volumes count = %d, want %d", len(got), len(want))
		return
	}
	for index, wantVolume := range want {
		if got[index].Kind != yaml.ScalarNode || got[index].Value != wantVolume {
			t.Errorf("volume[%d] = kind %d value %q, want short volume %q", index, got[index].Kind, got[index].Value, wantVolume)
		}
	}
}

func requireEMQXComposeVolumes(t *testing.T, got []yaml.Node, wantShort []string) {
	t.Helper()
	if len(got) != len(wantShort)+1 {
		t.Fatalf("EMQX volumes count = %d, want %d", len(got), len(wantShort)+1)
	}
	requireShortComposeVolumes(t, got[:len(wantShort)], wantShort)
	secretNode := got[len(wantShort)]
	if secretNode.Kind != yaml.MappingNode {
		t.Fatalf("EMQX API key volume kind = %d, want long-form mapping", secretNode.Kind)
	}
	var secret baselineComposeLongVolumeContract
	if err := secretNode.Decode(&secret); err != nil {
		t.Fatalf("decode EMQX API key volume: %v", err)
	}
	if secret.Type != "bind" {
		t.Errorf("EMQX API key volume type = %q, want bind", secret.Type)
	}
	if secret.Source != "../../.runtime/secrets/emqx-api-key.conf" {
		t.Errorf("EMQX API key volume source = %q", secret.Source)
	}
	if secret.Target != "/opt/emqx/etc/default_api_key.conf" {
		t.Errorf("EMQX API key volume target = %q", secret.Target)
	}
	if !secret.ReadOnly {
		t.Error("EMQX API key volume must be read-only")
	}
	if secret.Bind.CreateHostPath == nil {
		t.Fatal("EMQX API key volume must set bind.create_host_path explicitly")
	}
	if *secret.Bind.CreateHostPath {
		t.Error("EMQX API key volume bind.create_host_path must be false")
	}
}

func requireComposeHealthcheck(t *testing.T, got baselineComposeHealthcheckContract, test []string, interval, timeout string, retries int) {
	t.Helper()
	requireStringSlice(t, "healthcheck test", got.Test, test)
	if got.Interval != interval {
		t.Errorf("healthcheck interval = %q, want %q", got.Interval, interval)
	}
	if got.Timeout != timeout {
		t.Errorf("healthcheck timeout = %q, want %q", got.Timeout, timeout)
	}
	if got.Retries != retries {
		t.Errorf("healthcheck retries = %d, want %d", got.Retries, retries)
	}
}

func validateBaselineComposeServiceNames(services map[string]baselineComposeServiceContract) error {
	required := []string{"curl", "emqx", "etcd", "grpcurl", "nats", "postgres-bootstrap", "redis", "tdengine"}
	for _, name := range required {
		if _, ok := services[name]; !ok {
			return fmt.Errorf("required service %q is missing", name)
		}
	}
	allowed := map[string]bool{
		"postgres-bootstrap": true,
		"redis":              true,
		"etcd":               true,
		"nats":               true,
		"tdengine":           true,
		"emqx":               true,
		"grpcurl":            true,
		"curl":               true,
		"timedjob":           true,
		"sys":                true,
		"dm":                 true,
		"dg":                 true,
		"api":                true,
	}
	for name := range services {
		if !allowed[name] {
			return fmt.Errorf("unknown service %q is not allowed (services: %v)", name, sortedMapKeys(services))
		}
	}
	return nil
}

func sortedMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func validPostgresBootstrapEnvironment() map[string]string {
	return map[string]string{
		"POSTGRES_ADMIN_HOST":         "host.docker.internal",
		"POSTGRES_ADMIN_PORT":         "5432",
		"POSTGRES_ADMIN_USER":         "postgres",
		"POSTGRES_ADMIN_PASSWORD":     "admin-password",
		"THINGS_BASELINE_DB":          "ming_iot_things_baseline",
		"THINGS_BASELINE_DB_USER":     "ming_iot_things_baseline",
		"THINGS_BASELINE_DB_PASSWORD": "app-password",
	}
}

func fakePSQLEnvironment(t *testing.T, failCall, exitCode int) (string, map[string]string) {
	t.Helper()
	directory := t.TempDir()
	fake := `#!/bin/sh
set -eu

count_file="$FAKE_PSQL_DIR/count"
count=0
if [ -f "$count_file" ]; then
  IFS= read -r count < "$count_file"
fi
count=$((count + 1))
printf '%s\n' "$count" > "$count_file"
printf '%s\n' "$@" > "$FAKE_PSQL_DIR/argv.$count"
cat > "$FAKE_PSQL_DIR/stdin.$count"
if [ "$count" -eq "$FAKE_PSQL_FAIL_CALL" ]; then
  exit "$FAKE_PSQL_EXIT_CODE"
fi
`
	if err := os.WriteFile(filepath.Join(directory, "psql"), []byte(fake), 0o755); err != nil {
		t.Fatalf("write fake psql: %v", err)
	}
	environment := validPostgresBootstrapEnvironment()
	environment["PATH"] = directory + string(os.PathListSeparator) + os.Getenv("PATH")
	environment["FAKE_PSQL_DIR"] = directory
	environment["FAKE_PSQL_FAIL_CALL"] = strconv.Itoa(failCall)
	environment["FAKE_PSQL_EXIT_CODE"] = strconv.Itoa(exitCode)
	return directory, environment
}

func fakePSQLCallCount(t *testing.T, directory string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(directory, "count"))
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("read fake psql call count: %v", err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parse fake psql call count %q: %v", data, err)
	}
	return count
}

func readFakePSQLArtifact(t *testing.T, directory, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(directory, name))
	if err != nil {
		t.Fatalf("read fake psql artifact %s: %v", name, err)
	}
	return string(data)
}

func runPostgresBootstrap(t *testing.T, filename string, environment map[string]string) (string, int) {
	t.Helper()
	command := exec.Command("sh", filename)
	path := os.Getenv("PATH")
	if configuredPath := environment["PATH"]; configuredPath != "" {
		path = configuredPath
	}
	command.Env = []string{"PATH=" + path}
	for name, value := range environment {
		if name == "PATH" {
			continue
		}
		command.Env = append(command.Env, name+"="+value)
	}
	output, err := command.CombinedOutput()
	if err == nil {
		return string(output), 0
	}
	exitError, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("run PostgreSQL bootstrap script: %v", err)
	}
	return string(output), exitError.ExitCode()
}

func requireExactMapKeys[T any](t *testing.T, field string, got map[string]T, want []string) {
	t.Helper()
	gotKeys := make([]string, 0, len(got))
	for key := range got {
		gotKeys = append(gotKeys, key)
	}
	sort.Strings(gotKeys)
	wantKeys := append([]string(nil), want...)
	sort.Strings(wantKeys)
	requireStringSlice(t, field, gotKeys, wantKeys)
}

func requireExactStringMap(t *testing.T, field string, got, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s = %v, want %v", field, got, want)
		return
	}
	for key, wantValue := range want {
		if gotValue, ok := got[key]; !ok || gotValue != wantValue {
			t.Errorf("%s = %v, want %v", field, got, want)
			return
		}
	}
}

func hasYAMLMappingKey(node yaml.Node, key string) bool {
	if node.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(node.Content); index += 2 {
			if node.Content[index].Value == key {
				return true
			}
		}
	}
	for _, child := range node.Content {
		if hasYAMLMappingKey(*child, key) {
			return true
		}
	}
	return false
}

func yamlContainsScalarToken(node yaml.Node, token string) bool {
	if node.Kind == yaml.ScalarNode && strings.Contains(strings.ToLower(node.Value), strings.ToLower(token)) {
		return true
	}
	for _, child := range node.Content {
		if yamlContainsScalarToken(*child, token) {
			return true
		}
	}
	return false
}

func sameStageIDs(got, want []StageID) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func sameErrorCodes(got, want []ErrorCode) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
