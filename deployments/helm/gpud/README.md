# GPUd Helm Chart

This Helm chart deploys [GPUd](https://github.com/leptonai/gpud) as a [DaemonSet](https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/) on a Kubernetes cluster.

GPUd is a lightweight, high-performance daemon that monitors GPU resources. This chart is the recommended way to deploy GPUd on Kubernetes.

## Prerequisites

- Kubernetes 1.19+
- [Helm](https://helm.sh/) 3.16.1+

## Installing the Chart

To install the chart with the release name `my-gpud`:

```bash
helm install my-gpud <YOUR_REPO_NAME>/gpud \
  --create-namespace \
  --namespace gpud
```

### Installing with Custom Values

To install with a specific image tag and disable telemetry:

```bash
helm install my-gpud <YOUR_REPO_NAME>/gpud \
  --create-namespace \
  --namespace gpud \
  --set image.tag="<MY_IMAGE_TAG>" \
  --set gpud.telemetry.enabled=false
```

You can also provide a custom `values.yaml` file:

```bash
helm install my-gpud <YOUR_REPO_NAME>/gpud \
  --namespace gpud -f my-values.yaml
```

## Uninstalling the Chart

To uninstall the `my-gpud` release:

```bash
helm uninstall my-gpud --namespace gpud
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Configuration

For a full list of configurable parameters, see the [values.yaml](values.yaml) file.
