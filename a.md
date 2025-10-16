# IB Port Drop vs Flap – Observations, Root Cause, and Proposal (Oct 16, 2025 UTC)

This note captures what we observed on three nodes, why GPUd behaves differently for IB port drops vs flaps, and a concrete, minimal proposal to harmonize the operator experience without losing the original design intent.

## TL;DR
- Flaps are sticky on purpose: once we detect repeated DOWN→ACTIVE cycles, GPUd stays Unhealthy and suggests hardware inspection until SetHealthy is called.
- Drops are ephemeral by design: when thresholds fail (e.g., only 7 ports ≥400G and a port is Polling or down too long), GPUd goes Unhealthy and suggests inspection, but flips back to Healthy as soon as thresholds recover on the next 30s cycle.
- This feels inconsistent in practice. Proposal: keep drops “sticky” for a short stabilization window (e.g., 10 minutes or N consecutive healthy checks) after recovery; leave flaps unchanged (still require SetHealthy).

---

## Observed Events (UTC)

### Node: fargate-ip-10-0-81-136.ap-south-1.compute.internal
- Check time: 2025-10-16 12:06:16
- Current link: mlx5_9/1 ACTIVE, LinkUp, 400 Gb/sec (4X NDR), counters clean.
- Historical flap transitions (≥25s down interval followed by ACTIVE, repeated ≥3x):

```
{"level":"warn","ts":"2025-10-16T17:24:02.709+0530","msg":"ib port reverted back to active (potential flap)","device":"mlx5_9","port":1,"down1":"2025-10-14T05:47:02.000Z","down2":"2025-10-14T05:47:32.000Z","revertedAt":"2025-10-14T05:48:02.000Z"}
{"level":"warn","ts":"2025-10-16T17:24:02.709+0530","msg":"ib port reverted back to active (potential flap)","device":"mlx5_9","port":1,"down1":"2025-10-15T23:30:32.000Z","down2":"2025-10-15T23:31:02.000Z","revertedAt":"2025-10-15T23:31:32.000Z"}
{"level":"warn","ts":"2025-10-16T17:24:02.709+0530","msg":"ib port reverted back to active (potential flap)","device":"mlx5_9","port":1,"down1":"2025-10-15T23:32:02.000Z","down2":"2025-10-15T23:32:32.000Z","revertedAt":"2025-10-15T23:47:32.000Z"}
{"level":"warn","ts":"2025-10-16T17:24:02.709+0530","msg":"ib port reverted back to active (potential flap)","device":"mlx5_9","port":1,"down1":"2025-10-16T08:00:02.000Z","down2":"2025-10-16T08:00:32.000Z","revertedAt":"2025-10-16T08:02:32.000Z"}
```

- Persisted flap event (first threshold breach):
```
{"level":"info","ts":"2025-10-16T17:24:02.722+0530","msg":"set event","table":"infiniband_device_port_history_v0_5_1","timestamp":"2025-10-15T23:47:32.000Z","device":"mlx5_9","port":1,"event":"ib_port_flap","reason":"mlx5_9 port 1 down since 2025-10-15T23:32:02Z (and flapped back to active)"}
```

- Result: GPUd Unhealthy with reason "device(s) flapping between ACTIVE<>DOWN: mlx5_9" until SetHealthy.

---

### Node: fargate-ip-10-0-83-166.ap-south-1.compute.internal
- Check time: 2025-10-16 12:28:28
- Current link: mlx5_9/1 ACTIVE, LinkUp, 400 Gb/sec, counters clean.
- Historical flap + state:
```
{"level":"warn","ts":"2025-10-16T17:57:28.423+0530","msg":"ib port reverted back to active (potential flap)","device":"mlx5_9","port":1,"down1":"2025-10-16T07:59:58.000Z","down2":"2025-10-16T08:00:28.000Z","revertedAt":"2025-10-16T08:02:28.000Z"}
{"level":"info","ts":"2025-10-16T17:57:28.437+0530","msg":"set event","table":"infiniband_device_port_history_v0_5_1","timestamp":"2025-10-15T23:47:31.000Z","device":"mlx5_9","port":1,"event":"ib_port_flap","reason":"mlx5_9 port 1 down since 2025-10-15T23:32:01Z (and flapped back to active)"}
{"level":"warn","ts":"2025-10-16T17:57:29.228+0530","msg":"device(s) flapping between ACTIVE<>DOWN: mlx5_9"}
```
- Result: Unhealthy persists due to flap history until SetHealthy.

---

