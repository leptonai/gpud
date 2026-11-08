# NVL72 fault-isolation evidence

GPUd reports observations from GB200/GB300 compute trays. These observations
identify affected GPUs and connectivity domains; they do not establish which
switch, cable, power shelf or tray caused a shared failure. A physical
replacement decision still requires the platform's wiring inventory and
corroborating diagnostics.

## Reported evidence

- **GB300 fabric health.** `extra_info.fabric_states` contains an entry for
  every visible GPU, including failed queries. Incomplete registration,
  unsuccessful status and off-nominal health flags fail health. External fabrics
  do not require a locally visible NVSwitch. Code:
  `pkg/nvidia/product/capabilities.go`,
  `components/accelerator/nvidia/fabric-manager/`.
- **Physical position.** Each GPU has raw platform coordinates, platform API
  version and independent `deviceModuleId`. Zero and NVML sentinel values are
  preserved. Query failures are explicit. Conflicting chassis serials do not
  select an arbitrary rack. Code: `pkg/machine-info/platform.go`,
  `api/v1/platform.go`.
- **IMEX.** `extra_info.imex` includes local daemon status, peers and a directed
  connectivity matrix. It is read-only. Code:
  `components/accelerator/nvidia/imex/`.
- **Evaluation coverage.** Omitted `evaluated` means a legacy producer; `false`
  means the check did not fully evaluate. It must not be interpreted as a passed
  check. Known failures remain Unhealthy even with incomplete coverage. Code:
  `api/v1/evaluation.go`, NVIDIA component checks.
- **Row remapping.** GB300 supports evaluation; pending remaps suggest reset and
  failed remaps suggest inspection. Query failures cannot be overwritten by
  Healthy. Code: `components/accelerator/nvidia/remapped-rows/`.
- **InfiniBand.** The GB300 reference profile is four ports at 800 Gb/s.
  Defaults require explicit InfiniBand intent; explicit profiles win. Code:
  `components/accelerator/nvidia/infiniband/threshold_default.go`.
- **Fabric identity.** The existing all-readable-GPUs-agree rule for cluster
  UUID and clique ID remains in place. Code: `pkg/machine-info/machine_info.go`.
- **NVLink topology.** Each link includes remote PCI address/type, field
  availability and raw NVLink 5 counters with scope and sample time. Code:
  `components/accelerator/nvidia/nvlink/topology.go`.
- **GPU attribution.** Per-GPU failures name the GPU in the reason and provide
  sorted, unique `extra_info.gpu_uuids`. These identify observers/affected GPUs,
  not proven physical culprits. Code: `api/v1/gpu_evidence.go`, NVIDIA component
  checks.

`not_evaluated_reason` includes `not_supported_on_product`,
`no_expected_port_states`, `no_expected_link_states`, `no_expected_values`,
`no_data_yet`, `collection_failed` and `partial_data`. Detailed query errors
remain in `error` or the per-observation error map. An unavailable NVLink
counter is omitted from the numeric JSON fields and from its legacy Prometheus
series, rather than emitted as a measured zero. NVLink 5 counters keep their own
names; they are not interchangeable with legacy replay/CRC counters. Consumers
computing deltas must handle driver resets and counter decreases.

`fault_scope=nvlink_domain` and `fault_scope=imex_domain` describe shared
connectivity observations. Their failure suggestion is `CHECK_SHARED_FABRIC`,
not an instruction to reboot every observing tray. Consumers must understand
this action and suppress legacy NVLink hardware-inspection fallbacks before
enabling automated repair.

## IMEX configuration

The component is registered with the normal NVIDIA components. It is required by
default on GB200/GB300. Persistent IMEX deployments must enable the NVIDIA
command service and make its configuration and binary accessible to GPUd. The
checker uses the documented `-N` snapshot text format, validates the complete
matrix, limits output to 2 MiB, bounds the check to 20 seconds, and polls every
120–149 seconds. `LastHealthStates` serves the cached result. Missing
command-service access is incomplete evidence; a failed command alone does not
establish that the daemon is stopped.

Configuration lives under `nvidia_tool_overwrites.imex` in the GPUd config:

```json
{
  "required": true,
  "command": ["nvidia-imex-ctl"],
  "config_file": "/etc/nvidia-imex/config.cfg",
  "service_command": ["systemctl", "is-active", "nvidia-imex"],
  "expected_addresses": ["192.0.2.1", "192.0.2.2"]
}
```

