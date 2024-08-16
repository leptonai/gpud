# Components

## GPU components

- [**`accelerator-nvidia-clock`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/clock-event): Monitors NVIDIA GPU clock events of all GPUs, such as HW Slowdown events. [nvidia, gpu, clock, event]
- [**`accelerator-nvidia-clock-speed`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/clock-speed): Tracks the per-GPU clock speed. [nvidia, gpu, clock, speed]
- [**`accelerator-nvidia-ecc`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/ecc): Tracks the per-GPU ECC errors. [nvidia, gpu, ecc]
- [**`accelerator-nvidia-error`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/error): Tracks NVIDIA GPU errors real-time in the SMI queries -- likely requires host restarts. [nvidia, gpu, error]
- [**`accelerator-nvidia-error-sxid`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/error/sxid): Tracks the NVIDIA GPU SXid errors scanning the dmesg -- see [fabric manager documentation](https://docs.nvidia.com/datacenter/tesla/pdf/fabric-manager-user-guide.pdf). [nvidia, gpu, error, sxid]
- [**`accelerator-nvidia-error-xid`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/error/xid): Tracks the NVIDIA GPU Xid errors scanning the dmesg and using the NVIDIA Management Library (NVML) -- see [Xid messages](https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages). [nvidia, gpu, error, xid]
- [**`accelerator-nvidia-fabric-manager`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/fabric-manager): Tracks the fabric manager version and its activeness. [nvidia, gpu, fabric-manager]
- [**`accelerator-nvidia-infiniband`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/infiniband): Monitors the infiniband status of the system. Optional, enabled if the host has NVIDIA GPUs. [nvidia, gpu, infiniband, ibstat]
- [**`accelerator-nvidia-info`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/info): Serves relatively static information about the NVIDIA accelerator (e.g., GPU product names). [nvidia, gpu, info]
- [**`accelerator-nvidia-memory`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/memory): Monitors the per-GPU memory usage. [nvidia, gpu, memory]
- [**`accelerator-nvidia-nvlink`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/nvlink): Monitors the per-GPU nvlink devices. [nvidia, gpu, nvlink]
- [**`accelerator-nvidia-peermem`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/peermem): Monitors the peermem module status. Optional, enabled if the host has NVIDIA GPUs. [nvidia, gpu, peermem]
- [**`accelerator-nvidia-power`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/power): Tracks the per-GPU power usage. [nvidia, gpu, power]
- [**`accelerator-nvidia-processes`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/processes): Tracks the per-GPU processes. [nvidia, gpu, processes]
- [**`accelerator-nvidia-temperature`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/temperature): Tracks the per-GPU temperatures. [nvidia, gpu, temperature]
- [**`accelerator-nvidia-utilization`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/utilization): Tracks the per-GPU utilization. [nvidia, gpu, utilization]

## General Hardware components

- [**`cpu`**](https://pkg.go.dev/github.com/leptonai/gpud/components/cpu): Tracks the CPU usage combined all the CPUs (not per-CPU). [cpu]
- [**`disk`**](https://pkg.go.dev/github.com/leptonai/gpud/components/disk): Tracks the disk usage of all the mount points specified in the configuration. [disk]
- [**`memory`**](https://pkg.go.dev/github.com/leptonai/gpud/components/memory): Tracks the memory usage of the host. [memory]
- [**`network-latency`**](https://pkg.go.dev/github.com/leptonai/gpud/components/network/latency): Tracks global network connectivity statistics. [netcheck]
- [**`power-supply`**](https://pkg.go.dev/github.com/leptonai/gpud/components/power-supply): Tracks the power supply/usage on the host. [power]

## System components

- [**`info`**](https://pkg.go.dev/github.com/leptonai/gpud/components/info): Provides static information about the host (e.g., labels, IDs). [info]
- [**`os`**](https://pkg.go.dev/github.com/leptonai/gpud/components/os): Queries the host OS information (e.g., kernel version). [os]
- [**`systemd`**](https://pkg.go.dev/github.com/leptonai/gpud/components/systemd): Tracks the systemd state and unit files. [systemd]
- [**`dmesg`**](https://pkg.go.dev/github.com/leptonai/gpud/components/dmesg): Scans and watches the /var/log/dmesg file for errors, as specified in the configuration (e.g., regex match NVIDIA GPU errors). [dmesg, log, error]
- [**`file-descriptor`**](https://pkg.go.dev/github.com/leptonai/gpud/components/fd): Tracks the number of file descriptors used on the host. [file, descriptors]

## Misc. components

- [**`containerd-pod`**](https://pkg.go.dev/github.com/leptonai/gpud/components/containerd/pod): Tracks the current pods from the containerd CRI. [containerd, pod]
- [**`k8s-pod`**](https://pkg.go.dev/github.com/leptonai/gpud/components/k8s/pod): Tracks the current pods from the kubelet read-only port. [k8s, kubernetes, pod]
- [**`docker-container`**](https://pkg.go.dev/github.com/leptonai/gpud/components/docker/container): Tracks the current containers from the docker runtime. [docker, container]
- [**`tailscale`**](https://pkg.go.dev/github.com/leptonai/gpud/components/tailscale): Tracks the tailscale state (e.g., version) if available. [tailscale]
