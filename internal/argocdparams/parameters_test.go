package argocdparams

import (
	"reflect"
	"strings"
	"testing"
)

func TestSubstituteFrom(t *testing.T) {
	t.Parallel()

	got, err := SubstituteFrom(`[{"name":"substitute-from","array":["globals","example-app"]}]`)
	if err != nil {
		t.Fatalf("SubstituteFrom() error = %v", err)
	}
	want := []string{"globals", "example-app"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SubstituteFrom() = %#v, want %#v", got, want)
	}
}

func TestSubstituteFromRejectsInvalidParameters(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"empty environment": "",
		"malformed JSON":    `[`,
		"trailing JSON":     `[] []`,
		"missing parameter": `[]`,
		"unknown parameter": `[{"name":"other","array":["globals"]}]`,
		"wrong type":        `[{"name":"substitute-from","string":"globals"}]`,
		"empty array":       `[{"name":"substitute-from","array":[]}]`,
		"null array":        `[{"name":"substitute-from","array":null}]`,
		"empty source":      `[{"name":"substitute-from","array":[""]}]`,
		"duplicate source":  `[{"name":"substitute-from","array":["globals","globals"]}]`,
		"duplicate param":   `[{"name":"substitute-from","array":["one"]},{"name":"substitute-from","array":["two"]}]`,
		"unknown field":     `[{"name":"substitute-from","array":["one"],"extra":true}]`,
	}

	for name, input := range tests {
		name, input := name, input
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := SubstituteFrom(input); err == nil {
				t.Fatalf("SubstituteFrom(%q) error = nil, want error", input)
			}
		})
	}
}

func TestSubstituteFromLimitsSourceCount(t *testing.T) {
	t.Parallel()

	sources := make([]string, maxSubstitutionSources+1)
	for i := range sources {
		sources[i] = `"source"`
	}
	input := `[{"name":"substitute-from","array":[` + strings.Join(sources, ",") + `]}]`
	if _, err := SubstituteFrom(input); err == nil {
		t.Fatal("SubstituteFrom() error = nil, want source-count error")
	}
}
