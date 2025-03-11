# NVIDIA GPU SXid errors

See [NVIDIA GPU Fabric Manager User Guide](https://docs.nvidia.com/datacenter/tesla/pdf/fabric-manager-user-guide.pdf) for more details.

The Xid and SXid errors often happen together:

> [6781741.548768] NVRM: GPU at PCI:0000:91:00: GPU-b6c3b2be-c55b-d076-fa0e-d464e4c7e08b
>
> [6781741.548776] NVRM: GPU Board Serial Number: 1653723052734
>
> [6781741.548779] NVRM: Xid (PCI:0000:91:00): 79, pid='<unknown>', name=<unknown>, GPU has fallen off the bus.
>
> [6781741.548783] NVRM: GPU 0000:91:00.0: GPU has fallen off the bus.
>
> [6781741.548786] NVRM: GPU 0000:91:00.0: GPU serial number is 1653723052734.
>
> [6781753.400584] nvidia-nvswitch1: SXid (PCI:0000:06:00.0): 20034, Fatal, Link 48 LTSSM Fault Up
>
> [6781753.404587] nvidia-nvswitch0: SXid (PCI:0000:05:00.0): 20034, Fatal, Link 63 LTSSM Fault Up
>
> [6781753.404848] nvidia-nvswitch2: SXid (PCI:0000:07:00.0): 20034, Severity 1 Engine instance 60 Sub-engine instance 00
>
> [6781753.406566] nvidia-nvswitch0: SXid (PCI:0000:05:00.0): 20034, Data {0x10000000, 0x10000000, 0x00000000, 0x10000000, 0x00000000, 0x00000000, 0x00000000, 0x00000000, 0x00000000}
>
> [6781753.407899] nvidia-nvswitch2: SXid (PCI:0000:07:00.0): 20034, Fatal, Link 37 LTSSM Fault Up
>
> [6781753.408138] nvidia-nvswitch0: SXid (PCI:0000:05:00.0): 20034, Fatal, Link 62 LTSSM Fault Up
>
> [6781753.409504] nvidia-nvswitch2: SXid (PCI:0000:07:00.0): 20034, Severity 1 Engine instance 37 Sub-engine instance 00
>
> [6781753.409792] nvidia-nvswitch0: SXid (PCI:0000:05:00.0): 20034, Severity 1 Engine instance 62 Sub-engine instance 00

The Xid 79 indicates that "GPU has fallen off the bus". And the SXid 20034 indicates that "associated link has gone down from active". This specific issue requires a restart of the guest VM.

Such case may be identified by other sources.

`nvidia-smi` fails with the following error:

> Unable to determine the device handle for GPU0000:91:00.0: Unknown Error

The nvidia GPU feature discovery container may fail with the following error:

> level=error msg="StartContainer for \"76866e1cf89662344e632e85ece44ebf6215e36f6436da32810699c083ab80dc\" failed" error="failed to create containerd task: failed to create shim task: OCI runtime create failed: runc create failed: unable to start container process: error during container init: error running hook #0: error running hook: exit status 1, stdout: , stderr: nvidia-container-cli.real: detection error: nvml error: unknown error: unknown"
