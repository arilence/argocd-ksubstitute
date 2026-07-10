// Package render orchestrates Kustomize and post-build substitution.
package render

import (
	"context"
	"errors"
	"fmt"

	"github.com/arilence/argocd-ksubstitute/internal/argocdparams"
	"github.com/arilence/argocd-ksubstitute/internal/kustomize"
	"github.com/arilence/argocd-ksubstitute/internal/manifest"
	"github.com/arilence/argocd-ksubstitute/internal/substitute"
	"github.com/arilence/argocd-ksubstitute/internal/values"
)

// Options contains the operator and Application inputs to a render.
type Options struct {
	Directory      string
	ValuesRoot     string
	ParametersJSON string
	MaxOutputBytes int64
}

// Generate builds and substitutes a complete manifest stream. It returns no
// output unless every stage succeeds.
func Generate(ctx context.Context, builder kustomize.Builder, options Options) ([]byte, error) {
	if builder == nil {
		return nil, errors.New("kustomize builder is required")
	}
	if options.Directory == "" {
		return nil, errors.New("source directory cannot be empty")
	}
	if options.ValuesRoot == "" {
		return nil, errors.New("values root cannot be empty")
	}
	if options.MaxOutputBytes <= 0 {
		return nil, errors.New("maximum manifest output must be positive")
	}

	sources, err := argocdparams.SubstituteFrom(options.ParametersJSON)
	if err != nil {
		return nil, err
	}
	loadedValues, err := values.LoadSources(options.ValuesRoot, sources)
	if err != nil {
		return nil, err
	}
	rendered, err := builder.Build(ctx, options.Directory)
	if err != nil {
		return nil, err
	}
	substituted, err := substitute.ApplyLimited(rendered, loadedValues, options.MaxOutputBytes)
	if err != nil {
		return nil, err
	}
	if err := manifest.Validate(substituted); err != nil {
		return nil, fmt.Errorf("validate rendered manifests: %w", err)
	}
	return substituted, nil
}
