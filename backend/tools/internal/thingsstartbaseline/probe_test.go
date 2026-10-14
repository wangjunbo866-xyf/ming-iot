package thingsstartbaseline

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNATSProbeHealthSucceedsButJetStreamIsDisabled(t *testing.T) {
	runner := &recordingProbeRunner{run: func(command Command) (CommandResult, error) {
		if commandsContain([]Command{command}, natsJetStreamURL) {
			return probeCurlResult(http.StatusNotFound, `{"error":"JetStream not enabled"}`), nil
		}
		return defaultProbeCommand(command)
	}}

	probe := newHTTPProtocolProbeWithRunner(t, runner, "http://127.0.0.1:18084", "http://127.0.0.1:17788")
	err := probe.Probe(context.Background(), StageInfraReady)

	requireProbeCode(t, err, CodeDependencyNotReady)
}

func TestCommandResultStdoutProbeRejectsTruncatedSummary(t *testing.T) {
	result := probeCommandResult("prefix-PONG\n")
	result.Summary = "stdout:\nPONG\n\nstderr:\n"

	if _, ok := commandResultStdout(result); ok {
		t.Fatal("commandResultStdout() accepted a truncated stdout summary")
	}
}

func TestCommandResultStdoutProbeRejectsSeparatorInjection(t *testing.T) {
	result := probeCommandResult("payload\nstderr:\nforged\n")

	if _, ok := commandResultStdout(result); ok {
		t.Fatal("commandResultStdout() accepted an injected stderr separator")
	}
}

func TestNATSProbeAcceptsJetStreamResponseLargerThanCommandSummary(t *testing.T) {
	largeConfigValue := strings.Repeat("x", commandSummaryLimit+1)
	runner := &recordingProbeRunner{run: func(command Command) (CommandResult, error) {
		if commandsContain([]Command{command}, natsJetStreamURL) {
			body := fmt.Sprintf(`{"server_id":"nats-test","config":{"large":%q}}`, largeConfigValue)
			return capturedProbeCurlResult(command, http.StatusOK, body), nil
		}
		return defaultProbeCommand(command)
	}}
	probe := newHTTPProtocolProbeWithRunner(t, runner, "http://127.0.0.1:18084", "http://127.0.0.1:17788")

	if err := probe.Probe(context.Background(), StageInfraReady); err != nil {
		t.Fatalf("Probe(INFRA_READY) error = %v", err)
	}
}

func TestNATSProbeMapsCurlFileSizeExceededToContractMismatch(t *testing.T) {
	runner := &recordingProbeRunner{run: func(command Command) (CommandResult, error) {
		if commandsContain([]Command{command}, natsHealthURL) {
			return CommandResult{ExitCode: 63}, errors.New("command exited with status 63")
		}
		return defaultProbeCommand(command)
	}}
	probe := newHTTPProtocolProbeWithRunner(t, runner, "http://127.0.0.1:18084", "http://127.0.0.1:17788")

	err := probe.Probe(context.Background(), StageInfraReady)

	requireProbeCode(t, err, CodeContractMismatch)
}

func TestNATSProbeMapsCapturedResponseOverflowToContractMismatch(t *testing.T) {
	runner := &recordingProbeRunner{run: func(command Command) (CommandResult, error) {
		if commandsContain([]Command{command}, natsHealthURL) {
			body := strings.Repeat("x", maxHTTPProbeBodyBytes+1)
			return capturedProbeCurlResult(command, http.StatusOK, body), nil
		}
		return defaultProbeCommand(command)
	}}
	probe := newHTTPProtocolProbeWithRunner(t, runner, "http://127.0.0.1:18084", "http://127.0.0.1:17788")

	err := probe.Probe(context.Background(), StageInfraReady)

	requireProbeCode(t, err, CodeContractMismatch)
}

