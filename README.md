# argocd-ksubstitute

An Argo CD config management plugin (CMP) for substituting variables in rendered manifests, akin to
Flux CD's post-build variable substitution. Enables the use of `${VAR_NAME}` inside of Argo CD
applications, replacing `${VAR_NAME}` with the value associated with the `VAR_NAME` key in a Secret
manifest.

> **AI use disclaimer:** This project is a proof of concept, primarily made via LLMs. I would not
> use this in production.

## Prerequisites

- [Nix](https://nixos.org/download/) with the `nix-command` and `flakes`
  experimental features enabled.

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
go run ./cmd/ksubstitute --help
```

## Adding to Argo CD

The following example is intended for installations managed with the [Argo CD Helm
chart](https://github.com/argoproj/argo-helm/tree/main/charts/argo-cd).

The sidecar image contains `ksubstitute`, but intentionally does not contain Kustomize. An init
container copies Kustomize shipped with Argo CD into the shared `var-files` volume, and the sidecar
mounts that binary at `/usr/local/bin/kustomize`. The example also mounts a Secret key as
`ksub-globals.yaml`, making the source alias `ksub-globals` available to Applications.

Add the following to the chart's values, replacing the sidecar image and substitution Secret with
values appropriate for your cluster:

```yaml
repoServer:
  initContainers:
    - name: copy-kustomize
      image: '{{ default .Values.global.image.repository .Values.repoServer.image.repository }}:{{ default (include "argo-cd.defaultTag" .) .Values.repoServer.image.tag }}'
      command:
        - /bin/cp
      args:
        - /usr/local/bin/kustomize
        - /var/run/argocd/kustomize
      volumeMounts:
        - name: var-files
          mountPath: /var/run/argocd

  volumes:
    - name: ksubstitute-vars
      secret:
        secretName: ksub-globals
        items:
          - key: values.yaml
            path: ksub-globals.yaml
    - name: ksubstitute-tmp
      emptyDir: {}

  extraContainers:
    - name: cmp-ksubstitute
      image: registry.example.com/ksubstitute:latest
      command:
        - /var/run/argocd/argocd-cmp-server
      env:
        - name: HOME
          value: /tmp
      volumeMounts:
        - name: var-files
          mountPath: /var/run/argocd
        - name: var-files
          mountPath: /usr/local/bin/kustomize
          subPath: kustomize
          readOnly: true
        - name: plugins
          mountPath: /home/argocd/cmp-server/plugins
        - name: ksubstitute-vars
          mountPath: /.config/ksubstitute-vars
          readOnly: true
        - name: ksubstitute-tmp
          mountPath: /tmp
```

The referenced Secret must exist in the same namespace as Argo CD and contain a `values.yaml` key.
For example:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: ksub-globals
  namespace: argocd
type: Opaque
stringData:
  values.yaml: |
    GLOBAL_TIMEZONE: America/Vancouver
    GLOBAL_BASE_DOMAIN: example.com
```

Enable the plugin in desired Argo CD Applications:

```yaml
spec:
  source:
    plugin:
      name: ksubstitute-v1
      parameters:
        - name: substitute-from
          array:
            - ksub-globals
```

Each value in the array resolves to `<values-root>/<alias>.yaml`; in this example, `ksub-globals`
resolves to `/.config/ksubstitute-vars/ksub-globals.yaml`.

After changing the plugin configuration, perform a Helm upgrade so the repo-server Pod is restarted
with the new ConfigMap content.

## Test and Lint

To run full test and lint:

```console
nix flake check --print-build-logs
```

To test while in the development environment:

```console
go test -v ./...
```

## Update Go Dependencies

Nix builds dependencies from `gomod2nix.toml`; changing only `go.mod` is not enough. Whenever Go
module dependencies change:

```console
nix develop
go get example.com/some/module@latest
go mod tidy
gomod2nix generate
```

See the upstream
[gomod2nix getting-started guide](https://github.com/nix-community/gomod2nix/blob/master/docs/getting-started.md)
for more background.
