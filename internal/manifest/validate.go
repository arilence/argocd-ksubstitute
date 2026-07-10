// Package manifest validates rendered Kubernetes manifest streams.
package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// Validate checks that input contains at least one YAML or JSON document whose
// root is a mapping with non-empty apiVersion and kind string fields.
func Validate(input []byte) error {
	decoder := yaml.NewDecoder(bytes.NewReader(input))
	objects := 0
	for documentNumber := 1; ; documentNumber++ {
		var document yaml.Node
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("parse manifest document %d: %w", documentNumber, err)
		}
		if len(document.Content) == 0 || isNullDocument(document.Content[0]) {
			continue
		}

		root := document.Content[0]
		if root.Kind != yaml.MappingNode {
			return fmt.Errorf("manifest document %d must be a mapping", documentNumber)
		}
		if err := requireStringField(root, "apiVersion"); err != nil {
			return fmt.Errorf("manifest document %d: %w", documentNumber, err)
		}
		if err := requireStringField(root, "kind"); err != nil {
			return fmt.Errorf("manifest document %d: %w", documentNumber, err)
		}
		objects++
	}

	if objects == 0 {
		return errors.New("rendered manifests contain no Kubernetes objects")
	}
	return nil
}

func isNullDocument(node *yaml.Node) bool {
	return node.Kind == yaml.ScalarNode && node.Tag == "!!null"
}

func requireStringField(mapping *yaml.Node, field string) error {
	for i := 0; i < len(mapping.Content); i += 2 {
		key := mapping.Content[i]
		if key.Kind != yaml.ScalarNode || key.Value != field {
			continue
		}
		value := mapping.Content[i+1]
		if value.Kind != yaml.ScalarNode || value.Tag != "!!str" || strings.TrimSpace(value.Value) == "" {
			return fmt.Errorf("field %q must be a non-empty string", field)
		}
		return nil
	}
	return fmt.Errorf("required field %q is missing", field)
}
