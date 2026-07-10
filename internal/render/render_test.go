package render

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeBuilder struct {
	output    []byte
	err       error
	directory string
}

func (builder *fakeBuilder) Build(_ context.Context, directory string) ([]byte, error) {
	builder.directory = directory
	return builder.output, builder.err
}

func TestGenerate(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "globals.yaml"), []byte("NAMESPACE: apps\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	builder := &fakeBuilder{output: []byte("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: ${NAMESPACE}\n  labels:\n    unresolved: ${UNKNOWN}\n")}
	parameters := `[{"name":"substitute-from","array":["globals"]}]`

	got, err := Generate(context.Background(), builder, Options{
		Directory:      "/source",
		ValuesRoot:     root,
		ParametersJSON: parameters,
		MaxOutputBytes: 1 << 20,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if builder.directory != "/source" {
		t.Errorf("Build() directory = %q, want /source", builder.directory)
	}
	if want := "apiVersion: v1\nkind: Namespace\nmetadata:\n  name: apps\n  labels:\n    unresolved: ${UNKNOWN}\n"; string(got) != want {
		t.Errorf("Generate() = %q, want %q", got, want)
	}
}

func TestGenerateReturnsNoOutputOnStageFailures(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "globals.yaml"), []byte("PRESENT: value\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	parameters := `[{"name":"substitute-from","array":["globals"]}]`
	tests := map[string]*fakeBuilder{
		"build":      {err: errors.New("build failed")},
		"validation": {output: []byte("not-a-kubernetes-object: true\n")},
	}

	for name, builder := range tests {
		name, builder := name, builder
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := Generate(context.Background(), builder, Options{
				Directory:      ".",
				ValuesRoot:     root,
				ParametersJSON: parameters,
				MaxOutputBytes: 1 << 20,
			})
			if err == nil {
				t.Fatal("Generate() error = nil, want error")
			}
			if len(got) != 0 {
				t.Errorf("Generate() output = %q, want empty", got)
			}
		})
	}
}
