// Package argocdparams parses parameters supplied to an Argo CD config
// management plugin.
package argocdparams

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const maxSubstitutionSources = 64

type parameter struct {
	Name   string            `json:"name"`
	String *string           `json:"string,omitempty"`
	Array  []string          `json:"array,omitempty"`
	Map    map[string]string `json:"map,omitempty"`
}

// SubstituteFrom parses ARGOCD_APP_PARAMETERS and returns the ordered value
// source aliases from its required substitute-from array parameter.
func SubstituteFrom(input string) ([]string, error) {
	if strings.TrimSpace(input) == "" {
		return nil, errors.New("ARGOCD_APP_PARAMETERS is empty")
	}

	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.DisallowUnknownFields()

	var parameters []parameter
	if err := decoder.Decode(&parameters); err != nil {
		return nil, fmt.Errorf("parse ARGOCD_APP_PARAMETERS: %w", err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(parameters))
	var sources []string
	for _, parameter := range parameters {
		if parameter.Name == "" {
			return nil, errors.New("plugin parameter name cannot be empty")
		}
		if _, exists := seen[parameter.Name]; exists {
			return nil, fmt.Errorf("plugin parameter %q is repeated", parameter.Name)
		}
		seen[parameter.Name] = struct{}{}

		if parameter.Name != "substitute-from" {
			return nil, fmt.Errorf("unsupported plugin parameter %q", parameter.Name)
		}
		if parameter.String != nil || parameter.Map != nil || parameter.Array == nil {
			return nil, errors.New("plugin parameter \"substitute-from\" must be an array")
		}
		if len(parameter.Array) == 0 {
			return nil, errors.New("plugin parameter \"substitute-from\" cannot be empty")
		}
		if len(parameter.Array) > maxSubstitutionSources {
			return nil, fmt.Errorf("plugin parameter \"substitute-from\" exceeds %d sources", maxSubstitutionSources)
		}

		sourceSeen := make(map[string]struct{}, len(parameter.Array))
		for _, source := range parameter.Array {
			if source == "" {
				return nil, errors.New("plugin parameter \"substitute-from\" contains an empty source")
			}
			if _, exists := sourceSeen[source]; exists {
				return nil, fmt.Errorf("plugin parameter \"substitute-from\" repeats source %q", source)
			}
			sourceSeen[source] = struct{}{}
		}
		sources = append([]string(nil), parameter.Array...)
	}

	if sources == nil {
		return nil, errors.New("required plugin parameter \"substitute-from\" is missing")
	}
	return sources, nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var trailing interface{}
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("parse ARGOCD_APP_PARAMETERS: %w", err)
	}
	return errors.New("parse ARGOCD_APP_PARAMETERS: unexpected trailing JSON value")
}
