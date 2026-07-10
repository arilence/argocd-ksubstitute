package kustomize

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestMain(testingMain *testing.M) {
	switch os.Getenv("KSUBSTITUTE_KUSTOMIZE_HELPER") {
	case "success":
		_, _ = fmt.Fprint(os.Stdout, "apiVersion: v1\nkind: ConfigMap\n")
		_, _ = fmt.Fprint(os.Stderr, "kustomize warning\n")
		os.Exit(0)
	case "failure":
		_, _ = fmt.Fprint(os.Stderr, "fixture build failed\n")
		os.Exit(7)
	case "large":
		_, _ = fmt.Fprint(os.Stdout, strings.Repeat("x", 20))
		os.Exit(0)
	}
	os.Exit(testingMain.Run())
}

func TestExternalBuilder(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable() error = %v", err)
	}
	t.Setenv("KSUBSTITUTE_KUSTOMIZE_HELPER", "success")
	var diagnostics bytes.Buffer
	builder := ExternalBuilder{
		Executable:         executable,
		MaxOutputBytes:     1 << 20,
		MaxDiagnosticBytes: 1 << 20,
		Diagnostics:        &diagnostics,
	}

	got, err := builder.Build(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if want := "apiVersion: v1\nkind: ConfigMap\n"; string(got) != want {
		t.Errorf("Build() = %q, want %q", got, want)
	}
	if got := diagnostics.String(); got != "kustomize warning\n" {
		t.Errorf("Build() diagnostics = %q, want warning", got)
	}
}

func TestExternalBuilderReportsFailure(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable() error = %v", err)
	}
	t.Setenv("KSUBSTITUTE_KUSTOMIZE_HELPER", "failure")
	builder := ExternalBuilder{
		Executable:         executable,
		MaxOutputBytes:     1 << 20,
		MaxDiagnosticBytes: 1 << 20,
	}

	got, err := builder.Build(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("Build() error = nil, want process error")
	}
	if len(got) != 0 {
		t.Errorf("Build() output = %q, want empty", got)
	}
	if !strings.Contains(err.Error(), "fixture build failed") {
		t.Errorf("Build() error = %q, want fixture diagnostic", err)
	}
}

func TestExternalBuilderLimitsOutput(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable() error = %v", err)
	}
	t.Setenv("KSUBSTITUTE_KUSTOMIZE_HELPER", "large")
	builder := ExternalBuilder{
		Executable:         executable,
		MaxOutputBytes:     5,
		MaxDiagnosticBytes: 1 << 20,
	}

	got, err := builder.Build(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("Build() error = nil, want output-size error")
	}
	if len(got) != 0 {
		t.Errorf("Build() output = %q, want empty", got)
	}
}

func TestBoundedBuffer(t *testing.T) {
	t.Parallel()

	buffer := newBoundedBuffer(5)
	if written, err := buffer.Write([]byte("123")); err != nil || written != 3 {
		t.Fatalf("Write() = %d, %v; want 3, nil", written, err)
	}
	if written, err := buffer.Write([]byte("4567")); err != nil || written != 4 {
		t.Fatalf("Write() = %d, %v; want 4, nil", written, err)
	}
	if got := buffer.String(); got != "12345" {
		t.Errorf("String() = %q, want %q", got, "12345")
	}
	if !buffer.Exceeded() {
		t.Error("Exceeded() = false, want true")
	}
}
