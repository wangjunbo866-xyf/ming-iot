package thingsstartbaseline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

const (
	maxHTTPProbeBodyBytes   = 1 << 20
	maxHTTPProbeOutputBytes = maxHTTPProbeBodyBytes + len("\nTHINGS_HTTP_STATUS:000\n")
	curlMaximumFileSize     = "1048576"
	curlFileSizeExceeded    = 63
	curlHTTPStatusWriteOut  = "\nTHINGS_HTTP_STATUS:%{http_code}\n"
	natsHealthURL           = "http://nats:8222/healthz"
	natsJetStreamURL        = "http://nats:8222/jsz"
	tdenginePingURL         = "http://tdengine:6041/-/ping"
	tdengineSQLURL          = "http://tdengine:6041/rest/sql"
)

// PostgreSQLProbeConfig contains the non-secret PostgreSQL connection fields
// and the password passed to the compose tool container through its environment.
type PostgreSQLProbeConfig struct {
	Host     string
	Port     string
	User     string
	Password string
}

// ProtocolProbeConfig fixes the command and HTTP endpoints used by ProtocolProbe.
type ProtocolProbeConfig struct {
	ComposeCommand Command
	Contract       *Contract
	PostgreSQL     PostgreSQLProbeConfig

	TDengineUsername string
	TDenginePassword string
	EMQXURL          string
	EMQXAPIKey       string
	EMQXSecretKey    string
	APIURL           string
}

// ProtocolProbe verifies startup stages at their native protocol boundaries.
type ProtocolProbe struct {
	runner           CommandRunner
	composeCommand   Command
	contract         *Contract
	postgres         PostgreSQLProbeConfig
	tdengineUsername string
	tdenginePassword string
	emqxURL          *url.URL
	emqxAPIKey       string
	emqxSecretKey    string
	apiURL           string
	httpClient       *http.Client
}

var _ StageProbe = (*ProtocolProbe)(nil)

// NewProtocolProbe creates a production stage probe using an argv-only compose
// command and an HTTP client which never follows redirects.
func NewProtocolProbe(runner CommandRunner, config ProtocolProbeConfig) (*ProtocolProbe, error) {
	if runner == nil {
		return nil, errors.New("command runner is nil")
	}
	if strings.TrimSpace(config.ComposeCommand.Path) == "" {
		return nil, errors.New("compose command path is empty")
	}
	if err := ValidateContract(config.Contract); err != nil {
		return nil, fmt.Errorf("startup contract is invalid: %w", err)
	}
	if strings.TrimSpace(config.PostgreSQL.Password) == "" {
		return nil, errors.New("PostgreSQL probe password is empty")
	}
	if strings.TrimSpace(config.TDengineUsername) == "" || strings.TrimSpace(config.TDenginePassword) == "" {
		return nil, errors.New("TDengine probe credentials are incomplete")
	}
	if strings.TrimSpace(config.EMQXAPIKey) == "" || strings.TrimSpace(config.EMQXSecretKey) == "" {
		return nil, errors.New("EMQX probe credentials are incomplete")
	}
	emqxURL, err := parseEMQXBaseURL(config.EMQXURL)
	if err != nil {
		return nil, err
	}
	apiURL, err := url.Parse(config.APIURL)
	if err != nil || (apiURL.Scheme != "http" && apiURL.Scheme != "https") || apiURL.Host == "" || apiURL.User != nil {
		return nil, errors.New("API probe endpoint is invalid")
	}
	compose := config.ComposeCommand
	compose.Args = append([]string(nil), compose.Args...)
	compose.Env = append([]string(nil), compose.Env...)
	return &ProtocolProbe{
		runner:           runner,
		composeCommand:   compose,
		contract:         cloneProbeContract(config.Contract),
		postgres:         config.PostgreSQL,
		tdengineUsername: config.TDengineUsername,
		tdenginePassword: config.TDenginePassword,
		emqxURL:          emqxURL,
		emqxAPIKey:       config.EMQXAPIKey,
		emqxSecretKey:    config.EMQXSecretKey,
		apiURL:           strings.TrimRight(config.APIURL, "/"),
		httpClient: &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}, nil
}

