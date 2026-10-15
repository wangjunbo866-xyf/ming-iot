package thingsbaseline

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

func TestExecGoRunnerBoundsHashesAndRedactsDiagnostics(t *testing.T) {
	directory := t.TempDir()
	secret := "fixture-super-secret"
	script := fmt.Sprintf(`printf 'stdout-proof'; printf 'https://user:password@example.invalid %s %s\n' >&2; i=0; while [ "$i" -lt 12000 ]; do printf x >&2; i=$((i+1)); done; printf 'TAIL-SENTINEL' >&2; exit 7`, secret, directory)
	command := GoCommand{
		Path: "/bin/sh", Dir: directory, Args: []string{"-c", script},
		Env: []string{"PATH=" + os.Getenv("PATH"), "SECRET=" + secret, "LC_ALL=C"},
	}
	report, err := (execGoRunner{}).Run(context.Background(), command)
	if err == nil || report.ExitCode != 7 {
		t.Fatalf("execGoRunner.Run() report=%+v err=%v, want exit 7", report, err)
	}
	if len(report.StderrTail) > maxCommandDiagnosticTail || !strings.Contains(report.StderrTail, "TAIL-SENTINEL") {
		t.Fatalf("stderr tail is not bounded/latest: len=%d tail=%q", len(report.StderrTail), report.StderrTail)
	}
	if !strings.Contains(report.StdoutTail, "stdout-proo") {
		t.Fatalf("stdout tail lost failure diagnostic: %q", report.StdoutTail)
	}
	for _, forbidden := range []string{"user:password", secret, directory} {
		if strings.Contains(report.StderrTail, forbidden) || strings.Contains(err.Error(), forbidden) {
			t.Fatalf("diagnostic leaked %q: report=%+v err=%v", forbidden, report, err)
		}
	}
	if report.StdoutSHA256 == "" || report.StderrSHA256 == "" {
		t.Fatalf("command output hashes missing: %+v", report)
	}
}

func TestExecGoRunnerRedactsHTTPSGOPROXYEntriesFromDiagnostics(t *testing.T) {
	for _, tt := range []struct {
		name  string
		proxy string
	}{
		{name: "comma separated chain", proxy: "https://private-one.example.invalid/path-one-sentinel,direct,https://private-two.example.invalid/path-two-sentinel"},
		{name: "pipe separated chain", proxy: "https://private-one.example.invalid/path-one-sentinel|direct|https://private-two.example.invalid/path-two-sentinel"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			command := GoCommand{
				Path: "/bin/sh", Dir: t.TempDir(), Args: []string{"-c", `printf 'GOPROXY=%s\nSAFE-PROXY-REDACTION-SENTINEL direct off\n' "$GOPROXY"; exit 23`},
				Env: []string{"PATH=/usr/bin:/bin", "GOPROXY=" + tt.proxy, "LC_ALL=C"},
			}
			report, err := (execGoRunner{}).Run(context.Background(), command)
			if err == nil || report.ExitCode != 23 {
				t.Fatalf("execGoRunner.Run() report=%+v err=%v, want exit 23", report, err)
			}
			for _, forbidden := range []string{"https://private-one.example.invalid", "https://private-two.example.invalid", "path-one-sentinel", "path-two-sentinel"} {
				if strings.Contains(report.StdoutTail, forbidden) || strings.Contains(report.StderrTail, forbidden) || strings.Contains(err.Error(), forbidden) {
					t.Fatalf("diagnostic leaked GOPROXY entry %q: report=%+v err=%v", forbidden, report, err)
				}
			}
			for _, preserved := range []string{"direct", "off", "SAFE-PROXY-REDACTION-SENTINEL"} {
				if !strings.Contains(report.StdoutTail, preserved) {
					t.Fatalf("diagnostic lost safe value %q: %q", preserved, report.StdoutTail)
				}
			}
		})
	}
}

