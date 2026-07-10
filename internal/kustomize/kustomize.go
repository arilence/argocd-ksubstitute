// Package kustomize runs an external Kustomize executable.
package kustomize

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Builder produces manifests from a Kustomize source directory.
type Builder interface {
	Build(context.Context, string) ([]byte, error)
}

// ExternalBuilder invokes the Kustomize CLI without a shell.
type ExternalBuilder struct {
	Executable         string
	MaxOutputBytes     int64
	MaxDiagnosticBytes int64
	Diagnostics        io.Writer
}

// Build runs "kustomize build ." with directory as the working directory.
func (builder ExternalBuilder) Build(ctx context.Context, directory string) ([]byte, error) {
	if builder.Executable == "" {
		return nil, errors.New("kustomize executable cannot be empty")
	}
	if builder.MaxOutputBytes <= 0 {
		return nil, errors.New("maximum Kustomize output must be positive")
	}
	if builder.MaxDiagnosticBytes <= 0 {
		return nil, errors.New("maximum Kustomize diagnostics must be positive")
	}

	stdout := newBoundedBuffer(builder.MaxOutputBytes)
	stderr := newBoundedBuffer(builder.MaxDiagnosticBytes)
	command := exec.CommandContext(ctx, builder.Executable, "build", ".")
	command.Dir = directory
	command.Stdout = stdout
	command.Stderr = stderr

	err := command.Run()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if stderr.Exceeded() {
			message += " (diagnostics truncated)"
		}
		if message != "" {
			return nil, fmt.Errorf("kustomize build failed: %w: %s", err, message)
		}
		return nil, fmt.Errorf("kustomize build failed: %w", err)
	}
	if stdout.Exceeded() {
		return nil, fmt.Errorf("kustomize output exceeds %d bytes", builder.MaxOutputBytes)
	}
	if stderr.Len() != 0 && builder.Diagnostics != nil {
		if _, err := builder.Diagnostics.Write(stderr.Bytes()); err != nil {
			return nil, fmt.Errorf("write kustomize diagnostics: %w", err)
		}
		if stderr.Exceeded() {
			_, _ = io.WriteString(builder.Diagnostics, "\nksubstitute: kustomize diagnostics truncated\n")
		}
	}

	return append([]byte(nil), stdout.Bytes()...), nil
}

type boundedBuffer struct {
	buffer   bytes.Buffer
	maxBytes int64
	exceeded bool
}

func newBoundedBuffer(maxBytes int64) *boundedBuffer {
	return &boundedBuffer{maxBytes: maxBytes}
}

func (buffer *boundedBuffer) Write(input []byte) (int, error) {
	written := len(input)
	remaining := buffer.maxBytes - int64(buffer.buffer.Len())
	if remaining <= 0 {
		buffer.exceeded = true
		return written, nil
	}
	if int64(len(input)) > remaining {
		_, _ = buffer.buffer.Write(input[:int(remaining)])
		buffer.exceeded = true
		return written, nil
	}
	_, _ = buffer.buffer.Write(input)
	return written, nil
}

func (buffer *boundedBuffer) Bytes() []byte {
	return buffer.buffer.Bytes()
}

func (buffer *boundedBuffer) String() string {
	return buffer.buffer.String()
}

func (buffer *boundedBuffer) Len() int {
	return buffer.buffer.Len()
}

func (buffer *boundedBuffer) Exceeded() bool {
	return buffer.exceeded
}