func TestNATSProbeRejectsEmptyJetStreamResponse(t *testing.T) {
	runner := &recordingProbeRunner{run: func(command Command) (CommandResult, error) {
		if commandsContain([]Command{command}, natsJetStreamURL) {
			return probeCurlResult(http.StatusOK, `{}`), nil
		}
		return defaultProbeCommand(command)
	}}
	probe := newHTTPProtocolProbeWithRunner(t, runner, "http://127.0.0.1:18084", "http://127.0.0.1:17788")

	err := probe.Probe(context.Background(), StageInfraReady)

	requireProbeCode(t, err, CodeDependencyNotReady)
}

func TestTDengineProbePingSucceedsButSQLProtocolFails(t *testing.T) {
	runner := &recordingProbeRunner{run: func(command Command) (CommandResult, error) {
		if commandsContain([]Command{command}, tdengineSQLURL) {
			return probeCurlResult(http.StatusOK, `{"code":9728,"desc":"database unavailable"}`), nil
		}
		return defaultProbeCommand(command)
	}}

	probe := newHTTPProtocolProbeWithRunner(t, runner, "http://127.0.0.1:18084", "http://127.0.0.1:17788")
	err := probe.Probe(context.Background(), StageInfraReady)

	requireProbeCode(t, err, CodeDependencyNotReady)
}

func TestTDengineProbeRejectsSuccessStatusWithoutCode(t *testing.T) {
	for _, status := range []string{"succ", "success"} {
		t.Run(status, func(t *testing.T) {
			runner := &recordingProbeRunner{run: func(command Command) (CommandResult, error) {
				if commandsContain([]Command{command}, tdengineSQLURL) {
					return probeCurlResult(http.StatusOK, fmt.Sprintf(`{"status":%q}`, status)), nil
				}
				return defaultProbeCommand(command)
			}}
			probe := newHTTPProtocolProbeWithRunner(t, runner, "http://127.0.0.1:18084", "http://127.0.0.1:17788")

			err := probe.Probe(context.Background(), StageInfraReady)

			requireProbeCode(t, err, CodeDependencyNotReady)
		})
	}
}

func TestNATSProbeUsesComposeCurlServiceNetworkEndpoints(t *testing.T) {
	runner := &recordingProbeRunner{run: defaultProbeCommand}
	hostEndpoint := "http://127.0.0.1:18084"
	probe := newHTTPProtocolProbeWithRunner(t, runner, hostEndpoint, hostEndpoint)

	if err := probe.Probe(context.Background(), StageInfraReady); err != nil {
		t.Fatalf("Probe(INFRA_READY) error = %v", err)
	}
	wantPrefix := []string{
		"compose", "--profile", "tools", "run", "--rm", "--no-deps", "curl",
		"--silent", "--show-error", "--max-filesize", "1048576",
		"--write-out", "\nTHINGS_HTTP_STATUS:%{http_code}\n",
	}
	for _, endpoint := range []string{"http://nats:8222/healthz", "http://nats:8222/jsz"} {
		want := append(append([]string(nil), wantPrefix...), endpoint)
		if !commandsHaveExactInvocation(runner.Commands(), "docker", want) {
			t.Errorf("NATS probe did not run exact compose curl argv %q", want)
		}
	}
	if commandsContain(runner.Commands(), hostEndpoint) {
		t.Errorf("NATS probe command used host endpoint %q", hostEndpoint)
	}
}

