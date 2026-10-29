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
`v0.0.0-20261029220550-542735d3b294` (commit `542735d3b294`),
which is merged upstream and includes [health-validation #247](https://github.com/dsx-ai-factory/health-validation/pull/247).
No unmerged health-validation change is required.
The GPUd base is `24b9e76c`.

| Collection | Imported package |
| --- | --- |
| BPF JIT accounting | `collect/sensors/memory` |
| Kernel records | `collect/sensors/kmsg`, `collect/source/kmsg` |
| Xid parsing in the component shim | `collect/signals/xid` |
| OOM and panic | `collect/signals/kmsg/memory` |
| InfiniBand sysfs | `collect/sensors/infiniband` |

Memory's `bpfJITBytesFromObservations` and InfiniBand's
`devicesFromObservations`
implement that conversion. Kernel-consuming components own `kernel_sensor.go`
shims that convert observations into their GPUd events or event-loop input. They
share the pure `components.KernelMessages` decoder and the
`api/v1.KernelMessage`
data type. The Xid shim selects Xid messages and calls the library's
`xid.Match` and `xid.Parse`, then converts the result and applies GPUd's catalog
and NVLink recovery rules. Raw boot identity, sequence, timestamp, and body stay
with the record. GPUd no longer extracts Xid fields with its own regex/parser.
Shared OOM/panic and fallen-off-bus patterns support the interpretation that
remains in GPUd.

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

GPUd's entire `pkg/kmsg` directory is removed. The component runtime owns one
health-validation `NewStream` and generic `kmsg.New` sensor per `GPUdInstance`,
shared by all registered components. It delivers observation batches to
component-owned shims;
there is no replacement GPUd watcher/syncer compatibility API. Native reading,
boot-time lookup, parsing, and the stream's bounded storage belong to the
library.
The generic sensor uses a catch-all pattern and selects only `kernel.message`
and `kernel.priority`, preserving empty messages and continuation lines. The
same raw observations reach every subscriber; each component chooses which
messages matter to it. The shared runtime has no Xid-specific interpretation.

`GPUdInstance.ObserveKernel` manages subscriptions and serializes
`Sensor.Measure`
at most once per 100 ms polling interval. Each component receives fresh record
observations in encounter order, using boot ID and sequence to suppress replay.
Message and priority observations stay paired. Unchanged windows reuse the prior
observation batch without rebuilding it, and late subscribers receive retained
history. A slow component does not block another component's delivery. Stopping
a
subscription waits for its callback; the last subscription closes the source.

The library retains its default 4,096-record window. Slow consumers and startup
bursts can still lose records through window eviction. Sequence gaps after a
component's first observed record are logged; loss before its first snapshot is
not detectable. Partial results and collection failures reach every subscriber.
Pure `EPIPE` overruns are logged once by the shared runtime and collection
continues. This depends on #247's `SourceErrorOverrun` classification: the
library
already keeps reading after an overrun, but GPUd previously treated its sticky
loss report as terminal. Deduplication cannot repair a stopped subscription.
A joined terminal error is not classified as a recoverable overrun and still
releases subscriptions after delivering partial data. Xid parsing happens only
inside its component shim, so a malformed Xid cannot stop other subscribers.

Components retain their own matching and health interpretation, including
multi-record OOM/panic reconstruction and NVSentinel suppression. GPUd event
coalescing and persistent duplicate lookup now live in `eventstore.Recorder`.
Content deduplication remains component-local; InfiniBand and peermem retain
their
first-occurrence event windows, and NVLink retains its boot-specific event key.

Xid, SXid, NCCL, and NVLink own `kernel_snapshot.go` shims. All use the generic
kmsg sensor with its default `ReadAll` source, and convert observations into
GPUd results through the same component shims used by subscriptions.
They remain separate from the streaming window so on-demand checks can see
records still in the kernel ring beyond the stream's
4,096-record window. This is not complete journal/boot history. JSON record
fields are preserved by `api/v1.KernelMessage`; boot identity is not serialized.
Xid event addresses now consistently use the `PCI:` prefix, including input
lines that omit it. The shared parser also recognizes digits in NVLink unit
names such as `TLW_RX_PIPE0`; their logged `Fatal` severity now reaches GPUd's
existing severity rules instead of being dropped by the old regex.

The removed directory also contained fault injection's writer, which is outside
health collection. Its request types and priority policy now belong to
`pkg/fault-injector`. Line splitting, printk length limits, tab normalization,
and
truncation use `github.com/siderolabs/go-kmsg` v0.1.6. This keeps the
fault-injection
API while deleting the copied writer implementation.

The shared reader opens `/dev/kmsg` on Linux. Non-default `KMSG_FILE_PATH`
values
now return an explicit unsupported-path error; they are not silently ignored.
Non-Linux reads return the shared reader's unsupported error.

## Dependency cost

The current health-validation repository is one Go module. Importing collection
packages raises Go from 1.26.10 to 1.27.3 and upgrades shared dependencies
through
Go's module version selection. This includes go-nvlib 0.13.0, go-nvml 0.13.4-1,
gRPC, protobuf, OpenTelemetry, and several golang.org/x modules.

The Docker builder follows the new Go requirement. Mockey 1.4.8 is required for
Go 1.27; GPUd's previous 1.4.7 does not compile with that toolchain. NVIDIA test
doubles now implement the updated interfaces. GPM uses the NVML struct's zero
value instead of spelling its array length, which changed from 210 to 477.

`go list -deps` for the affected packages includes only health-validation's
`collect` packages. It does not link the health catalog, CEL engine, toolkit
CLI,
or DCGM. The shared dependency upgrades still affect other GPUd packages.

## Remaining ownership boundaries

These collectors still require behavior beyond a direct library import:

- NVML needs GPUd's shared instance lifetime, native-call gate, GPU identity,
and
  lost/reset handling. Its instance interface is not interchangeable with the
  sensor's interface.
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
- Race tests for the component sensor runtime, all kernel-consuming components,
  event storage, fault injection, and affected API adapters. Earlier migration
  checks also covered GPM and `pkg/nvidia/nvml/...`. Tests use
  `-gcflags='all=-N -l -d=checkptr=0'` as required by GPUd's test script.
- The repository's custom golangci-lint 2.14.0 configuration over the changed
  packages: zero issues.
- `go mod tidy -diff`, changed-file goimports, `git diff --check`, and
  Markdown lint for this note.

The tests preserve existing kernel/Xid fixtures and the 3,977,216-byte BPF
fixture. New cases cover malformed and missing BPF data, permission errors,
integer overflow, long lines, and sensor outcome conversion. InfiniBand tests
check filtered fixture parity, byte units, missing versus zero counters,
required
versus optional errors, exclusions, numeric port order, and EINVAL cache
lifetime.
Existing component tests still cover port flaps and recovery windows. Kernel
shim tests cover fixture parsing through the shared parser and sensor, complete
message/priority conversion, partial failures, replay suppression, sequence
gaps, and cancellation while the output channel is full. Runtime subscription
tests
cover independent dedup policies, concurrent subscribers, one shared
measurement,
late subscription, slow consumers, boot identity, partial failures, and
reference-counted shutdown. New regressions verify continuation after sticky
EPIPE for multiple subscribers, terminal errors joined with EPIPE, and isolation
of malformed Xids. The overrun regression fails against the previous runtime.
Xid shim tests prove use of parsed observations even when the raw body changes,
original event timestamps and boot identity, and rejection of invalid facts.
Snapshot coverage checks history beyond the streaming window, partial errors,
and content dedup. Event-store tests retain epoch-aligned
and first-occurrence windows plus deduplication across restarts. API tests cover
JSON compatibility and fault-injection requests. Updated Linux-only component
tests also compile using a temporary build-constraint overlay on macOS; this
does
not establish native Linux runtime behavior.
Linux GPU hardware, container builds, and live daemon behavior remain untested.

The upstream Xid/kmsg packages also passed Linux arm64 Bazel tests and static
analysis, macOS race tests, Gazelle, formatting, and module-tidy checks.