func cloneProbeContract(source *Contract) *Contract {
	cloned := *source
	cloned.Database.ForbiddenNames = append([]string(nil), source.Database.ForbiddenNames...)
	cloned.Roles = append([]RoleContract(nil), source.Roles...)
	cloned.Stages = make([]StageContract, len(source.Stages))
	for index, stage := range source.Stages {
		cloned.Stages[index] = stage
		cloned.Stages[index].Components = append([]string(nil), stage.Components...)
	}
	return &cloned
}

func parseEMQXBaseURL(endpoint string) (*url.URL, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("EMQX probe endpoint is invalid")
	}
	if parsed.Opaque != "" || parsed.Path != "" || parsed.RawPath != "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return nil, errors.New("EMQX probe endpoint must be a clean origin")
	}
	hostname := parsed.Hostname()
	ip := net.ParseIP(hostname)
	if !strings.EqualFold(hostname, "localhost") && (ip == nil || !ip.IsLoopback()) {
		return nil, errors.New("EMQX probe endpoint must use a loopback host")
	}
	return parsed, nil
}

// Probe verifies the native protocol associated with stage.
func (probe *ProtocolProbe) Probe(ctx context.Context, stage StageID) error {
	if probe == nil {
		return diagnostic(CodeContractMismatch, errors.New("protocol probe is nil"))
	}
	if ctx == nil {
		return diagnostic(CodeContractMismatch, errors.New("probe context is nil"))
	}
	if err := ctx.Err(); err != nil {
		return timeoutDiagnostic(err)
	}
	switch stage {
	case StagePrecheck:
		return nil
	case StageDatabaseBootstrap:
		return probe.probePostgreSQL(ctx)
	case StageInfraReady:
		return probe.probeInfrastructure(ctx)
	case StageTimedJobReady:
		return probe.probeGRPC(ctx, "timedjob")
	case StageSysReady:
		return probe.probeGRPC(ctx, "sys")
	case StageDMReady:
		return probe.probeGRPC(ctx, "dm")
	case StageDGProcessReady:
		return probe.probeGRPC(ctx, "dg")
	case StageDGLinkReady:
		return probe.probeDGLink(ctx)
	case StageAPIReady:
		return probe.probeAPI(ctx)
	case StageVerified:
		return probe.probeAll(ctx)
	default:
		return diagnostic(CodeContractMismatch, errors.New("unknown startup stage"))
	}
}

func (probe *ProtocolProbe) probeAll(ctx context.Context) error {
	checks := []func(context.Context) error{
		probe.probePostgreSQL,
		probe.probeInfrastructure,
		func(ctx context.Context) error { return probe.probeGRPC(ctx, "timedjob") },
		func(ctx context.Context) error { return probe.probeGRPC(ctx, "sys") },
		func(ctx context.Context) error { return probe.probeGRPC(ctx, "dm") },
		func(ctx context.Context) error { return probe.probeGRPC(ctx, "dg") },
		probe.probeDGLink,
		probe.probeAPI,
	}
	checkContext, cancel := context.WithCancel(ctx)
	defer cancel()
	var wait sync.WaitGroup
	var first sync.Once
	var firstErr error
	for _, check := range checks {
		check := check
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := check(checkContext); err != nil {
				first.Do(func() {
					firstErr = err
					cancel()
				})
			}
		}()
	}
	wait.Wait()
	if firstErr != nil {
		return firstErr
	}
	if err := ctx.Err(); err != nil {
		return timeoutDiagnostic(err)
	}
	return nil
}