func TestTDengineProbeUsesComposeCurlServiceNetworkEndpoints(t *testing.T) {
	runner := &recordingProbeRunner{run: defaultProbeCommand}
	hostEndpoint := "http://127.0.0.1:18084"
	probe := newHTTPProtocolProbeWithRunner(t, runner, hostEndpoint, hostEndpoint)

	if err := probe.Probe(context.Background(), StageInfraReady); err != nil {
		t.Fatalf("Probe(INFRA_READY) error = %v", err)
	}
	wantPing := []string{
		"compose", "--profile", "tools", "run", "--rm", "--no-deps", "curl",
		"--silent", "--show-error", "--max-filesize", "1048576",
		"--write-out", "\nTHINGS_HTTP_STATUS:%{http_code}\n",
		"http://tdengine:6041/-/ping",
	}
	if !commandsHaveExactInvocation(runner.Commands(), "docker", wantPing) {
		t.Errorf("TDengine ping did not run exact compose curl argv %q", wantPing)
	}
	wantSQL := []string{
		"compose", "--profile", "tools", "run", "--rm", "--no-deps", "-e", "TDENGINE_ROOT_PASSWORD", "curl",
		"--silent", "--show-error", "--max-filesize", "1048576",
		"--write-out", "\nTHINGS_HTTP_STATUS:%{http_code}\n",
		"--variable", "%TDENGINE_ROOT_PASSWORD", "--expand-user", "root:{{TDENGINE_ROOT_PASSWORD}}",
		"--request", "POST", "--header", "Content-Type: text/plain", "--data-binary", "show databases",
		"http://tdengine:6041/rest/sql",
	}
	if !commandsHaveExactInvocation(runner.Commands(), "docker", wantSQL) {
		t.Errorf("TDengine SQL did not run exact compose curl argv %q", wantSQL)
	}
	for _, command := range runner.Commands() {
		if strings.Contains(strings.Join(command.Args, " "), "td-secret") {
			t.Error("TDengine password appeared in compose curl argv")
		}
	}
	if commandsContain(runner.Commands(), hostEndpoint) {
		t.Errorf("TDengine probe command used host endpoint %q", hostEndpoint)
	}
}

func TestEMQXDGLinkProbeRequiresDGRPCClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v5/clients" {
			t.Errorf("request path = %q", request.URL.Path)
		}
		if got := request.URL.Query().Get("like_clientid"); got != "dg.rpc_" {
			t.Errorf("like_clientid = %q, want %q", got, "dg.rpc_")
		}
		username, password, ok := request.BasicAuth()
		if !ok || username != "emqx-key" || password != "emqx-secret" {
			t.Errorf("EMQX basic auth = (%q, %q, %t)", username, password, ok)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":[{"clientid":"another-client","connected":true}]}`))
	}))
	defer server.Close()

	probe := newHTTPProtocolProbe(t, server.URL, server.URL)
	err := probe.Probe(context.Background(), StageDGLinkReady)

	requireProbeCode(t, err, CodeDependencyNotReady)
	if strings.Contains(fmt.Sprint(err), "emqx-key") || strings.Contains(fmt.Sprint(err), "emqx-secret") {
		t.Fatalf("probe error leaked EMQX credentials: %v", err)
	}
}

func TestNewProtocolProbeRejectsRemoteEMQXOrigin(t *testing.T) {
	config := validProtocolProbeConfig(t, "http://emqx.example.com:18084", "http://api.invalid")

	if _, err := NewProtocolProbe(probeRunner{}, config); err == nil {
		t.Fatal("NewProtocolProbe() accepted a non-loopback EMQX origin")
	}
}

func TestNewProtocolProbeRejectsNonOriginEMQXBaseURL(t *testing.T) {
	tests := []string{
		"http://127.0.0.1:18084/api",
		"http://127.0.0.1:18084?source=probe",
		"http://127.0.0.1:18084#fragment",
		"http://user:password@127.0.0.1:18084",
	}
	for _, endpoint := range tests {
		t.Run(endpoint, func(t *testing.T) {
			config := validProtocolProbeConfig(t, endpoint, "http://api.invalid")
			if _, err := NewProtocolProbe(probeRunner{}, config); err == nil {
				t.Fatalf("NewProtocolProbe() accepted EMQX base URL %q", endpoint)
			}
		})
	}
}

func TestNewProtocolProbeRejectsInvalidContract(t *testing.T) {
	config := validProtocolProbeConfig(t, "http://127.0.0.1:18084", "http://127.0.0.1:17788")
	config.Contract.SchemaVersion = 0

	if _, err := NewProtocolProbe(probeRunner{}, config); err == nil {
		t.Fatal("NewProtocolProbe() accepted an invalid startup contract")
	}
}

