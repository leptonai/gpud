# Components

## GPU components

- [**`accelerator-nvidia-bad-envs`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/bad-envs): Tracks any bad environment variables that are globally set for the NVIDIA GPUs.
- [**`accelerator-nvidia-clock`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/clock): Monitors NVIDIA GPU clock events of all GPUs, such as HW Slowdown events.
- [**`accelerator-nvidia-clock-speed`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/clock-speed): Tracks the per-GPU clock speed.
- [**`accelerator-nvidia-ecc`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/ecc): Tracks the NVIDIA per-GPU ECC errors and other ECC related information.
- [**`accelerator-nvidia-error`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/error): Tracks NVIDIA GPU errors real-time in the SMI queries -- likely requires host restarts.
- [**`accelerator-nvidia-error-sxid`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/error/sxid): Tracks the NVIDIA GPU SXid errors scanning the dmesg -- see [fabric manager documentation](https://docs.nvidia.com/datacenter/tesla/pdf/fabric-manager-user-guide.pdf).
- [**`accelerator-nvidia-error-xid`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/error/xid): Tracks the NVIDIA GPU Xid errors scanning the dmesg and using the NVIDIA Management Library (NVML) -- see [Xid messages](https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages).
- [**`accelerator-nvidia-fabric-manager`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/fabric-manager): Tracks the fabric manager version and its activeness.
- [**`accelerator-nvidia-gsp-firmware`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/fabric-manager): Tracks the GSP firmware mode.
- [**`accelerator-nvidia-infiniband`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/infiniband): Monitors the infiniband status of the system. Optional, enabled if the host has NVIDIA GPUs.
- [**`accelerator-nvidia-info`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/info): Serves relatively static information about the NVIDIA accelerators (e.g., GPU product names).
- [**`accelerator-nvidia-memory`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/memory): Monitors the NVIDIA per-GPU memory usage.
- [**`accelerator-nvidia-gpm`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/gpm): Monitors the NVIDIA per-GPU GPM metrics.
- [**`accelerator-nvidia-nvlink`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/nvlink): Monitors the NVIDIA per-GPU nvlink devices.
- [**`accelerator-nvidia-peermem`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/peermem): Monitors the peermem module status. Optional, enabled if the host has NVIDIA GPUs.
- [**`accelerator-nvidia-persistence-mode`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/persistence-mode): Tracks the NVIDIA persistence mode.
- [**`accelerator-nvidia-nccl`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/nccl): Monitors the NCCL (NVIDIA Collective Communications Library) status. Optional, enabled if the host has NVIDIA GPUs.
- [**`accelerator-nvidia-power`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/power): Tracks the NVIDIA per-GPU power usage.
- [**`accelerator-nvidia-processes`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/processes): Tracks the NVIDIA per-GPU processes.
- [**`accelerator-nvidia-remapped-rows`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/remapped-rows): Tracks the NVIDIA per-GPU remapped rows (which indicates whether to reset the GPU or not).
- [**`accelerator-nvidia-temperature`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/temperature): Tracks the NVIDIA per-GPU temperatures.
- [**`accelerator-nvidia-utilization`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/utilization): Tracks the NVIDIA per-GPU utilization.

## General Hardware components

- [**`cpu`**](https://pkg.go.dev/github.com/leptonai/gpud/components/cpu): Tracks the combined usage of all CPUs (not per-CPU).
- [**`disk`**](https://pkg.go.dev/github.com/leptonai/gpud/components/disk): Tracks the disk usage of all the mount points specified in the configuration.
- [**`memory`**](https://pkg.go.dev/github.com/leptonai/gpud/components/memory): Tracks the memory usage of the host.
- [**`network-latency`**](https://pkg.go.dev/github.com/leptonai/gpud/components/network/latency): Tracks global network connectivity statistics.
- [**`power-supply`**](https://pkg.go.dev/github.com/leptonai/gpud/components/power-supply): Tracks the power supply/usage on the host.

## System components

- [**`info`**](https://pkg.go.dev/github.com/leptonai/gpud/components/info): Provides static information about the host (e.g., labels, IDs).
- [**`os`**](https://pkg.go.dev/github.com/leptonai/gpud/components/os): Queries the host OS information (e.g., kernel version).
- [**`systemd`**](https://pkg.go.dev/github.com/leptonai/gpud/components/systemd): Tracks the systemd state and unit files.
- [**`dmesg`**](https://pkg.go.dev/github.com/leptonai/gpud/components/dmesg): Scans and watches dmesg outputs for errors,, as specified in the configuration (e.g., regex match NVIDIA GPU errors).
- [**`file-descriptor`**](https://pkg.go.dev/github.com/leptonai/gpud/components/fd): Tracks the number of file descriptors used on the host.

## Misc. components

- [**`containerd-pod`**](https://pkg.go.dev/github.com/leptonai/gpud/components/containerd/pod): Tracks the current pods from the containerd CRI.
- [**`k8s-pod`**](https://pkg.go.dev/github.com/leptonai/gpud/components/k8s/pod): Tracks the current pods from the kubelet read-only port.
- [**`docker-container`**](https://pkg.go.dev/github.com/leptonai/gpud/components/docker/container): Tracks the current containers from the docker runtime.
- [**`tailscale`**](https://pkg.go.dev/github.com/leptonai/gpud/components/tailscale): Tracks the tailscale state (e.g., version) if available.
- [**`file`**](https://pkg.go.dev/github.com/leptonai/gpud/components/file): Returns healthy if and only if all the specified files exist.
- [**`library`**](https://pkg.go.dev/github.com/leptonai/gpud/components/library): Returns healthy if and only if all the specified libraries exist.