func (probe *ProtocolProbe) probePostgreSQL(ctx context.Context) error {
	database := strings.TrimSpace(probe.contract.Database.RequiredName)
	if strings.TrimSpace(probe.postgres.Host) == "" || strings.TrimSpace(probe.postgres.Port) == "" ||
		strings.TrimSpace(probe.postgres.User) == "" || database == "" {
		return diagnostic(CodeContractMismatch, errors.New("PostgreSQL probe configuration is incomplete"))
	}
	environment := composeProbeEnvironment(probe.composeCommand.Env, probe.postgres.Password)
	connectionArguments := []string{
		"-h", probe.postgres.Host,
		"-p", probe.postgres.Port,
		"-U", probe.postgres.User,
		"-d", database,
	}
	readinessArguments := []string{
		"--profile", "tools", "run", "--rm", "--no-deps", "-e", "PGPASSWORD",
		"--entrypoint", "pg_isready", "postgres-bootstrap",
	}
	readinessArguments = append(readinessArguments, connectionArguments...)
	result, err := probe.runComposeWithEnvironment(ctx, environment, readinessArguments...)
	if err != nil || result.ExitCode != 0 {
		return probeCommandFailure(ctx, "PostgreSQL readiness", result, err)
	}
	queryArguments := []string{
		"--profile", "tools", "run", "--rm", "--no-deps", "-e", "PGPASSWORD",
		"--entrypoint", "psql", "postgres-bootstrap",
	}
	queryArguments = append(queryArguments, connectionArguments...)
	queryArguments = append(queryArguments, "-Atqc", "SELECT current_database(), postgis_full_version()")
	result, err = probe.runComposeWithEnvironment(ctx, environment, queryArguments...)
	if err != nil || result.ExitCode != 0 {
		return probeCommandFailure(ctx, "PostgreSQL identity", result, err)
	}
	stdout, ok := commandResultStdout(result)
	if !ok || !validPostgreSQLIdentity(stdout, database) {
		return diagnostic(CodeDependencyNotReady, errors.New("PostgreSQL database or PostGIS identity is not ready"))
	}
	return nil
}

func validPostgreSQLIdentity(stdout, database string) bool {
	line := strings.TrimSuffix(stdout, "\n")
	actualDatabase, postGISVersion, ok := strings.Cut(line, "|")
	return ok && actualDatabase == database && strings.Contains(strings.ToLower(postGISVersion), "postgis")
}

func composeProbeEnvironment(environment []string, password string) []string {
	result := make([]string, 0, len(environment)+2)
	for _, item := range environment {
		name, _, _ := strings.Cut(item, "=")
		if name != "PGPASSWORD" && name != "THINGS_BASELINE_DB_PASSWORD" {
			result = append(result, item)
		}
	}
	return append(result, "PGPASSWORD="+password, "THINGS_BASELINE_DB_PASSWORD="+password)
}

func (probe *ProtocolProbe) probeInfrastructure(ctx context.Context) error {
	if result, err := probe.runCompose(ctx, "exec", "-T", "redis", "redis-cli", "--raw", "ping"); err != nil || result.ExitCode != 0 {
		return probeCommandFailure(ctx, "Redis", result, err)
	} else if stdout, ok := commandResultStdout(result); !ok || stdout != "PONG\n" {
		return diagnostic(CodeDependencyNotReady, errors.New("Redis PING did not return PONG"))
	}
	if result, err := probe.runCompose(ctx, "exec", "-T", "etcd", "etcdctl", "endpoint", "health"); err != nil || result.ExitCode != 0 {
		return probeCommandFailure(ctx, "etcd", result, err)
	}
	if err := probe.probeNATS(ctx); err != nil {
		return err
	}
	if err := probe.probeTDengine(ctx); err != nil {
		return err
	}
	if result, err := probe.runCompose(ctx, "exec", "-T", "emqx", "/opt/emqx/bin/emqx", "ctl", "status"); err != nil || result.ExitCode != 0 {
		return probeCommandFailure(ctx, "EMQX", result, err)
	}
	return nil
}

