# Argo CD CMP Design for `ksubstitute`

## Purpose

This document records the Argo CD Config Management Plugin (CMP) contract that drives the design of
the `ksubstitute` Go application, its container image, and the Argo CD Helm configuration used to
install it.

`ksubstitute` is intended to provide behavior similar to Flux's post-build variable substitution.
Unlike Flux's `substituteFrom`, it does not read ConfigMaps or Secrets from a destination cluster.
Values are placed in files mounted into the CMP sidecar that runs beside `argocd-repo-server`.

The intended render pipeline is:

```text
Git source
  -> Argo CD repo-server
  -> argocd-cmp-server sidecar
  -> kustomize build
  -> ksubstitute post-build substitution
  -> Kubernetes YAML/JSON on stdout
  -> Argo CD comparison and sync
```

## Current Design Decisions

The implementation uses the following decisions:

- Install `ksubstitute` as a sidecar CMP on the repo-server Pod.
- Require Applications to select `ksubstitute-v1` explicitly.
- Run Kustomize inside the CMP and substitute the rendered output.
- Use the Kustomize executable supplied by the repo-server's Argo CD image so the native and CMP
  render paths always use the same version.
- Read each substitution source from a structured YAML file under a configured values root.
- Select ordered sources through the Application's `substitute-from` plugin parameter.
- Treat later sources as higher precedence than earlier sources.
- Replace known variables and leave unknown expressions unchanged.
- Keep KSOPS, SOPS keys, and other decryption tools out of the CMP container.
- Bake the CMP configuration into the container image at the path expected by `argocd-cmp-server`.
- Run without a shell, as numeric user 999, with a read-only root filesystem and a dedicated
  writable `/tmp` volume.

## Argo CD CMP Execution Model

### Sidecar communication

Every repo-server replica must contain a `ksubstitute` sidecar. Argo CD's lightweight
`argocd-cmp-server` process runs as the sidecar entrypoint and communicates with the main
repo-server over a Unix socket in the shared `plugins` volume.

The Argo CD Helm chart provides two volumes used by the sidecar:

- `var-files` exposes `/var/run/argocd/argocd-cmp-server`. A chart-managed init container copies the
  Argo-owned executable into this volume.
- `plugins` is shared with repo-server and provides `/home/argocd/cmp-server/plugins`, where the CMP
  server creates its socket.

These volumes do not contain substitution data.

The sidecar command must be:

```yaml
command:
  - /var/run/argocd/argocd-cmp-server
```

The image does not set a default user or filesystem policy. The Helm sidecar configuration must
enforce the runtime security contract:

```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 999
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  capabilities:
    drop:
      - ALL
  seccompProfile:
    type: RuntimeDefault
```

## Plugin Configuration

### Configuration file

The CMP server requires exactly one plugin configuration at:

```text
/home/argocd/cmp-server/config/plugin.yaml
```

The document looks like a Kubernetes resource, but `ConfigManagementPlugin` is not a Kubernetes CRD.
It is a configuration file read by `argocd-cmp-server`.

The `ksubstitute` image contains this file. The Nix container build installs the repository's
`config.yaml` at the required path, so the Argo CD Helm configuration does not need an
`argocd-cmp-cm` entry or a plugin configuration volume mount for this sidecar. Changing the plugin
configuration requires building and deploying a new image.

### Baked configuration

```yaml
apiVersion: argoproj.io/v1alpha1
kind: ConfigManagementPlugin
metadata:
  name: ksubstitute
spec:
  version: v1
  generate:
    command:
      - /bin/ksubstitute
    args:
      - render
      - --values-root=/.config/ksubstitute-vars
      - .
  parameters:
    static:
      - name: substitute-from
        title: Mounted substitution sources
        required: true
        collectionType: array
        array: []
  preserveFileMode: false
  provideGitCreds: false
```

Because the configuration names the plugin `ksubstitute` and gives it `version: v1`, Applications
must select `ksubstitute-v1`.

Parameter announcements only describe fields to the Argo CD UI. Their defaults are not sent to the
plugin. The Go program must apply its own defaults, and a required `substitute-from` value should be
explicitly present in the Application.

