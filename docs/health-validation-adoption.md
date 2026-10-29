# Reusing health-validation in GPUd

The component integration contract is:

```text
health-validation Sensor.Measure -> ObservationBatch -> component shim
                                                     -> GPUd-defined data
```

Sensors collect data. Each GPUd component owns a shim that converts observations
into its existing types. The conversion performs no host I/O. Health decisions,
event storage, deduplication, polling, and recovery windows remain in GPUd.
Sources can be supplied when constructing a sensor, but the component collects
through `Sensor.Measure`, not `Source.Read`.

The dependency is pinned to
`github.com/dsx-ai-factory/health-validation` at
`v0.0.0-20261029153733-0281fe125892` (commit `0281fe125892`).
The GPUd base is `24b9e76c`.

| Collection | Imported package |
| --- | --- |
| BPF JIT accounting | `collect/sensors/memory` |
| Kernel records | `collect/source/kmsg` |
| Xid extraction | `collect/signals/kmsg/xid` |
| OOM and panic | `collect/signals/kmsg/memory` |
| InfiniBand sysfs | `collect/sensors/infiniband` |

Memory's `bpfJITBytesFromObservations` and InfiniBand's
`devicesFromObservations` implement that conversion. Kernel-record parsing and
Xid/OOM/panic pattern imports are helper reuse; kernel streaming has not yet
migrated to a sensor.

The InfiniBand adapter retains GPUd's device/port structs for metrics, JSON, and
history. It preserves nullable counters, byte units, numeric port order, device
exclusions, and InfiniBand port data. As requested, Ethernet ports are omitted.
Device metadata remains available for HCAs with no InfiniBand ports.
Required-field failures fail the check; missing or unsupported counters remain
absent. One sensor lives for the component's lifetime, so EINVAL counter
reads stay cached across checks and do not repeatedly trigger ACCESS_REG errors.
The old parser options, filesystem helper, numeric helper, and their tests are
removed. A full output fixture captured from the old collector checks parity
after applying the selected InfiniBand-only port filter.

The sensor reports inaccessible counter directories as per-counter errors. The
shim treats missing, unsupported, or permission-denied counters as absent. It
does not re-read directories to distinguish directory permission errors from
individual counter permission errors. Required fields and whole-collection
failures still fail the check.

The shared source only reads defined counters, so unrelated counter files no
longer cause parse failures. Its signed observation values reject counters above
`MaxInt64` after unit conversion, instead of accepting the full `uint64` range.

The BPF adapter selects only the BPF signal, so it does not read `proc/meminfo`.
It converts collection failures into errors and preserves GPUd's existing
non-Linux and non-root behavior. The sensor rejects integer overflow and reads
records beyond the old scanner's 64 KiB line limit. It reads the whole vmalloc
file into memory; large-host memory and timing costs have not been measured.

The kernel parser retains message bodies, priorities, sequence numbers, and the
caller-provided boot time. It now rejects timestamp multiplication overflow.
GPUd keeps its watcher, content deduplication, channels, and event-store syncer.
Basic Xid extraction preserves PCI identifier spelling. GPUd still reconstructs
multi-record events and interprets NVLink subcodes, severity, and recovery actions.

## Dependency cost

The current health-validation repository is one Go module. Importing collection
packages raises Go from 1.26.10 to 1.27.3 and upgrades shared dependencies through
Go's module version selection. This includes go-nvlib 0.13.0, go-nvml 0.13.4-1,
gRPC, protobuf, OpenTelemetry, and several golang.org/x modules.

The Docker builder follows the new Go requirement. Mockey 1.4.8 is required for
Go 1.27; GPUd's previous 1.4.7 does not compile with that toolchain. NVIDIA test
doubles now implement the updated interfaces. GPM uses the NVML struct's zero
value instead of spelling its array length, which changed from 210 to 477.

`go list -deps` for the affected packages includes only health-validation's
`collect` packages. It does not link the health catalog, CEL engine, toolkit CLI,
or DCGM. The shared dependency upgrades still affect other GPUd packages.

## Remaining ownership boundaries

These collectors still require behavior beyond a direct library import:

- NVML needs GPUd's shared instance lifetime, native-call gate, GPU identity, and
  lost/reset handling. Its instance interface is not interchangeable with the
  sensor's interface.
- Kernel streaming needs record order and multi-record reconstruction. The
  shared stream uses a bounded window, which can evict events before polling.
- Disk and FUSE need GPUd's mount selection and autofs precedence. The shared
  parsers have stricter input validation and different partial-error semantics.
- Pstore needs GPUd's persisted history, deduplication, and timestamp handling.
- General host memory keeps gopsutil's accounting. The shared `UsedBytes` is
  total minus available, which would change the metric definition.

## Validation

Local validation passed on macOS arm64 with Go 1.27.3:

- `go build ./...`.
- Repository-wide test compilation with `go test -run '^$'` and mockey's
  `-gcflags='all=-N -l'`. This does not execute the full test suite.
- Race tests for `components/{memory,os}`, `pkg/kmsg`,
  `components/accelerator/nvidia/{xid,gpm,infiniband/...}`, and
  `pkg/nvidia/nvml/...`, using
  `-gcflags='all=-N -l -d=checkptr=0'` as required by GPUd's test script.
- The repository's custom golangci-lint 2.14.0 configuration over the same
  packages: zero issues.
- `go mod tidy -diff`, changed-file goimports, `git diff --check`, and
  Markdown lint for this note.

The tests preserve existing kernel/Xid fixtures and the 3,977,216-byte BPF
fixture. New cases cover malformed and missing BPF data, permission errors,
integer overflow, long lines, and kernel timestamp overflow. InfiniBand tests
check filtered fixture parity, byte units, missing versus zero counters, required
versus optional errors, exclusions, numeric port order, and EINVAL cache lifetime.
Existing component tests still cover port flaps and recovery windows.
Linux GPU hardware, container builds, and live daemon behavior remain untested.
