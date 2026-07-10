package values

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadSourcesMergesOrderedAliases(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeSource(t, root, "base.yaml", "DOMAIN: base.example\nNAMESPACE: apps\n")
	writeSource(t, root, "override.yaml", "DOMAIN: app.example\n")

	got, err := LoadSources(root, []string{"base", "override"})
	if err != nil {
		t.Fatalf("LoadSources() error = %v", err)
	}
	want := map[string]string{"DOMAIN": "app.example", "NAMESPACE": "apps"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoadSources() = %#v, want %#v", got, want)
	}
}

func TestLoadSourcesAllowsContainedSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	data := filepath.Join(root, "..data")
	if err := os.Mkdir(data, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	writeSource(t, data, "globals.yaml", "DOMAIN: example.com\n")
	if err := os.Symlink(filepath.Join("..data", "globals.yaml"), filepath.Join(root, "globals.yaml")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	got, err := LoadSources(root, []string{"globals"})
	if err != nil {
		t.Fatalf("LoadSources() error = %v", err)
	}
	if got["DOMAIN"] != "example.com" {
		t.Errorf("LoadSources()[DOMAIN] = %q, want example.com", got["DOMAIN"])
	}
}

func TestLoadSourcesRejectsEscapingSymlink(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	root := filepath.Join(parent, "values")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	writeSource(t, parent, "outside.yaml", "SECRET: exposed\n")
	if err := os.Symlink(filepath.Join("..", "outside.yaml"), filepath.Join(root, "outside.yaml")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	if _, err := LoadSources(root, []string{"outside"}); err == nil {
		t.Fatal("LoadSources() error = nil, want escaping-symlink error")
	}
}

func TestLoadSourcesRejectsInvalidAlias(t *testing.T) {
	t.Parallel()

	if _, err := LoadSources(t.TempDir(), []string{"../outside"}); err == nil {
		t.Fatal("LoadSources() error = nil, want invalid-alias error")
	}
}

func TestParse(t *testing.T) {
	t.Parallel()

	got, err := Parse([]byte("DOMAIN: example.com\nREPLICAS: \"3\"\nENABLED: 'true'\nEMPTY: \"\"\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	want := map[string]string{
		"DOMAIN":   "example.com",
		"REPLICAS": "3",
		"ENABLED":  "true",
		"EMPTY":    "",
	}
	for name, wantValue := range want {
		if gotValue := got[name]; gotValue != wantValue {
			t.Errorf("Parse()[%q] = %q, want %q", name, gotValue, wantValue)
		}
	}
}

func TestParseRejectsInvalidSources(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"empty":          "",
		"sequence":       "- value\n",
		"number":         "COUNT: 3\n",
		"boolean":        "ENABLED: true\n",
		"nested mapping": "NESTED:\n  KEY: value\n",
		"invalid name":   "not-valid: value\n",
		"duplicate":      "NAME: first\nNAME: second\n",
		"multiple docs":  "NAME: first\n---\nNAME: second\n",
		"explicit null":  "NAME: null\n",
		"unquoted null":  "NAME:\n",
	}

	for name, source := range tests {
		name, source := name, source
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Parse([]byte(source)); err == nil {
				t.Fatalf("Parse(%q) error = nil, want error", source)
			}
		})
	}
}

func TestParseErrorDoesNotIncludeValues(t *testing.T) {
	t.Parallel()

	const secret = "do-not-log-this"
	_, err := Parse([]byte("VALID: " + secret + "\nINVALID-NAME: " + secret + "\n"))
	if err == nil {
		t.Fatal("Parse() error = nil, want error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("Parse() error contains a value: %q", err)
	}
}

func writeSource(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