## Application Contract

An Application explicitly selects the versioned plugin and its ordered mounted value sources:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: example-app
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://example.invalid/platform/config.git
    targetRevision: main
    path: kubernetes/apps/example-app
    plugin:
      name: ksubstitute-v1
      parameters:
        - name: substitute-from
          array:
            - ksub-globals
  destination:
    server: https://kubernetes.default.svc
    namespace: example-app
```

The `substitute-from` array is ordered. In the following example, values in
`example-app` override values from `ksub-globals`:

```yaml
parameters:
  - name: substitute-from
    array:
      - ksub-globals
      - example-app
```

An Application should not also depend on `spec.source.kustomize` overrides. Move Kustomize behavior
into the source repository or add an explicit, narrowly defined CMP parameter when repository
configuration cannot express it.

For app-of-apps installations, configure the CMP on each child Application that needs substitution.
The root Application only needs the CMP when the manifests it renders contain substitution
expressions.

## Mounted Values Contract

### Structured source files

Each source should be a YAML mapping from valid variable names to string values:

```yaml
GLOBAL_TIMEZONE: America/Vancouver
GLOBAL_BASE_DOMAIN: example.com
GLOBAL_NAS_DOMAIN: nas.example.com
```

All values should be strings. The loader should reject nested structures, arrays, booleans, and
numbers rather than applying surprising YAML conversions. Quoted numeric and boolean-looking values
remain strings:

```yaml
REPLICA_COUNT: "3"
FEATURE_ENABLED: "true"
```

### Secret format

A Kubernetes Secret can store the source as one structured value:

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
    GLOBAL_NAS_DOMAIN: nas.example.com
```

The Secret must be in the same namespace as the repo-server Pod. The example values are not
confidential and may be stored in a ConfigMap instead. A Secret is appropriate when the same
mechanism carries genuinely sensitive values.

### Projected volume

A projected volume can collect multiple Secret or ConfigMap sources in one directory while assigning
stable filenames:

```yaml
repoServer:
  volumes:
    - name: ksubstitute-values
      projected:
        sources:
          - secret:
              name: ksub-globals
              items:
                - key: values.yaml
                  path: ksub-globals.yaml
          - secret:
              name: example-app-values
              items:
                - key: values.yaml
                  path: example-app.yaml
```

The sidecar mounts the directory once:

```yaml
- name: ksubstitute-values
  mountPath: /.config/ksubstitute-vars
  readOnly: true
```

Mount the entire directory rather than individual files with `subPath`. Kubernetes eventually
propagates updates to normal Secret and ConfigMap volume mounts, while `subPath` mounts do not
receive those updates.

### Source resolution and validation

The loader must treat source names as untrusted Application input. It should:

- Accept a conservative source-name character set.
- Reject empty, absolute, or traversal paths.
- Resolve only `<values-root>/<source-name>.yaml`.
- Verify that the final path remains beneath the configured values root.
- Allow symlinks only when their resolved targets remain beneath the values root, as Kubernetes
  Secret, ConfigMap, and projected volumes use symlinks.
- Reject non-regular files.
- Apply explicit per-file and aggregate size limits.
- Never include file contents in error messages or logs.

Missing sources should fail by default. Optional sources may be added later as an explicit
interface; silent absence should not be the initial behavior.

## Go CLI

The command contract is:

```console
ksubstitute render --values-root=/.config/ksubstitute-vars .
```

Useful optional flags include:

```text
--kustomize=/usr/local/bin/kustomize
--max-render-bytes=<limit>
--max-diagnostic-bytes=<limit>
```

Argo-specific parameter parsing belongs in the `render` command. The lower-level value loading and
substitution packages should remain usable in unit tests without an Argo environment.

### Render algorithm

The command should perform these steps:

