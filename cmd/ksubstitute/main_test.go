package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSubstitute(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	base := writeValues(t, dir, "base.yaml", "DOMAIN: base.example\nNAMESPACE: apps\n")
	override := writeValues(t, dir, "override.yaml", "DOMAIN: app.example\n")
	stdin := strings.NewReader("apiVersion: v1\nmetadata:\n  namespace: ${NAMESPACE}\n  name: ${DOMAIN}\n")
	var stdout, stderr bytes.Buffer

	exitCode := run(
		[]string{"substitute", "--values", base, "--values", override},
		stdin,
		&stdout,
		&stderr,
	)

	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	want := "apiVersion: v1\nmetadata:\n  namespace: apps\n  name: app.example\n"
	if stdout.String() != want {
		t.Errorf("run() stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Errorf("run() stderr = %q, want empty", stderr.String())
	}
}

func TestRunSubstituteLeavesUnknownExpressionsUnchanged(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	values := writeValues(t, dir, "values.yaml", "PRESENT: value\n")
	var stdout, stderr bytes.Buffer

	exitCode := run(
		[]string{"substitute", "--values", values},
		strings.NewReader("known: ${PRESENT}\nunknown: ${MISSING}\n"),
		&stdout,
		&stderr,
	)

	if exitCode != 0 {
		t.Errorf("run() exit code = %d, want 0", exitCode)
	}
	if want := "known: value\nunknown: ${MISSING}\n"; stdout.String() != want {
		t.Errorf("run() stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Errorf("run() stderr = %q, want empty", stderr.String())
	}
}

func TestRunRequiresValueFile(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"substitute"}, strings.NewReader("name: value\n"), &stdout, &stderr)
	if exitCode != 1 {
		t.Errorf("run() exit code = %d, want 1", exitCode)
	}
	if stdout.Len() != 0 {
		t.Errorf("run() stdout = %q, want empty", stdout.String())
	}
}

func TestRunRejectsEmptyManifestInput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	values := writeValues(t, dir, "values.yaml", "NAME: value\n")
	var stdout, stderr bytes.Buffer
	exitCode := run(
		[]string{"substitute", "--values", values},
		strings.NewReader(" \n\t"),
		&stdout,
		&stderr,
	)
	if exitCode != 1 {
		t.Errorf("run() exit code = %d, want 1", exitCode)
	}
	if stdout.Len() != 0 {
		t.Errorf("run() stdout = %q, want empty", stdout.String())
	}
}

func TestRunSubstituteHelpSucceeds(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"substitute", "--help"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 0 {
		t.Errorf("run() exit code = %d, want 0", exitCode)
	}
}

func TestRunRenderRequiresValuesRoot(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"render"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 1 {
		t.Errorf("run() exit code = %d, want 1", exitCode)
	}
	if stdout.Len() != 0 {
		t.Errorf("run() stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "--values-root is required") {
		t.Errorf("run() stderr = %q, want missing values root error", stderr.String())
	}
}

func TestRunRenderHelpSucceeds(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"render", "--help"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 0 {
		t.Errorf("run() exit code = %d, want 0", exitCode)
	}
}

func writeValues(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write value file: %v", err)
	}
	return path
}
