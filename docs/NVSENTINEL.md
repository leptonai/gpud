# NVSentinel integration

GPUd can consume health events from a local [NVSentinel](https://github.com/NVIDIA/NVSentinel)
deployment. When the integration is enabled, components that have an NVSentinel
counterpart prefer the NVSentinel data point and fall back to their own
detection when no corresponding NVSentinel data point exists.

## Why

NVSentinel's local unix socket is push-only. It serves one RPC,
`PlatformConnector.HealthEventOccurredV1`, so that health monitors can report
events into NVSentinel. There is no RPC to pull events back out.

The platform-connector gRPC sink connector is NVSentinel's supported
egress path. It forwards every health event to a configured gRPC target.
The platform-connectors run as a DaemonSet. They mount the host's
`/var/run/nvsentinel` directory at container path `/var/run`. A socket GPUd
creates under that host directory is reachable from the node's
platform-connector.

The integration uses this path:

1. GPUd serves the same `PlatformConnector` API on a local unix socket.
2. The NVSentinel platform-connector forwards each health event to GPUd.
3. GPUd translates each event into the matching component's own event format
   and stores it in the component's event bucket. Thresholds, health state
   evaluation, and the events API work unchanged.
4. When GPUd's own detector sees the same incident, it checks the recent
   NVSentinel events first. If NVSentinel already reported the same data point
   within the dedup window, GPUd skips its own copy. One incident, one event.

## Setup

GPUd configuration (top level of the config file):

```json
{
  "nvsentinel": {
    "enabled": true,
    "socket_path": "/var/run/nvsentinel/gpud.sock",
    "event_dedup_window": "2m"
  }
}
```

- `enabled` (default `false`): turns the NVSentinel receiver on.
- `socket_path` (default `/var/run/nvsentinel/gpud.sock`): the unix socket GPUd
  serves. Must be reachable from the NVSentinel platform-connector container.
- `event_dedup_window` (default `2m`): how long a received NVSentinel
  event suppresses GPUd's own duplicate detection of the same data point. It
  only needs to cover the delivery skew between the two detectors.

NVSentinel Helm values:

```yaml
platformConnector:
  grpcSinkConnector:
    enabled: true
    # The platform-connector container sees the host's /var/run/nvsentinel at
    # /var/run, so the GPUd socket above is /var/run/gpud.sock from inside
    # the container.
    target: "unix:///var/run/gpud.sock"
```

## Covered data points

| NVSentinel event | GPUd component |
|---|---|
| GPU-class event with a numeric error code (Xid), for example check `SysLogsXIDError` | `accelerator-nvidia-error-xid` |
| Check `SysLogsSXIDError` or NVSWITCH-class event with a numeric error code (SXid) | `accelerator-nvidia-error-sxid` |
| Check `SysLogsNICDriverError`, patterns `pci_power_insufficient`, `port_module_high_temp`, `access_reg_failed` | `accelerator-nvidia-infiniband` |

Translation rules:

- Severity: NVSentinel `isFatal` maps to a fatal GPUd event; a non-fatal
  unhealthy event maps to critical.
- Repair actions: NVSentinel `RESTART_VM` and `RESTART_BM` map to GPUd
  `REBOOT_SYSTEM`; `CONTACT_SUPPORT` and `REPLACE_VM` map to
  `HARDWARE_INSPECTION`. When the NVSentinel action has no GPUd counterpart,
  the GPUd catalog entry for the error code provides the suggested action.
- Stored events carry `data_source: nvsentinel` plus the NVSentinel check
  name and event ID, so the origin stays visible.
- Xid 63/64 (row remapping) stay with the remapped-rows component, matching
  GPUd's kmsg behavior.

## Limits

- NVSentinel recovery events (`isHealthy: true`) are logged but not stored.
  GPUd heals its component state through the existing lookback windows and
  reboot tracking, not through NVSentinel recovery events.
- Events that NVSentinel forwards while GPUd is down are dropped after the
  platform-connector's retry budget. GPUd's own detection still covers those
  incidents (the fallback stays active), so no incident is lost.
- `gpud scan` is a raw one-shot diagnostic and keeps reading kmsg directly.
- GPUd never acts on NVSentinel quarantine or drain decisions. NVSentinel
  remains the remediation owner; GPUd mirrors its health data points.