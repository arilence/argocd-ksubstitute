package manifest

import "testing"

func TestValidate(t *testing.T) {
	t.Parallel()

	input := []byte(`# generated manifests
apiVersion: v1
kind: ConfigMap
metadata:
  name: first
---
{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"second"}}
`)
	if err := Validate(input); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsInvalidStreams(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"empty":               "",
		"only empty document": "---\n",
		"sequence root":       "- apiVersion: v1\n  kind: ConfigMap\n",
		"missing apiVersion":  "kind: ConfigMap\n",
		"missing kind":        "apiVersion: v1\n",
		"non-string kind":     "apiVersion: v1\nkind: 3\n",
		"malformed YAML":      "apiVersion: v1\nkind: ConfigMap\nmetadata: [\n",
	}

	for name, input := range tests {
		name, input := name, input
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := Validate([]byte(input)); err == nil {
				t.Fatalf("Validate(%q) error = nil, want error", input)
			}
		})
	}
}
