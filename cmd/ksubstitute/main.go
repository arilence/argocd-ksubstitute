package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/arilence/argocd-ksubstitute/internal/kustomize"
	"github.com/arilence/argocd-ksubstitute/internal/render"
	"github.com/arilence/argocd-ksubstitute/internal/substitute"
	"github.com/arilence/argocd-ksubstitute/internal/values"
)

const maxInputBytes int64 = 10 << 20

const (
	defaultMaxRenderBytes     int64 = 10 << 20
	defaultMaxDiagnosticBytes int64 = 1 << 20
)

type valueFiles []string

func (files *valueFiles) String() string {
	return fmt.Sprint([]string(*files))
}

func (files *valueFiles) Set(path string) error {
	if path == "" {
		return errors.New("value file path cannot be empty")
	}
	*files = append(*files, path)
	return nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(runContext(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return runContext(context.Background(), args, stdin, stdout, stderr)
}

func runContext(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		writeUsage(stderr)
		return 2
	}

	switch args[0] {
	case "substitute":
		if err := runSubstitute(args[1:], stdin, stdout, stderr); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			_, _ = fmt.Fprintf(stderr, "ksubstitute: %v\n", err)
			return 1
		}
		return 0
	case "render":
		if err := runRender(ctx, args[1:], stdout, stderr); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			_, _ = fmt.Fprintf(stderr, "ksubstitute: %v\n", err)
			return 1
		}
		return 0
	case "help", "-h", "--help":
		writeUsage(stdout)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "ksubstitute: unknown command %q\n", args[0])
		writeUsage(stderr)
		return 2
	}
}

func runSubstitute(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("substitute", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "Usage: ksubstitute substitute --values FILE [--values FILE ...]")
	}

	var files valueFiles
	flags.Var(&files, "values", "YAML value file; repeat for ordered overrides")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}
	if len(files) == 0 {
		return errors.New("at least one --values file is required")
	}

	merged := make(map[string]string)
	for _, path := range files {
		loaded, err := values.LoadFile(path)
		if err != nil {
			return fmt.Errorf("load values from %q: %w", path, err)
		}
		for name, value := range loaded {
			merged[name] = value
		}
	}

	input, err := io.ReadAll(io.LimitReader(stdin, maxInputBytes+1))
	if err != nil {
		return fmt.Errorf("read manifest input: %w", err)
	}
	if int64(len(input)) > maxInputBytes {
		return fmt.Errorf("manifest input exceeds %d bytes", maxInputBytes)
	}
	if len(bytes.TrimSpace(input)) == 0 {
		return errors.New("manifest input is empty")
	}

	output, err := substitute.Apply(input, merged)
	if err != nil {
		return err
	}
	if _, err := stdout.Write(output); err != nil {
		return fmt.Errorf("write manifest output: %w", err)
	}
	return nil
}

func runRender(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("render", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "Usage: ksubstitute render --values-root DIR [DIRECTORY]")
	}

	var valuesRoot, kustomizePath string
	maxRenderBytes := defaultMaxRenderBytes
	maxDiagnosticBytes := defaultMaxDiagnosticBytes
	flags.StringVar(&valuesRoot, "values-root", "", "root directory containing mounted YAML value sources")
	flags.StringVar(&kustomizePath, "kustomize", "/usr/local/bin/kustomize", "path to the Kustomize executable")
	flags.Int64Var(&maxRenderBytes, "max-render-bytes", defaultMaxRenderBytes, "maximum bytes accepted from Kustomize and after substitution")
	flags.Int64Var(&maxDiagnosticBytes, "max-diagnostic-bytes", defaultMaxDiagnosticBytes, "maximum bytes captured from Kustomize stderr")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("expected at most one source directory, got: %v", flags.Args())
	}
	if valuesRoot == "" {
		return errors.New("--values-root is required")
	}
	if maxRenderBytes <= 0 {
		return errors.New("--max-render-bytes must be positive")
	}
	if maxDiagnosticBytes <= 0 {
		return errors.New("--max-diagnostic-bytes must be positive")
	}

	directory := "."
	if flags.NArg() == 1 {
		directory = flags.Arg(0)
	}
	parameters, exists := os.LookupEnv("ARGOCD_APP_PARAMETERS")
	if !exists {
		return errors.New("ARGOCD_APP_PARAMETERS is not set")
	}

	builder := kustomize.ExternalBuilder{
		Executable:         kustomizePath,
		MaxOutputBytes:     maxRenderBytes,
		MaxDiagnosticBytes: maxDiagnosticBytes,
		Diagnostics:        stderr,
	}
	output, err := render.Generate(ctx, builder, render.Options{
		Directory:      directory,
		ValuesRoot:     valuesRoot,
		ParametersJSON: parameters,
		MaxOutputBytes: maxRenderBytes,
	})
	if err != nil {
		return err
	}
	if _, err := stdout.Write(output); err != nil {
		return fmt.Errorf("write rendered manifests: %w", err)
	}
	return nil
}

func writeUsage(w io.Writer) {
	_, _ = io.WriteString(w, "Usage: ksubstitute <command>\n\n"+
		"Commands:\n"+
		"  render      Build a Kustomize source and apply Argo CD substitutions\n"+
		"  substitute  Substitute ${NAME} expressions in manifests from stdin\n")
}
