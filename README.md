<img src="./assets/gpud.svg" height="100" alt="GPUd logo">

![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/leptonai/gpud?sort=semver)
[![Go Reference](https://pkg.go.dev/badge/github.com/leptonai/gpud.svg)](https://pkg.go.dev/github.com/leptonai/gpud)
[![codecov](https://codecov.io/gh/leptonai/gpud/graph/badge.svg?token=G8MGRK9X4A)](https://codecov.io/gh/leptonai/gpud)

## Overview

[GPUd](https://www.gpud.ai) is designed to ensure GPU efficiency and reliability by actively monitoring GPUs and effectively managing AI/ML workloads.

## Why GPUd

GPUd is built on years of experience operating large-scale GPU clusters at Meta, Alibaba Cloud, Uber, and Lepton AI. It is carefully designed to be self-contained and to integrate seamlessly with other systems such as Docker, containerd, Kubernetes, and NVIDIA ecosystems.

- **First-class GPU support**: GPUd is GPU-centric, providing a unified view of critical GPU metrics and issues.
- **Easy to run at scale**: GPUd is a self-contained binary that runs on supported Linux machines with a low footprint.
- **Production grade**: GPUd is used in [DGX Cloud Lepton](https://www.nvidia.com/en-us/data-center/dgx-cloud-lepton/)'s production infrastructure.

GPUd keeps monitoring off the workload critical path and minimizes CPU and memory overhead. See [*architecture*](./docs/ARCHITECTURE.md) for more details.

## Get Started

The fastest way to see `gpud` in action is to watch our 40-second demo video below. For more detailed guides, see our [Tutorials page](./docs/TUTORIALS.md).

<a href="https://www.youtube.com/watch?v=sq-7_Zrv7-8" target="_blank">
<img src="https://i3.ytimg.com/vi/sq-7_Zrv7-8/maxresdefault.jpg" alt="gpud-2025-06-01-01-install-and-scan" />
</a>

### Installation

To install from the official release on Linux amd64 (x86_64) machine:

```bash
curl -fsSL https://pkg.gpud.dev/install.sh | sh
```

To install the latest published version explicitly:

```bash
curl -fsSL https://pkg.gpud.dev/install.sh | sh -s -- "$(curl -fsSL https://pkg.gpud.dev/unstable_latest.txt)"
```

The install script supports Linux on amd64 and arm64.

---

### Run GPUd on a Host

This section covers running `gpud` directly on a host machine.

#### Requirements for DGX Cloud Lepton

Before adding a machine to DGX Cloud Lepton, review the current
[NVIDIA DGX Cloud Lepton BYOC Requirements](https://docs.nvidia.com/dgx-cloud/lepton/compute/bring-your-own-compute/requirements/).

#### With `systemd` (Recommended for Linux)

**Start the service:**

```bash
sudo gpud up
```

To add the machine to DGX Cloud Lepton, open
[Node Groups](https://dashboard.dgxc-lepton.nvidia.com/workspace-redirect/node-groups/list),
select **Add Machines > Add via Local Command**, and use the generated
command. It installs GPUd and registers the machine with this `gpud up` form:

```bash
sudo gpud up \
  --token <DGXC_LEPTON_REGISTRATION_TOKEN> \
  --endpoint <DGXC_LEPTON_ENDPOINT> \
  --node-group <DGXC_LEPTON_NODE_GROUP>
```

**Stop the service:**

```bash
sudo gpud down
```

**Uninstall:**

```bash
sudo rm /usr/local/bin/gpud
sudo rm /etc/systemd/system/gpud.service
```

#### Without `systemd` (Linux)

**Run in the foreground:**

```bash
gpud run
```

**Run in the background:**

```bash
nohup sudo /usr/local/bin/gpud run &>> <your_log_file_path> &
```

**Uninstall:**

```bash
sudo rm /usr/local/bin/gpud
```

---

### Run GPUd with Kubernetes

The recommended way to deploy GPUd on Kubernetes is with our official
[Helm chart](./deployments/helm/gpud/README.md), published through both
[GitHub Pages](https://leptonai.github.io/gpud) and the
[NGC catalog](https://catalog.ngc.nvidia.com/orgs/nvidia/lepton/helm-charts/gpud/-).
The default [`nvcr.io/nvidia/lepton/gpud` image](https://catalog.ngc.nvidia.com/orgs/nvidia/lepton/containers/gpud/-/tags)
is public, so it does not require an image pull secret or NGC API key.

Install or upgrade to the latest published release from GitHub Pages:

```bash
helm repo add gpud https://leptonai.github.io/gpud
helm repo update gpud

GPUD_VERSION="$(curl -fsSL https://pkg.gpud.dev/unstable_latest.txt)"
GPUD_VERSION="${GPUD_VERSION#v}"

helm upgrade --install gpud gpud/gpud \
  --version "$GPUD_VERSION" \
  --set image.repository=nvcr.io/nvidia/lepton/gpud \
  --create-namespace \
  --namespace gpud
```

Or pull the same chart from NGC:

```bash
helm pull https://helm.ngc.nvidia.com/nvidia/lepton/charts/gpud-0.12.24.tgz
```

### Build with Docker

A Dockerfile is provided to build a container image from source. For complete instructions, please see our [Docker guide in CONTRIBUTING.md](CONTRIBUTING.md#building-with-docker).

---

## Key Features

- Monitors critical GPU and GPU fabric metrics (power, temperature).
- Reports GPU and GPU fabric status (nvidia-smi parser, error checking).
- Detects critical GPU and GPU fabric errors (kmsg, hardware slowdown, NVML Xid event, DCGM).
- Monitors overall system metrics (CPU, memory, disk).

Check out [*components*](./docs/COMPONENTS.md) for a detailed list of components and their features.

## Integration

For users looking to set up a platform to collect and process data from gpud, please refer to [INTEGRATION](./docs/INTEGRATION.md).

## FAQs

### Does GPUd send data to DGX Cloud Lepton?

GPUd connects to DGX Cloud Lepton only after it has been registered with the
platform. The authenticated session exchanges the machine, health, and runtime
information needed to manage the node.

### How to update GPUd?

GPUd is still in active development, regularly releasing new versions for critical bug fixes and new features. We strongly recommend always being on the latest version of GPUd.

Host installations started with `gpud up` enable automatic updates by default.
To disable them, append `--enable-auto-update=false` to the existing `FLAGS`
value in `/etc/default/gpud`, then restart the service.

## Learn more

- [Why GPUd](./docs/WHY.md)
- [Install GPUd](./docs/INSTALL.md)
- [GPUd components](./docs/COMPONENTS.md)
- [GPUd architecture](./docs/ARCHITECTURE.md)

## Contributing

Please see the [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on how to contribute to this project.
