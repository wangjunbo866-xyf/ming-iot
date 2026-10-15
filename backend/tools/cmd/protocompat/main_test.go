package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func strPtr(s string) *string { return &s }

func int32Ptr(v int32) *int32 { return &v }

func labelPtr(v descriptorpb.FieldDescriptorProto_Label) *descriptorpb.FieldDescriptorProto_Label {
	return &v
}

func typePtr(v descriptorpb.FieldDescriptorProto_Type) *descriptorpb.FieldDescriptorProto_Type {
	return &v
}

func boolPtr(v bool) *bool { return &v }

// writeProtoset builds a tiny sys package descriptor set with N services/M
// methods and writes it to a temp .protoset file.
func writeProtoset(t *testing.T, path string, services int, methodsPerService int) {
	t.Helper()
	set := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:    strPtr("a.proto"),
				Package: strPtr("sys"),
				Options: &descriptorpb.FileOptions{GoPackage: strPtr("pb/sys")},
				MessageType: []*descriptorpb.DescriptorProto{
					{Name: strPtr("Empty")},
				},
			},
		},
	}
	var svcs []*descriptorpb.ServiceDescriptorProto
	for i := 0; i < services; i++ {
		var methods []*descriptorpb.MethodDescriptorProto
		for j := 0; j < methodsPerService; j++ {
			methods = append(methods, &descriptorpb.MethodDescriptorProto{
				Name:       strPtr("m" + itoa(j)),
				InputType:  strPtr(".sys.Empty"),
				OutputType: strPtr(".sys.Empty"),
			})
		}
		svcs = append(svcs, &descriptorpb.ServiceDescriptorProto{
			Name:   strPtr("svc" + itoa(i)),
			Method: methods,
		})
	}
	set.File[0].Service = svcs
	data, err := proto.Marshal(set)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func itoa(i int) string {
	// avoid importing strconv just for this in a tiny helper.
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// buildBin compiles the protocompat CLI into a temp binary once per test run.
func buildBin(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "protocompat")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func TestCLICompatible(t *testing.T) {
	t.Parallel()
	bin := buildBin(t)
	dir := t.TempDir()
	old := filepath.Join(dir, "old.protoset")
	newF := filepath.Join(dir, "new.protoset")
	// 2 services * 3 methods, 1 message.
	writeProtoset(t, old, 2, 3)
	writeProtoset(t, newF, 2, 3)

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(bin, "-old", old, "-new", newF, "-package", "sys")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, stderr.String())
	}
	got := strings.TrimSpace(stdout.String())
	want := "compatible: 2 services, 6 methods, 1 messages"
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestCLIDiffExit1(t *testing.T) {
	t.Parallel()
	bin := buildBin(t)
	dir := t.TempDir()
	old := filepath.Join(dir, "old.protoset")
	newF := filepath.Join(dir, "new.protoset")
	writeProtoset(t, old, 2, 3)
	writeProtoset(t, newF, 2, 4) // one extra method

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(bin, "-old", old, "-new", newF, "-package", "sys")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit, got nil")
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("expected exit 1, got %v", err)
	}
	if !strings.Contains(stderr.String(), "service:sys.svc0/method:m3") {
		t.Fatalf("expected diff line in stderr, got:\n%s", stderr.String())
	}
	// stdout must be empty on diff.
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
}

func TestCLIBadInputExit2(t *testing.T) {
	t.Parallel()
	bin := buildBin(t)
	dir := t.TempDir()
	garbage := filepath.Join(dir, "garbage.protoset")
	if err := os.WriteFile(garbage, []byte("not a protoset"), 0o644); err != nil {
		t.Fatal(err)
	}
	good := filepath.Join(dir, "good.protoset")
	writeProtoset(t, good, 1, 1)

	var stderr bytes.Buffer
	cmd := exec.Command(bin, "-old", garbage, "-new", good, "-package", "sys")
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit, got nil")
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 2 {
		t.Fatalf("expected exit 2, got %v", err)
	}
}

func TestCLIMissingPackageExit2(t *testing.T) {
	t.Parallel()
	bin := buildBin(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "a.protoset")
	b := filepath.Join(dir, "b.protoset")
	writeProtoset(t, a, 1, 1)
	writeProtoset(t, b, 1, 1)

	var stderr bytes.Buffer
	cmd := exec.Command(bin, "-old", a, "-new", b, "-package", "nope")
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit, got nil")
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 2 {
		t.Fatalf("expected exit 2, got %v", err)
	}
}

func TestCLISummaryJSON(t *testing.T) {
	t.Parallel()
	bin := buildBin(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "a.protoset")
	b := filepath.Join(dir, "b.protoset")
	summary := filepath.Join(dir, "summary.json")
	writeProtoset(t, a, 2, 3)
	writeProtoset(t, b, 2, 3)

	var stderr bytes.Buffer
	cmd := exec.Command(bin, "-old", a, "-new", b, "-package", "sys", "-summary-json", summary)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, stderr.String())
	}
	data, err := os.ReadFile(summary)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	var s summaryDoc
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("unmarshal summary: %v\n%s", err, data)
	}
	if s.Package != "sys" {
		t.Errorf("package = %q, want sys", s.Package)
	}
	if s.Services != 2 {
		t.Errorf("services = %d, want 2", s.Services)
	}
	if s.Methods != 6 {
		t.Errorf("methods = %d, want 6", s.Methods)
	}
	if s.Messages != 1 {
		t.Errorf("messages = %d, want 1", s.Messages)
	}
	if len(s.ServiceNames) != 2 {
		t.Errorf("service names = %v, want 2", s.ServiceNames)
	}
}

// summaryDoc type is declared in main.go; tests reuse it directly.
