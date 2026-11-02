# Components

- [**`accelerator-nvidia-bad-envs`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/bad-envs): Tracks any bad environment variables that are globally set for the NVIDIA GPUs.
- [**`accelerator-nvidia-clock-speed`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/clock-speed): Tracks the per-GPU clock speed.
- [**`accelerator-nvidia-ecc`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/ecc): Tracks the NVIDIA per-GPU ECC errors and other ECC related information.
- [**`accelerator-nvidia-error-sxid`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/sxid): Tracks the NVIDIA GPU SXid errors scanning the kmsg -- see [fabric manager documentation](https://docs.nvidia.com/datacenter/tesla/pdf/fabric-manager-user-guide.pdf).
- [**`accelerator-nvidia-error-xid`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/xid): Tracks the NVIDIA GPU Xid errors scanning the kmsg and using the NVIDIA Management Library (NVML) -- see [Xid messages](https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages).
- [**`accelerator-nvidia-fabric-manager`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/fabric-manager): Tracks the fabric manager version and its activeness.
- [**`accelerator-nvidia-gpm`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/gpm): Monitors the NVIDIA per-GPU GPM metrics.
- [**`accelerator-nvidia-hw-slowdown`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/hw-slowdown): Monitors NVIDIA GPU hardware slowdown clock events of all GPUs.
- [**`accelerator-nvidia-infiniband`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/infiniband): Monitors the infiniband status of the system and Mellanox kernel events. Optional, enabled if the host has NVIDIA GPUs.
- [**`accelerator-nvidia-memory`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/memory): Monitors the NVIDIA per-GPU memory usage.
- [**`accelerator-nvidia-nccl`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/nccl): Monitors the NCCL (NVIDIA Collective Communications Library) status. Optional, enabled if the host has NVIDIA GPUs.
- [**`accelerator-nvidia-nvlink`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/nvlink): Monitors the NVIDIA per-GPU nvlink devices.
- [**`accelerator-nvidia-peermem`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/peermem): Monitors the peermem module status. Optional, enabled if the host has NVIDIA GPUs.
- [**`accelerator-nvidia-persistence-mode`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/persistence-mode): Tracks the NVIDIA persistence mode.
- [**`accelerator-nvidia-power`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/power): Tracks the NVIDIA per-GPU power usage.
- [**`accelerator-nvidia-processes`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/processes): Tracks the NVIDIA per-GPU processes.
- [**`accelerator-nvidia-remapped-rows`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/remapped-rows): Tracks the NVIDIA per-GPU remapped rows (which indicates whether to reset the GPU or not).
- [**`accelerator-nvidia-temperature`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/temperature): Tracks the NVIDIA per-GPU temperatures.
- [**`accelerator-nvidia-utilization`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/utilization): Tracks the NVIDIA per-GPU utilization.
- [**`containerd`**](https://pkg.go.dev/github.com/leptonai/gpud/components/containerd): Tracks the current containerd status.
- [**`cpu`**](https://pkg.go.dev/github.com/leptonai/gpud/components/cpu): Tracks the combined usage of all CPUs (not per-CPU).
- [**`disk`**](https://pkg.go.dev/github.com/leptonai/gpud/components/disk): Tracks the disk usage of all the mount points specified in the configuration.
- [**`docker`**](https://pkg.go.dev/github.com/leptonai/gpud/components/docker): Tracks the current containers from the docker runtime.
- [**`fuse`**](https://pkg.go.dev/github.com/leptonai/gpud/components/fuse): Tracks the FUSE connections.
- [**`kernel-module`**](https://pkg.go.dev/github.com/leptonai/gpud/components/kernel-module): Monitors the FUSE (Filesystem in Userspace).
- [**`library`**](https://pkg.go.dev/github.com/leptonai/gpud/components/library): Checks system libraries such as "libnvidia-ml.so" and "libcuda.so", if applicable.
- [**`memory`**](https://pkg.go.dev/github.com/leptonai/gpud/components/memory): Tracks the memory usage of the host.
- [**`network-latency`**](https://pkg.go.dev/github.com/leptonai/gpud/components/network/latency): Tracks global network connectivity statistics.
- [**`nfs`**](https://pkg.go.dev/github.com/leptonai/gpud/components/nfs): Tracks the NFS volume healthiness.
- [**`os`**](https://pkg.go.dev/github.com/leptonai/gpud/components/os): Queries the host OS information (e.g., kernel version, file descriptor usage) and tracks persistent D-state (uninterruptible sleep) processes.
- [**`pci`**](https://pkg.go.dev/github.com/leptonai/gpud/components/pci): Tracks the PCI devices and their Access Control Services (ACS) status.
- [**`tailscale`**](https://pkg.go.dev/github.com/leptonai/gpud/components/tailscale): Tracks the tailscale state (e.g., version) if available.

## XID 154 recovery guidance

XID 154 supplements the originating fault. GPUd preserves the numeric recovery-action
transition and the driver-printed names in scan output and persisted event payloads.
Policy uses the numeric destination, not words such as `Reboot` in a printed name.
The XID component reports suggested actions; this change adds no automatic repairs.

| Destination | GPUd health | Suggested action and preconditions |
| --- | --- | --- |
| `0x0` None | Healthy for this recovery report | No new repair. Retain independently actionable originating faults. |
| `0x1` GPU Reset Required (R610 PF FLR) | Unhealthy | Terminate GPU processes before reset. GPUd uses its existing system-reboot recommendation for GPU-reset conditions. |
| `0x2` Node Reboot Required | Unhealthy | Reboot the node before restarting applications. A warm reboot is sufficient. |
| `0x3` Drain P2P | Degraded | Stop new scheduling, quiesce P2P traffic and UVM persistence, then re-query recovery guidance. Reset only if Drain P2P remains. No blind reboot. |
| `0x4` Drain and Reset | Unhealthy | Stop new work; let unaffected work finish or checkpoint before reset. GPUd recommends a reboot under its GPU-reset convention after draining. |
| Unknown or malformed | Degraded | Investigate the originating XIDs and driver guidance. No guessed reboot. |

The latest XID 154 governs only its GPU's recovery guidance. A later None report
retires that GPU's recovery recommendation, not another GPU's report or an
originating fault. Hardware-inspection recommendations remain stronger than a
reboot recommendation, even when both reports are fatal.

One recovery report is sufficient to recommend an action. This is separate from
the default threshold of two failed reboot cycles before recommending hardware
inspection. `--xid-thresholds` can override XID 154's reboot threshold.
Only reboots after an active reboot recommendation count as failed recovery cycles.

The NVIDIA catalog's `XID_154` bucket means follow the reported recovery guidance.
`XID_154_EVAL` means follow XID 154 when present; otherwise restart the affected app.
`IGNORE` is non-paging guidance, not permission to discard diagnostic evidence or
associated investigation guidance.

Sources: [NVIDIA XID catalog](https://docs.nvidia.com/deploy/xid-errors/analyzing-xid-catalog.html)
and [NVIDIA GPU Recovery Action guidance](https://docs.nvidia.com/deploy/nvidia-smi/index.html).


## NFS health policy

GPUd reports unresolved NFS server warnings, lock-reclaim failures, and repeated
writeback stack hints as **Degraded**, without suggested repair actions. These
messages do not prove a persistent client hang or a local hardware fault.
Rebooting a client does not fix an NFS server or network outage.

A later `OK` message clears warnings for that server. Messages from earlier boots
do not affect the current check. While qualifying messages remain, GPUd skips
filesystem probes because NFS I/O may block. Repeated failures after reboot do
not trigger a reboot or hardware-inspection recommendation.


## Containerd dangling pods

By default, GPUd counts a `SANDBOX_READY` containerd sandbox as dangling on the
first successful Kubernetes API pod listing that does not contain its pod.
The check runs once per minute. Counts of 6–10 are Degraded; 11 or more are
Unhealthy with a reboot recommendation.

To delay counting a sandbox until its pod has been absent for a grace period:

```bash
gpud run --containerd-dangling-pod-grace-period=10m
```

The default is `0` (no grace period); negative durations are rejected. This
replaces the mandatory 10-minute delay in v0.13.0. A positive duration starts
at the first observed absence for each sandbox, resets when its pod reappears
or the sandbox disappears or leaves READY, and is lost when GPUd restarts.
Failed API queries report detection as unavailable and do not count as evidence
of pod absence. With zero grace, a force-deleted pod whose sandbox is still
tearing down can temporarily count as dangling.
