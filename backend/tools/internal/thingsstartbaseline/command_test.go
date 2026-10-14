package thingsstartbaseline

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
)

func TestCommandCapturesRequestedStdoutBeyondSummary(t *testing.T) {
	payload := strings.Repeat("bounded-command-output-", 512)
	capture := newBoundedCommandCapture(len(payload))
	environment := replaceEnvironment(os.Environ(), "THINGS_BOUNDED_COMMAND_OUTPUT", payload)

	result, err := NewCommandRunner().Run(context.Background(), Command{
		Path:          os.Args[0],
		Args:          []string{"-test.run=TestBoundedCommandCaptureHelperProcess", "--"},
		Dir:           t.TempDir(),
		Env:           environment,
		stdoutCapture: capture,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	stdout, overflow := capture.Result()
	if overflow {
		t.Fatal("bounded stdout capture unexpectedly overflowed")
	}
	if got := string(stdout); got != payload {
		t.Fatalf("captured stdout length = %d, want %d", len(got), len(payload))
	}
	if len(result.Summary) > commandSummaryLimit {
		t.Fatalf("summary length = %d, limit %d", len(result.Summary), commandSummaryLimit)
	}
}

func TestBoundedCommandCaptureHelperProcess(t *testing.T) {
	payload := os.Getenv("THINGS_BOUNDED_COMMAND_OUTPUT")
	if payload == "" {
		return
	}
	_, _ = io.WriteString(os.Stdout, payload)
	os.Exit(0)
}