func (probe *ProtocolProbe) probeGRPC(ctx context.Context, roleID string) error {
	endpoint := ""
	for _, role := range probe.contract.Roles {
		if role.ID == roleID {
			endpoint = strings.TrimSpace(role.Endpoint)
			break
		}
	}
	if endpoint == "" {
		return diagnostic(CodeContractMismatch, fmt.Errorf("gRPC endpoint for role %s is missing", roleID))
	}
	result, err := probe.runCompose(ctx,
		"--profile", "tools", "run", "--rm", "--no-deps", "grpcurl",
		"-plaintext", endpoint, "list",
	)
	if err != nil || result.ExitCode != 0 {
		return probeCommandFailure(ctx, roleID+" gRPC", result, err)
	}
	return nil
}

func (probe *ProtocolProbe) probeNATS(ctx context.Context) error {
	health, err := probe.runComposeCurl(ctx, "NATS health", nil, nil, natsHealthURL)
	if err != nil {
		return err
	}
	if health.status < 200 || health.status >= 300 {
		return diagnostic(CodeDependencyNotReady, errors.New("NATS health endpoint is not ready"))
	}
	jetStream, err := probe.runComposeCurl(ctx, "NATS JetStream", nil, nil, natsJetStreamURL)
	if err != nil {
		return err
	}
	if jetStream.status < 200 || jetStream.status >= 300 || !validJetStreamResponse(jetStream.body) {
		return diagnostic(CodeDependencyNotReady, errors.New("NATS JetStream is not ready"))
	}
	return nil
}

func validJetStreamResponse(body []byte) bool {
	var response struct {
		ServerID string          `json:"server_id"`
		Config   json.RawMessage `json:"config"`
		Error    json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &response) != nil || strings.TrimSpace(response.ServerID) == "" {
		return false
	}
	if len(response.Error) != 0 && string(response.Error) != "null" {
		return false
	}
	var config map[string]json.RawMessage
	return len(response.Config) != 0 && json.Unmarshal(response.Config, &config) == nil && config != nil
}

func (probe *ProtocolProbe) probeTDengine(ctx context.Context) error {
	ping, err := probe.runComposeCurl(ctx, "TDengine ping", nil, nil, tdenginePingURL)
	if err != nil {
		return err
	}
	if ping.status < 200 || ping.status >= 300 {
		return diagnostic(CodeDependencyNotReady, errors.New("TDengine ping endpoint is not ready"))
	}
	if strings.TrimSpace(probe.tdengineUsername) == "" || probe.tdenginePassword == "" {
		return diagnostic(CodeContractMismatch, errors.New("TDengine probe credentials are incomplete"))
	}
	environment := composeTDengineEnvironment(probe.composeCommand.Env, probe.tdenginePassword)
	sql, err := probe.runComposeCurl(ctx, "TDengine SQL", environment, []string{"-e", "TDENGINE_ROOT_PASSWORD"}, tdengineSQLURL,
		"--variable", "%TDENGINE_ROOT_PASSWORD",
		"--expand-user", probe.tdengineUsername+":{{TDENGINE_ROOT_PASSWORD}}",
		"--request", http.MethodPost,
		"--header", "Content-Type: text/plain",
		"--data-binary", "show databases",
	)
	if err != nil {
		return err
	}
	if sql.status < 200 || sql.status >= 300 || !tdengineSQLSucceeded(sql.body) {
		return diagnostic(CodeDependencyNotReady, errors.New("TDengine SQL probe failed"))
	}
	return nil
}

func composeTDengineEnvironment(environment []string, password string) []string {
	result := make([]string, 0, len(environment)+1)
	for _, item := range environment {
		name, _, _ := strings.Cut(item, "=")
		if name != "TDENGINE_ROOT_PASSWORD" {
			result = append(result, item)
		}
	}
	return append(result, "TDENGINE_ROOT_PASSWORD="+password)
}

