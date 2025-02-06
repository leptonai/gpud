# NVIDIA GPU Xid errors

This `accelerator-nvidia-error-xid` components detects the NVIDIA GPU Xid errors (1) by scanning the dmesg and (2) by using the NVIDIA Management Library (NVML) to catch the Xid events.

The dmesg scan is done with the `dmesg` command and the regex match with the rule:

```regex
NVRM: Xid.*?: (\d+),
```

For example, with the following dmesg outputs:

```bash
dmesg --ctime --nopager --buffer-size 163920
```

> [Fri Aug 30 11:11:22 2024] nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing
> [Fri Aug 30 11:43:09 2024] NVRM: Xid (PCI:0000:cb:00): 13, pid='<unknown>', name=<unknown>, Graphics SM Warp Exception on (GPC 7, TPC 7, SM 0): Illegal Instruction Parameter
> [Fri Aug 30 11:43:09 2024] NVRM: Xid (PCI:0000:cb:00): 13, pid='<unknown>', name=<unknown>, Graphics SM Global Exception on (GPC 7, TPC 7, SM 0): Multiple Warp Errors
> [Fri Aug 30 11:43:09 2024] NVRM: Xid (PCI:0000:cb:00): 13, pid='<unknown>', name=<unknown>, Graphics Exception: ESR 0x57c730=0xc04000b 0x57c734=0x24 0x57c728=0x1f81fb60 0x57c72c=0x1174
> [Fri Aug 30 11:43:14 2024] NVRM: Xid (PCI:0000:cb:00): 109, pid=1797828, name=pt_main_thread, Ch 00000008, errorString CTX SWITCH TIMEOUT, Info 0x58005