1. Parse CLI flags without evaluating a shell.
2. Parse and validate `ARGOCD_APP_PARAMETERS`.
3. Require one ordered `substitute-from` array.
4. Resolve and load each structured source file.
5. Merge source mappings from left to right.
6. Add only explicitly supported Argo build variables, if that feature is enabled.
7. Execute `/usr/local/bin/kustomize build .` directly with `os/exec`.
8. Capture Kustomize stdout within a configured size limit and forward safe diagnostics from stderr.
9. Return Kustomize's failure as a non-zero plugin exit.
10. Substitute known variables and leave unknown expressions unchanged.
11. Validate that the result is a non-empty stream of Kubernetes YAML or JSON.
12. Write only the final manifest stream to stdout.

The command must never turn a failed Kustomize execution into a successful, empty render. Argo CD
can prune every managed resource when a plugin returns an empty desired state, so error propagation
is a safety requirement.

### Output and logging

Argo CD treats stdout from `generate` as manifests. Consequently:

- stdout must contain only Kubernetes YAML or JSON.
- Informational and error messages must go to stderr.
- Neither stream should log substitution values.
- Error messages should identify source aliases and variable names only when doing so does not
  expose sensitive data.
- Every failure must return a non-zero exit status.

Argo exposes plugin stderr in the UI and repo-server logs, including successful runs at suitable log
levels. Treat stderr as operator-visible rather than secret.

## Container Image Requirements

### Required contents

The image exposes these stable paths:

```text
/bin/ksubstitute
/home/argocd/cmp-server/config/plugin.yaml
```

The Go application is added to the image root so `/bin/ksubstitute` is stable. Kustomize must not be
included in this image; it is injected from the exact Argo CD image used by repo-server and mounted
at `/usr/local/bin/kustomize`.

The image includes CA certificates and OpenSSL, but does not include Git or a shell. The policy for
remote Kustomize bases and network access remains an open product decision.

The image does not need to include `argocd-cmp-server`. The Argo CD chart makes that binary
available through `var-files`, and the sidecar command overrides the image's default command.

## Caching and Value Updates

Normal Secret and ConfigMap volume mounts eventually receive updates from the Kubernetes kubelet.
Argo CD, however, does not include mounted file contents in its manifest cache key. A changed values
file does not by itself cause a new manifest render.

The production cache-invalidation mechanism is the Application annotation
`argocd.argoproj.io/refresh: hard`. A hard refresh invalidates Argo CD's manifest and target-cluster
state caches before refreshing the Application. The annotation is consumed and removed by the
application controller.

The Secret update and hard refresh must be ordered to avoid rendering with the old mounted values:

1. Update the Secret or ConfigMap.
2. Wait until the expected value-file digest is visible in every repo-server Pod. Every replica must
   be checked because Argo CD may send generation to any of them. The image includes `/bin/openssl`,
   so automation can execute `openssl dgst -sha256` in each `cmp-ksubstitute` sidecar without
   printing the file contents.
3. Annotate every affected Application with a hard refresh.

Waiting a fixed number of seconds is weaker than checking the mounted digest, because the maximum
propagation delay depends on kubelet configuration. If the value-management system cannot inspect
every replica, the deterministic fallback is to restart or roll out the repo-server Deployment after
updating the Secret, wait for all replicas to become Ready, and then request the hard refresh.

Request a hard refresh with:

```console
kubectl annotate application example-app \
  -n argocd \
  argocd.argoproj.io/refresh=hard \
  --overwrite
```

## Upstream References

- [Argo CD Config Management Plugins](https://argo-cd.readthedocs.io/en/latest/operator-manual/config-management-plugins/)
- [Argo CD build environment](https://argo-cd.readthedocs.io/en/latest/user-guide/build-environment/)
- [Argo CD annotations and labels](https://argo-cd.readthedocs.io/en/latest/user-guide/annotations-and-labels/)
- [Argo CD Helm chart values](https://github.com/argoproj/argo-helm/blob/main/charts/argo-cd/values.yaml)
- [Flux post-build variable substitution](https://fluxcd.io/flux/components/kustomize/kustomizations/#post-build-variable-substitution)
- [Kubernetes Secret volumes](https://kubernetes.io/docs/concepts/storage/volumes/#secret)
- [Kubernetes projected volumes](https://kubernetes.io/docs/concepts/storage/projected-volumes/)
