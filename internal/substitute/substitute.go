// Package substitute replaces variable expressions in rendered manifests.
package substitute

import (
	"bytes"
	"fmt"
	"regexp"
)

var expression = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Apply replaces ${NAME} expressions that have a corresponding value. Unknown
// expressions are left unchanged.
func Apply(input []byte, values map[string]string) ([]byte, error) {
	return ApplyLimited(input, values, 0)
}

// ApplyLimited replaces expressions like Apply and rejects output larger than
// maxBytes. A non-positive limit disables the output-size check.
func ApplyLimited(input []byte, values map[string]string, maxBytes int64) ([]byte, error) {
	matches := expression.FindAllSubmatchIndex(input, -1)
	outputBytes := int64(len(input))
	for _, match := range matches {
		value, ok := values[string(input[match[2]:match[3]])]
		if !ok {
			continue
		}
		outputBytes += int64(len(value) - (match[1] - match[0]))
	}

	if maxBytes > 0 && outputBytes > maxBytes {
		return nil, fmt.Errorf("substituted manifests exceed %d bytes", maxBytes)
	}

	var output bytes.Buffer
	if outputBytes <= int64(^uint(0)>>1) {
		output.Grow(int(outputBytes))
	}
	previous := 0
	for _, match := range matches {
		_, _ = output.Write(input[previous:match[0]])
		if value, ok := values[string(input[match[2]:match[3]])]; ok {
			_, _ = output.WriteString(value)
		} else {
			_, _ = output.Write(input[match[0]:match[1]])
		}
		previous = match[1]
	}
	_, _ = output.Write(input[previous:])

	return output.Bytes(), nil
}