func (probe *ProtocolProbe) runComposeCurl(
	ctx context.Context,
	name string,
	environment []string,
	composeArguments []string,
	endpoint string,
	curlArguments ...string,
) (boundedHTTPResponse, error) {
	arguments := []string{"--profile", "tools", "run", "--rm", "--no-deps"}
	arguments = append(arguments, composeArguments...)
	arguments = append(arguments,
		"curl",
		"--silent", "--show-error",
		"--max-filesize", curlMaximumFileSize,
		"--write-out", curlHTTPStatusWriteOut,
	)
	arguments = append(arguments, curlArguments...)
	arguments = append(arguments, endpoint)
	if environment == nil {
		environment = probe.composeCommand.Env
	}
	capture := newBoundedCommandCapture(maxHTTPProbeOutputBytes)
	result, err := probe.runComposeWithCapture(ctx, environment, capture, arguments...)
	stdout, overflow := capture.Result()
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return boundedHTTPResponse{}, probeCommandFailure(ctx, name, result, err)
	}
	if overflow || result.ExitCode == curlFileSizeExceeded {
		return boundedHTTPResponse{}, diagnostic(CodeContractMismatch, fmt.Errorf("%s response exceeds 1 MiB", name))
	}
	if err != nil || result.ExitCode != 0 {
		return boundedHTTPResponse{}, probeCommandFailure(ctx, name, result, err)
	}
	response, ok := commandResultHTTPResponse(result, stdout)
	if !ok {
		return boundedHTTPResponse{}, diagnostic(CodeDependencyNotReady, fmt.Errorf("%s response is malformed", name))
	}
	return response, nil
}

func commandResultHTTPResponse(result CommandResult, capturedStdout []byte) (boundedHTTPResponse, bool) {
	stdout := string(capturedStdout)
	if len(capturedStdout) == 0 {
		var ok bool
		stdout, ok = commandResultStdout(result)
		if !ok {
			return boundedHTTPResponse{}, false
		}
	}
	const statusPrefix = "\nTHINGS_HTTP_STATUS:"
	marker := strings.LastIndex(stdout, statusPrefix)
	if marker < 0 {
		return boundedHTTPResponse{}, false
	}
	statusText := strings.TrimSuffix(stdout[marker+len(statusPrefix):], "\n")
	if len(statusText) != 3 {
		return boundedHTTPResponse{}, false
	}
	status, err := strconv.Atoi(statusText)
	if err != nil || status < 100 || status > 999 {
		return boundedHTTPResponse{}, false
	}
	return boundedHTTPResponse{status: status, body: []byte(stdout[:marker])}, true
}

func tdengineSQLSucceeded(body []byte) bool {
	var response struct {
		Code *int `json:"code"`
	}
	if json.Unmarshal(body, &response) != nil {
		return false
	}
	return response.Code != nil && *response.Code == 0
}

func (probe *ProtocolProbe) probeDGLink(ctx context.Context) error {
	endpoint, err := url.JoinPath(probe.emqxURL.String(), "api/v5/clients")
	if err != nil {
		return diagnostic(CodeContractMismatch, errors.New("construct EMQX client endpoint"))
	}
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil {
		return diagnostic(CodeContractMismatch, errors.New("parse EMQX client endpoint"))
	}
	query := parsedEndpoint.Query()
	query.Set("like_clientid", "dg.rpc_")
	parsedEndpoint.RawQuery = query.Encode()
	response, err := probe.doHTTP(ctx, http.MethodGet, parsedEndpoint.String(), nil, probe.emqxAPIKey, probe.emqxSecretKey)
	if err != nil {
		return err
	}
	if response.status < 200 || response.status >= 300 || !containsDGRPCClient(response.body) {
		return diagnostic(CodeDependencyNotReady, errors.New("EMQX does not report the dg.rpc client"))
	}
	return nil
}