func TestNewProtocolProbeRejectsMissingRequiredCredentials(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ProtocolProbeConfig)
	}{
		{name: "PostgreSQL password", mutate: func(config *ProtocolProbeConfig) { config.PostgreSQL.Password = "" }},
		{name: "TDengine username", mutate: func(config *ProtocolProbeConfig) { config.TDengineUsername = "" }},
		{name: "TDengine password", mutate: func(config *ProtocolProbeConfig) { config.TDenginePassword = "" }},
		{name: "EMQX API key", mutate: func(config *ProtocolProbeConfig) { config.EMQXAPIKey = "" }},
		{name: "EMQX secret key", mutate: func(config *ProtocolProbeConfig) { config.EMQXSecretKey = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := validProtocolProbeConfig(t, "http://127.0.0.1:18084", "http://127.0.0.1:17788")
			test.mutate(&config)
			if _, err := NewProtocolProbe(probeRunner{}, config); err == nil {
				t.Fatalf("NewProtocolProbe() accepted missing %s", test.name)
			}
		})
	}
}

func TestNewProtocolProbeDeepCopiesContract(t *testing.T) {
	config := validProtocolProbeConfig(t, "http://127.0.0.1:18084", "http://127.0.0.1:17788")
	source := config.Contract
	probe, err := NewProtocolProbe(probeRunner{}, config)
	if err != nil {
		t.Fatalf("NewProtocolProbe() error = %v", err)
	}

	source.Database.ForbiddenNames[0] = "mutated-database"
	source.Roles[0].Endpoint = "mutated-role:1"
	source.Stages[0].Components[0] = "mutated-component"

	if probe.contract == source {
		t.Fatal("NewProtocolProbe() retained the source contract pointer")
	}
	if probe.contract.Database.ForbiddenNames[0] != "iThings" {
		t.Errorf("copied forbidden database = %q", probe.contract.Database.ForbiddenNames[0])
	}
	if probe.contract.Roles[0].Endpoint != requiredRoles[0].Endpoint {
		t.Errorf("copied role endpoint = %q", probe.contract.Roles[0].Endpoint)
	}
	if probe.contract.Stages[0].Components[0] != requiredStageComponents[0][0] {
		t.Errorf("copied stage component = %q", probe.contract.Stages[0].Components[0])
	}
}

func TestAPIProbeRejectsHTTPBodyLargerThanOneMiB(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(strings.Repeat("x", 1<<20+1)))
	}))
	defer server.Close()

	probe := newHTTPProtocolProbe(t, server.URL, server.URL)
	err := probe.Probe(context.Background(), StageAPIReady)

	requireProbeCode(t, err, CodeContractMismatch)
}

func TestPostgreSQLProbeRunsReadinessAndIdentityQueriesThroughCompose(t *testing.T) {
	runner := &recordingProbeRunner{run: func(command Command) (CommandResult, error) {
		joined := strings.Join(command.Args, " ")
		switch {
		case strings.Contains(joined, "--entrypoint pg_isready"):
			return probeCommandResult("accepting connections\n"), nil
		case strings.Contains(joined, "--entrypoint psql"):
			return probeCommandResult("ming_iot_things_baseline|POSTGIS=3.5.2\n"), nil
		default:
			return CommandResult{ExitCode: 127}, errors.New("unexpected probe command")
		}
	}}
	probe := newHTTPProtocolProbeWithRunner(t, runner, "http://127.0.0.1:18084", "http://127.0.0.1:17788")

	err := probe.Probe(context.Background(), StageDatabaseBootstrap)

	if err != nil {
		t.Fatalf("Probe(DATABASE_BOOTSTRAP) error = %v", err)
	}
	commands := runner.Commands()
	if len(commands) != 2 {
		t.Fatalf("PostgreSQL command count = %d, want 2", len(commands))
	}
	if got := strings.Join(commands[0].Args, " "); !strings.Contains(got, "--profile tools run --rm --no-deps -e PGPASSWORD --entrypoint pg_isready postgres-bootstrap") {
		t.Errorf("pg_isready command = %q", got)
	}
	queryCommand := strings.Join(commands[1].Args, " ")
	if !strings.Contains(queryCommand, "--entrypoint psql postgres-bootstrap") || !strings.Contains(queryCommand, "SELECT current_database(), postgis_full_version()") {
		t.Errorf("PostgreSQL identity command = %q", queryCommand)
	}
	for _, command := range commands {
		if strings.Contains(strings.Join(command.Args, " "), "database-secret") {
			t.Errorf("PostgreSQL password appeared in argv")
		}
		if !probeContainsEnvironment(command.Env, "PGPASSWORD=database-secret") {
			t.Errorf("PostgreSQL command environment does not contain PGPASSWORD")
		}
	}
}