`expected_addresses` is optional inventory validation and includes the local
node. The NVIDIA configuration determines the queried domain. Command arrays are
argv prefixes, not shell scripts. Container deployments can supply an
appropriate `nsenter` prefix for both commands and must provide the necessary
host namespace access. This change does not grant privileges or change host
services.

For job-scoped IMEX, set `required: false` when the service is intentionally
absent and retain scheduler-owned job health validation. This configuration is
read at GPUd startup; it is not a job lifecycle controller. Confirm the
deployment model before rollout. Unsupported output versions remain explicitly
unevaluated; do not silently accept a different format or use daemon readiness
as a substitute for peer connectivity.

## InfiniBand intent

The manager's `accelerator-nvidia-infiniband` configuration accepts `enabled`
alongside `at_least_ports` and `at_least_rate`:

- `enabled: false` disables validation, including on Ethernet platforms.
- `enabled: true` with both thresholds zero selects the known product profile.
- Positive explicit thresholds remain authoritative. Partial profiles do not
  silently acquire different values.
- An absent `enabled` preserves legacy zero-threshold behavior.

The reference GB300 NVL72 compute tray uses four ConnectX-8 interfaces with 800
Gb/s capability. Confirm the actual InfiniBand topology and configured link rate
for the deployed platform; OEM and partitioned configurations should supply
explicit thresholds. GPU count expectations remain an independent inventory
check and must be configured to detect missing GPUs.

## Rollout and qualification

Deploy compatible consumer schemas and repair handling before upgrading GPUd.
This branch is stacked on the health-validation integration; it reuses that
branch's InfiniBand collection path while retaining the existing NVML device
abstraction for raw GPU evidence. The new NVML and IMEX evidence does not depend
on the experimental collection flag.

Before declaring a production rack qualified, record the driver, firmware, IMEX
version, IMEX deployment mode, domain membership and networking profile, then
verify:

1. Every tray reports all four expected GPUs. Compare each platform object with
   `nvidia-smi -q`; verify module identifiers independently and check
   zero/sentinel coordinates against platform inventory.
2. Healthy fabric reports complete/successful registration and nominal supported
   flags. A controlled fabric failure names affected GPUs; one failed query
   retains the other GPUs' entries and marks coverage incomplete.
3. During an approved maintenance window, stop IMEX on one tray. Other trays
   must name that peer; command-service isolation, timeout, malformed output and
   missing rows must never look like a passed check. Restore IMEX and verify
   recovery. Measure snapshot cost on the full domain.
4. Validate four 800 Gb/s InfiniBand ports on the target profile, explicit
   overrides, and Ethernet/disabled behavior. Verify row-remapping collection
   without inducing a destructive memory fault.
5. Verify remote NVLink endpoint data against the platform wiring map. Confirm
   unavailable legacy counters are omitted on Blackwell, and supported field
   counters have the correct scope, sample times and reset behavior.
6. Exercise the consumer path through storage and APIs. Shared faults must not
   start independent tray repair, and incomplete required evidence must not
   clear a drain or restore Production.

Unit tests and API fixtures validate software behavior. They do not replace
these hardware and deployment qualification steps.

## NVIDIA references

- [NVML change log](https://docs.nvidia.com/deploy/nvml-api/change-log.html):
  driver/API availability and Blackwell NVLink counter changes.
- [NVML platform
  information](https://docs.nvidia.com/deploy/nvml-api/latest/api/structnvmlPlatformInfo__v2__t.html):
  raw platform fields.
- [NVML field
  identifiers](https://docs.nvidia.com/deploy/nvml-api/latest/api/group__nvmlFieldValueEnums.html):
  link-scoped counter meanings.
- [IMEX command
  service](https://docs.nvidia.com/multi-node-nvlink-systems/imex-guide/cmdservice.html):
  snapshot format and connectivity statuses.
- [IMEX deployment
  models](https://docs.nvidia.com/multi-node-nvlink-systems/imex-guide/deployment.html):
  persistent and job-scoped operation.
- [NVIDIA cloud
  architecture](https://docs.nvidia.com/dsx/ncp/software-reference-guide/data-center-architecture):
  reference GB300 networking topology.