func containsDGRPCClient(body []byte) bool {
	var response struct {
		ClientID  string `json:"clientid"`
		Connected bool   `json:"connected"`
		Data      []struct {
			ClientID  string `json:"clientid"`
			Connected bool   `json:"connected"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &response) != nil {
		return false
	}
	if response.Connected && strings.HasPrefix(response.ClientID, "dg.rpc_") {
		return true
	}
	for _, client := range response.Data {
		if client.Connected && strings.HasPrefix(client.ClientID, "dg.rpc_") {
			return true
		}
	}
	return false
}

func (probe *ProtocolProbe) probeAPI(ctx context.Context) error {
	response, err := probe.doHTTP(ctx, http.MethodGet, probe.apiURL, nil, "", "")
	if err != nil {
		return err
	}
	if response.status >= 500 {
		return diagnostic(CodeDependencyNotReady, errors.New("API returned a server error"))
	}
	return nil
}

type boundedHTTPResponse struct {
	status int
	body   []byte
}

func (probe *ProtocolProbe) doHTTP(ctx context.Context, method, endpoint string, body []byte, username, password string) (boundedHTTPResponse, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return boundedHTTPResponse{}, diagnostic(CodeContractMismatch, errors.New("create HTTP probe request"))
	}
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "text/plain")
	}
	if username != "" || password != "" {
		request.SetBasicAuth(username, password)
	}
	response, err := probe.httpClient.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return boundedHTTPResponse{}, timeoutDiagnostic(ctxErr)
		}
		return boundedHTTPResponse{}, diagnostic(CodeDependencyNotReady, errors.New("HTTP probe request failed"))
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxHTTPProbeBodyBytes+1))
	if err != nil {
		return boundedHTTPResponse{}, diagnostic(CodeDependencyNotReady, errors.New("read HTTP probe response"))
	}
	if len(responseBody) > maxHTTPProbeBodyBytes {
		return boundedHTTPResponse{}, diagnostic(CodeContractMismatch, errors.New("HTTP probe response exceeds 1 MiB"))
	}
	return boundedHTTPResponse{status: response.StatusCode, body: responseBody}, nil
}

func (probe *ProtocolProbe) runCompose(ctx context.Context, arguments ...string) (CommandResult, error) {
	return probe.runComposeWithEnvironment(ctx, probe.composeCommand.Env, arguments...)
}

func (probe *ProtocolProbe) runComposeWithEnvironment(ctx context.Context, environment []string, arguments ...string) (CommandResult, error) {
	return probe.runComposeWithCapture(ctx, environment, nil, arguments...)
}

func (probe *ProtocolProbe) runComposeWithCapture(ctx context.Context, environment []string, capture *boundedCommandCapture, arguments ...string) (CommandResult, error) {
	command := probe.composeCommand
	command.Args = append(append([]string(nil), probe.composeCommand.Args...), arguments...)
	command.Env = append([]string(nil), environment...)
	command.stdoutCapture = capture
	return probe.runner.Run(ctx, command)
}

func probeCommandFailure(ctx context.Context, name string, result CommandResult, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return timeoutDiagnostic(ctxErr)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		if errors.Is(err, context.DeadlineExceeded) {
			return timeoutDiagnostic(context.DeadlineExceeded)
		}
		return timeoutDiagnostic(context.Canceled)
	}
	if err != nil {
		return diagnostic(CodeDependencyNotReady, fmt.Errorf("%s probe command failed", name))
	}
	return diagnostic(CodeDependencyNotReady, fmt.Errorf("%s probe command exited with status %d", name, result.ExitCode))
}

func commandResultStdout(result CommandResult) (string, bool) {
	const prefix = "stdout:\n"
	const separator = "\nstderr:\n"
	if !strings.HasPrefix(result.Summary, prefix) {
		return "", false
	}
	stdout, _, ok := strings.Cut(strings.TrimPrefix(result.Summary, prefix), separator)
	if !ok {
		return "", false
	}
	digest := sha256.Sum256([]byte(stdout))
	if hex.EncodeToString(digest[:]) != result.StdoutSHA256 {
		return "", false
	}
	return stdout, true
}
