# GPUd Helm Chart

This Helm chart deploys [GPUd](https://github.com/leptonai/gpud) as a [DaemonSet](https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/) on a Kubernetes cluster.

GPUd is a lightweight, high-performance daemon that monitors GPU resources. This chart is the recommended way to deploy GPUd on Kubernetes.

The default `nvcr.io/nvidia/lepton/gpud` image is public. No image pull secret
or NGC API key is required.
Packaged charts derive the image tag from the pushed Git tag through
`appVersion`; `values.yaml` does not pin a release tag.

## Prerequisites

- Kubernetes 1.19+
- [Helm](https://helm.sh/) 3.16.1+

## Installing the Chart

Add the GPUd chart repository and resolve the latest published version from
`unstable_latest.txt`:

```bash
helm repo add gpud https://leptonai.github.io/gpud
helm repo update gpud

GPUD_VERSION="$(curl -fsSL https://pkg.gpud.dev/unstable_latest.txt)"
GPUD_VERSION="${GPUD_VERSION#v}"

helm upgrade --install my-gpud gpud/gpud \
  --version "$GPUD_VERSION" \
  --set image.repository=nvcr.io/nvidia/lepton/gpud \
  --create-namespace \
  --namespace gpud
```

### Migrating from the OCI chart registry

GitHub Container Registry no longer hosts GPUd charts. Replace an OCI install
such as `oci://ghcr.io/leptonai/gpud` with the `gpud/gpud` chart after adding
the repository above:

```bash
helm upgrade --install my-gpud gpud/gpud \
  --version "$GPUD_VERSION" \
  --set image.repository=nvcr.io/nvidia/lepton/gpud \
  --create-namespace \
  --namespace gpud
```

The container image remains independently configurable through
`image.repository` and `image.tag`.

To let Helm select the newest chart version automatically:

```bash
helm upgrade --install my-gpud gpud/gpud \
  --set image.repository=nvcr.io/nvidia/lepton/gpud \
  --create-namespace \
  --namespace gpud
```

### Installing with Custom Values

To install with a specific image tag and disable telemetry:

```bash
helm install my-gpud gpud/gpud \
  --create-namespace \
  --namespace gpud \
  --set image.repository=nvcr.io/nvidia/lepton/gpud \
  --set image.tag="<MY_IMAGE_TAG>" \
  --set gpud.telemetry.enabled=false
```

You can also provide a custom `values.yaml` file:

```bash
helm install my-gpud gpud/gpud \
  --namespace gpud -f my-values.yaml
```

### Reboot Support Inside DaemonSet Pods

The chart passes `gpud.rebootCommands` to `gpud run --reboot-commands`. By
default, it uses `nsenter` to enter the host namespaces through PID 1 and ask
the host init system to reboot the node. This is the path needed when GPUd runs
inside a privileged DaemonSet pod rather than directly as a host systemd
service.

The default reboot path depends on these chart defaults:

- `hostPID: true`
- `securityContext.privileged: true`
- `securityContext.runAsUser: 0`
- `securityContext.allowPrivilegeEscalation: true`

Use a values file for multi-line reboot commands:

```yaml
gpud:
  rebootCommands: |
    set -o errexit
    set -o nounset

    if command -v nsenter >/dev/null 2>&1; then
      if nsenter --target 1 --mount --uts --ipc --net --pid --cgroup --root --wd=/ -- /usr/bin/systemctl reboot; then
        exit 0
      fi
      nsenter --target 1 --mount --uts --ipc --net --pid --cgroup --root --wd=/ -- /sbin/reboot
      exit 0
    fi

    sudo reboot
```

Set `gpud.rebootCommands: ""` to keep GPUd's built-in reboot behavior instead.
If `gpud.commandOverride` is set, the default startup wrapper is bypassed, so
include `--reboot-commands` in the override when node reboot support is needed.

### Disk Inspection Inside DaemonSet Pods

The disk component shells out to `findmnt` and `lsblk` and measures filesystem
usage via `statfs`. Run from inside a container, these report the container's
overlay rootfs instead of the node's real disks (the root disk and any data
disks). The chart therefore wraps each of them with `nsenter --target 1 --mount`
by default so they run in the host's mount namespace:

```yaml
gpud:
  findmntCommands: "nsenter --target 1 --mount -- findmnt"
  lsblkCommands: "nsenter --target 1 --mount -- lsblk"
  blockdevUsageCommands: "nsenter --target 1 --mount -- df"
```

These map to `gpud run --findmnt-commands`, `--lsblk-commands`, and
`--blockdev-usage-commands`. The value is the invocation prefix; GPUd appends the
flags it controls (for example `findmnt --target ... --json --df`, or
`df -T -B1 -P`). They depend on the same chart defaults as `rebootCommands`
(`hostPID: true`, `securityContext.privileged: true`, `runAsUser: 0`).

Set any of these to `""` to keep GPUd's built-in in-namespace behavior, which is
appropriate when GPUd runs directly on the host (e.g. as a systemd service) or in
a non-privileged pod. As with reboot, if `gpud.commandOverride` is set the
default startup wrapper is bypassed, so include these flags in the override when
host disk inspection is needed.

### Containerd Monitoring Inside DaemonSet Pods

The containerd component checks the host's containerd socket and CRI endpoint,
reads `/etc/containerd/config.toml` to verify the NVIDIA runtime is configured,
and checks whether the containerd systemd service is active. Run from inside a
container, the socket/config are not visible and the systemd check only sees the
container's own service manager. The chart addresses both:

```yaml
gpud:
  # Bind-mount the host's containerd dirs (default true):
  #   /run/containerd (read-write)  -> socket + CRI endpoint
  #   /etc/containerd (read-only)   -> config.toml NVIDIA-runtime check
  # Both use hostPath DirectoryOrCreate, so the pod still starts without containerd.
  mountContainerd: true

  # Check whether the host's containerd service is active (exit code 0 = active),
  # mapped to "gpud run --containerd-service-active-commands".
  containerdServiceActiveCommands: "nsenter --target 1 --mount -- systemctl is-active containerd"
```

Set `gpud.mountContainerd: false` and `gpud.containerdServiceActiveCommands: ""`
on nodes that do not run containerd (e.g. docker or cri-o only).

### Session Token from a Secret

When `nodeLabelExporter.enabled=true`, the chart's init container can read the
session token from a node label. To read it from an existing Kubernetes Secret
instead, set `gpud.tokenSecret`; the token is injected as the `TOKEN` env via
`secretKeyRef` and takes priority over any token label:

```yaml
gpud:
  tokenSecret:
    name: gpud-token   # existing Secret in the release namespace
    key: TOKEN
```

To have the Helm release create and own that Secret, also set `create` and
`value`:

```yaml
gpud:
  tokenSecret:
    create: true
    name: gpud-token
    key: TOKEN
    value: "<YOUR_GPUD_SESSION_TOKEN>"
```

The token is stored in Helm values and release state when `value` is used.
Restrict access to both.

### BYOK Worker Cluster Example

The public default image also works for BYOK clusters without an image pull
secret. Install or upgrade with a values file like this (a full BYOK overlay):

```yaml
gpud:
  endpoint: gpud-manager-dev02.dev02.dgxc-lepton-dev.nvidia.com
  # Create and use the gpud-token Secret as part of this Helm release.
  tokenSecret:
    create: true
    name: gpud-token
    key: TOKEN
    value: "<YOUR_GPUD_SESSION_TOKEN>"
  # The node also exposes /dev/mem on this platform.
  mountHostDevMem: true
  # reboot / disk (findmnt,lsblk,df) / containerd command overrides and the
  # containerd socket+config mounts are chart defaults (nsenter wrappers).

# Pass the per-node platform machine ID from the node label to
# "gpud run --machine-id", so GPUd rejoins as the SAME machine after a pod
# replacement, node reboot, or reimage (even if /etc/machine-id changes).
# This is intentionally init-only: gpud reads the file once at startup, so a
# sidecar update could not reconfigure the already-running process.
nodeLabelExporter:
  enabled: true
  labelKeys:
    machineId: lepton.ai/machine-id
  resources: null

# GPU workloads on these nodes request whole nodes, so the DaemonSet should
# consume none of the node's allocatable resources. null (not {}) deletes the
# chart's default requests/limits; the pod runs as BestEffort and its
# system-node-critical priority still keeps it scheduled first, evicted last.
resources: null

# Schedule only on the intended worker nodes.
affinity:
  nodeAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      nodeSelectorTerms:
        - matchExpressions:
            - key: node.lepton.ai/dedicated-node-group-id
              operator: Exists
            - key: lepton.ai/machine-id
              operator: Exists
```

Install or upgrade with that values file in the same namespace:

```bash
helm upgrade --install gpud gpud/gpud \
  --create-namespace \
  --namespace gpud \
  --set image.repository=nvcr.io/nvidia/lepton/gpud \
  -f byok-values.yaml
```

This makes a `helm install` a drop-in replacement for a hand-written GPUd
DaemonSet: the machine ID is preserved (node label + the persisted
`/var/lib/gpud` hostPath), the token comes from the Secret, and the disk and
containerd components report the node's real state via the nsenter wrappers.

## Uninstalling the Chart

To uninstall the `my-gpud` release:

```bash
helm uninstall my-gpud --namespace gpud
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Configuration

For a full list of configurable parameters, see the [values.yaml](values.yaml) file.
