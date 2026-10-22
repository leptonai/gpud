# Integration with GPUd

To interact with GPUd API endpoints:

```bash
# healthiness of the GPUd process itself
curl -kL https://localhost:15132/healthz

# basic machine information
curl -kL https://localhost:15132/machine-info | jq | less

# list of health check states
curl -kL https://localhost:15132/v1/states | jq | less

# list of systemd events per GPUd component
# (e.g., xid)
curl -kL https://localhost:15132/v1/events | jq | less

# list of system metrics per GPUd component
# (e.g., GPU temperature)
curl -kL https://localhost:15132/v1/metrics | jq | less
```

Following defines the response types for the GPUd APIs above:

- [API types in Go struct](https://github.com/leptonai/gpud/blob/main/api/v1/types.go)
- [OpenAPI spec in JSON](https://github.com/leptonai/gpud/blob/main/docs/apis/swagger.json)
- [OpenAPI spec in YAML](https://github.com/leptonai/gpud/blob/main/docs/apis/swagger.yaml)

Or use the [`client/v1`](http://pkg.go.dev/github.com/leptonai/gpud/client/v1)
library to interact with GPUd in Go.

## CPU topology

`gpud machine-info` and `/machine-info` report CPU topology in `cpuInfo`.
Login and gossip use the same collector. On Linux, GPUd reads the online CPU
set and core sibling groups from sysfs; it does not infer SMT from architecture.

- `logicalCores` keeps its existing logical CPU count semantics.
- `cpusPerCore` is the uniform number of online hardware threads per physical core.
- `numCores` is the total number of physical cores represented by online CPUs
  across all sockets, not the count per socket.
- `numSockets` counts sockets represented by online CPUs.

Unavailable topology fields are omitted. If online thread counts differ between
cores, `cpusPerCore` is omitted, but the measured core and socket counts remain.
If the kernel cannot identify CPU packages, `numSockets` is omitted, but the
measured core counts remain. Offline CPUs do not contribute to these topology
fields. Non-Linux hosts omit the new fields. Consumers must treat an absent
value as unknown, not as one.

For JSON output, use `gpud machine-info --output-format json`; the fields are
under `machine_info.cpuInfo`. Plain output includes measured thread, core, and
socket counts. Logical CPU and topology reads are separate snapshots and can
differ during CPU hotplug or when procfs and sysfs expose different CPU sets.

## Firmware versions

`gpud machine-info`, `gpud scan` and `/machine-info` report firmware and host
software versions in `firmware`, one entry per component and version:

```json
{
  "component": "GPU",
  "version": "97.10.4A.00.0B",
  "instances": 4,
  "instanceNames": ["gpu1", "gpu2", "gpu3", "gpu4"],
  "lastUpdated": "2026-10-21T08:00:00Z"
}
```

| Component | Source | Instance names |
|-----------|--------|----------------|
| `GPU` | NVML VBIOS version | `gpu1`..`gpuN`, ranked by PCI bus ID |
| `cx8` | ConnectX-8 (PCI device `0x1023`) `fw_ver` in sysfs | InfiniBand device name of each card |
| `cx8-aws` | A `cx8` card whose PSID (`board_id`) starts with `AS` | Same as `cx8` |
| `bf3_nic` | BlueField-3 (PCI device `0xa2dc`) `fw_ver` in sysfs | Same as `cx8` |
| `hmc-sbios` | SMBIOS BIOS firmware inventory entry, else the BIOS version | None |
| `doca_host` | First installed of `doca-ofed`, `doca-all`, `doca-networking`, `doca-roce`, `doca-ofed-userspace` in the dpkg database | None |

- Instances of one component that run the same version share one entry, and
  `instances` counts them. Mixed versions produce one entry per version.
- A NIC card counts once: SR-IOV VFs are skipped and PFs that share a
  `sys_image_guid` or a PCI slot are grouped. The card is named after its PF
  with the lowest PCI address, and is `cx8-aws` when any of its PFs has an
  AWS PSID. Other NICs such as ConnectX-7 and EFA are not reported.
- `hmc-sbios` is reported only on bare-metal arm64 hosts whose SMBIOS processor
  manufacturer is NVIDIA (Grace). Reading the SMBIOS table requires root.
- Versions are reported as read, without normalization. Entries are sorted by
  component and then version; instance names are in natural order.
- `lastUpdated` is the UTC time of the read that produced the entry. The daemon
  reads at startup and every 4 hours, and waits up to 30 seconds for the first
  read before it starts the control plane session. Gossip and `/machine-info`
  return the cached result; a login request sent before the daemon starts
  carries no entries.

Sources are read from sysfs, the SMBIOS table and the dpkg database without
running external commands. A source that cannot be read is skipped without
affecting the others. Each read replaces the previous result, so the entries of
a source or GPU that fails during a read stay absent until the next successful
read, up to 4 hours later. Non-Linux hosts report only GPU entries. Consumers
must treat an absent entry as unknown. For JSON output, use
`gpud machine-info --output-format json`; the entries are under
`machine_info.firmware`.

## Public addresses and provider detection

The provider section of `gpud machine-info` and login network data report public
addresses assigned to the machine, not a shared NAT or proxy egress address.
Provider metadata takes precedence. If public-address metadata is unavailable,
unsupported or empty, an external echo is retained only when the address is
actually assigned to a local interface. Successful provider detection alone
does not prove that public-address metadata was read.

Private, loopback, link-local and RFC 6598 shared addresses are not public
assignments. If local assignments cannot be enumerated, the fallback stays empty
rather than guessing. A dedicated 1:1 public mapping that is not present on a
local interface still needs authoritative provider metadata; an echo alone
cannot distinguish that mapping from shared NAT.

Egress discovery remains available for ASN-based provider identification even
when no public address is reported. Consumers must treat an empty public address
as absent or unknown, not as a reason to use the NAT address for inbound access.

Login and the CLI also retain the egress address for internal private-address
exclusion when no public address is reported. A matching default-route address
is not reported as private. This comparison does not publish the echo as a
public address and does not discard non-RFC1918 internal addresses that differ
from the echo.

## Lepton-managed diagnostics

When GPUd is connected to gpud-manager through the session stream, gpud-manager
can send a `method: "diagnostic"` request. GPUd only accepts fixed diagnostic
types. The current Lepton-managed type is:

```text
nvidia_bug_report -> /usr/bin/nvidia-bug-report.sh
```

The session request carries `report_id`, `type`, and `timeout_seconds`; it does
not carry user-provided commands, arguments, scripts, or environment variables.

GPUd runs the fixed diagnostic asynchronously after accepting the request. The
default execution timeout is 10 minutes. If the script exceeds the timeout,
GPUd cancels the process, treats the diagnostic as failed, and notifies
gpud-manager through:

```http
POST /api/v1/diagnostics/{report_id}/failure
```

GPUd also uses the failure endpoint when the script produces no artifact, gzip
conversion fails, or report upload fails after retries. If the command exits
nonzero but still produces a report, GPUd uploads the report so operators can
inspect it. Report uploads and failure notifications use the machine session
token, the machine id headers, and the control-plane origin header. GPUd must
not log report contents, captured script output, storage URLs, or tokens.

## Lepton-managed KAP credentials

The KAP credential session commands manage validated credential files and client
certificate reloads, not the agent service lifecycle. The package controller and
the `kap-mtls-agent` package's `init.sh` own installation, startup and recovery.

- Initial credentials can be staged while the agent is inactive. The update
  returns without waiting for startup or holding the package lock across it.
- Ordinary renewals preserve the existing CA and gateway configuration, select a
  complete immutable credential generation atomically, and signal a running
  agent with SIGHUP. Success means files were selected and notification was
  delivered, not proof that the agent finished loading the certificate.
- A durable pending-notification marker is written before selecting credentials
  and cleared only after successful SIGHUP delivery. It survives an inactive
  agent, failed notification or GPUd exit so notification can be retried.
- Status remains read-only. Certificate fields describe valid selected files;
  agent readiness requires both a successful readiness probe and no pending
  notification. Neither field proves which certificate the agent has loaded.
- The existing `activateKAPMTLS` command retries a pending notification when the
  agent is running. Otherwise it only checks readiness; it never starts or
  restarts the service and does not wait for package-owned recovery.

Changing the gateway CA, client-CA fingerprint, gateway endpoint or server name
requires a separate maintenance procedure with an explicit agent restart. The
renewal command rejects these changes before writing files, even while the
agent is inactive. It compares the selected configuration, not historical
generations or process state. Corrupt selected credentials require maintenance
instead of silently treating their startup configuration as absent.
Do not use readiness as proof that a changed
endpoint or trust pool is active. This flow requires a SIGHUP-capable agent;
asynchronous certificate reload failures remain visible in the agent's logs.
