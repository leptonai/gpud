# Components

## Fabric registration

On multi-GPU systems advertising NVML fabric-state support, the NVLink and
fabric-manager checks distinguish registration from connectivity. NVIDIA documents
that [NVLink peer access requires fabric registration](https://docs.nvidia.com/datacenter/tesla/fabric-manager-user-guide/)
and that the [registration status code is valid only after completion](https://docs.nvidia.com/deploy/nvml-api/620/api/structnvmlGpuFabricInfo__v3__t.html).

- `Not Started` or `In Progress`, without an explicit fabric fault, reports
  `Degraded` with a registration deadline. This applies even when Fabric Manager
  is still starting or every peer returns `NS`.
- Registration remaining incomplete for five minutes from the first observation
  reports `Unhealthy` with `IGNORE_NO_ACTION_REQUIRED`. The interval is a GPUd
  policy budget, not an NVIDIA hardware guarantee. It measures observed
  registration time rather than host boot time.
- The first observation is checkpointed per component and boot ID in GPUd's
  existing SQLite database. Daemon restarts, alternating pending states, query
  errors, and diagnostic event retention do not restart the interval. Full healthy
  registration or a new boot allows a new interval. In-process elapsed time uses
  the monotonic clock. After restart, the persisted wall-clock age seeds the new
  monotonic deadline; a future checkpoint is clamped to the current time.
- Failed completed registration, unhealthy fabric telemetry, unknown states,
  missing GPU data, and query errors remain `Unhealthy` immediately. GPU lost/reset
  errors and NVLink kernel faults retain their existing checks and actions.
- Once registration completes, peer and port checks apply normally, including
  all-pairs-`NS` failures and configured port thresholds.

Pending registration, an expired deadline, or an explicit fabric fault carries
`IGNORE_NO_ACTION_REQUIRED` unless an independent GPU lost/reset error requires
recovery. This action suppresses automatic host repair, not the health failure.
The unhealthy result directs operators to inspect fabric management, switch
connectivity, and driver compatibility. Inactive ports and failed P2P probes
remain in the diagnostics, but cannot override this policy while the fabric is
unavailable. On GB200/GB300 external-fabric candidates, link/P2P failures alone
also do not request host reboots after registration completes. The product name
does not establish that those failures are host-local.

When local Fabric Manager is applicable, GPUd checks its executable and activeness
even during pending registration. A missing executable is unhealthy immediately;
an inactive daemon is reported alongside the registration state and deadline.
An active daemon cannot make incomplete registration healthy. These checks do
not identify a specific version mismatch; inspect service startup errors and
driver/FM versions when the daemon cannot start. Rack-managed fabrics do not
require a local Fabric Manager daemon.

GPUd does not issue a speculative first reboot or count recovery attempts from
boot-ID changes. A consumer that adds automatic recovery must track actual
attempts per fault episode and stop or escalate when recovery fails; a cooldown
alone is not an attempt budget. An unavailable fabric does not establish a
defective compute tray, so escalation must permit fabric/software investigation.

Unavailable telemetry alone does not diagnose a hardware defect. Consumers
should use readiness to control workload
admission and health/action signals to decide repairs; `Degraded` is not proof that
CUDA or peer access is ready. This also avoids consumers treating `Initializing`
as an instruction to retain a previous unhealthy condition.

One-shot scans without a database can identify incomplete registration but cannot
establish how long it has persisted. Restart persistence also requires the daemon's
database to survive restarts; `--db-in-memory` cannot provide it. Systems without
fabric-state support keep their existing NVLink and Fabric Manager behavior.
NVLink falls through to peer and port checks when all GPUs explicitly report
unsupported fabric telemetry, no switch is detected, and the product does not
identify NVL72. Mixed support, faults, explicit NVL72 names, and detected switches
remain checked.

For Vera Rubin, GPUd uses the NVML architecture even when the driver reports
`NVIDIA Graphics Device` (displayed as `NVIDIA-Graphics-Device` after existing
product-name normalization). Enabled C2C marks an external-fabric candidate,
using the same candidate policy as GB200/GB300. It does not establish a rack
layout. Disabled or unavailable C2C leaves the host topology unknown; GPUd does
not infer PCIe, NVL8, or local Fabric Manager ownership from it. Rubin fabric
state is probed even on single-GPU partitions. Rack size is never an expected
local GPU count.

Rubin NVLink collection checks up to 36 indices and preserves unexpected query
errors and partial results. Unsupported counters remain optional. Non-Rubin
families retain the previous 18-index range and query-error tolerance. Link or
fabric-query failures on Rubin include fabric investigation guidance without
suggesting a compute-host reboot; independent GPU-loss/reset errors retain their
recovery advice. The additional all-links-active rule on registered C2C Rubin
fabric is opt-in.

Rubin uses fabric-info V4 when available, preserving every `(type, id)` clique
assignment in `cliques` in machine-info JSON and per-GPU scan telemetry, with
readable names in text. The legacy `cliqueID` remains the first Unicast Pointer
assignment; it is not a Logical Endpoint ID. It is omitted if V4 provides no
Unicast Pointer assignment. Common machine identifiers require matching,
successfully registered data from every GPU. Legacy APIs retain their single
field without inventing typed assignments.

V4 falls back to V3 and V1 for API compatibility errors. Rubin preserves actual
query failures instead of masking them with older APIs. Other families retain
their previous V3-to-V1 fallback. Configuration and disconnected-GFM health bits
are decoded for all families but only add new faults on opted-in Rubin
installations. IMEX process telemetry is informational and collected only for Rubin external-fabric candidates.

Rubin memory checks include bank remapping, channel/TPC repair, and unrepairable
memory when the driver supports those APIs. Version mismatches report unavailable
telemetry. Query errors are unhealthy on Rubin; other families retain their
existing row-remapping health policy. Pending bank/channel/TPC repair and
unrepairable-memory flags are telemetry only by default. When opted in,
unrepairable memory takes precedence over reset advice. Xid 63/64 retain existing
memory-component ownership; recovered
bank-remapping fault behavior still requires hardware qualification before any
new Xid-only escalation is introduced.

The additional Rubin rules require `GPUD_ENABLE_RUBIN_HEALTH_RULES=true`:
all-links-active enforcement, configuration/GFM fault bits, and repair-flag-driven
reboot or hardware-inspection suggestions. Leave this unset for bring-up; raw
telemetry remains visible. Existing registration, summary health, query failures,
row-remapping rules, and independent GPU-loss/reset handling still apply. Scan
representative production trays and qualify recovery behavior before enabling the
new rules in a daemon connected to automated remediation. A healthy development
snapshot does not qualify idle partition behavior or fault recovery.

C2C is queried only on Rubin. Two diagnostic changes apply across families:
a failed fabric query omits common machine fabric identifiers, while chassis
serial still uses the first readable GPU; fabric-manager also includes the query
error in `HealthState.Error` without changing the existing health/action policy.

The NVML dependency update corrects the GPM support query version. To avoid
silently enabling previously inactive sampling paths, GPM sampling now requires
`GPUD_ENABLE_GPM=true`. Qualify sampling on each GPU family before opting in.
This explicit opt-in is a behavior change for any installation already sampling GPM.
Disabled sampling is reported as `GPM sampling disabled`, separately from hardware
that reports GPM unsupported.

Rubin identity does not establish the installed InfiniBand NIC inventory. No
Rubin-specific port-count or rate default is selected. Configure
`--infiniband-expected-port-states` from the actual inventory. See
[Vera Rubin hardware qualification](VERA_RUBIN_QUALIFICATION.md) for the snapshot
`machine-info`/`scan` procedure. Older-family hardware regression testing and
representative production hardware qualification remain release requirements.

## Available components

- [**`accelerator-nvidia-bad-envs`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/bad-envs): Tracks any bad environment variables that are globally set for the NVIDIA GPUs.
- [**`accelerator-nvidia-clock-speed`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/clock-speed): Tracks the per-GPU clock speed.
- [**`accelerator-nvidia-ecc`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/ecc): Tracks the NVIDIA per-GPU ECC errors and other ECC related information.
- [**`accelerator-nvidia-error-sxid`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/sxid): Tracks the NVIDIA GPU SXid errors scanning the kmsg -- see [fabric manager documentation](https://docs.nvidia.com/datacenter/tesla/pdf/fabric-manager-user-guide.pdf).
- [**`accelerator-nvidia-error-xid`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/xid): Tracks the NVIDIA GPU Xid errors scanning the kmsg and using the NVIDIA Management Library (NVML) -- see [Xid messages](https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages).
- [**`accelerator-nvidia-fabric-manager`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/fabric-manager): Tracks fabric registration, health, and fabric manager activeness. See [fabric registration](#fabric-registration).
- [**`accelerator-nvidia-gpm`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/gpm): Monitors the NVIDIA per-GPU GPM metrics.
- [**`accelerator-nvidia-hw-slowdown`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/hw-slowdown): Monitors NVIDIA GPU hardware slowdown clock events of all GPUs.
- [**`accelerator-nvidia-infiniband`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/infiniband): Monitors the infiniband status of the system and Mellanox kernel events. Optional, enabled if the host has NVIDIA GPUs.
- [**`accelerator-nvidia-memory`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/memory): Monitors the NVIDIA per-GPU memory usage.
- [**`accelerator-nvidia-nccl`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/nccl): Monitors the NCCL (NVIDIA Collective Communications Library) status. Optional, enabled if the host has NVIDIA GPUs.
- [**`accelerator-nvidia-nvlink`**](https://pkg.go.dev/github.com/leptonai/gpud/components/accelerator/nvidia/nvlink): Monitors NVIDIA NVLink ports and peer connectivity after [fabric registration](#fabric-registration).
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
| `0x2` Node Reboot Required (R610 OS Reboot) | Unhealthy | Reboot the node before restarting applications. A warm reboot is sufficient. |
| `0x3` Drain P2P | Degraded | Stop new scheduling, quiesce P2P traffic and UVM persistence, then re-query recovery guidance. Reset only if Drain P2P remains. No blind reboot. |
| `0x4` Drain and Reset | Healthy for this recovery report | Keep the drain/checkpoint guidance as evidence. The remapped-rows component owns pending-remap reboot and remapping-failure decisions. |
| `0x5` Recover IMEX Domain | Degraded | Operator investigation and coordinated recovery of the multi-node IMEX domain. No inferred local reboot or hardware failure. |
| `0x6` Bus Reset | Unhealthy | Reboot the node before restarting GPU applications. |
| `0x7` System Reboot | Unhealthy | Reboot the node before restarting GPU applications. |
| Unknown or malformed | Degraded | Investigate the originating XIDs and driver guidance. No guessed reboot. |
| Legacy event without transition data | Preserve stored severity | Keep the original description and actions. Older native Warning/no-action events remain informational. |

The latest recognized XID 154 governs only its GPU's recovery guidance. A later
None report retires that GPU's recovery recommendation. Other GPUs and originating
faults remain independent. Unknown or malformed reports cannot replace a known
request; their evidence remains in the Events API. A reboot clears all active
recovery guidance because the driver starts with a fresh recovery state.

The event store's timestamp column has one-second precision. XID 154 also stores
the full producer timestamp and uses it when ordering recovery transitions.
For tied timestamps, including older records, a report whose numeric `From`
matches the current `To` advances the transition chain. This lets a matching None
retire a request within the same second. Missing timestamps or unmatched ties
retain the stronger request; a later None or reboot clears it.

While the native kmsg stream is running, GPUd drops NVSentinel's XID 154 copies
before storage or metric counting. NVSentinel stamps journal batches with
processing time, so replaying them could revive recovery requests already cleared
by the native stream. NVSentinel supplies XID 154 when no native stream is running,
including after the native channel closes. Its timestamp orders its own reports;
it does not establish kernel order across producers.

XID 154 bypasses NVSentinel's time-window deduplication. Native collection and scan
deduplicate these reports by exact kernel timestamp, sequence number, and text,
so request → None → request survives even within one second. Identical reports
do not escalate health or count toward hardware inspection. Other XIDs retain
their existing deduplication window, with PCI IDs normalized to GPU UUIDs when
NVML provides the mapping.

One recovery report is sufficient to recommend an action. XID 154 never escalates
to hardware inspection, including after repeated reboots or with a threshold
override. It reports recovery guidance and cannot diagnose hardware failure.
Originating XIDs retain their existing reboot thresholds and precedence.

The reboot executor must enforce a per-node reboot budget and stop automatic
retries for operator investigation when the same request recurs after recovery.
A cooldown limits the rate but does not permanently break a deterministic
driver/kernel reboot loop. GPUd keeps reporting the driver's request; this
component does not enforce an executor budget or diagnose repeated requests as
hardware failures.

When combining originating faults with recovery guidance, GPUd selects health and
actions together: higher health severity wins, then repair priority breaks a tie.
Both reasons remain visible. Producers persist the classification; reading events
fills missing details without replacing stored severity, descriptions, or actions.

Sources: [NVIDIA XID catalog](https://docs.nvidia.com/deploy/xid-errors/analyzing-xid-catalog.html)
and [NVIDIA GPU Recovery Action guidance](https://docs.nvidia.com/deploy/nvidia-smi/index.html#gpu-recovery-action).
The R610 mappings also follow [NVSentinel's XID 154 parser](https://github.com/NVIDIA/NVSentinel/blob/1b15313777c71bd06657e4f6350f2e16756fb4db/health-monitors/syslog-health-monitor/pkg/xid/parser/csv.go).


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