func TestExecGoRunnerRedactsTransportProxyValuesFromDiagnostics(t *testing.T) {
	proxies := map[string]string{
		"HTTP_PROXY":  "upper-http-user:upper-http-password@proxy.example.invalid/upper-http-path-secret?token=upper-http-query-secret",
		"HTTPS_PROXY": "proxy.example.invalid/upper-https-path-secret?credential=upper-https-query-secret",
		"http_proxy":  "http://lower-http.example.invalid/lower-http-path-secret?token=lower-http-query-secret",
		"https_proxy": "https://lower-https.example.invalid/lower-https-path-secret?token=lower-https-query-secret",
	}
	command := GoCommand{
		Path: "/bin/sh", Dir: t.TempDir(),
		Args: []string{"-c", `printf '%s\n%s\n' "$HTTP_PROXY" "$https_proxy"; printf '%s\n%s\n' "$HTTPS_PROXY" "$http_proxy" >&2; exit 29`},
		Env: []string{
			"PATH=/usr/bin:/bin",
			"HTTP_PROXY=" + proxies["HTTP_PROXY"],
			"HTTPS_PROXY=" + proxies["HTTPS_PROXY"],
			"http_proxy=" + proxies["http_proxy"],
			"https_proxy=" + proxies["https_proxy"],
			"LC_ALL=C",
		},
	}

	report, err := (execGoRunner{}).Run(context.Background(), command)
	if err == nil || report.ExitCode != 29 {
		t.Fatalf("execGoRunner.Run() report=%+v err=%v, want exit 29", report, err)
	}
	diagnostic := report.StdoutTail + "\n" + report.StderrTail + "\n" + err.Error()
	if !strings.Contains(diagnostic, "[REDACTED]") {
		t.Fatalf("diagnostic did not record redaction: %q", diagnostic)
	}
	for _, proxy := range proxies {
		if strings.Contains(diagnostic, proxy) {
			t.Errorf("diagnostic leaked complete transport proxy value %q: %q", proxy, diagnostic)
		}
	}
	for _, fragment := range []string{
		"upper-http-password", "upper-http-path-secret", "upper-http-query-secret",
		"upper-https-path-secret", "upper-https-query-secret",
		"lower-http-path-secret", "lower-http-query-secret",
		"lower-https-path-secret", "lower-https-query-secret",
	} {
		if strings.Contains(diagnostic, fragment) {
			t.Errorf("diagnostic leaked transport proxy fragment %q: %q", fragment, diagnostic)
		}
	}
}

func TestExecGoRunnerStreamsBothDiagnosticsThroughUnifiedRedaction(t *testing.T) {
	directory := t.TempDir()
	secret := strings.Repeat("s", 511) + "!"
	stdoutRaw := append(bytes.Repeat([]byte("p"), 4090), []byte(secret)...)
	stdoutRaw = append(stdoutRaw, bytes.Repeat([]byte("q"), 1700)...)
	stdoutRaw = append(stdoutRaw, []byte("STDOUT-FAILURE-SENTINEL")...)
	stderrRaw := bytes.Repeat([]byte("x"), 9001)
	stderrRaw = append(stderrRaw, []byte("https://")...)
	stderrRaw = append(stderrRaw, bytes.Repeat([]byte("u"), 5000)...)
	stderrRaw = append(stderrRaw, []byte("@host.example/path/STDERR-TAIL-SENTINEL")...)
	script := `head -c 4090 /dev/zero | tr '\0' p; printf '%s' "$BOUNDARY_SECRET"; head -c 1700 /dev/zero | tr '\0' q; printf 'STDOUT-FAILURE-SENTINEL'; head -c 9001 /dev/zero | tr '\0' x >&2; printf 'https://' >&2; head -c 5000 /dev/zero | tr '\0' u >&2; printf '@host.example/path/STDERR-TAIL-SENTINEL' >&2; exit 17`
	command := GoCommand{
		Path: "/bin/sh", Dir: directory, Args: []string{"-c", script},
		Env: []string{"PATH=/usr/bin:/bin", "BOUNDARY_SECRET=" + secret, "LC_ALL=C"},
	}
	report, err := (execGoRunner{}).Run(context.Background(), command)
	if err == nil || report.ExitCode != 17 {
		t.Fatalf("execGoRunner.Run() report=%+v err=%v, want exit 17", report, err)
	}
	if len(report.StdoutTail) > maxCommandDiagnosticTail || len(report.StderrTail) > maxCommandDiagnosticTail || len(report.StdoutTail)+len(report.StderrTail) > maxCommandDiagnosticTotal {
		t.Fatalf("command tails exceed budget: stdout=%d stderr=%d", len(report.StdoutTail), len(report.StderrTail))
	}
	if !utf8.ValidString(report.StdoutTail) || !utf8.ValidString(report.StderrTail) {
		t.Fatalf("command tails are not valid UTF-8: stdout=%q stderr=%q", report.StdoutTail, report.StderrTail)
	}
	if !strings.Contains(report.StdoutTail, "[REDACTED]") || !strings.Contains(report.StdoutTail, "STDOUT-FAILURE-SENTINEL") {
		t.Fatalf("stdout tail lost redaction/sentinel: %q", report.StdoutTail)
	}
	if !strings.Contains(report.StderrTail, "https://[REDACTED]/path/STDERR-TAIL-SENTINEL") {
		t.Fatalf("stderr tail lost safe URL diagnostic: %q", report.StderrTail)
	}
	for _, forbidden := range []string{strings.Repeat("s", 32), strings.Repeat("u", 32), "host.example", directory} {
		if strings.Contains(report.StdoutTail, forbidden) || strings.Contains(report.StderrTail, forbidden) || strings.Contains(err.Error(), forbidden) {
			t.Fatalf("streamed diagnostic leaked %q: report=%+v err=%v", forbidden, report, err)
		}
	}
	if got, want := report.StdoutSHA256, sha256Digest(stdoutRaw); got != want {
		t.Fatalf("stdout raw SHA-256 = %q, want %q", got, want)
	}
	if got, want := report.StderrSHA256, sha256Digest(stderrRaw); got != want {
		t.Fatalf("stderr raw SHA-256 = %q, want %q", got, want)
	}
}