func TestRedisProbeRequiresExactPONG(t *testing.T) {
	runner := &recordingProbeRunner{run: func(command Command) (CommandResult, error) {
		if strings.Contains(strings.Join(command.Args, " "), "redis-cli --raw ping") {
			return probeCommandResult("PONG \n"), nil
		}
		return probeCommandResult("healthy\n"), nil
	}}
	probe := newHTTPProtocolProbeWithRunner(t, runner, "http://127.0.0.1:18084", "http://127.0.0.1:17788")

	err := probe.Probe(context.Background(), StageInfraReady)

	requireProbeCode(t, err, CodeDependencyNotReady)
}

func TestEtcdProbeUsesEndpointHealth(t *testing.T) {
	runner := &recordingProbeRunner{run: defaultProbeCommand}
	probe := newHTTPProtocolProbeWithRunner(t, runner, "http://127.0.0.1:18084", "http://127.0.0.1:17788")

	if err := probe.Probe(context.Background(), StageInfraReady); err != nil {
		t.Fatalf("Probe(INFRA_READY) error = %v", err)
	}
	if !commandsContain(runner.Commands(), "etcdctl endpoint health") {
		t.Fatal("INFRA_READY did not run etcdctl endpoint health")
	}
}

func TestEMQXProcessProbeUsesCtlStatus(t *testing.T) {
	runner := &recordingProbeRunner{run: defaultProbeCommand}
	probe := newHTTPProtocolProbeWithRunner(t, runner, "http://127.0.0.1:18084", "http://127.0.0.1:17788")

	if err := probe.Probe(context.Background(), StageInfraReady); err != nil {
		t.Fatalf("Probe(INFRA_READY) error = %v", err)
	}
	if !commandsContain(runner.Commands(), "/opt/emqx/bin/emqx ctl status") {
		t.Fatal("INFRA_READY did not run EMQX ctl status")
	}
}

func TestGRPCRoleProbesUseGrpcurlList(t *testing.T) {
	tests := []struct {
		stage    StageID
		endpoint string
	}{
		{stage: StageTimedJobReady, endpoint: "timedjob:3256"},
		{stage: StageSysReady, endpoint: "sys:9180"},
		{stage: StageDMReady, endpoint: "dm:9081"},
		{stage: StageDGProcessReady, endpoint: "dg:6166"},
	}
	for _, test := range tests {
		t.Run(string(test.stage), func(t *testing.T) {
			runner := &recordingProbeRunner{run: defaultProbeCommand}
			probe := newHTTPProtocolProbeWithRunner(t, runner, "http://127.0.0.1:18084", "http://127.0.0.1:17788")

			if err := probe.Probe(context.Background(), test.stage); err != nil {
				t.Fatalf("Probe(%s) error = %v", test.stage, err)
			}
			want := "--profile tools run --rm --no-deps grpcurl -plaintext " + test.endpoint + " list"
			if !commandsContain(runner.Commands(), want) {
				t.Fatalf("Probe(%s) did not run %q", test.stage, want)
			}
		})
	}
}

func TestAPIProbeAcceptsNon5xxWithoutFollowingRedirects(t *testing.T) {
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("API probe followed a redirect")
	}))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Location", destination.URL)
		writer.WriteHeader(http.StatusFound)
	}))
	defer redirect.Close()

	probe := newHTTPProtocolProbe(t, redirect.URL, redirect.URL)
	if err := probe.Probe(context.Background(), StageAPIReady); err != nil {
		t.Fatalf("Probe(API_READY) error = %v, want nil for 302", err)
	}
}