The xid error code will be extracted as follows, and the `Detail` data is defined [here](https://pkg.go.dev/github.com/leptonai/gpud/nvidia-query/xid#Detail):

```yaml
detail:
  bus_error: true
  description: ""
  driver_error: true
  fb_corruption: true
  hw_error: true
  id: 109
  name: Context Switch Timeout Error
  system_memory_corruption: true
  thermal_issue: true
  user_app_error: true
detail_found: true
log_item:
  line: '[Fri Aug 30 11:43:14 2024] NVRM: Xid (PCI:0000:cb:00): 109, pid=1797828,
    name=pt_main_thread, Ch 00000008, errorString CTX SWITCH TIMEOUT, Info 0x58005'
  time: null

name: nvidia_nvrm_xid
owner_references:
- accelerator-nvidia-error
regex: 'NVRM: Xid.*?: (\d+),'

# {"level":"warn","ts":"2024-08-30T15:38:16Z","caller":"diagnose/scan.go:145","msg":"known xid","line":"[Fri Aug 30 11:43:09 2024] NVRM: Xid (PCI:0000:cb:00): 13, pid='<unknown>', name=<unknown>, Graphics Exception: ESR 0x57c730=0xc04000b 0x57c734=0x24 0x57c728=0x1f81fb60 0x57c72c=0x1174"}
detail:
  bus_error: true
  description: Run DCGM and Field diagnostics to confirm if the issue is related to
    hardware. If not, debug the user application using guidance from https://docs.nvidia.com/deploy/xid-errors/index.html.
    If the latter, see Report a GPU Issue at https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#reporting-gpu-issue.
  driver_error: true
  fb_corruption: true
  hw_error: true
  id: 13
  name: Graphics Engine Exception
  system_memory_corruption: true
  thermal_issue: true
  user_app_error: true
detail_found: true
log_item:
  line: '[Fri Aug 30 11:43:09 2024] NVRM: Xid (PCI:0000:cb:00): 13, pid=''<unknown>'',
    name=<unknown>, Graphics Exception: ESR 0x57c730=0xc04000b 0x57c734=0x24 0x57c728=0x1f81fb60
    0x57c72c=0x1174'
  time: null

name: nvidia_nvrm_xid
owner_references:
- accelerator-nvidia-error
regex: 'NVRM: Xid.*?: (\d+),'

# {"level":"warn","ts":"2024-08-30T15:38:16Z","caller":"diagnose/scan.go:145","msg":"known xid","line":"[Fri Aug 30 11:43:09 2024] NVRM: Xid (PCI:0000:cb:00): 13, pid='<unknown>', name=<unknown>, Graphics SM Global Exception on (GPC 7, TPC 7, SM 0): Multiple Warp Errors"}
detail:
  bus_error: true
  description: Run DCGM and Field diagnostics to confirm if the issue is related to
    hardware. If not, debug the user application using guidance from https://docs.nvidia.com/deploy/xid-errors/index.html.
    If the latter, see Report a GPU Issue at https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#reporting-gpu-issue.
  driver_error: true
  fb_corruption: true
  hw_error: true
  id: 13
  name: Graphics Engine Exception
  system_memory_corruption: true
  thermal_issue: true
  user_app_error: true
detail_found: true
log_item:
  line: '[Fri Aug 30 11:43:09 2024] NVRM: Xid (PCI:0000:cb:00): 13, pid=''<unknown>'',
    name=<unknown>, Graphics SM Global Exception on (GPC 7, TPC 7, SM 0): Multiple
    Warp Errors'
  time: null

name: nvidia_nvrm_xid
owner_references:
- accelerator-nvidia-error
regex: 'NVRM: Xid.*?: (\d+),'

# {"level":"warn","ts":"2024-08-30T15:38:16Z","caller":"diagnose/scan.go:145","msg":"known xid","line":"[Fri Aug 30 11:43:09 2024] NVRM: Xid (PCI:0000:cb:00): 13, pid='<unknown>', name=<unknown>, Graphics SM Warp Exception on (GPC 7, TPC 7, SM 0): Illegal Instruction Parameter"}
detail:
  bus_error: true
  description: Run DCGM and Field diagnostics to confirm if the issue is related to
    hardware. If not, debug the user application using guidance from https://docs.nvidia.com/deploy/xid-errors/index.html.
    If the latter, see Report a GPU Issue at https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#reporting-gpu-issue.
  driver_error: true
  fb_corruption: true
  hw_error: true
  id: 13
  name: Graphics Engine Exception
  system_memory_corruption: true
  thermal_issue: true
  user_app_error: true
detail_found: true
log_item:
  line: '[Fri Aug 30 11:43:09 2024] NVRM: Xid (PCI:0000:cb:00): 13, pid=''<unknown>'',
    name=<unknown>, Graphics SM Warp Exception on (GPC 7, TPC 7, SM 0): Illegal Instruction
    Parameter'
  time: null
```

> [Sat Aug 31 07:06:03 2024] NVRM: Xid (PCI:0000:cb:00): 31, pid=626486, name=pt_main_thread, Ch 00000008, intr 00000000. MMU Fault: ENGINE GRAPHICS GPCCLIENT_T1_14 faulted @ 0x7f2f_cca58000. Fault is of type FAULT_PDE ACCESS_TYPE_VIRT_READ
> [Sat Aug 31 07:54:50 2024] perf: interrupt took too long (4931 > 4921), lowering kernel.perf_event_max_sample_rate to 40500
> [Sat Aug 31 08:01:31 2024] hrtimer: interrupt took 1263236 ns
> [Sat Aug 31 08:53:21 2024] NVRM: Xid (PCI:0000:cb:00): 109, pid=1158578, name=pt_main_thread, Ch 00000008, errorString CTX SWITCH TIMEOUT, Info 0x58005
> [Sat Aug 31 08:53:22 2024] nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing

```yaml
# {"level":"warn","ts":"2024-08-31T09:01:20Z","caller":"diagnose/scan.go:145","msg":"known xid","line":"[Sat Aug 31 08:53:21 2024] NVRM: Xid (PCI:0000:cb:00): 109, pid=1158578, name=pt_main_thread, Ch 00000008, errorString CTX SWITCH TIMEOUT, Info 0x58005"}
detail:
  bus_error: true
  description: ""
  driver_error: true
  fb_corruption: true
  hw_error: true
  id: 109
  name: Context Switch Timeout Error
  system_memory_corruption: true
  thermal_issue: true
  user_app_error: true
detail_found: true
log_item:
  line: '[Sat Aug 31 08:53:21 2024] NVRM: Xid (PCI:0000:cb:00): 109, pid=1158578,
    name=pt_main_thread, Ch 00000008, errorString CTX SWITCH TIMEOUT, Info 0x58005'
  time: null

name: nvidia_nvrm_xid
owner_references:
- accelerator-nvidia-error
regex: 'NVRM: Xid.*?: (\d+),'

# {"level":"warn","ts":"2024-08-31T09:01:20Z","caller":"diagnose/scan.go:145","msg":"known xid","line":"[Sat Aug 31 07:06:03 2024] NVRM: Xid (PCI:0000:cb:00): 31, pid=626486, name=pt_main_thread, Ch 00000008, intr 00000000. MMU Fault: ENGINE GRAPHICS GPCCLIENT_T1_14 faulted @ 0x7f2f_cca58000. Fault is of type FAULT_PDE ACCESS_TYPE_VIRT_READ"}
detail:
  bus_error: false
  description: Debug the user application unless the issue is new and there have been
    no changes to the application but there has been changes to GPU driver or other
    GPU system software. If the latter, see Report a GPU Issue via https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#reporting-gpu-issue.
  driver_error: true
  fb_corruption: false
  hw_error: true
  id: 31
  name: GPU memory page fault
  system_memory_corruption: false
  thermal_issue: false
  user_app_error: true
detail_found: true
log_item:
  line: '[Sat Aug 31 07:06:03 2024] NVRM: Xid (PCI:0000:cb:00): 31, pid=626486, name=pt_main_thread,
    Ch 00000008, intr 00000000. MMU Fault: ENGINE GRAPHICS GPCCLIENT_T1_14 faulted
    @ 0x7f2f_cca58000. Fault is of type FAULT_PDE ACCESS_TYPE_VIRT_READ'
  time: null
```