func TestExecGoRunnerReturnsValidUTF8TailsAfterByteBoundaryTruncation(t *testing.T) {
	payload := strings.Repeat("界", 1000) + "UTF8-TAIL-SENTINEL"
	command := GoCommand{
		Path: "/bin/sh", Dir: t.TempDir(), Args: []string{"-c", `printf '%s' "$UTF8_PAYLOAD"; exit 19`},
		Env: []string{"PATH=/usr/bin:/bin", "UTF8_PAYLOAD=" + payload, "LC_ALL=C"},
	}
	report, err := (execGoRunner{}).Run(context.Background(), command)
	if err == nil || report.ExitCode != 19 {
		t.Fatalf("execGoRunner.Run() report=%+v err=%v, want exit 19", report, err)
	}
	if !utf8.ValidString(report.StdoutTail) || len(report.StdoutTail) > maxCommandDiagnosticTail || !strings.Contains(report.StdoutTail, "UTF8-TAIL-SENTINEL") {
		t.Fatalf("UTF-8 stdout tail = valid %v len %d value %q", utf8.ValidString(report.StdoutTail), len(report.StdoutTail), report.StdoutTail)
	}
	if got, want := report.StdoutSHA256, sha256Digest([]byte(payload)); got != want {
		t.Fatalf("UTF-8 stdout raw SHA-256 = %q, want %q", got, want)
	}
}

func TestCommandTailRedactionPreservesOrdinaryShortValues(t *testing.T) {
	directory := t.TempDir()
	command := GoCommand{
		Dir: directory,
		Env: []string{"LC_ALL=C", "SHLVL=1", "GOENV=off", "GOWORK=off", "SECRET=fixture-super-secret"},
	}
	input := "download github.com/cenkalti/backoff/v4 locale C mode off fixture-super-secret " + directory
	redacted := redactCommandTail(input, command)
	for _, preserved := range []string{"github.com/cenkalti/backoff/v4", "locale C", "mode off"} {
		if !strings.Contains(redacted, preserved) {
			t.Fatalf("redaction removed ordinary value %q: %q", preserved, redacted)
		}
	}
	for _, forbidden := range []string{"fixture-super-secret", directory} {
		if strings.Contains(redacted, forbidden) {
			t.Fatalf("redaction leaked %q: %q", forbidden, redacted)
		}
	}
}

func TestExecGoRunnerCancellationKillsAndWaitsForProcessGroup(t *testing.T) {
	directory := t.TempDir()
	pidFile := filepath.Join(directory, "child.pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	command := GoCommand{
		Path: "/bin/sh", Dir: directory,
		Args: []string{"-c", `sleep 30 & child=$!; printf '%s' "$child" > child.pid; wait`},
		Env:  []string{"PATH=" + os.Getenv("PATH"), "LC_ALL=C"},
	}
	type result struct {
		report CommandReport
		err    error
	}
	done := make(chan result, 1)
	go func() {
		report, err := (execGoRunner{}).Run(ctx, command)
		done <- result{report: report, err: err}
	}()

	var childPID int
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(pidFile)
		if err == nil {
			childPID, err = strconv.Atoi(string(data))
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if childPID == 0 {
		t.Fatal("child process did not start")
	}
	cancel()
	select {
	case got := <-done:
		if got.err == nil || got.err != context.Canceled {
			t.Fatalf("cancelled runner report=%+v err=%v", got.report, got.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runner did not wait for killed process group")
	}

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		err := unix.Kill(childPID, 0)
		if err == unix.ESRCH {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("child process %d survived process-group cancellation", childPID)
}
