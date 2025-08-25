<img src="./assets/gpud.svg" height="100" alt="GPUd logo">

[![Go Report Card](https://goreportcard.com/badge/github.com/leptonai/gpud)](https://goreportcard.com/report/github.com/leptonai/gpud)
![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/leptonai/gpud?sort=semver)
[![Go Reference](https://pkg.go.dev/badge/github.com/leptonai/gpud.svg)](https://pkg.go.dev/github.com/leptonai/gpud)
[![codecov](https://codecov.io/gh/leptonai/gpud/graph/badge.svg?token=G8MGRK9X4A)](https://codecov.io/gh/leptonai/gpud)

## Overview

[GPUd](https://www.gpud.ai) is designed to ensure GPU efficiency and reliability by actively monitoring GPUs and effectively managing AI/ML workloads.

## Why GPUd

GPUd is built on years of experience operating large-scale GPU clusters at Meta, Alibaba Cloud, Uber, and Lepton AI. It is carefully designed to be self-contained and to integrate seamlessly with other systems such as Docker, containerd, Kubernetes, and Nvidia ecosystems.

- **First-class GPU support**: GPUd is GPU-centric, providing a unified view of critical GPU metrics and issues.
- **Easy to run at scale**: GPUd is a self-contained binary that runs on any machine with a low footprint.
- **Production grade**: GPUd is used in [Lepton AI](https://lepton.ai/)'s production infrastructure.

Most importantly, GPUd operates with minimal CPU and memory overhead in a non-critical path and requires only read-only operations. See [*architecture*](./docs/ARCHITECTURE.md) for more details.

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

To specify a version:

```bash
curl -fsSL https://pkg.gpud.dev/install.sh | sh -s v0.6.0
```

Note that the install script does not currenlty support other architectures (e.g., arm64) and OSes (e.g., macOS).

---

### Run GPUd on a Host

This section covers running `gpud` directly on a host machine.

#### With `systemd` (Recommended for Linux)

**Start the service:**

```bash
sudo gpud up [--token <LEPTON_AI_TOKEN>]
```

> **Note:** The optional `--token` connects `gpud` to the Lepton Platform. You can get a token from the [Settings > Tokens page](https://dashboard.lepton.ai/workspace-redirect/settings/api-tokens) on your dashboard. You can then view your machine's status on the [self-managed nodes dashboard](https://dashboard.lepton.ai/workspace-redirect/machines/self-managed-nodes).

**Stop the service:**

```bash
sudo gpud down
```

**Uninstall:**

```bash
sudo rm /usr/local/bin/gpud
sudo rm /etc/systemd/system/gpud.service
```

#### Without `systemd` (e.g., macOS)

**Run in the foreground:**

```bash
gpud run [--token <LEPTON_AI_TOKEN>]
```

**Run in the background:**

```bash
nohup sudo /usr/local/bin/gpud run [--token <LEPTON_AI_TOKEN>] &>> <your_log_file_path> &
```

**Uninstall:**

```bash
sudo rm /usr/local/bin/gpud
```

---

### Run GPUd with Kubernetes

The recommended way to deploy GPUd on Kubernetes is with our official [Helm chart](./deployments/helm/gpud/README.md).

### Build with Docker

A Dockerfile is provided to build a container image from source. For complete instructions, please see our [Docker guide in CONTRIBUTING.md](CONTRIBUTING.md#building-with-docker).

---

## Key Features

- Monitor critical GPU and GPU fabric metrics (power, temperature).
- Reports  GPU and GPU fabric status (nvidia-smi parser, error checking).
- Detects critical GPU and GPU fabric errors (kmsg, hardware slowdown, NVML Xid event, DCGM).
- Monitor overall system metrics (CPU, memory, disk).

Check out [*components*](./docs/COMPONENTS.md) for a detailed list of components and their features.

## Integration

For users looking to set up a platform to collect and process data from gpud, please refer to [INTEGRATION](./docs/INTEGRATION.md).

## FAQs

### Does GPUd send data to lepton.ai?

GPUd collects a small anonymous usage signal by default to help the engineering team better understand usage frequencies. The data is strictly anonymized and **does not contain any sensitive data**. You can disable this behavior by setting `GPUD_NO_USAGE_STATS=true`. If GPUd is run with systemd (default option for the `gpud up` command), you can add the line `GPUD_NO_USAGE_STATS=true` to the `/etc/default/gpud` environment file and restart the service.

If you opt-in to log in to the Lepton AI platform, to assist you with more helpful GPU health states, GPUd periodically sends system runtime related information about the host to the platform. All these info are system workload and health info, and contain no user data. The data are sent via secure channels.

### How to update GPUd?

GPUd is still in active development, regularly releasing new versions for critical bug fixes and new features. We strongly recommend always being on the latest version of GPUd.

When GPUd is registered with the Lepton platform, the platform will automatically update GPUd to the latest version. To disable such auto-updates, if GPUd is run with `systemd` (default option for the `gpud up` command), you may add the flag `FLAGS="--enable-auto-update=false"` to the `/etc/default/gpud` environment file and restart the service.

## Learn more

- [Why GPUd](./docs/WHY.md)
- [Install GPUd](./docs/INSTALL.md)
- [GPUd components](./docs/COMPONENTS.md)
- [GPUd architecture](./docs/ARCHITECTURE.md)

## Contributing

Please see the [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on how to contribute to this project.