### Node: fargate-ip-10-0-83-4.ap-south-1.compute.internal
- Check time: 2025-10-16 12:51:27
- Current link: mlx5_9/1 ACTIVE, LinkUp, 400 Gb/sec, counters clean. GPUd state Healthy "ok; no infiniband port issue" at 12:51:18.
- Control-plane recall (drop case) at 2025-10-16 08:47:29:
```
{"level":"info","ts":"2025-10-16T08:47:29Z","msg":"The machine will be Recalled...","unhealthyComponent":"accelerator-nvidia-infiniband","unhealthyReason":"only 7 port(s) are active and >=400 Gb/s, expect >=8 port(s); 1 device(s) physical state Polling (mlx5_9) -- connecton lost...; device(s) down too long: mlx5_9"}
```
- Historical single flap instance (not reaching flap threshold):
```
{"level":"warn","ts":"2025-10-16T18:21:19.794+0530","msg":"ib port reverted back to active (potential flap)","device":"mlx5_9","port":1,"down1":"2025-10-16T07:59:49.000Z","down2":"2025-10-16T08:00:19.000Z","revertedAt":"2025-10-16T08:02:49.000Z"}
```
- Result: At 08:47:29 thresholds failed and an ib_port_drop was detected → Unhealthy + HARDWARE_INSPECTION; later, once the port recovered and thresholds passed, component flipped back to Healthy on the next 30s cycle (no SetHealthy needed).

---

## Why This Happens (Cause → Result)

- Flaps (sticky by design)
  - Detection: down interval ≥25s and returns to ACTIVE at least 3 times within the scan window.
  - Processing: always processed regardless of current thresholds; component remains Unhealthy with reason "device(s) flapping between ACTIVE<>DOWN" and suggested_actions HARDWARE_INSPECTION until SetHealthy.

- Drops (ephemeral by design)
  - Detection: persistent DOWN ≥4 minutes ("down too long"), commonly with phys_state=Polling.
  - Processing: only considered when thresholds are not met (e.g., fewer than required ports at target rate). Once thresholds pass, drop events are ignored and component becomes Healthy on the next cycle — even if we just suggested inspection.

- Where in code (for reference):
  - component.go drop handling around case EventTypeIbPortDrop only appends when len(cr.unhealthyIBPorts) > 0 (thresholds failing now).
  - component.go flap handling is "always process".

---

## Proposal: Make Drops Briefly Sticky (Keep Flaps As-Is)

- Goal
  - Avoid immediate Healthy flip after suggesting HARDWARE_INSPECTION for ib_port_drop; give operators a short stabilization period to observe.

- Design
  - Introduce a short sticky window for ib_port_drop (default 10 minutes), or require N consecutive healthy checks.
  - Continue to process ib_port_drop if either thresholds are failing now OR the last drop is "recent" (within sticky window).
  - Keep flap behavior unchanged (still sticky until SetHealthy).

- Suggested shape (conceptual)
  - New field on the component: `dropStickyWindow time.Duration` with default `10 * time.Minute`.
  - In the drop case, change the gate from "thresholds failing only" to "thresholds failing OR recent drop".

```
// Pseudocode illustrating new gate
dropIsRecent := now.Sub(event.Time) < dropStickyWindow
if len(cr.unhealthyIBPorts) > 0 || dropIsRecent {
    log.Warn(event.EventReason)
    ibDropDevs = append(ibDropDevs, event.Port.Device)
}
```

- Alternatives (configurable knobs)
  - Use "N consecutive good checks" instead of time-based window.
  - Only escalate Unhealthy for drop if repeats ≥M within 12h, otherwise mark Degraded.
  - Add a config flag to treat drops exactly like flaps (require SetHealthy) for environments that prefer maximum stickiness.

---

## Test Plan (Operator-Focused)

- Drop path
  - Force thresholds to fail and hold a port in Polling long enough to trigger ib_port_drop.
  - Confirm Unhealthy + HARDWARE_INSPECTION renders immediately.
  - Recover the port to ACTIVE and meet thresholds.
  - Verify component stays Unhealthy during the sticky window, then becomes Healthy after window or after N healthy checks.

- Flap path
  - Generate ≥3 down→active cycles to trigger flap threshold.
  - Confirm Unhealthy persists until SetHealthy is called (unchanged behavior).

---

## Rationale & Risk

- Rationale
  - Operators receive a consistent experience: both flaps and significant drops prompt an inspection signal that does not disappear instantly.
  - Short stickiness avoids penalizing dormant ports while preventing “blink-and-you-miss-it” recalls.

- Risk
  - Slightly longer Unhealthy durations for transient drops that self-heal within seconds; mitigated by keeping the sticky window short and configurable.

---

## Glossary
- Thresholds: minimum count of ports and rate (e.g., at_least_ports=8, at_least_rate=400 Gb/s).
- ib_port_drop: persistent down state (≥4 minutes) detected for a port.
- ib_port_flap: ≥3 sequences of down (≥25s) followed by active within the scan window.
- SetHealthy: administrative action that tombstones IB event history so only new events surface.

---

## Appendix: Quick Reality Checks
- Sticky only for recent drops; flaps remain sticky until SetHealthy.
- Immediate Healthy flip for drops goes away; immediate detection when thresholds fail stays intact.
- The change is local to infiniband component logic; no schema changes.

