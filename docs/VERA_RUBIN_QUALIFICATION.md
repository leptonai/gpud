# Vera Rubin hardware qualification

This change enables Rubin identification through NVML architecture,
including drivers reporting `NVIDIA Graphics Device`. Both `gpud machine-info`
(text and JSON) and `gpud scan` use that identity without a product-name override.
`machine-info` collects inventory; `scan` runs component health checks. Neither
command installs GPUd, starts its daemon, resets a GPU, or executes repair advice.

## Choose the target and binary

Obtain an accessible test host from its owner. A development host does not
establish the final production topology
or NIC inventory. Record the tested hostname, OS, driver, local GPU count,
host attachment, and NIC inventory with the results. Do not infer a local GPU
count of 72 from the rack size, or rely on cluster UUID as the sole domain ID.

Use a CI snapshot built from the exact PR commit under review. Hardware testing
must not create a release: publish a version only after QA approval under the
project's release process. Download the matching artifact from the PR's
`goreleaser-dev` workflow (replace `RUN_ID` with its run ID):

```bash
gh run download RUN_ID --repo leptonai/gpud \
  --name gpud-linux_arm64_ubuntu24.04 --dir ./vr-build
archive=$(find ./vr-build -name '*.tgz' -print -quit)
test -n "$archive"
mkdir -p ./vr-build/extracted
tar -xzf "$archive" -C ./vr-build/extracted
gpud_binary=$(realpath "$(find ./vr-build/extracted -type f -name gpud -print -quit)")
test -x "$gpud_binary"
```

Use Linux **arm64** for a Vera CPU (`uname -m` = `aarch64`); use **amd64** only
for an x86 host. Select the archive matching the host's Ubuntu/glibc baseline.
Record `gpud --version`, `sha256sum`, and the artifact's source commit. Transfer
the binary and `scripts/qualify-vera-rubin.sh` from that same commit to the
selected host. Run on the host with root permissions for kernel logs and device
access. Keep the installed GPUd binary/service in place; run this binary by its
absolute path. The script requires Bash, GNU `timeout`, `sha256sum`, and
`nvidia-smi`; absent optional `lspci`/`ibstat` tools are recorded in the manifest.

## Collect a baseline

```bash
sudo bash ./scripts/qualify-vera-rubin.sh \
  "$gpud_binary" "/tmp/vr-baseline-$(date -u +%Y%m%dT%H%M%SZ)"
```

The output directory must be new. It contains binary version/hash, OS and driver
information, raw NVIDIA telemetry, machine-info text/JSON, scan output, stderr,
exact commands, and individual exit codes. Each command has a five-minute limit.
No network upload occurs. Do not use `--gpu-product-name` or failure injection
flags for baseline qualification: they simulate hardware and bypass detection.

Once the owner confirms the local GPU count and NIC layout, repeat with explicit
expectations. For example **only for a host confirmed to have four visible GPUs
and eight active 800 Gb/s InfiniBand ports**:

```bash
sudo bash ./scripts/qualify-vera-rubin.sh \
  "$gpud_binary" "/tmp/vr-inventory-$(date -u +%Y%m%dT%H%M%SZ)" \
  --gpu-count 4 \
  --infiniband-expected-port-states '{"at_least_ports":8,"at_least_rate":800}'
```

The explicit InfiniBand inventory takes precedence over product defaults,
including an explicit `{}` to disable that threshold. Generic names have no
implicit NIC inventory. `scan` uses text output; JSON is available through
`machine-info --output-format json`.

## Qualify health policy before enabling remediation

The baseline collects Rubin link, fabric-configuration, and repair telemetry.
`GPUD_ENABLE_RUBIN_HEALTH_RULES` is unset by default: the additional all-links-active
rule, configuration/GFM faults, and repair-flag actions are not enforced. Existing
registration, summary health, query-error, and row-remapping checks still apply.

On the first representative production trays, collect both the default baseline
and an opted-in **one-shot scan** before enabling these rules in an automated
remediation system:

```bash
sudo env GPUD_ENABLE_RUBIN_HEALTH_RULES=true bash ./scripts/qualify-vera-rubin.sh \
  "$gpud_binary" "/tmp/vr-policy-$(date -u +%Y%m%dT%H%M%SZ)" --gpu-count 4
```

Use the actual visible GPU count. Validate active and unwired link indices, counter
queries, idle/no-partition configuration, and pending/unrepairable memory behavior
with the hardware owner. Controlled fault tests require a separate agreed test
plan; this script does not inject faults or execute suggested repairs. Review
both raw fields and proposed actions before setting the opt-in on the daemon.
Removing the opt-in returns those additional rules to telemetry only.

## Review acceptance evidence

| Area | Evidence required |
| --- | --- |
| Identity | Machine-info reports `architecture: rubin`, retains the driver product name with spaces normalized to hyphens, and lists every visible GPU UUID/PCI bus ID and memory capacity. Compare with `nvidia-smi`; do not count the entire rack. |
| Topology | Confirm C2C and actual switch ownership with the operator. A resolved Vera/Rubin C2C tray must not demand a local Fabric Manager. Unknown host attachment must not be silently treated as NVL72 or as local-switch ownership. |
| Fabric | For a connected NVL72 tray, every visible GPU reports Completed/Success and no fabric-health faults. Compare detailed configuration, partition and GFM fields with `nvidia-smi -q`. V4 also reports typed Unicast/Multicast Pointer and Logical Endpoint clique assignments. Compare every type and ID; the deprecated single clique field denotes only the first Unicast Pointer assignment. Older APIs cannot supply the typed list. Unsupported or pending telemetry is not qualification evidence. |
| NVLink | Compare all links exposed by the driver, including upper indices through 35, with `nvidia-smi nvlink -s`. Registered rack links should all be active. Check peer topology when more than one GPU is visible. Rack faults alone must not suggest a host reboot. |
| Memory repair | Review row and bank remapping, channel/TPC repair, and unrepairable-memory fields. `N/A` means unavailable, not a measured zero/false. Unexpected missing APIs or query errors require driver/hardware follow-up before sign-off. With the health-rule opt-in, pending repair suggests reset/reboot and unrepairable memory takes precedence with hardware inspection; otherwise these flags remain telemetry. |
| Xid and other checks | Review Xid, ECC, temperature, power, slowdown, counts, and other component summaries. Xid 63/64 retain existing memory-component ownership. Recovered bank-remapping fault behavior needs a separate controlled hardware test; this procedure does not inject faults. Permission errors reading kernel logs invalidate that portion of the check. |
| Networking | Compare discovered ports, link layer and speed with the actual dev host inventory. Do not assume the reference NVL72 NIC layout merely from Rubin identity. |
| Completeness | Both commands finish without a crash or symbol-resolution failure. Review every command's exit code/stderr and every component's health. `scan complete` and exit 0 mean collection completed, not that every component is healthy. |

A one-shot scan cannot demonstrate persistent registration timeout behavior,
long-running event ingestion, or stability under workload. Record initializing
states and repeat after fabric readiness; qualify persistence separately in a
daemon test agreed with the owner. Test representative production hardware before release. CI and mocked CLI tests do not substitute for
these hardware results. Hardware sign-off remains pending until captured results
have been reviewed.
