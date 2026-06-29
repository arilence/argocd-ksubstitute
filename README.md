# argocd-ksubstitute

The future place for a Argo CD config management plugin for substituting variables in manifests.

Currently does nothing.

## Prerequisites

- [Nix](https://nixos.org/download/) with the `nix-command` and `flakes`
  experimental features enabled.
- Docker, only if you want to load the container image into a local Docker
  daemon.

## Build

Build the default package:

```console
nix build
```

The build creates the `result` symlink. Run the executable with:

```console
./result/bin/ksubstitute
```

To enter the development environment and run the program from source:

```console
nix develop
go run .
```

## Test and Lint

To test while in the development environment:

```console
go test -v .
```

To run full test and lint:

```console
nix flake check --print-build-logs
```

Format Go code with:

```console
go fmt .
```

## Update Go Dependencies

Nix builds dependencies from `gomod2nix.toml`; changing only `go.mod` is not
enough. Whenever Go module dependencies change:

```console
nix develop
go get example.com/some/module@latest
go mod tidy
gomod2nix generate
```

For projects with many dependencies, generate pre-build metadata to improve
subsequent build times:

```console
gomod2nix generate --with-deps
```

If the dependencies are already in the local Go module cache, this optional
command can import them into the Nix store instead of downloading them again:

```console
gomod2nix import
```

See the upstream
[gomod2nix getting-started guide](https://github.com/nix-community/gomod2nix/blob/master/docs/getting-started.md)
for more background.
