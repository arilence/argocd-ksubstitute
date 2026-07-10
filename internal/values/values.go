// Package values loads strict string-to-string YAML value mappings.
package values

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const maxFileBytes int64 = 1 << 20

const maxAggregateBytes int64 = 8 << 20

var variableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var sourceName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

// LoadSources resolves ordered source aliases beneath root and merges their
// mappings from left to right. Symlinks are accepted only when their resolved
// targets remain inside root, which supports Kubernetes projected volumes
// without allowing Application parameters to escape the mounted values tree.
func LoadSources(root string, sources []string) (map[string]string, error) {
	if len(sources) == 0 {
		return nil, errors.New("at least one value source is required")
	}

	rootPath, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve values root: %w", err)
	}
	rootPath, err = filepath.EvalSymlinks(rootPath)
	if err != nil {
		return nil, fmt.Errorf("resolve values root: %w", err)
	}
	rootInfo, err := os.Stat(rootPath)
	if err != nil {
		return nil, fmt.Errorf("inspect values root: %w", err)
	}
	if !rootInfo.IsDir() {
		return nil, errors.New("values root is not a directory")
	}

	merged := make(map[string]string)
	var aggregateBytes int64
	for _, source := range sources {
		path, size, err := resolveSource(rootPath, source)
		if err != nil {
			return nil, fmt.Errorf("resolve value source %q: %w", source, err)
		}
		if size > maxAggregateBytes-aggregateBytes {
			return nil, fmt.Errorf("value sources exceed %d bytes in total", maxAggregateBytes)
		}
		aggregateBytes += size

		loaded, err := LoadFile(path)
		if err != nil {
			return nil, fmt.Errorf("load value source %q: %w", source, err)
		}
		for name, value := range loaded {
			merged[name] = value
		}
	}
	return merged, nil
}

func resolveSource(root, source string) (string, int64, error) {
	if !sourceName.MatchString(source) {
		return "", 0, errors.New("source name must contain only letters, numbers, underscores, and hyphens")
	}

	candidate := filepath.Join(root, source+".yaml")
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", 0, err
	}
	contained, err := pathContainedBy(root, resolved)
	if err != nil {
		return "", 0, err
	}
	if !contained {
		return "", 0, errors.New("resolved source is outside the values root")
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() {
		return "", 0, errors.New("source is not a regular file")
	}
	return resolved, info.Size(), nil
}

func pathContainedBy(root, path string) (bool, error) {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false, err
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)), nil
}

// LoadFile loads a value mapping from path. Files are intentionally bounded so
// a mounted source cannot make the plugin consume unbounded memory.
func LoadFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	data, readErr := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(data)) > maxFileBytes {
		return nil, fmt.Errorf("file exceeds %d bytes", maxFileBytes)
	}
	return Parse(data)
}

// Parse parses exactly one YAML document containing a mapping whose keys are
// valid variable names and whose values are YAML strings.
func Parse(data []byte) (map[string]string, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("value source is empty")
		}
		return nil, fmt.Errorf("parse YAML: %w", err)
	}

	var trailing yaml.Node
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, fmt.Errorf("parse YAML: %w", err)
		}
		return nil, errors.New("value source must contain exactly one YAML document")
	}

	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("value source must be a YAML mapping")
	}

	result := make(map[string]string)
	mapping := document.Content[0]
	for i := 0; i < len(mapping.Content); i += 2 {
		key := mapping.Content[i]
		value := mapping.Content[i+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || !variableName.MatchString(key.Value) {
			return nil, fmt.Errorf("invalid variable name %q", key.Value)
		}
		if _, exists := result[key.Value]; exists {
			return nil, fmt.Errorf("duplicate variable name %q", key.Value)
		}
		if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
			return nil, fmt.Errorf("value for %q must be a string", key.Value)
		}
		result[key.Value] = value.Value
	}
	return result, nil
}