func TestAPIProbeRejects5xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	probe := newHTTPProtocolProbe(t, server.URL, server.URL)
	err := probe.Probe(context.Background(), StageAPIReady)

	requireProbeCode(t, err, CodeDependencyNotReady)
}

func TestVerifiedProbeRechecksAllProtocols(t *testing.T) {
	runner := &recordingProbeRunner{run: func(command Command) (CommandResult, error) {
		joined := strings.Join(command.Args, " ")
		switch {
		case strings.Contains(joined, "--entrypoint pg_isready"):
			return probeCommandResult("accepting connections\n"), nil
		case strings.Contains(joined, "--entrypoint psql"):
			return probeCommandResult("ming_iot_things_baseline|POSTGIS=3.5.2\n"), nil
		default:
			return defaultProbeCommand(command)
		}
	}}
	var pathsMu sync.Mutex
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		pathsMu.Lock()
		paths = append(paths, request.URL.Path)
		pathsMu.Unlock()
		switch request.URL.Path {
		case "/api/v5/clients":
			_, _ = writer.Write([]byte(`{"data":[{"clientid":"dg.rpc_1234","connected":true}]}`))
		case "/":
			writer.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	probe := newHTTPProtocolProbeWithRunner(t, runner, server.URL, server.URL)

	if err := probe.Probe(context.Background(), StageVerified); err != nil {
		t.Fatalf("Probe(VERIFIED) error = %v", err)
	}
	for _, fragment := range []string{
		"--entrypoint pg_isready", "--entrypoint psql", "redis-cli --raw ping",
		"etcdctl endpoint health", "/opt/emqx/bin/emqx ctl status",
		"grpcurl -plaintext timedjob:3256 list", "grpcurl -plaintext sys:9180 list",
		"grpcurl -plaintext dm:9081 list", "grpcurl -plaintext dg:6166 list",
	} {
		if !commandsContain(runner.Commands(), fragment) {
			t.Errorf("VERIFIED did not run command containing %q", fragment)
		}
	}
	pathsMu.Lock()
	defer pathsMu.Unlock()
	for _, path := range []string{"/api/v5/clients", "/"} {
		if !containsString(paths, path) {
			t.Errorf("VERIFIED did not request %q; paths = %v", path, paths)
		}
	}
}

func TestVerifiedProbeRunsIndependentChecksConcurrently(t *testing.T) {
	runner := newBarrierProbeRunner(6, 25*time.Millisecond)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v5/clients":
			_, _ = writer.Write([]byte(`{"data":[{"clientid":"dg.rpc_1234","connected":true}]}`))
		case "/":
			writer.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	probe := newHTTPProtocolProbeWithRunner(t, runner, server.URL, server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := probe.Probe(ctx, StageVerified); err != nil {
		t.Fatalf("Probe(VERIFIED) error = %v", err)
	}
}

func TestPostgreSQLProbeTimeoutDoesNotLeakPassword(t *testing.T) {
	runner := &recordingProbeRunner{run: func(Command) (CommandResult, error) {
		return CommandResult{ExitCode: 124}, fmt.Errorf("database-secret: %w", context.DeadlineExceeded)
	}}
	probe := newHTTPProtocolProbeWithRunner(t, runner, "http://127.0.0.1:18084", "http://127.0.0.1:17788")

	err := probe.Probe(context.Background(), StageDatabaseBootstrap)

	requireProbeCode(t, err, CodeTimeout)
	if strings.Contains(fmt.Sprint(err), "database-secret") {
		t.Fatalf("probe timeout leaked PostgreSQL password: %v", err)
	}
}

func TestProtocolProbeRejectsNilContext(t *testing.T) {
	probe := newHTTPProtocolProbe(t, "http://127.0.0.1:18084", "http://127.0.0.1:17788")

	err := probe.Probe(nil, StageAPIReady)

	requireProbeCode(t, err, CodeContractMismatch)
}

func newHTTPProtocolProbe(t *testing.T, emqxURL, apiURL string) *ProtocolProbe {
	t.Helper()
	return newHTTPProtocolProbeWithRunner(t, probeRunner{}, emqxURL, apiURL)
}

func newHTTPProtocolProbeWithRunner(t *testing.T, runner CommandRunner, emqxURL, apiURL string) *ProtocolProbe {
	t.Helper()
	probe, err := NewProtocolProbe(runner, validProtocolProbeConfig(t, emqxURL, apiURL))
	if err != nil {
		t.Fatalf("NewProtocolProbe() error = %v", err)
	}
	return probe
}

func validProtocolProbeConfig(t *testing.T, emqxURL, apiURL string) ProtocolProbeConfig {
	t.Helper()
	return ProtocolProbeConfig{
		ComposeCommand: Command{Path: "docker", Args: []string{"compose"}, Dir: t.TempDir()},
		Contract:       validContract(),
		PostgreSQL: PostgreSQLProbeConfig{
			Host:     "host.docker.internal",
			Port:     "5432",
			User:     "things",
			Password: "database-secret",
		},
		TDengineUsername: "root",
		TDenginePassword: "td-secret",
		EMQXURL:          emqxURL,
		EMQXAPIKey:       "emqx-key",
		EMQXSecretKey:    "emqx-secret",
		APIURL:           apiURL,
	}
}

func validContract() *Contract {
	roles := append([]RoleContract(nil), requiredRoles...)
	stages := make([]StageContract, len(requiredStages))
	for index, id := range requiredStages {
		stages[index] = StageContract{ID: id, Components: append([]string(nil), requiredStageComponents[index]...)}
	}
	return &Contract{
		SchemaVersion: 1,
		Profile:       "things-baseline",
		ProjectName:   "ming-iot-things-baseline",
		Database: DatabaseContract{
			RequiredName:   "ming_iot_things_baseline",
			ForbiddenNames: []string{"iThings"},
		},
		Roles:  roles,
		Stages: stages,
		Timeouts: TimeoutContract{
			ProbeSeconds: 90,
			TotalSeconds: 900,
		},
	}
}

type probeRunner struct{}

func (probeRunner) Run(_ context.Context, command Command) (CommandResult, error) {
	return defaultProbeCommand(command)
}

func probeCommandResult(stdout string) CommandResult {
	stdoutDigest := sha256.Sum256([]byte(stdout))
	stderrDigest := sha256.Sum256(nil)
	return CommandResult{
		ExitCode:     0,
		StdoutSHA256: fmt.Sprintf("%x", stdoutDigest),
		StderrSHA256: fmt.Sprintf("%x", stderrDigest),
		Summary:      "stdout:\n" + stdout + "\nstderr:\n",
	}
}

func probeCurlResult(status int, body string) CommandResult {
	return probeCommandResult(fmt.Sprintf("%s\nTHINGS_HTTP_STATUS:%03d\n", body, status))
}

func capturedProbeCurlResult(command Command, status int, body string) CommandResult {
	stdout := fmt.Sprintf("%s\nTHINGS_HTTP_STATUS:%03d\n", body, status)
	if command.stdoutCapture != nil {
		_, _ = command.stdoutCapture.Write([]byte(stdout))
	}
	result := probeCommandResult(stdout)
	result.Summary = commandSummary(stdout, "", nil)
	return result
}

type recordingProbeRunner struct {
	mu       sync.Mutex
	commands []Command
	run      func(Command) (CommandResult, error)
}

type barrierProbeRunner struct {
	mu       sync.Mutex
	expected int
	delay    time.Duration
	groups   map[string]struct{}
	release  chan struct{}
	once     sync.Once
}

func newBarrierProbeRunner(expected int, delay time.Duration) *barrierProbeRunner {
	return &barrierProbeRunner{
		expected: expected,
		delay:    delay,
		groups:   make(map[string]struct{}),
		release:  make(chan struct{}),
	}
}

func (runner *barrierProbeRunner) Run(ctx context.Context, command Command) (CommandResult, error) {
	group := probeCommandGroup(command)
	if group != "" {
		runner.mu.Lock()
		_, seen := runner.groups[group]
		if !seen {
			runner.groups[group] = struct{}{}
			if len(runner.groups) == runner.expected {
				runner.once.Do(func() { close(runner.release) })
			}
		}
		runner.mu.Unlock()
		if !seen {
			select {
			case <-runner.release:
			case <-ctx.Done():
				return CommandResult{ExitCode: 124}, ctx.Err()
			}
		}
	}
	timer := time.NewTimer(runner.delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		joined := strings.Join(command.Args, " ")
		if strings.Contains(joined, "--entrypoint pg_isready") {
			return probeCommandResult("accepting connections\n"), nil
		}
		if strings.Contains(joined, "--entrypoint psql") {
			return probeCommandResult("ming_iot_things_baseline|POSTGIS=3.5.2\n"), nil
		}
		return defaultProbeCommand(command)
	case <-ctx.Done():
		return CommandResult{ExitCode: 124}, ctx.Err()
	}
}

func probeCommandGroup(command Command) string {
	joined := strings.Join(command.Args, " ")
	switch {
	case strings.Contains(joined, "--entrypoint pg_isready"), strings.Contains(joined, "--entrypoint psql"):
		return "postgres"
	case strings.Contains(joined, "grpcurl -plaintext timedjob:3256"):
		return "grpc-timedjob"
	case strings.Contains(joined, "grpcurl -plaintext sys:9180"):
		return "grpc-sys"
	case strings.Contains(joined, "grpcurl -plaintext dm:9081"):
		return "grpc-dm"
	case strings.Contains(joined, "grpcurl -plaintext dg:6166"):
		return "grpc-dg"
	default:
		return "infra"
	}
}

func (runner *recordingProbeRunner) Run(_ context.Context, command Command) (CommandResult, error) {
	runner.mu.Lock()
	runner.commands = append(runner.commands, command)
	runner.mu.Unlock()
	return runner.run(command)
}

func (runner *recordingProbeRunner) Commands() []Command {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return append([]Command(nil), runner.commands...)
}

func defaultProbeCommand(command Command) (CommandResult, error) {
	joined := strings.Join(command.Args, " ")
	switch {
	case strings.Contains(joined, "redis-cli --raw ping"):
		return probeCommandResult("PONG\n"), nil
	case strings.Contains(joined, natsHealthURL):
		return probeCurlResult(http.StatusOK, "ok"), nil
	case strings.Contains(joined, natsJetStreamURL):
		return probeCurlResult(http.StatusOK, `{"server_id":"nats-test","config":{"max_memory":0},"streams":0}`), nil
	case strings.Contains(joined, tdenginePingURL):
		return probeCurlResult(http.StatusOK, "pong"), nil
	case strings.Contains(joined, tdengineSQLURL):
		return probeCurlResult(http.StatusOK, `{"code":0,"rows":1}`), nil
	}
	return probeCommandResult("healthy\n"), nil
}

func commandsContain(commands []Command, fragment string) bool {
	for _, command := range commands {
		if strings.Contains(strings.Join(command.Args, " "), fragment) {
			return true
		}
	}
	return false
}

func commandsHaveExactInvocation(commands []Command, path string, arguments []string) bool {
	wantArguments := strings.Join(arguments, "\x00")
	for _, command := range commands {
		if command.Path == path && strings.Join(command.Args, "\x00") == wantArguments {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func probeContainsEnvironment(environment []string, want string) bool {
	for _, item := range environment {
		if item == want {
			return true
		}
	}
	return false
}

func requireProbeCode(t *testing.T, err error, want ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("Probe() error = nil, want %s", want)
	}
	if got := DiagnosticCode(err); got != want {
		t.Fatalf("DiagnosticCode(Probe()) = %s, want %s (error: %v)", got, want, err)
	}
}
